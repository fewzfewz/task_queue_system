package standard

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/smtp"
	"os"
	"strings"
	"time"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/worker/plugin"
)

// EmailJob holds the typed fields expected inside an "email" job's Payload.
type EmailJob struct {
	To      string
	Subject string
	Body    string
}

// EmailPlugin implements plugin.JobPlugin for jobs of type "email".
// When SMTP_HOST is set, sends real mail; otherwise simulates delivery (logs only).
type EmailPlugin struct {
	logger *slog.Logger
}

// NewEmailPlugin creates an EmailPlugin with the provided logger.
func NewEmailPlugin(logger *slog.Logger) *EmailPlugin {
	return &EmailPlugin{logger: logger}
}

func init() {
	p := NewEmailPlugin(slog.Default())
	plugin.RegisterGlobal(p)
}

func (p *EmailPlugin) Type() string {
	return "email"
}

func smtpConfigured() bool {
	return os.Getenv("SMTP_HOST") != ""
}

// Execute extracts email fields from the payload and sends via SMTP or simulates.
func (p *EmailPlugin) Execute(ctx context.Context, job *jobs.Job) (interface{}, error) {
	if job.Version > 1 {
		p.logger.Warn("unsupported job version, falling back to v1 logic", "version", job.Version, "job_id", job.ID)
	}

	payload := job.Payload
	to, _ := payload["to"].(string)
	subject, _ := payload["subject"].(string)
	body, _ := payload["body"].(string)

	if to == "" {
		return nil, fmt.Errorf("email plugin: missing required field 'to'")
	}

	if smtpConfigured() {
		if err := sendSMTP(to, subject, body); err != nil {
			return nil, fmt.Errorf("email plugin: smtp send failed: %w", err)
		}
		p.logger.Info("email sent via SMTP", "to", to, "subject", subject)
		return fmt.Sprintf("email sent to %s via SMTP", to), nil
	}

	p.logger.Info("sending email (simulated — set SMTP_HOST for real delivery)", "to", to, "subject", subject)
	time.Sleep(50 * time.Millisecond)
	p.logger.Info("email sent successfully (simulated)", "to", to)
	return fmt.Sprintf("email sent to %s (simulated)", to), nil
}

func sendSMTP(to, subject, body string) error {
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	if port == "" {
		port = "587"
	}
	user := os.Getenv("SMTP_USER")
	pass := os.Getenv("SMTP_PASSWORD")
	from := os.Getenv("SMTP_FROM")
	if from == "" {
		from = user
	}
	if from == "" {
		from = "noreply@taskqueue.local"
	}

	addr := host + ":" + port
	msg := buildMIME(from, to, subject, body)

	if user != "" && pass != "" {
		auth := smtp.PlainAuth("", user, pass, host)
		return sendWithAuth(addr, auth, from, []string{to}, msg)
	}
	return smtp.SendMail(addr, nil, from, []string{to}, msg)
}

func buildMIME(from, to, subject, body string) []byte {
	headers := []string{
		"From: " + from,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=\"utf-8\"",
		"",
		body,
	}
	return []byte(strings.Join(headers, "\r\n"))
}

func sendWithAuth(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	host, _, _ := strings.Cut(addr, ":")
	c, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer c.Close()

	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return err
		}
	}
	if err := c.Auth(auth); err != nil {
		return err
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

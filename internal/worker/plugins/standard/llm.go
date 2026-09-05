package standard

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/worker/plugin"
)

// LLMPlugin implements plugin.JobPlugin for jobs of type "llm".
// Simulates calling an LLM API (like OpenAI or Gemini) to generate text.
type LLMPlugin struct {
	logger *slog.Logger
}

func NewLLMPlugin(logger *slog.Logger) *LLMPlugin {
	return &LLMPlugin{logger: logger}
}

func init() {
	plugin.RegisterGlobal(NewLLMPlugin(slog.Default()))
}

func (p *LLMPlugin) Type() string {
	return "llm"
}

func (p *LLMPlugin) Execute(ctx context.Context, job *jobs.Job) (interface{}, error) {
	prompt, _ := job.Payload["prompt"].(string)
	model, _ := job.Payload["model"].(string)

	if prompt == "" {
		return nil, fmt.Errorf("llm plugin: missing required field 'prompt'")
	}
	if model == "" {
		model = "gpt-4o" // default model
	}

	p.logger.Info("executing llm prompt", "model", model, "prompt_length", len(prompt), "job_id", job.ID)

	// Simulate streaming inference / token generation latency
	for i := 1; i <= 5; i++ {
		select {
		case <-ctx.Done():
			p.logger.Warn("llm inference cancelled", "job_id", job.ID)
			return nil, ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
		// Report progress during generation
		plugin.ReportProgress(ctx, float64(i)*20.0)
	}

	// Mock response
	generatedText := fmt.Sprintf("This is a simulated AI response for the prompt: %q using model %s.", prompt, model)
	
	p.logger.Info("llm inference completed", "job_id", job.ID)

	return map[string]interface{}{
		"response":   generatedText,
		"model":      model,
		"tokens":     42,
		"status":     "success",
	}, nil
}

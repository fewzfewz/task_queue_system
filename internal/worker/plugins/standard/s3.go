package standard

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/worker/plugin"
)

// S3Plugin implements plugin.JobPlugin for jobs of type "s3_upload".
// Simulates uploading data to an Amazon S3 bucket.
type S3Plugin struct {
	logger *slog.Logger
}

func NewS3Plugin(logger *slog.Logger) *S3Plugin {
	return &S3Plugin{logger: logger}
}

func init() {
	plugin.RegisterGlobal(NewS3Plugin(slog.Default()))
}

func (p *S3Plugin) Type() string {
	return "s3_upload"
}

func (p *S3Plugin) Execute(ctx context.Context, job *jobs.Job) (interface{}, error) {
	bucket, _ := job.Payload["bucket"].(string)
	objectKey, _ := job.Payload["object_key"].(string)
	data, _ := job.Payload["data"].(string)

	if bucket == "" {
		return nil, fmt.Errorf("s3 plugin: missing required field 'bucket'")
	}
	if objectKey == "" {
		return nil, fmt.Errorf("s3 plugin: missing required field 'object_key'")
	}
	if data == "" {
		return nil, fmt.Errorf("s3 plugin: missing required field 'data'")
	}

	p.logger.Info("starting s3 upload", "bucket", bucket, "object_key", objectKey, "size", len(data), "job_id", job.ID)

	// Simulate upload progress
	select {
	case <-ctx.Done():
		p.logger.Warn("s3 upload cancelled", "job_id", job.ID)
		return nil, ctx.Err()
	case <-time.After(600 * time.Millisecond):
	}

	s3URL := fmt.Sprintf("s3://%s/%s", bucket, objectKey)
	httpsURL := fmt.Sprintf("https://%s.s3.amazonaws.com/%s", bucket, objectKey)
	
	p.logger.Info("s3 upload completed", "s3_url", s3URL)

	return map[string]string{
		"s3_url":    s3URL,
		"https_url": httpsURL,
		"status":    "uploaded",
	}, nil
}

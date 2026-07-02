package dto

import (
	"testing"
	"time"

	"task-queue-system/internal/jobs"
)

func TestCreateJobRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		req     CreateJobRequest
		wantErr bool
	}{
		{
			name: "valid email job",
			req: CreateJobRequest{
				Type:    "email",
				Payload: map[string]interface{}{"to": "user@example.com"},
			},
			wantErr: false,
		},
		{
			name: "valid image job",
			req: CreateJobRequest{
				Type:    "image",
				Payload: map[string]interface{}{"source_url": "https://example.com/img.png"},
			},
			wantErr: false,
		},
		{
			name: "valid with all optional fields",
			req: CreateJobRequest{
				Type:            "email",
				Payload:         map[string]interface{}{"to": "user@example.com"},
				Priority:        "high",
				MaxRetries:      5,
				BackoffAlgorithm: "linear",
				BackoffJitter:    "full",
				RunAt:           time.Now().Add(time.Hour).Format(time.RFC3339),
				CorrelationID:   "corr-1",
				Timeout:         30,
				Version:         2,
				TenantID:        "tenant-a",
				DedupKey:        "dedup-1",
				Dependencies:    []string{"job-1"},
				ShardKey:        "shard-1",
			},
			wantErr: false,
		},
		{
			name: "missing type",
			req: CreateJobRequest{
				Payload: map[string]interface{}{"to": "user@example.com"},
			},
			wantErr: true,
		},
		{
			name: "missing payload",
			req: CreateJobRequest{
				Type: "email",
			},
			wantErr: true,
		},
		{
			name: "email missing to field",
			req: CreateJobRequest{
				Type:    "email",
				Payload: map[string]interface{}{"subject": "hello"},
			},
			wantErr: true,
		},
		{
			name: "image missing source_url",
			req: CreateJobRequest{
				Type:    "image",
				Payload: map[string]interface{}{"format": "png"},
			},
			wantErr: true,
		},
		{
			name: "invalid backoff_algorithm",
			req: CreateJobRequest{
				Type:            "email",
				Payload:         map[string]interface{}{"to": "user@example.com"},
				BackoffAlgorithm: "unknown",
			},
			wantErr: true,
		},
		{
			name: "invalid backoff_jitter",
			req: CreateJobRequest{
				Type:         "email",
				Payload:      map[string]interface{}{"to": "user@example.com"},
				BackoffJitter: "unknown",
			},
			wantErr: true,
		},
		{
			name: "invalid cron expression",
			req: CreateJobRequest{
				Type:    "email",
				Payload: map[string]interface{}{"to": "user@example.com"},
				CronExpr: "bad-cron",
			},
			wantErr: true,
		},
		{
			name: "valid cron expression",
			req: CreateJobRequest{
				Type:    "email",
				Payload: map[string]interface{}{"to": "user@example.com"},
				CronExpr: "*/5 * * * *",
			},
			wantErr: false,
		},
		{
			name: "run_at in past",
			req: CreateJobRequest{
				Type:    "email",
				Payload: map[string]interface{}{"to": "user@example.com"},
				RunAt:   time.Now().Add(-2 * time.Minute).Format(time.RFC3339),
			},
			wantErr: true,
		},
		{
			name: "run_at valid future",
			req: CreateJobRequest{
				Type:    "email",
				Payload: map[string]interface{}{"to": "user@example.com"},
				RunAt:   time.Now().Add(time.Hour).Format(time.RFC3339),
			},
			wantErr: false,
		},
		{
			name: "empty payload map still fails Validate",
			req: CreateJobRequest{
				Type:    "email",
				Payload: map[string]interface{}{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestBatchJobRequestValidate(t *testing.T) {
	t.Run("empty batch fails", func(t *testing.T) {
		req := BatchJobRequest{Jobs: []CreateJobRequest{}}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for empty batch")
		}
	})

	t.Run("single valid job succeeds", func(t *testing.T) {
		req := BatchJobRequest{
			Jobs: []CreateJobRequest{
				{Type: "email", Payload: map[string]interface{}{"to": "user@example.com"}},
			},
		}
		if err := req.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("exceeding max batch size fails", func(t *testing.T) {
		jobs := make([]CreateJobRequest, 101)
		for i := range jobs {
			jobs[i] = CreateJobRequest{
				Type:    "email",
				Payload: map[string]interface{}{"to": "user@example.com"},
			}
		}
		req := BatchJobRequest{Jobs: jobs}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for batch > 100")
		}
	})

	t.Run("invalid inner job fails batch", func(t *testing.T) {
		req := BatchJobRequest{
			Jobs: []CreateJobRequest{
				{Type: "email", Payload: map[string]interface{}{"to": "user@example.com"}},
				{Type: "", Payload: map[string]interface{}{"to": "user@example.com"}},
			},
		}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for invalid inner job")
		}
	})
}

func TestFromJob(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	job := &jobs.Job{
		ID:         "job-1",
		Type:       "email",
		Payload:    map[string]interface{}{"to": "user@example.com"},
		Labels:     map[string]string{"env": "test"},
		Priority:   jobs.PriorityHigh,
		Status:     jobs.StatusPending,
		Retries:    1,
		MaxRetries: 3,
		BackoffAlgorithm: jobs.BackoffExponential,
		BackoffJitter:    jobs.JitterNone,
		CronExpr:         "*/5 * * * *",
		CreatedAt:     now,
		UpdatedAt:     now,
		RunAt:         now,
		CorrelationID: "corr-1",
		Timeout:       30,
		Version:       2,
		TenantID:      "tenant-a",
		Paused:        false,
		Progress:      0.5,
		DedupKey:      "dedup-1",
		Dependencies:  []string{"job-0"},
		ShardKey:      "shard-1",
	}

	resp := FromJob(job)

	if resp.ID != "job-1" {
		t.Fatalf("expected ID job-1, got %s", resp.ID)
	}
	if resp.Type != "email" {
		t.Fatalf("expected type email, got %s", resp.Type)
	}
	if resp.Priority != "high" {
		t.Fatalf("expected priority high, got %s", resp.Priority)
	}
	if resp.Status != "pending" {
		t.Fatalf("expected status pending, got %s", resp.Status)
	}
	if resp.Retries != 1 {
		t.Fatalf("expected retries 1, got %d", resp.Retries)
	}
	if resp.MaxRetries != 3 {
		t.Fatalf("expected max retries 3, got %d", resp.MaxRetries)
	}
	if resp.BackoffAlgorithm != "exponential" {
		t.Fatalf("expected backoff exponential, got %s", resp.BackoffAlgorithm)
	}
	if resp.BackoffJitter != "none" {
		t.Fatalf("expected jitter none, got %s", resp.BackoffJitter)
	}
	if resp.CronExpr != "*/5 * * * *" {
		t.Fatalf("expected cron */5 * * * *, got %s", resp.CronExpr)
	}
	if resp.CreatedAt != now.Format("2006-01-02T15:04:05Z07:00") {
		t.Fatalf("unexpected created_at: %s", resp.CreatedAt)
	}
	if resp.CorrelationID != "corr-1" {
		t.Fatalf("expected correlation_id corr-1, got %s", resp.CorrelationID)
	}
	if resp.Timeout != 30 {
		t.Fatalf("expected timeout 30, got %d", resp.Timeout)
	}
	if resp.Version != 2 {
		t.Fatalf("expected version 2, got %d", resp.Version)
	}
	if resp.TenantID != "tenant-a" {
		t.Fatalf("expected tenant_id tenant-a, got %s", resp.TenantID)
	}
	if resp.DedupKey != "dedup-1" {
		t.Fatalf("expected dedup_key dedup-1, got %s", resp.DedupKey)
	}
	if len(resp.Dependencies) != 1 || resp.Dependencies[0] != "job-0" {
		t.Fatalf("unexpected dependencies: %v", resp.Dependencies)
	}
	if resp.ShardKey != "shard-1" {
		t.Fatalf("expected shard_key shard-1, got %s", resp.ShardKey)
	}
	if resp.Progress != 0.5 {
		t.Fatalf("expected progress 0.5, got %f", resp.Progress)
	}
}

func TestErrorResponse(t *testing.T) {
	errResp := ErrorResponse{Code: "NOT_FOUND", Error: "job not found"}
	if errResp.Code != "NOT_FOUND" {
		t.Fatalf("expected code NOT_FOUND, got %s", errResp.Code)
	}
	if errResp.Error != "job not found" {
		t.Fatalf("expected error message, got %s", errResp.Error)
	}
}

func TestBatchJobResponse(t *testing.T) {
	resp := BatchJobResponse{
		Successful: []BatchJobResult{
			{Index: 0, Job: &JobResponse{ID: "job-1"}},
		},
		Failed: []BatchJobError{
			{Index: 1, Error: "invalid type"},
		},
		Total:     2,
		Processed: 1,
	}
	if resp.Total != 2 {
		t.Fatalf("expected total 2, got %d", resp.Total)
	}
	if resp.Processed != 1 {
		t.Fatalf("expected processed 1, got %d", resp.Processed)
	}
	if len(resp.Successful) != 1 {
		t.Fatalf("expected 1 successful, got %d", len(resp.Successful))
	}
	if len(resp.Failed) != 1 {
		t.Fatalf("expected 1 failed, got %d", len(resp.Failed))
	}
}

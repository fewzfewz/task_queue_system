package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"task-queue-system/internal/api/middleware"
	"task-queue-system/internal/jobs"
	"task-queue-system/internal/storage/models"
)

func TestCheckJobTenantAccess_ClientIsolation(t *testing.T) {
	h, store := newTestHandler()
	job := &jobs.Job{ID: "j1", TenantID: "tenant-a", Type: "email", Status: jobs.StatusPending}
	_ = store.Save(context.Background(), job)

	tests := []struct {
		name       string
		ctxTenant  string
		wantStatus int
	}{
		{"client own tenant", "tenant-a", http.StatusOK},
		{"client other tenant", "tenant-b", http.StatusForbidden},
		{"operator unrestricted", "operator", http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/jobs/j1", nil)
			req.SetPathValue("id", "j1")
			ctx := context.WithValue(req.Context(), middleware.ContextKeyTenantID, tc.ctxTenant)
			ctx = context.WithValue(ctx, middleware.ContextKeyAuthType, middleware.AuthTypeAPIKey)
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()

			h.GetJobStatus(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("expected %d, got %d: %s", tc.wantStatus, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCountJobs_Efficient(t *testing.T) {
	store := models.NewInMemoryStore()
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		j := jobs.NewJob("email", map[string]interface{}{"to": "a@b.com"}, nil, jobs.PriorityMedium, 3, time.Time{}, "", 60, 1, "t1")
		j.Status = jobs.StatusCompleted
		_ = store.Save(ctx, j)
	}
	n, err := store.CountJobs(ctx, models.JobFilter{TenantID: "t1", Status: string(jobs.StatusCompleted)})
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("expected 5, got %d", n)
	}
}

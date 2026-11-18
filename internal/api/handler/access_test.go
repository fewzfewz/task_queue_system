package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"task-queue-system/internal/api/dto"
	"task-queue-system/internal/api/middleware"
	"task-queue-system/internal/jobs"
	"task-queue-system/internal/storage/models"
	"task-queue-system/internal/webhooks"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
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

func TestCreateJob_TenantIsolation(t *testing.T) {
	h, store := newTestHandler()

	tests := []struct {
		name         string
		ctxTenant    string
		reqTenant    string
		wantStatus   int
		expectTenant string
	}{
		{
			name:         "Tenant A submits without tenant ID -> accepted under Tenant A",
			ctxTenant:    "tenant-a",
			reqTenant:    "",
			wantStatus:   http.StatusCreated,
			expectTenant: "tenant-a",
		},
		{
			name:         "Tenant A submits with Tenant A's ID -> accepted",
			ctxTenant:    "tenant-a",
			reqTenant:    "tenant-a",
			wantStatus:   http.StatusCreated,
			expectTenant: "tenant-a",
		},
		{
			name:         "Tenant A submits with Tenant B's ID -> rejected with 403",
			ctxTenant:    "tenant-a",
			reqTenant:    "tenant-b",
			wantStatus:   http.StatusForbidden,
			expectTenant: "",
		},
		{
			name:         "Operator submits with Tenant B's ID -> accepted under Tenant B",
			ctxTenant:    "operator",
			reqTenant:    "tenant-b",
			wantStatus:   http.StatusCreated,
			expectTenant: "tenant-b",
		},
		{
			name:         "Operator submits without tenant ID -> accepted",
			ctxTenant:    "operator",
			reqTenant:    "",
			wantStatus:   http.StatusCreated,
			expectTenant: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]interface{}{
				"type":     "email",
				"payload":  map[string]interface{}{"to": "test@example.com"},
				"priority": "medium",
			}
			if tc.reqTenant != "" {
				payload["tenant_id"] = tc.reqTenant
			}
			body, _ := json.Marshal(payload)

			req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if tc.ctxTenant != "" {
				ctx := context.WithValue(req.Context(), middleware.ContextKeyTenantID, tc.ctxTenant)
				ctx = context.WithValue(ctx, middleware.ContextKeyAuthType, middleware.AuthTypeAPIKey)
				req = req.WithContext(ctx)
			}
			rec := httptest.NewRecorder()

			h.CreateJob(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", tc.wantStatus, rec.Code, rec.Body.String())
			}

			if tc.wantStatus == http.StatusCreated {
				var resp dto.JobResponse
				if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if resp.TenantID != tc.expectTenant {
					t.Fatalf("expected job tenant %q, got %q", tc.expectTenant, resp.TenantID)
				}
				job, err := store.GetByID(context.Background(), resp.ID)
				if err != nil {
					t.Fatalf("job not found in store: %v", err)
				}
				if job.TenantID != tc.expectTenant {
					t.Fatalf("expected store job tenant %q, got %q", tc.expectTenant, job.TenantID)
				}
			}
		})
	}
}

func TestCreateJobBatch_TenantIsolation(t *testing.T) {
	h, store := newTestHandler()

	tests := []struct {
		name         string
		ctxTenant    string
		reqTenants   []string
		wantStatus   int
		expectTenant string
	}{
		{
			name:         "Tenant A submits batch without tenant ID -> accepted under Tenant A",
			ctxTenant:    "tenant-a",
			reqTenants:   []string{"", ""},
			wantStatus:   http.StatusOK,
			expectTenant: "tenant-a",
		},
		{
			name:         "Tenant A submits batch with Tenant A's ID -> accepted",
			ctxTenant:    "tenant-a",
			reqTenants:   []string{"tenant-a", "tenant-a"},
			wantStatus:   http.StatusOK,
			expectTenant: "tenant-a",
		},
		{
			name:         "Tenant A submits batch with Tenant B's ID in one job -> rejected with 403",
			ctxTenant:    "tenant-a",
			reqTenants:   []string{"tenant-a", "tenant-b"},
			wantStatus:   http.StatusForbidden,
			expectTenant: "",
		},
		{
			name:         "Operator submits batch with mixed tenant IDs -> accepted",
			ctxTenant:    "operator",
			reqTenants:   []string{"tenant-a", "tenant-b"},
			wantStatus:   http.StatusOK,
			expectTenant: "mixed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var jobsList []map[string]interface{}
			for _, tid := range tc.reqTenants {
				j := map[string]interface{}{
					"type":     "email",
					"payload":  map[string]interface{}{"to": "test@example.com"},
					"priority": "medium",
				}
				if tid != "" {
					j["tenant_id"] = tid
				}
				jobsList = append(jobsList, j)
			}
			payload := map[string]interface{}{
				"jobs": jobsList,
			}
			body, _ := json.Marshal(payload)

			req := httptest.NewRequest(http.MethodPost, "/jobs/batch", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if tc.ctxTenant != "" {
				ctx := context.WithValue(req.Context(), middleware.ContextKeyTenantID, tc.ctxTenant)
				ctx = context.WithValue(ctx, middleware.ContextKeyAuthType, middleware.AuthTypeAPIKey)
				req = req.WithContext(ctx)
			}
			rec := httptest.NewRecorder()

			h.CreateJobBatch(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", tc.wantStatus, rec.Code, rec.Body.String())
			}

			if tc.wantStatus == http.StatusOK {
				var resp dto.BatchJobResponse
				if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if len(resp.Successful) != len(tc.reqTenants) {
					t.Fatalf("expected %d successful, got %d", len(tc.reqTenants), len(resp.Successful))
				}
				for i, res := range resp.Successful {
					expected := tc.expectTenant
					if tc.expectTenant == "mixed" {
						expected = tc.reqTenants[i]
					}
					if res.Job.TenantID != expected {
						t.Fatalf("job %d expected tenant %q, got %q", i, expected, res.Job.TenantID)
					}
					stored, err := store.GetByID(context.Background(), res.Job.ID)
					if err != nil {
						t.Fatalf("job %d not in store: %v", i, err)
					}
					if stored.TenantID != expected {
						t.Fatalf("stored job %d expected tenant %q, got %q", i, expected, stored.TenantID)
					}
				}
			}
		})
	}
}

func TestTenantJobListingIsolation(t *testing.T) {
	h, store := newTestHandler()

	// Tenant A creates a job
	jobA := jobs.NewJob("email", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 60, 1, "tenant-a")
	if err := store.Save(context.Background(), jobA); err != nil {
		t.Fatal(err)
	}

	// Tenant B queries jobs
	reqB := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	ctxB := context.WithValue(reqB.Context(), middleware.ContextKeyTenantID, "tenant-b")
	ctxB = context.WithValue(ctxB, middleware.ContextKeyAuthType, middleware.AuthTypeAPIKey)
	reqB = reqB.WithContext(ctxB)
	recB := httptest.NewRecorder()

	h.ListJobs(recB, reqB)
	if recB.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recB.Code, recB.Body.String())
	}
	var respB struct {
		Jobs []dto.JobResponse `json:"jobs"`
	}
	if err := json.NewDecoder(recB.Body).Decode(&respB); err != nil {
		t.Fatal(err)
	}
	if len(respB.Jobs) != 0 {
		t.Fatalf("expected 0 jobs in Tenant B listing, got %d", len(respB.Jobs))
	}

	// Tenant B tries to explicitly pass tenant_id=tenant-a -> 403 Forbidden
	reqBSpoof := httptest.NewRequest(http.MethodGet, "/jobs?tenant_id=tenant-a", nil)
	reqBSpoof = reqBSpoof.WithContext(ctxB)
	recBSpoof := httptest.NewRecorder()
	h.ListJobs(recBSpoof, reqBSpoof)
	if recBSpoof.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", recBSpoof.Code)
	}

	// Tenant A queries jobs -> sees own job
	reqA := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	ctxA := context.WithValue(reqA.Context(), middleware.ContextKeyTenantID, "tenant-a")
	ctxA = context.WithValue(ctxA, middleware.ContextKeyAuthType, middleware.AuthTypeAPIKey)
	reqA = reqA.WithContext(ctxA)
	recA := httptest.NewRecorder()

	h.ListJobs(recA, reqA)
	if recA.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recA.Code, recA.Body.String())
	}
	var respA struct {
		Jobs []dto.JobResponse `json:"jobs"`
	}
	if err := json.NewDecoder(recA.Body).Decode(&respA); err != nil {
		t.Fatal(err)
	}
	if len(respA.Jobs) != 1 || respA.Jobs[0].ID != jobA.ID {
		t.Fatalf("expected Tenant A to see job %q, got %+v", jobA.ID, respA.Jobs)
	}
}

func TestRegisterWebhook_TenantIsolation(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Skip("skipping: miniredis not available")
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ws := webhooks.NewWebhookStore(rdb)
	h, _ := newTestHandler()
	h.SetWebhookStore(ws)

	tests := []struct {
		name         string
		ctxTenant    string
		reqTenant    string
		url          string
		wantStatus   int
		expectTenant string
	}{
		{
			name:         "Client tenant registers without a tenant ID -> accepted under own tenant",
			ctxTenant:    "tenant-a",
			reqTenant:    "",
			url:          "https://example.com/webhook1",
			wantStatus:   http.StatusCreated,
			expectTenant: "tenant-a",
		},
		{
			name:         "Client tenant registers for its own tenant -> accepted",
			ctxTenant:    "tenant-a",
			reqTenant:    "tenant-a",
			url:          "https://example.com/webhook2",
			wantStatus:   http.StatusCreated,
			expectTenant: "tenant-a",
		},
		{
			name:         "Client tenant attempts another tenant -> rejected with HTTP 403",
			ctxTenant:    "tenant-a",
			reqTenant:    "tenant-b",
			url:          "https://example.com/webhook3",
			wantStatus:   http.StatusForbidden,
			expectTenant: "",
		},
		{
			name:         "Authorized operator registers for another tenant -> accepted under specified tenant",
			ctxTenant:    "operator",
			reqTenant:    "tenant-b",
			url:          "https://example.com/webhook4",
			wantStatus:   http.StatusCreated,
			expectTenant: "tenant-b",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]interface{}{
				"url":    tc.url,
				"secret": "testsecret",
				"events": []string{"completed"},
			}
			if tc.reqTenant != "" {
				payload["tenant_id"] = tc.reqTenant
			}
			body, _ := json.Marshal(payload)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if tc.ctxTenant != "" {
				ctx := context.WithValue(req.Context(), middleware.ContextKeyTenantID, tc.ctxTenant)
				ctx = context.WithValue(ctx, middleware.ContextKeyAuthType, middleware.AuthTypeAPIKey)
				req = req.WithContext(ctx)
			}
			rec := httptest.NewRecorder()

			h.RegisterWebhook(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", tc.wantStatus, rec.Code, rec.Body.String())
			}

			if tc.wantStatus == http.StatusCreated {
				var wh webhooks.RegisteredWebhook
				if err := json.NewDecoder(rec.Body).Decode(&wh); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if wh.TenantID != tc.expectTenant {
					t.Fatalf("response expected tenant %q, got %q", tc.expectTenant, wh.TenantID)
				}
				// Verify persisted tenant ownership in store
				stored, err := ws.GetByID(context.Background(), wh.ID)
				if err != nil {
					t.Fatalf("webhook not found in store: %v", err)
				}
				if stored.TenantID != tc.expectTenant {
					t.Fatalf("persisted webhook expected tenant %q, got %q", tc.expectTenant, stored.TenantID)
				}
			}
		})
	}
}

func TestWebhookMutationAndAccess_TenantIsolation(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Skip("skipping: miniredis not available")
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ws := webhooks.NewWebhookStore(rdb)
	h, _ := newTestHandler()
	h.SetWebhookStore(ws)

	ctx := context.Background()
	whA, err := ws.Create(ctx, "tenant-a", "https://tenant-a.com/hook", "secretA", []string{"completed"})
	if err != nil {
		t.Fatalf("failed to create webhook for tenant-a: %v", err)
	}

	whB, err := ws.Create(ctx, "tenant-b", "https://tenant-b.com/hook", "secretB", []string{"completed"})
	if err != nil {
		t.Fatalf("failed to create webhook for tenant-b: %v", err)
	}

	// 1. Tenant A accesses its own webhook -> 200 OK
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks/"+whA.ID, nil)
		req.SetPathValue("id", whA.ID)
		ctxTenantA := context.WithValue(req.Context(), middleware.ContextKeyTenantID, "tenant-a")
		rec := httptest.NewRecorder()
		h.GetWebhook(rec, req.WithContext(ctxTenantA))
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for tenant-a accessing own webhook, got %d", rec.Code)
		}
	}

	// 2. Tenant B attempts to read Tenant A's webhook -> 403 Forbidden
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks/"+whA.ID, nil)
		req.SetPathValue("id", whA.ID)
		ctxTenantB := context.WithValue(req.Context(), middleware.ContextKeyTenantID, "tenant-b")
		rec := httptest.NewRecorder()
		h.GetWebhook(rec, req.WithContext(ctxTenantB))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for tenant-b reading tenant-a webhook, got %d", rec.Code)
		}
	}

	// 3. Tenant B attempts to get Tenant A's deliveries -> 403 Forbidden
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks/"+whA.ID+"/deliveries", nil)
		req.SetPathValue("id", whA.ID)
		ctxTenantB := context.WithValue(req.Context(), middleware.ContextKeyTenantID, "tenant-b")
		rec := httptest.NewRecorder()
		h.GetWebhookDeliveries(rec, req.WithContext(ctxTenantB))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for tenant-b getting deliveries of tenant-a webhook, got %d", rec.Code)
		}
	}

	// 4. Tenant B attempts to update Tenant A's webhook -> 403 Forbidden
	{
		body, _ := json.Marshal(map[string]interface{}{"url": "https://attacker.com/leak"})
		req := httptest.NewRequest(http.MethodPut, "/api/v1/webhooks/"+whA.ID, bytes.NewReader(body))
		req.SetPathValue("id", whA.ID)
		req.Header.Set("Content-Type", "application/json")
		ctxTenantB := context.WithValue(req.Context(), middleware.ContextKeyTenantID, "tenant-b")
		rec := httptest.NewRecorder()
		h.UpdateWebhook(rec, req.WithContext(ctxTenantB))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for tenant-b updating tenant-a webhook, got %d", rec.Code)
		}
	}

	// 5. Tenant B attempts to delete Tenant A's webhook -> 403 Forbidden
	{
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/webhooks/"+whA.ID, nil)
		req.SetPathValue("id", whA.ID)
		ctxTenantB := context.WithValue(req.Context(), middleware.ContextKeyTenantID, "tenant-b")
		rec := httptest.NewRecorder()
		h.DeleteWebhook(rec, req.WithContext(ctxTenantB))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for tenant-b deleting tenant-a webhook, got %d", rec.Code)
		}
	}

	// 6. Operator can read Tenant A's webhook -> 200 OK
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks/"+whA.ID, nil)
		req.SetPathValue("id", whA.ID)
		ctxOp := context.WithValue(req.Context(), middleware.ContextKeyTenantID, "operator")
		rec := httptest.NewRecorder()
		h.GetWebhook(rec, req.WithContext(ctxOp))
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for operator reading tenant-a webhook, got %d", rec.Code)
		}
	}

	// 7. Operator can update Tenant A's webhook -> 200 OK
	{
		body, _ := json.Marshal(map[string]interface{}{"url": "https://tenant-a.com/updated"})
		req := httptest.NewRequest(http.MethodPut, "/api/v1/webhooks/"+whA.ID, bytes.NewReader(body))
		req.SetPathValue("id", whA.ID)
		req.Header.Set("Content-Type", "application/json")
		ctxOp := context.WithValue(req.Context(), middleware.ContextKeyTenantID, "operator")
		rec := httptest.NewRecorder()
		h.UpdateWebhook(rec, req.WithContext(ctxOp))
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for operator updating tenant-a webhook, got %d", rec.Code)
		}
	}

	// 8. Tenant A lists webhooks -> only tenant-a's webhooks returned
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks", nil)
		ctxTenantA := context.WithValue(req.Context(), middleware.ContextKeyTenantID, "tenant-a")
		rec := httptest.NewRecorder()
		h.ListWebhooks(rec, req.WithContext(ctxTenantA))
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for tenant-a listing webhooks, got %d", rec.Code)
		}
		var list []*webhooks.RegisteredWebhook
		if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
			t.Fatalf("failed to decode list: %v", err)
		}
		if len(list) != 1 || list[0].ID != whA.ID {
			t.Fatalf("expected exactly tenant-a's webhook in list, got %+v", list)
		}
	}

	// 9. Tenant A deletes its own webhook -> 204 No Content
	{
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/webhooks/"+whA.ID, nil)
		req.SetPathValue("id", whA.ID)
		ctxTenantA := context.WithValue(req.Context(), middleware.ContextKeyTenantID, "tenant-a")
		rec := httptest.NewRecorder()
		h.DeleteWebhook(rec, req.WithContext(ctxTenantA))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204 for tenant-a deleting own webhook, got %d", rec.Code)
		}
	}

	// 10. Verify Tenant B's webhook is still intact
	{
		storedB, err := ws.GetByID(ctx, whB.ID)
		if err != nil || storedB == nil {
			t.Fatalf("expected tenant-b webhook to be intact, got err: %v", err)
		}
	}
}

# Multi-Tenant Submission Path & Lifecycle Security Audit

**Document:** `TENANT_SUBMISSION_PATH_AUDIT.md`  
**Audit Date:** 2026-11-09  
**Auditor:** Senior Go Backend Engineer (Distributed Systems, Concurrency & Security)  
**Target Codebase:** Go Task Queue System (`commit b762762` + Phase 1 Remediations)  

---

## 1. Executive Summary

This security audit inspects all externally accessible HTTP routes, RPC/internal service entry points, asynchronous dispatch pipelines, and webhook handlers that:
1. Create or ingest jobs.
2. Submit batch jobs.
3. Schedule or promote delayed/recurring jobs.
4. Retry failed jobs or replay jobs from the Dead-Letter Queue (DLQ).
5. Accept caller-supplied tenant identifiers.

Each entry point has been evaluated against tenant isolation, identity derivation, authorization checks, and potential spoofing vectors.

---

## 2. Ingestion & Job Creation Entry Points

### 2.1 Single Job Submission (`POST /jobs`)
* **Entry Point:** [`POST /jobs`](file:///home/fewzan/Projects/task-queue-system/internal/api/routes/routes.go#L121) $\rightarrow$ [`JobHandler.CreateJob`](file:///home/fewzan/Projects/task-queue-system/internal/api/handler/handler.go#L123) $\rightarrow$ [`JobService.CreateJob`](file:///home/fewzan/Projects/task-queue-system/internal/service/job_service.go#L198)
* **Authentication Requirements:** `clientWrite` middleware:
  - `RequireAuth`: Validates `X-API-Key` (SHA-256 hash lookup in DB/cache) or session cookie.
  - `RequireAnyRole(RoleAdmin, RoleTenantClient)`.
  - CSRF protection enabled (`CSRFProtect`).
* **Source of Tenant Identity:** Request context via `middleware.TenantIDFromContext(r.Context())`. Populated by `RequireAuth` from the verified client record in the database.
* **Caller-Supplied Tenant IDs Accepted:**
  - In JSON payload: `req.TenantID`.
* **Existing Enforcement:**
  - **Handler Layer:** If caller is a client tenant (`middleware.IsClientTenant(ctxTenant)`), any explicit `req.TenantID != "" && req.TenantID != ctxTenant` is rejected with **HTTP 403 Forbidden** (`apperr.CodePermissionDenied`, `"access denied for tenant"`). If valid or omitted, `req.TenantID` is strictly assigned to `ctxTenant`.
  - **Service Layer (Defense-in-Depth):** [`JobService.CreateJob`](file:///home/fewzan/Projects/task-queue-system/internal/service/job_service.go#L198) inspects `ctx` using `middleware.TenantIDFromContext(ctx)`. If caller is a client tenant and passes a conflicting `tenantID`, it returns `apperr.NewForbidden("access denied for tenant")`.
  - **Operator Behavior:** When an operator/admin (`!middleware.IsClientTenant(ctxTenant)`) calls the endpoint, caller-supplied `tenant_id` is preserved to permit administrative job creation on behalf of specific tenants.
* **Unresolved Risk:** **None**. Remediated and verified in Phase 1.

---

### 2.2 Batch Job Submission (`POST /jobs/batch`)
* **Entry Point:** [`POST /jobs/batch`](file:///home/fewzan/Projects/task-queue-system/internal/api/routes/routes.go#L122) $\rightarrow$ [`JobHandler.CreateJobBatch`](file:///home/fewzan/Projects/task-queue-system/internal/api/handler/handler.go#L180) $\rightarrow$ [`JobService.CreateJobBatch`](file:///home/fewzan/Projects/task-queue-system/internal/service/job_service.go#L110)
* **Authentication Requirements:** `clientWrite` (`RequireAuth` + `RequireAnyRole(RoleAdmin, RoleTenantClient)` + CSRF).
* **Source of Tenant Identity:** Request context via `middleware.TenantIDFromContext(r.Context())`.
* **Caller-Supplied Tenant IDs Accepted:**
  - In JSON payload array: `req.Jobs[i].TenantID`.
* **Existing Enforcement:**
  - **Pre-Validation:** If caller is a client tenant, the handler inspects all elements in `req.Jobs`. If any element contains `jr.TenantID != "" && jr.TenantID != ctxTenant`, the entire batch request is rejected with **HTTP 403 Forbidden** before any state is mutated or enqueued.
  - **Assignment:** In the construction loop, `jr.TenantID` is strictly assigned to `ctxTenant`.
  - **Service Layer (Defense-in-Depth):** [`JobService.CreateJobBatch`](file:///home/fewzan/Projects/task-queue-system/internal/service/job_service.go#L110) checks context tenant and returns `apperr.NewForbidden("access denied for tenant")` if any batch item has a mismatched tenant ID.
  - **Operator Behavior:** Operator retains ability to submit batches with heterogeneous tenant IDs.
* **Unresolved Risk:** **None**. Remediated and verified in Phase 1.

---

## 3. Replay, Retry, and DLQ Lifecycle Paths

### 3.1 Dead-Letter Queue Job Replay (`POST /api/v1/dlq/{id}/replay`)
* **Entry Point:** [`POST /api/v1/dlq/{id}/replay`](file:///home/fewzan/Projects/task-queue-system/internal/api/routes/routes.go#L127) $\rightarrow$ [`JobHandler.ReplayFailedJob`](file:///home/fewzan/Projects/task-queue-system/internal/api/handler/handler.go#L1101) $\rightarrow$ [`JobService.ReplayJob`](file:///home/fewzan/Projects/task-queue-system/internal/service/job_service.go#L785)
* **Authentication Requirements:** `clientWrite` (`RequireAuth` + `RequireAnyRole(RoleAdmin, RoleTenantClient)` + CSRF).
* **Source of Tenant Identity:**
  - Job ID extracted from URL path (`/api/v1/dlq/{id}/replay`).
  - Tenant ID derived strictly from context:
    ```go
    tenantID := middleware.TenantIDFromContext(r.Context())
    if !middleware.IsClientTenant(tenantID) {
        tenantID = ""
    }
    ```
* **Caller-Supplied Tenant IDs Accepted:** **No**. No JSON body or query parameter accepted.
* **Existing Enforcement:**
  - [`JobService.ReplayJob`](file:///home/fewzan/Projects/task-queue-system/internal/service/job_service.go#L785) loads the failed job from the store.
  - Enforces:
    ```go
    if tenantID != "" && tenantID != "operator" && job.TenantID != tenantID {
        return nil, apperr.NewForbidden("you do not own this job")
    }
    ```
  - When re-enqueued, `job.TenantID` remains untouched.
* **Unresolved Risk:** **None**.

---

### 3.2 Dead-Letter Queue Single Job Deletion (`DELETE /api/v1/dlq/{id}`)
* **Entry Point:** [`DELETE /api/v1/dlq/{id}``](file:///home/fewzan/Projects/task-queue-system/internal/api/routes/routes.go#L128) $\rightarrow$ [`JobHandler.DeleteFailedJob`](file:///home/fewzan/Projects/task-queue-system/internal/api/handler/handler.go#L1139) $\rightarrow$ [`JobService.DeleteDLQJob`](file:///home/fewzan/Projects/task-queue-system/internal/service/job_service.go#L901)
* **Authentication Requirements:** `clientWrite`.
* **Source of Tenant Identity:** `middleware.TenantIDFromContext(r.Context())`.
* **Caller-Supplied Tenant IDs Accepted:** **No**.
* **Existing Enforcement:** Service layer verifies `job.TenantID == tenantID` before executing store deletion.
* **Unresolved Risk:** **None**.

---

### 3.3 Dead-Letter Queue Bulk Purge (`DELETE /api/v1/dlq`)
* **Entry Point:** [`DELETE /api/v1/dlq`](file:///home/fewzan/Projects/task-queue-system/internal/api/routes/routes.go#L130) $\rightarrow$ [`JobHandler.BulkPurgeDLQ`](file:///home/fewzan/Projects/task-queue-system/internal/api/handler/handler.go#L1164)
* **Authentication Requirements:** `operatorWrite` (`RequireRole(RoleAdmin)` + CSRF). Machine tenant clients are rejected with HTTP 403.
* **Source of Tenant Identity:** Operator query parameter `?tenant_id=...` (optional filter).
* **Caller-Supplied Tenant IDs Accepted:** Yes, by administrative design.
* **Existing Enforcement:** Strict role check (`RoleAdmin`).
* **Unresolved Risk:** **None**.

---

### 3.4 Worker Non-Blocking Retries (`worker_processor.go` $\rightarrow$ `delayed_jobs`)
* **Entry Point:** [`WorkerProcessor.handleFailure`](file:///home/fewzan/Projects/task-queue-system/internal/worker/executor/worker_processor.go#L320) $\rightarrow$ [`JobService.ScheduleRetry`](file:///home/fewzan/Projects/task-queue-system/internal/service/job_service.go)
* **Authentication Requirements:** Internal worker loop; no external network ingress.
* **Source of Tenant Identity:** Preserved from original job struct (`job.TenantID`).
* **Caller-Supplied Tenant IDs Accepted:** Not applicable.
* **Existing Enforcement:** `job.TenantID` is immutable across retry state transitions and Redis delayed sorted set indexing.
* **Unresolved Risk:** **None**.

---

## 4. Scheduled & Asynchronous Dispatch Pipelines

### 4.1 Scheduled Job Promotion (`cmd/scheduler`)
* **Entry Point:** [`cmd/scheduler/main.go`](file:///home/fewzan/Projects/task-queue-system/cmd/scheduler/main.go) $\rightarrow$ `queue.PromoteScheduledJobs(ctx)`
* **Authentication Requirements:** Internal background service.
* **Source of Tenant Identity:** Persisted in Redis sorted set / PostgreSQL database when scheduled via `POST /jobs`.
* **Caller-Supplied Tenant IDs Accepted:** Not applicable.
* **Existing Enforcement:** Moves existing jobs from sorted set to priority list; cannot alter `TenantID`.
* **Unresolved Risk:** **None** with respect to tenant spoofing. (Scheduler leader election is a separate availability risk identified as HIGH-02).

---

### 4.2 Recurring Cron Jobs (`RecurringJobsDue`)
* **Entry Point:** [`JobService.RecurringJobsDue`](file:///home/fewzan/Projects/task-queue-system/internal/service/job_service.go#L638)
* **Authentication Requirements:** Internal background scheduler tick.
* **Source of Tenant Identity:** Stored recurring parent job definition in the database.
* **Caller-Supplied Tenant IDs Accepted:** Not applicable.
* **Existing Enforcement:** Each new child job spawned inherits `TenantID: parent.TenantID`.
* **Unresolved Risk:** **None**.

---

### 4.3 Map-Reduce Workflow Orchestration (`MapReducePlugin`)
* **Entry Point:** [`internal/worker/plugins/standard/map_reduce.go`](file:///home/fewzan/Projects/task-queue-system/internal/worker/plugins/standard/map_reduce.go#L70)
* **Authentication Requirements:** Worker plugin runtime.
* **Source of Tenant Identity:** Root job struct `job.TenantID`.
* **Caller-Supplied Tenant IDs Accepted:** Not applicable.
* **Existing Enforcement:** Spawns `map` and `reduce` tasks with explicit parameter `job.TenantID`.
* **Unresolved Risk:** **None**.

---

## 5. Webhook Ingestion & Management Entry Points

### 5.1 Register Webhook (`POST /api/v1/webhooks`)
* **Entry Point:** [`POST /api/v1/webhooks`](file:///home/fewzan/Projects/task-queue-system/internal/api/routes/routes.go#L150) $\rightarrow$ [`JobHandler.RegisterWebhook`](file:///home/fewzan/Projects/task-queue-system/internal/api/handler/handler.go#L615)
* **Authentication Requirements:** `clientWrite` (`RequireAuth` + `RequireAnyRole(RoleAdmin, RoleTenantClient)` + CSRF).
* **Source of Tenant Identity:** `middleware.TenantIDFromContext(r.Context())`.
* **Caller-Supplied Tenant IDs Accepted:** In JSON payload (`req.TenantID`).
* **Existing Enforcement:**
  - If `middleware.IsClientTenant(tenantID)` is true, any mismatch `req.TenantID != "" && req.TenantID != tenantID` returns **HTTP 403 Forbidden**.
  - Persisted record in `webhookStore` always uses `tenantID`.
  - Verified by comprehensive test suite [`TestRegisterWebhook_TenantIsolation`](file:///home/fewzan/Projects/task-queue-system/internal/api/handler/access_test.go#L326).
* **Unresolved Risk:** **None**.

---

### 5.2 Webhook Lifecycle by ID (`GET`, `PUT`, `DELETE /api/v1/webhooks/{id}`)
* **Entry Points:**
  - `GET /api/v1/webhooks/{id}` ([`GetWebhook`](file:///home/fewzan/Projects/task-queue-system/internal/api/handler/handler.go#L685))
  - `GET /api/v1/webhooks/{id}/deliveries` ([`GetWebhookDeliveries`](file:///home/fewzan/Projects/task-queue-system/internal/api/handler/handler.go#L706))
  - `PUT /api/v1/webhooks/{id}` ([`UpdateWebhook`](file:///home/fewzan/Projects/task-queue-system/internal/api/handler/handler.go#L740))
  - `DELETE /api/v1/webhooks/{id}` ([`DeleteWebhook`](file:///home/fewzan/Projects/task-queue-system/internal/api/handler/handler.go#L769))
* **Authentication Requirements:**
  - `GetWebhook`: `clientRead` (`RequireAuth`).
  - `GetWebhookDeliveries`: `clientRead` (`RequireAuth`).
  - `UpdateWebhook`: `clientWrite` (`RequireAuth` + `RequireAnyRole(RoleAdmin, RoleTenantClient)` + CSRF).
  - `DeleteWebhook`: `clientWrite` (`RequireAuth` + `RequireAnyRole(RoleAdmin, RoleTenantClient)` + CSRF).
* **Source of Tenant Identity:** Path parameter `{id}` only.
* **Caller-Supplied Tenant IDs Accepted:** None.
* **Existing Enforcement:**
  - **FINDING:** The handler retrieves the webhook solely by ID via `h.webhookStore.GetByID(r.Context(), id)`.
  - **Neither the handler nor the store checks whether `wh.TenantID == ctxTenant`**.
* **Confirmed Security Vulnerability:**
  > [!WARNING]
  > **[SEC-WEBHOOK-01] Cross-Tenant Webhook Modification / Deletion / Inspection:**  
  > If Tenant A discovers or enumerates the UUID of a webhook registered by Tenant B, Tenant A can:
  > 1. View Tenant B's webhook URL and secret (`GET /api/v1/webhooks/{id}`).
  > 2. View Tenant B's delivery history (`GET /api/v1/webhooks/{id}/deliveries`).
  > 3. Redirect Tenant B's webhook events to Tenant A's endpoint by modifying the URL (`PUT /api/v1/webhooks/{id}`).
  > 4. Delete Tenant B's webhook (`DELETE /api/v1/webhooks/{id}`).
  >
  > *Remediation Recommendation:* In [`handler.go`](file:///home/fewzan/Projects/task-queue-system/internal/api/handler/handler.go), implement a `loadWebhookForTenant(w, r, id)` helper (identical to `loadJobForTenant`) that rejects requests with HTTP 403 when `middleware.IsClientTenant(ctxTenant) && wh.TenantID != ctxTenant`.
  > *Status:* Reported separately; awaiting user approval before modifying code.

---

## 6. Job State Mutation Routes

| Route | Handler Method | Auth Role | Tenant Check Mechanism | Status |
| :--- | :--- | :--- | :--- | :--- |
| `GET /jobs` | `ListJobs` | Client/Admin | `filter.TenantID == ctxTenant`; 403 on mismatch | **SECURE** |
| `GET /jobs/{id}` | `GetJobStatus` | Client/Admin | `checkJobTenantAccess(job.TenantID == ctxTenant)`; 403 on mismatch | **SECURE** |
| `POST /jobs/{id}/cancel` | `CancelJob` | Client/Admin | `loadJobForTenant`; 403 on mismatch | **SECURE** |
| `POST /jobs/{id}/pause` | `PauseJob` | Client/Admin | `loadJobForTenant`; 403 on mismatch | **SECURE** |
| `POST /jobs/{id}/resume` | `ResumeJob` | Client/Admin | `loadJobForTenant`; 403 on mismatch | **SECURE** |
| `PATCH /jobs/{id}/progress` | `UpdateJobProgress` | Client/Admin | `loadJobForTenant`; 403 on mismatch | **SECURE** |
| `GET /api/v1/jobs/{id}/deps`| `GetJobDeps` | Client/Admin | `checkJobTenantAccess`; 403 on mismatch | **SECURE** |

---

## 7. Administrative & Operator Control Routes

| Route | Handler Method | Auth Role | Tenant Parameter Behavior | Status |
| :--- | :--- | :--- | :--- | :--- |
| `DELETE /api/v1/clients/{tenant_id}` | `RevokeClient` | `operatorWrite` (`RoleAdmin`) | Target tenant path param; operator only | **SECURE** |
| `POST /api/v1/clients/{tenant_id}/rotate` | `RotateClientKey` | `operatorWrite` (`RoleAdmin`) | Target tenant path param; operator only | **SECURE** |
| `PUT /api/v1/clients/{tenant_id}/config` | `UpdateClientConfig` | `operatorWrite` (`RoleAdmin`) | Target tenant path param; operator only | **SECURE** |
| `POST /api/v1/operator/queue/{type}/pause` | `PauseQueue` | `operatorWrite` (`RoleAdmin`) | Queue type only; operator only | **SECURE** |
| `POST /api/v1/operator/queue/{type}/resume`| `ResumeQueue` | `operatorWrite` (`RoleAdmin`) | Queue type only; operator only | **SECURE** |
| `POST /api/v1/job-types` | `CreateJobType` | `operatorWrite` (`RoleAdmin`) | Global type registration; operator only | **SECURE** |
| `DELETE /api/v1/job-types/{name}` | `DeleteJobType` | `operatorWrite` (`RoleAdmin`) | Global type deletion; operator only | **SECURE** |

---

## 8. Summary of Findings

1. **Job Ingestion & Batch Ingestion:** Fully secured against tenant spoofing at both handler and service layers.
2. **Webhook Registration:** Fully secured against tenant spoofing.
3. **Job Mutation & DLQ Replay:** Fully secured via `loadJobForTenant` and service-layer tenant ownership checks.
4. **New Security Finding ([SEC-WEBHOOK-01]):** Webhook retrieval, updating, delivery listing, and deletion (`GET/PUT/DELETE /api/v1/webhooks/{id}`) currently lack tenant ownership checks. A tenant who knows another tenant's webhook ID can read, modify, or delete it. This is documented for stakeholder approval prior to code changes.

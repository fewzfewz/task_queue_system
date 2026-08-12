package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"task-queue-system/internal/api/middleware"
)

type RegisterClientRequest struct {
	TenantID string `json:"tenant_id"`
}

type RegisterClientResponse struct {
	TenantID string `json:"tenant_id"`
	APIKey   string `json:"api_key"`
	Message  string `json:"message"`
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// RegisterClient handles POST /api/v1/register
func (h *JobHandler) RegisterClient(w http.ResponseWriter, r *http.Request) {
	if h.registerLimiter != nil && !h.registerLimiter.Allow(clientIP(r)) {
		middleware.SendJSONError(w, http.StatusTooManyRequests, "TOO_MANY_REQUESTS", "registration rate limit exceeded")
		return
	}

	var req RegisterClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.SendJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json payload")
		return
	}

	if req.TenantID == "" {
		middleware.SendJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "tenant_id is required")
		return
	}

	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		middleware.SendJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate key")
		return
	}

	rawKey := "tq_live_" + hex.EncodeToString(keyBytes)
	hash := sha256.Sum256([]byte(rawKey))
	hashHex := hex.EncodeToString(hash[:])

	if err := h.service.Store().RegisterClient(r.Context(), req.TenantID, hashHex); err != nil {
		h.logger.Error("failed to register client", "error", err, "tenant_id", req.TenantID)
		middleware.SendJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to register client")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(RegisterClientResponse{
		TenantID: req.TenantID,
		APIKey:   rawKey,
		Message:  "Store this API key safely. It will not be shown again.",
	})
}

// GetClientInfo handles GET /api/v1/client/me — returns the tenant for the current API key.
func (h *JobHandler) GetClientInfo(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantIDFromContext(r.Context())
	if !middleware.IsClientTenant(tenantID) {
		middleware.SendJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "not a client API key")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"tenant_id": tenantID})
}

// RevokeClient handles DELETE /api/v1/clients/{tenant_id} — admin only.
func (h *JobHandler) RevokeClient(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenant_id")
	if tenantID == "" {
		middleware.SendJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "tenant_id is required")
		return
	}
	if err := h.service.RevokeClient(r.Context(), tenantID); err != nil {
		middleware.SendJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to revoke client")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RotateClientKey handles POST /api/v1/clients/{tenant_id}/rotate — admin only.
func (h *JobHandler) RotateClientKey(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenant_id")
	if tenantID == "" {
		middleware.SendJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "tenant_id is required")
		return
	}
	rawKey, err := h.service.RotateClientKey(r.Context(), tenantID)
	if err != nil {
		middleware.SendJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to rotate key")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"tenant_id": tenantID,
		"api_key":   rawKey,
		"message":   "Store this new API key safely. The old key is now invalid.",
	})
}

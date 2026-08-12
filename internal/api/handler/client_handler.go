package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"

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

// RegisterClient handles POST /api/v1/register
func (h *JobHandler) RegisterClient(w http.ResponseWriter, r *http.Request) {
	var req RegisterClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.SendJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json payload")
		return
	}

	if req.TenantID == "" {
		middleware.SendJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "tenant_id is required")
		return
	}

	// Generate a secure random 32-byte API key
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		middleware.SendJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate key")
		return
	}
	
	rawKey := "tq_live_" + hex.EncodeToString(keyBytes)

	// Hash the key using SHA-256 for storage
	hash := sha256.Sum256([]byte(rawKey))
	hashHex := hex.EncodeToString(hash[:])

	// Store it
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

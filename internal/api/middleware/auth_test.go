package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"task-queue-system/internal/config"
)

func TestAuthRequired(t *testing.T) {
	cfg := &config.Config{
		ApiKey: "test-api-key",
	}

	handler := AuthRequired(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "OK")
	}))

	tests := []struct {
		name           string
		apiKeyHeader   string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Valid API Key",
			apiKeyHeader:   "test-api-key",
			expectedStatus: http.StatusOK,
			expectedBody:   "OK",
		},
		{
			name:           "Wrong API Key",
			apiKeyHeader:   "wrong-key",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Missing API Key",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/jobs", nil)
			if tt.apiKeyHeader != "" {
				req.Header.Set("X-API-Key", tt.apiKeyHeader)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}
			if tt.expectedBody != "" && rr.Body.String() != tt.expectedBody {
				t.Errorf("expected body %q, got %q", tt.expectedBody, rr.Body.String())
			}
		})
	}
}

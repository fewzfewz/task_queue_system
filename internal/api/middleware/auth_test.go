package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"task-queue-system/internal/config"

	"github.com/golang-jwt/jwt/v5"
)

func TestAuthRequired(t *testing.T) {
	// ── Setup RSA Keys ────────────────────────────────────────────────────────
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	publicKey := &privateKey.PublicKey
	pubKeyBytes, _ := x509.MarshalPKIXPublicKey(publicKey)
	pubKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKeyBytes,
	})

	cfg := &config.Config{
		ApiKey:       "test-api-key",
		JwtPublicKey: string(pubKeyPEM),
	}

	handler := AuthRequired(cfg, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID, _ := r.Context().Value(ContextKeyTenantID).(string)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("OK:%s", tenantID)))
	}))

	tests := []struct {
		name           string
		authHeader     string
		apiKeyHeader   string
		tokenFactory   func() string
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "Valid Token",
			tokenFactory: func() string {
				token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
					"tenant_id": "tenant-123",
					"exp":       time.Now().Add(time.Hour).Unix(),
				})
				s, _ := token.SignedString(privateKey)
				return "Bearer " + s
			},
			expectedStatus: http.StatusOK,
			expectedBody:   "OK:tenant-123",
		},
		{
			name: "Expired Token",
			tokenFactory: func() string {
				token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
					"tenant_id": "tenant-123",
					"exp":       time.Now().Add(-time.Hour).Unix(),
				})
				s, _ := token.SignedString(privateKey)
				return "Bearer " + s
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "Missing Tenant Claim",
			tokenFactory: func() string {
				token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
					"exp": time.Now().Add(time.Hour).Unix(),
				})
				s, _ := token.SignedString(privateKey)
				return "Bearer " + s
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "Wrong Algorithm (HS256)",
			tokenFactory: func() string {
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
					"tenant_id": "tenant-123",
					"exp":       time.Now().Add(time.Hour).Unix(),
				})
				s, _ := token.SignedString([]byte("secret"))
				return "Bearer " + s
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Legacy X-API-Key Success",
			apiKeyHeader:   "test-api-key",
			expectedStatus: http.StatusOK,
			expectedBody:   "OK:", // No tenant_id in legacy mode unless we add logic for it
		},
		{
			name:           "Legacy X-API-Key Failure",
			apiKeyHeader:   "wrong-key",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Missing Authorization",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/jobs", nil)
			if tt.tokenFactory != nil {
				req.Header.Set("Authorization", tt.tokenFactory())
			} else if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
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

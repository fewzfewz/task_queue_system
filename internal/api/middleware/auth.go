package middleware

import (
	"context"
	"crypto/rsa"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"task-queue-system/internal/config"
	"task-queue-system/internal/secrets"
)

type ContextKey string

const (
	ContextKeyTenantID ContextKey = "tenant_id"
	ContextKeyScopes   ContextKey = "scopes"
)

// AuthRequired returns a middleware that validates JWTs (RS256) or falls back to X-API-Key.
func AuthRequired(cfg *config.Config, secrets secrets.SecretsProvider) func(http.Handler) http.Handler {
	pubKey, err := loadPublicKey(cfg)
	if err != nil {
		fmt.Printf("Warning: failed to load JWT public key: %v\n", err)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")

			// 1. JWT Authentication
			if strings.HasPrefix(authHeader, "Bearer ") {
				tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
				
				if pubKey == nil {
					sendJSONError(w, http.StatusUnauthorized, "UNAUTHORIZED", "JWT authentication not configured on server")
					return
				}

				token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
					if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
						return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
					}
					return pubKey, nil
				})

				if err != nil || !token.Valid {
					sendJSONError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or expired token")
					return
				}

				claims, ok := token.Claims.(jwt.MapClaims)
				if !ok {
					sendJSONError(w, http.StatusUnauthorized, "UNAUTHORIZED", "failed to parse claims")
					return
				}

				tenantID, _ := claims["tenant_id"].(string)
				if tenantID == "" {
					sendJSONError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant_id claim")
					return
				}

				ctx := context.WithValue(r.Context(), ContextKeyTenantID, tenantID)
				if scopes, ok := claims["scopes"].([]interface{}); ok {
					ctx = context.WithValue(ctx, ContextKeyScopes, scopes)
				}
				
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// 2. Legacy X-API-Key Fallback
			clientKey := r.Header.Get("X-API-Key")
			tenantID := r.Header.Get("X-Tenant-ID")

			if clientKey != "" {
				// Use Vault/Env secrets if tenantID provided
				if tenantID != "" && secrets != nil {
					expectedKey, err := secrets.GetSecret(r.Context(), tenantID)
					if err == nil && expectedKey != "" {
						if clientKey == expectedKey {
							ctx := context.WithValue(r.Context(), ContextKeyTenantID, tenantID)
							next.ServeHTTP(w, r.WithContext(ctx))
							return
						}
					}
				}

				// Fallback to global config key
				if clientKey == cfg.ApiKey {
					next.ServeHTTP(w, r)
					return
				}
				sendJSONError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid legacy API key or tenant")
				return
			}

			sendJSONError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing authorization")
		})
	}
}

func loadPublicKey(cfg *config.Config) (*rsa.PublicKey, error) {
	var keyData []byte
	var err error

	if cfg.JwtPublicKeyPath != "" {
		keyData, err = os.ReadFile(cfg.JwtPublicKeyPath)
		if err != nil {
			return nil, err
		}
	} else if cfg.JwtPublicKey != "" {
		keyData = []byte(cfg.JwtPublicKey)
	} else {
		return nil, nil // No JWT configuration provided
	}

	return jwt.ParseRSAPublicKeyFromPEM(keyData)
}

func sendJSONError(w http.ResponseWriter, status int, code, err string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"code": "%s", "error": "%s"}`, code, err)
}

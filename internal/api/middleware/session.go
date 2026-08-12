package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"task-queue-system/internal/api/session"
	"task-queue-system/internal/config"
	"task-queue-system/internal/storage/models"
)

// SessionCookieName is the httpOnly cookie that carries the session ID.
const SessionCookieName = "tq_session"

// apiKeyCacheEntry caches a verified API key so we don't hit the DB on every request.
type apiKeyCacheEntry struct {
	tenantID  string
	expiresAt time.Time
}

// apiKeyCache is a short-lived in-memory cache for verified API keys (hash -> tenantID).
type apiKeyCache struct {
	mu    sync.RWMutex
	store map[string]apiKeyCacheEntry
}

func newAPIKeyCache() *apiKeyCache {
	return &apiKeyCache{store: make(map[string]apiKeyCacheEntry)}
}

func (c *apiKeyCache) get(hash string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.store[hash]
	if !ok || time.Now().After(e.expiresAt) {
		return "", false
	}
	return e.tenantID, true
}

func (c *apiKeyCache) set(hash, tenantID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[hash] = apiKeyCacheEntry{tenantID: tenantID, expiresAt: time.Now().Add(60 * time.Second)}
}

// RequireAuth authenticates a request either with a dynamic per-tenant X-API-Key header
// (machine clients, verified against the DB) or a valid session cookie (the operator UI).
// It injects the auth type, role, and tenant_id into the request context.
func RequireAuth(cfg *config.Config, sessions *session.Store, store models.Store) func(http.Handler) http.Handler {
	cache := newAPIKeyCache()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			rawKey := r.Header.Get("X-API-Key")

			// EventSource cannot set custom request headers, so the client
			// portal passes its API key as a query parameter on the SSE
			// endpoints only. Keep the fallback scoped to those routes.
			if rawKey == "" && isSSEEndpoint(r.URL.Path) {
				rawKey = r.URL.Query().Get("api_key")
			}

			if rawKey != "" {
				// Fast-path: operator's own static API key grants admin access
				// without a DB round-trip.
				if cfg.ApiKey != "" && rawKey == cfg.ApiKey {
					ctx = context.WithValue(ctx, ContextKeyAuthType, AuthTypeAPIKey)
					ctx = context.WithValue(ctx, ContextKeyRole, RoleAdmin)
					ctx = context.WithValue(ctx, ContextKeyTenantID, "operator")
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}

				// Hash the incoming key for safe DB lookup
				sum := sha256.Sum256([]byte(rawKey))
				hashHex := hex.EncodeToString(sum[:])

				// Check cache first to avoid hitting DB every request
				tenantID, cached := cache.get(hashHex)
				if !cached {
					var err error
					tenantID, err = store.VerifyClient(ctx, hashHex)
					if err != nil {
						sendJSONError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid API key")
						return
					}
					cache.set(hashHex, tenantID)
				}

				ctx = context.WithValue(ctx, ContextKeyAuthType, AuthTypeAPIKey)
				ctx = context.WithValue(ctx, ContextKeyRole, RoleAdmin)
				ctx = context.WithValue(ctx, ContextKeyTenantID, tenantID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			if c, err := r.Cookie(SessionCookieName); err == nil && c.Value != "" {
				if sess, ok := sessions.Get(c.Value); ok {
					ctx = context.WithValue(ctx, ContextKeySession, sess)
					ctx = context.WithValue(ctx, ContextKeyAuthType, AuthTypeSession)
					ctx = context.WithValue(ctx, ContextKeyRole, sess.Role)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			sendJSONError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid credentials")
		})
	}
}

// RequireRole rejects requests whose authenticated role does not match the
// required role. It must run behind RequireAuth.
func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if RoleFromContext(r.Context()) != role {
				sendJSONError(w, http.StatusForbidden, "FORBIDDEN", "insufficient permissions")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CSRFProtect requires a valid X-CSRF-Token header on state-changing requests
// that are authenticated via a session cookie. API-key requests are exempt:
// the shared key can only be used deliberately, never by a drive-by browser.
func CSRFProtect() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Context().Value(ContextKeyAuthType) == AuthTypeSession && stateChanging(r.Method) {
				sess := SessionFromContext(r.Context())
				if sess == nil || r.Header.Get("X-CSRF-Token") != sess.CSRF {
					sendJSONError(w, http.StatusForbidden, "CSRF_FAILED", "missing or invalid CSRF token")
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func stateChanging(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// isSSEEndpoint reports whether the path is one of the server-sent event
// stream routes — the only place an API key may be accepted as a query param.
func isSSEEndpoint(path string) bool {
	switch path {
	case "/events", "/api/v1/events":
		return true
	}
	return false
}

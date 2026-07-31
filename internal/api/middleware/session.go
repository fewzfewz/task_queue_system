package middleware

import (
	"context"
	"net/http"

	"task-queue-system/internal/api/session"
	"task-queue-system/internal/config"
)

// SessionCookieName is the httpOnly cookie that carries the session ID.
const SessionCookieName = "tq_session"

// RequireAuth authenticates a request either with the shared X-API-Key header
// (machine clients) or a valid session cookie (the operator UI). It injects the
// auth type and role into the request context and rejects everything else.
func RequireAuth(cfg *config.Config, sessions *session.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			if key := r.Header.Get("X-API-Key"); key != "" && key == cfg.ApiKey {
				ctx = context.WithValue(ctx, ContextKeyAuthType, AuthTypeAPIKey)
				ctx = context.WithValue(ctx, ContextKeyRole, RoleAdmin)
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

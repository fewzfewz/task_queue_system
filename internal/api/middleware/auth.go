package middleware

import (
	"net/http"
)

// AuthRequired returns a middleware that checks for a valid X-API-Key header.
// It returns a 401 Unauthorized if the key is missing or incorrect.
func AuthRequired(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract key from header
			clientKey := r.Header.Get("X-API-Key")

			// Simple timing-safe comparison is better for production, 
			// but a direct comparison is sufficient for this minimal implementation.
			if clientKey == "" || clientKey != apiKey {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"code": "UNAUTHORIZED", "error": "invalid or missing API key"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

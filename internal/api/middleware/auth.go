package middleware

import (
	"fmt"
	"net/http"

	"task-queue-system/internal/config"
)

// AuthRequired returns a middleware that validates X-API-Key header.
func AuthRequired(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientKey := r.Header.Get("X-API-Key")

			if clientKey == "" || clientKey != cfg.ApiKey {
				sendJSONError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid API key")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func sendJSONError(w http.ResponseWriter, status int, code, err string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"code": "%s", "error": "%s"}`, code, err)
}

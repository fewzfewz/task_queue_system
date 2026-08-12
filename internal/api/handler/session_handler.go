package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"task-queue-system/internal/api/middleware"
	apperr "task-queue-system/internal/errors"
	"task-queue-system/internal/sse"
)

// Login handles POST /api/v1/login.
//
// It verifies the operator credentials and, on success, issues a server-side
// session delivered as an httpOnly cookie. The shared API key is never exposed
// to the browser. The response includes the per-session CSRF token plus the
// assigned role so the UI can gate destructive actions.
//
// @Summary      Login
// @Description  Authenticates with operator credentials and starts a server-side session.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      object{username=string,password=string}  true  "Login credentials"
// @Success      200   {object}  map[string]interface{}
// @Failure      401   {object}  dto.ErrorResponse
// @Failure      429   {object}  dto.ErrorResponse
// @Router       /api/v1/login [post]
func (h *JobHandler) Login(w http.ResponseWriter, r *http.Request) {
	if !h.loginLimiter.Allow(clientIP(r)) {
		h.writeError(w, http.StatusTooManyRequests, apperr.CodeTooManyRequests, "too many login attempts, try again later")
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, apperr.CodeInvalidArgument, "invalid JSON body")
		return
	}
	defer r.Body.Close()

	if req.Username == "" || req.Password == "" {
		h.writeError(w, http.StatusBadRequest, apperr.CodeInvalidArgument, "username and password required")
		return
	}

	role := ""
	switch {
	case req.Username == h.adminUsername && req.Password == h.adminPassword:
		role = middleware.RoleAdmin
	case h.readonlyUsername != "" && req.Username == h.readonlyUsername && req.Password == h.readonlyPassword:
		role = middleware.RoleViewer
	default:
		h.writeError(w, http.StatusUnauthorized, apperr.CodeUnauthorized, "invalid credentials")
		return
	}

	sess, err := h.sessions.Create(req.Username, role)
	if err != nil {
		h.logger.Error("failed to create session", "error", err)
		h.writeError(w, http.StatusInternalServerError, apperr.CodeInternal, "an unexpected error occurred")
		return
	}

	http.SetCookie(w, sessionCookie(sess.ID, sess.ExpiresAt))

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"username":    sess.Username,
		"role":        sess.Role,
		"csrf_token":  sess.CSRF,
		"expires_at":  sess.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

// Logout handles POST /api/v1/logout. It revokes the session server-side so the
// cookie is useless even if the browser does not clear it.
//
// @Summary      Logout
// @Description  Revokes the current session and clears the session cookie.
// @Tags         auth
// @Success      204  "No Content"
// @Router       /api/v1/logout [post]
func (h *JobHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if sess := middleware.SessionFromContext(r.Context()); sess != nil {
		h.sessions.Delete(sess.ID)
	}
	http.SetCookie(w, sessionCookie("", time.Now().Add(-time.Hour)))
	w.WriteHeader(http.StatusNoContent)
}

// GetSession handles GET /api/v1/session. It reports whether the caller holds a
// valid session and, if so, its identity, role and CSRF token. The endpoint is
// intentionally unauthenticated so the SPA can check state on every load.
//
// @Summary      Get current session
// @Description  Returns the current session state, role and CSRF token.
// @Tags         auth
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Router       /api/v1/session [get]
func (h *JobHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(middleware.SessionCookieName)
	if err != nil {
		h.writeJSON(w, http.StatusOK, map[string]interface{}{"authenticated": false})
		return
	}

	sess, ok := h.sessions.Get(c.Value)
	if !ok {
		h.writeJSON(w, http.StatusOK, map[string]interface{}{"authenticated": false})
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"authenticated": true,
		"username":      sess.Username,
		"role":          sess.Role,
		"csrf_token":    sess.CSRF,
		"expires_at":    sess.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

// GetCircuitBreakers proxies the worker's circuit-breaker status through the
// authenticated API so the browser never reaches the worker directly.
//
// @Summary      List circuit breakers
// @Description  Returns the current circuit-breaker state for every plugin.
// @Tags         circuit-breaker
// @Produce      json
// @Success      200  {object}  map[string]string
// @Router       /api/v1/circuit-breakers [get]
func (h *JobHandler) GetCircuitBreakers(w http.ResponseWriter, r *http.Request) {
	h.proxyWorker(w, r, "/circuit-breaker")
}

// ResetCircuitBreaker proxies a circuit-breaker reset to the worker.
//
// @Summary      Reset a circuit breaker
// @Description  Closes the circuit breaker for a plugin type.
// @Tags         circuit-breaker
// @Produce      json
// @Param        type  path  string  true  "Plugin type"
// @Success      200   {object}  map[string]string
// @Router       /api/v1/circuit-breakers/reset/{type} [post]
func (h *JobHandler) ResetCircuitBreaker(w http.ResponseWriter, r *http.Request) {
	jobType := r.PathValue("type")
	if jobType == "" {
		h.writeError(w, http.StatusBadRequest, apperr.CodeInvalidArgument, "missing plugin type in path")
		return
	}
	h.proxyWorker(w, r, "/circuit-breaker/reset/"+url.PathEscape(jobType))
	if r.Context().Err() == nil && h.sseBroker != nil {
		h.sseBroker.Publish(sse.Event{
			Kind:   "circuit_breaker",
			Type:   jobType,
			Status: "reset",
		})
	}
}

func (h *JobHandler) proxyWorker(w http.ResponseWriter, r *http.Request, path string) {
	target := "http://" + h.workerAddr + path
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, nil)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, apperr.CodeInternal, "an unexpected error occurred")
		return
	}
	req.Header.Set("X-API-Key", h.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.writeError(w, http.StatusBadGateway, apperr.CodeInternal, "worker unreachable")
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func sessionCookie(id string, expires time.Time) *http.Cookie {
	c := &http.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	}
	if id == "" {
		c.MaxAge = -1
	} else {
		c.Expires = expires
	}
	return c
}


package middleware

import (
	"context"

	"task-queue-system/internal/api/session"
)

type ContextKey string

const (
	ContextKeyTenantID ContextKey = "tenant_id"
	ContextKeyScopes   ContextKey = "scopes"
	ContextKeySession  ContextKey = "session"
	ContextKeyAuthType ContextKey = "auth_type"
	ContextKeyRole     ContextKey = "role"
)

// Auth types stored under ContextKeyAuthType.
const (
	AuthTypeAPIKey  = "api_key"
	AuthTypeSession = "session"
)

// Roles assigned to authenticated principals.
const (
	RoleAdmin  = "admin"
	RoleViewer = "viewer"
)

// SessionFromContext returns the authenticated session, if any.
func SessionFromContext(ctx context.Context) *session.Session {
	s, _ := ctx.Value(ContextKeySession).(*session.Session)
	return s
}

// RoleFromContext returns the authenticated principal's role.
func RoleFromContext(ctx context.Context) string {
	role, _ := ctx.Value(ContextKeyRole).(string)
	return role
}

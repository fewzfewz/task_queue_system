package middleware

type ContextKey string

const (
	ContextKeyTenantID ContextKey = "tenant_id"
	ContextKeyScopes   ContextKey = "scopes"
)

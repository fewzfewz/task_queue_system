# Secrets Management

The task queue system supports three tiers of secret provisioning,
each appropriate for different deployment environments.

## 1. Environment Variables (default)

Used when no Vault address is configured. Secrets are read directly
from process environment variables.

```bash
export API_KEY=my-secret-key
export JWT_PUBLIC_KEY="$(cat path/to/public.pem)"
export REDIS_PASSWORD=my-redis-password
export POSTGRES_CONN_STR="postgres://user:pass@host:5432/db"
```

**When to use:** local development, single-node deployments, CI.

## 2. Kubernetes Secrets (production K8s)

Secrets are stored as Kubernetes Secret objects and mounted as env vars.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: task-queue-secrets
  namespace: task-queue
type: Opaque
stringData:
  api-key: "my-secret-key"
  jwt-public-key: |
    -----BEGIN PUBLIC KEY-----
    ...
    -----END PUBLIC KEY-----
```

Deployments reference secrets via `valueFrom.secretKeyRef` (see
`deploy/k8s/api-deployment.yaml` for an example).

**When to use:** production Kubernetes without Vault.

## 3. HashiCorp Vault (production)

Set `VAULT_ADDR`, `VAULT_ROLE_ID`, and `VAULT_SECRET_ID` to enable
AppRole authentication. The application fetches secrets at startup.

Vault path convention (configurable in code):

| Path | Secret |
|------|--------|
| `secret/data/task-queue/api-key` | API authentication key |
| `secret/data/task-queue/jwt-public-key` | JWT public key |
| `secret/data/task-queue/postgres` | Postgres connection string |

**When to use:** production environments where secret rotation and audit
logging are required.

## Startup Order

1. Load config from env vars (`config.Load()`)
2. If `VAULT_ADDR` is set, initialize Vault provider
3. If `VAULT_ADDR` is empty, use `EnvSecretsProvider`
4. API server uses `SecretsProvider` to fetch `api-key` for auth middleware
5. JWT public key is fetched from the same provider

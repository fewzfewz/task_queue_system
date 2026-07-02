# Secrets Management

The task queue system reads secrets from environment variables. This is the only supported method.

## Environment Variables

All secrets are configured via environment variables:

```bash
export API_KEY=my-secret-key
export REDIS_PASSWORD=my-redis-password
export POSTGRES_CONN_STR="postgres://user:pass@host:5432/db"
```

**When to use:** local development, CI, Docker, Kubernetes.

## Kubernetes

In Kubernetes, inject secrets via `valueFrom.secretKeyRef`:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: task-queue-secrets
type: Opaque
stringData:
  api-key: "my-secret-key"
```

Then reference in the deployment:

```yaml
env:
  - name: API_KEY
    valueFrom:
      secretKeyRef:
        name: task-queue-secrets
        key: api-key
```

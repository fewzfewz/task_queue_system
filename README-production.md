# Production Deployment Runbook

This document is the end-to-end runbook for deploying the task queue system to a
Kubernetes cluster with Redis and PostgreSQL. Every step is a copy-pasteable
command. The system is the four Go binaries in this repository:

| Binary            | Role                                             | Port | Health endpoints                     |
| ----------------- | ------------------------------------------------ | ---- | ------------------------------------ |
| `cmd/api`         | HTTP API, browser UI, SSE stream, DLQ admin      | 8080 | `/healthz`, `/readyz`, `/metrics`    |
| `cmd/worker`      | Executes jobs (email/image plugins), webhooks    | 8081 | `/healthz`, `/readyz`, `/metrics`, `/healthz/shutdown` |
| `cmd/scheduler`   | Orphan/in-flight reclaim loop, health            | 8082 | `/healthz`, `/readyz`, `/metrics`    |
| `cmd/cli`         | Maintenance CLI: `migrate-schema`, `migrate-jobs`, `migrate-down-schema` | - | - |

There is no in-app terminal. Day-to-day operator work happens from your shell:
`kubectl` for cluster operations, `make`/`go run ./cmd/cli` for maintenance, the
browser UI (`/login`) for job and DLQ operations.

Artifacts used by this runbook:

```
deploy/k8s/                         # canonical manifests (apply these)
  namespace.yaml  secrets.yaml  rbac.yaml  network-policy.yaml
  redis-service.yaml  redis-statefulset.yaml
  postgres-service.yaml  postgres-statefulset.yaml
  api-deployment.yaml  api-service.yaml
  worker-deployment.yaml  worker-service.yaml
  scheduler-deployment.yaml  scheduler-service.yaml
  ingress.yaml  certificate.yaml
  hpa.yaml  api-hpa.yaml  prometheus-adapter-configmap.yaml
deploy/monitoring/                  # k8s monitoring
  service-monitor.yaml  prometheus-rules.yaml
deploy/grafana/                     # dashboard.json + provisioning
deploy/helm/task-queue/             # Helm chart (NOT canonical; see §13)
deploy/local/docker/                # per-service Dockerfiles
```

The Helm chart exists but is **not** canonical: it has two known defects (see
§13). Use the raw manifests.

---

## 1. Prerequisites

Cluster requirements:

- Kubernetes with a CNI that enforces NetworkPolicy (Calico, Cilium, Weave)
- `ingress-nginx` installed (`ingress-nginx` namespace)
- `cert-manager` installed with a `ClusterIssuer` named `letsencrypt-prod`
- `kube-prometheus-stack` installed in the `monitoring` namespace (brings
  Prometheus, Prometheus Operator, the prometheus-adapter, and Grafana)
- A default StorageClass that supports `ReadWriteOnce` PVCs (Redis + Postgres
  use volume claims)
- A public DNS record for your API host, e.g. `api.task-queue.example.com`,
  pointing at the ingress controller's LoadBalancer

Workstation tools: `kubectl`, `helm`, `docker`, `make`, `go` 1.25, `jq`,
`openssl`.

### 1.1 Install cluster addons

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm repo add jetstack https://charts.jetstack.io
helm repo update

helm upgrade --install ingress-nginx ingress-nginx/ingress-nginx \
  --namespace ingress-nginx --create-namespace

helm upgrade --install cert-manager jetstack/cert-manager \
  --namespace cert-manager --create-namespace --set installCRDs=true

helm upgrade --install kube-prometheus-stack prometheus-community/kube-prometheus-stack \
  --namespace monitoring --create-namespace
```

Create the ACME ClusterIssuer that the ingress and certificate reference:

```bash
kubectl apply -f - <<'EOF'
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: ops@example.com
    privateKeySecretRef:
      name: letsencrypt-prod
    solvers:
      - http01:
          ingress:
            class: nginx
EOF
```

### 1.2 Verify

```bash
kubectl -n ingress-nginx get svc ingress-nginx-controller \
  -o jsonpath='{.status.loadBalancer.ingress}'
kubectl -n cert-manager get pods -l app.kubernetes.io/name=cert-manager
kubectl -n monitoring get deploy | grep -E 'prometheus|adapter|grafana'
```

Wait until the ingress LoadBalancer has an address and all pods are `Running`.
Point your DNS record at that address.

---

## 2. Build and publish images

Build the three service images from the per-service Dockerfiles:

```bash
docker build -f deploy/local/docker/api.Dockerfile        -t task-queue-api:latest .
docker build -f deploy/local/docker/worker.Dockerfile     -t task-queue-worker:latest .
docker build -f deploy/local/docker/scheduler.Dockerfile  -t task-queue-scheduler:latest .
```

**Local cluster (kind/minikube)** — load the images instead of pushing:

```bash
kind load docker-image task-queue-api:latest task-queue-worker:latest task-queue-scheduler:latest
# or: minikube image load task-queue-api:latest task-queue-worker:latest task-queue-scheduler:latest
```

**Cloud cluster** — push to a registry and point the manifests at it (the
manifests reference `task-queue-api:latest` etc. with `imagePullPolicy:
IfNotPresent`; for a real cluster use a registry + immutable tag):

```bash
export REGISTRY=your-registry.example.com
docker tag task-queue-api:latest        ${REGISTRY}/task-queue-api:latest
docker tag task-queue-worker:latest     ${REGISTRY}/task-queue-worker:latest
docker tag task-queue-scheduler:latest  ${REGISTRY}/task-queue-scheduler:latest
docker push ${REGISTRY}/task-queue-api:latest
docker push ${REGISTRY}/task-queue-worker:latest
docker push ${REGISTRY}/task-queue-scheduler:latest
```

> Pin images to immutable tags (e.g. a commit SHA) for rollback (§11). If you
> pushed to a registry, point the deployments at the registry images **after**
> applying them in §6.

---

## 3. Namespace and secrets

### 3.1 Namespace

```bash
kubectl apply -f deploy/k8s/namespace.yaml
```

### 3.2 Secrets

`deploy/k8s/secrets.yaml` ships with **empty values** so it fails closed: if you
apply it, the pods start with empty credentials instead of known defaults. It
is a template only — create the real secrets imperatively instead. The
manifests read from `task-queue-secrets` (`api-key`, `admin-password`,
`postgres-conn-str`) and `task-queue-postgres` (`password`, `conn-str`); create
both.

Choose a Postgres password, then generate the API key and admin password:

```bash
export TQ_POSTGRES_PASSWORD="$(openssl rand -hex 24)"
export TQ_API_KEY="$(openssl rand -hex 32)"
export TQ_ADMIN_PASSWORD="$(openssl rand -hex 24)"
export TQ_POSTGRES_CONN_STR="postgres://taskqueue:${TQ_POSTGRES_PASSWORD}@postgres.task-queue.svc.cluster.local:5432/taskqueue?sslmode=disable"
```

> Keep the `TQ_*` exports — they are reused throughout this runbook. Prefer a
> secrets manager (SOPS, sealed-secrets, external-secrets) in front of these
> `kubectl create secret` calls.

Create the secrets:

```bash
kubectl -n task-queue create secret generic task-queue-secrets \
  --from-literal=api-key="${TQ_API_KEY}" \
  --from-literal=admin-password="${TQ_ADMIN_PASSWORD}" \
  --from-literal=postgres-conn-str="${TQ_POSTGRES_CONN_STR}"

kubectl -n task-queue create secret generic task-queue-postgres \
  --from-literal=password="${TQ_POSTGRES_PASSWORD}" \
  --from-literal=conn-str="${TQ_POSTGRES_CONN_STR}"
```

The Postgres StatefulSet reads `POSTGRES_PASSWORD` from the
`task-queue-postgres` secret (key `password`), so the password lives only in
the secret, never in a committed manifest. It takes effect the first time the
StatefulSet initializes `PGDATA`; rotating the password on a running cluster
requires `ALTER USER` (see §11.3).

> The generated values live only in your shell and in the cluster. Do not
> commit `deploy/k8s/secrets.yaml` after this step.

Verify:

```bash
kubectl -n task-queue get secret task-queue-secrets
kubectl -n task-queue get secret task-queue-postgres
```

### 3.3 Vault (optional, not yet wired)

`internal/secrets/vault.go` implements a `VaultSecretsProvider` (AppRole login,
KV v2 at `secret/data/taskqueue/<tenant>`, key `api_key`) with a 5-minute TTL
cache, a 30s pre-expiry refresh window that serves stale values while
re-fetching in the background, and single-flight coalescing of concurrent reads.
**It is currently only a library with integration tests — no binary consumes
it.** API keys therefore come from the Kubernetes Secrets above. If you later
wire the provider into `cmd/api`, secret rotation is picked up at TTL expiry
(stale-while-revalidate) instead of requiring a pod restart; its behavior is
verified by `make test-vault` (see Appendix).

---

## 4. RBAC and network policy

```bash
kubectl apply -f deploy/k8s/rbac.yaml
kubectl apply -f deploy/k8s/network-policy.yaml
```

`rbac.yaml` creates one ServiceAccount per workload (`automountServiceAccountToken:
false`) plus minimal Roles/RoleBindings. `network-policy.yaml` implements
default-deny (ingress+egress) for the namespace and then allows exactly:

- kube-dns egress (UDP/TCP 53) for all pods in the namespace
- API ingress only from `ingress-nginx` (user traffic) and `monitoring`
  (Prometheus scrape) on :8080
- worker ingress from `monitoring` on :8081; scheduler ingress from
  `monitoring` on :8082
- Redis ingress from api/worker/scheduler on :6379; Postgres ingress from
  api/worker/scheduler on :5432
- egress from api/worker/scheduler to Redis (:6379) and Postgres (:5432)

If you enable `OTEL_EXPORTER_OTLP_ENDPOINT` pointing outside the cluster, add an
egress rule for the collector address (see §13).

---

## 5. Storage: Redis and Postgres

```bash
kubectl apply -f deploy/k8s/redis-service.yaml
kubectl apply -f deploy/k8s/redis-statefulset.yaml
kubectl apply -f deploy/k8s/postgres-service.yaml
kubectl apply -f deploy/k8s/postgres-statefulset.yaml

kubectl -n task-queue rollout status statefulset/redis --timeout=180s
kubectl -n task-queue rollout status statefulset/postgres --timeout=180s
```

Both are single-replica StatefulSets with RWO PVCs (Redis 1Gi, Postgres 10Gi).
Verify connectivity:

```bash
kubectl -n task-queue exec statefulset/redis -- redis-cli ping
kubectl -n task-queue exec statefulset/postgres -- pg_isready -U taskqueue
```

### 5.1 Postgres schema

Apply the versioned migrations with the CLI (`cmd/cli migrate-schema`). This is
a one-time step; `cmd/cli` tracks applied versions in `schema_migrations`:

```bash
kubectl -n task-queue port-forward statefulset/postgres 5432:5432 &
PF_PID=$!

POSTGRES_CONN_STR="postgres://taskqueue:${TQ_POSTGRES_PASSWORD}@localhost:5432/taskqueue?sslmode=disable" \
  go run ./cmd/cli migrate-schema --dir db/migrations

kill ${PF_PID}
```

Or, equivalently, with the Makefile target (the same env is honored):

```bash
POSTGRES_CONN_STR="postgres://taskqueue:${TQ_POSTGRES_PASSWORD}@localhost:5432/taskqueue?sslmode=disable" \
  make migrate-schema
```

> `STORE_BACKEND=redis` (the default) does not need Postgres at all; the
> migration step is only required for `postgres` or `dual`.

---

## 6. Deploy the applications

```bash
kubectl apply -f deploy/k8s/api-deployment.yaml
kubectl apply -f deploy/k8s/worker-deployment.yaml
kubectl apply -f deploy/k8s/scheduler-deployment.yaml

kubectl -n task-queue rollout status deployment/task-queue-api --timeout=180s
kubectl -n task-queue rollout status deployment/task-queue-worker --timeout=180s
kubectl -n task-queue rollout status deployment/task-queue-scheduler --timeout=180s
```

If you pushed the images to a registry, point the deployments at them now:

```bash
kubectl -n task-queue set image deployment/task-queue-api       api=${REGISTRY}/task-queue-api:latest
kubectl -n task-queue set image deployment/task-queue-worker    worker=${REGISTRY}/task-queue-worker:latest
kubectl -n task-queue set image deployment/task-queue-scheduler scheduler=${REGISTRY}/task-queue-scheduler:latest
kubectl -n task-queue rollout status deployment/task-queue-api --timeout=180s
```

The deployments read `API_KEY`, `ADMIN_PASSWORD`, and `POSTGRES_CONN_STR` from
the `task-queue-secrets` secret, talk to Redis at
`redis.task-queue.svc.cluster.local:6379`, and ship with resource requests so
the HPA resource metrics work. The worker drains in-flight jobs on termination
(`terminationGracePeriodSeconds: 75` ≥ `DRAIN_TIMEOUT` 60 + preStop 5).

Expose the services:

```bash
kubectl apply -f deploy/k8s/api-service.yaml
kubectl apply -f deploy/k8s/worker-service.yaml
kubectl apply -f deploy/k8s/scheduler-service.yaml
```

### 6.1 Backend selection

`STORE_BACKEND` defaults to `redis`. For durable storage, switch to `postgres`
(or `dual` to write both) on **all three** workloads:

```bash
kubectl -n task-queue set env deployment/task-queue-api       STORE_BACKEND=postgres
kubectl -n task-queue set env deployment/task-queue-worker    STORE_BACKEND=postgres
kubectl -n task-queue set env deployment/task-queue-scheduler STORE_BACKEND=postgres
kubectl -n task-queue rollout status deployment/task-queue-api --timeout=180s
```

### 6.2 Configuration knobs (env)

Defaults are applied by the binaries when the env var is unset.

| Env                         | Default                          | Binary    | Notes                              |
| --------------------------- | -------------------------------- | --------- | ---------------------------------- |
| `PORT`                      | `8080`                           | api       |                                    |
| `WORKER_PORT`               | `8081`                           | worker    |                                    |
| `SCHEDULER_PORT`            | `8082`                           | scheduler |                                    |
| `REDIS_HOST`                | `localhost:6379`                 | all       | set to `redis.task-queue.svc.cluster.local:6379` in manifests |
| `REDIS_PASSWORD`            | ``                               | all       |                                    |
| `REDIS_DB`                  | `0`                              | all       |                                    |
| `API_KEY`                   | `secret-api-key`                 | api       | from secret in manifests           |
| `ADMIN_USERNAME`            | `admin`                          | api       | UI login                           |
| `ADMIN_PASSWORD`            | `admin123`                       | api       | from secret in manifests           |
| `POSTGRES_CONN_STR`         | ``                               | all       | from secret in manifests           |
| `STORE_BACKEND`             | `redis`                          | all       | `redis`, `postgres`, or `dual`     |
| `WORKER_POOL_SIZE`          | `50`                             | worker    | concurrent job goroutines          |
| `JOB_RATE_LIMIT`            | `0` (unlimited)                  | worker    | jobs/sec                           |
| `DRAIN_TIMEOUT`             | `60`                             | worker    | seconds; keep < terminationGracePeriodSeconds |
| `SLA_TARGET_SECONDS`        | `5`                              | worker    | SLA compliance target              |
| `TENANT_RATE_LIMIT`         | `0` (unlimited)                  | api       | requests/sec per tenant            |
| `MAX_QUEUE_SIZE`            | `10000`                          | api       | pending job cap (0 = unlimited)    |
| `LOG_LEVEL`                 | `info`                           | all       | `info`, `error`, `debug`           |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | ``                             | all       | empty = tracing disabled           |

Example tuning:

```bash
kubectl -n task-queue set env deployment/task-queue-worker WORKER_POOL_SIZE=100 DRAIN_TIMEOUT=90 SLA_TARGET_SECONDS=10
kubectl -n task-queue set env deployment/task-queue-api TENANT_RATE_LIMIT=100
```

---

## 7. Ingress and TLS

Prerequisites: ingress-nginx running, cert-manager running, the `letsencrypt-prod`
ClusterIssuer created (§1), and DNS pointing at the ingress LoadBalancer.

Set your real public host, patch both manifests, and apply:

```bash
export TQ_HOST=api.task-queue.example.com
sed -i "s/api.task-queue.example.com/${TQ_HOST}/g" deploy/k8s/ingress.yaml deploy/k8s/certificate.yaml

kubectl apply -f deploy/k8s/ingress.yaml
kubectl apply -f deploy/k8s/certificate.yaml
```

cert-manager issues a certificate into the `task-queue-tls` secret (referenced
by both the `Certificate` and the ingress `tls` block). Watch until `Ready`:

```bash
kubectl -n task-queue get certificate task-queue-tls -w
kubectl -n task-queue get secret task-queue-tls
```

Verify end to end:

```bash
curl -sf https://${TQ_HOST}/healthz && echo HEALTHY
curl -sf https://${TQ_HOST}/readyz && echo READY
```

---

## 8. Autoscaling (HPA)

The shipped HPAs:

- `task-queue-api-hpa` — CPU 70%, memory 80%, external `task_queue_api_request_rate` 100 req/s; min 2, max 10
- `task-queue-worker-hpa` — external `task_queue_length` (average 100) and `task_queue_worker_busy_ratio` (0.2); min 2, max 20

Both use the default Prometheus registry metrics via the prometheus-adapter
exposed by `kube-prometheus-stack`.

### 8.1 Configure the prometheus-adapter

Point the adapter at the shipped rules (a ConfigMap in `monitoring`), overwrite
the adapter's own config, and restart it:

```bash
kubectl apply -f deploy/k8s/prometheus-adapter-configmap.yaml

PAYLOAD="$(kubectl -n monitoring get configmap prometheus-adapter-config \
  -o jsonpath='{.data.config\.yaml}' | jq -Rs '{data:{"config.yaml":.}}')"
kubectl -n monitoring patch configmap prometheus-adapter --type merge -p "${PAYLOAD}"
kubectl -n monitoring rollout restart deployment prometheus-adapter
kubectl -n monitoring rollout status deployment/prometheus-adapter --timeout=120s
```

> If you installed `kube-prometheus-stack` under a different release name, or
> run a standalone `prometheus-adapter` chart, adjust the deployment name and
> ensure it scrapes `prometheus-operated.monitoring.svc.cluster.local:9090`.

### 8.2 Enable scraping and apply the HPAs

```bash
kubectl apply -f deploy/monitoring/service-monitor.yaml
kubectl apply -f deploy/k8s/api-hpa.yaml
kubectl apply -f deploy/k8s/hpa.yaml
```

`service-monitor.yaml` matches the three services (they carry the
`app.kubernetes.io/part-of: task-queue` label) and scrapes `/metrics` on each.

### 8.3 Verify

```bash
kubectl -n task-queue get hpa
kubectl get --raw /apis/custom.metrics.k8s.io/v1beta1 | jq '.resources[].name'
kubectl get --raw "/apis/external.metrics.k8s.io/v1beta1/namespaces/task-queue/task_queue_length" | jq '.items[0].value'
kubectl get --raw "/apis/external.metrics.k8s.io/v1beta1/namespaces/task-queue/task_queue_api_request_rate" | jq '.items[0].value'
```

`<unknown>` values mean the metric is not being scraped yet — see §9.3 before
investigating HPA. Note that `task_queue_api_request_rate` only appears once
the API has served traffic (it is `rate(task_queue_api_request_total[5m])`).

---

## 9. Monitoring and alerting

### 9.1 Alerts

```bash
kubectl apply -f deploy/monitoring/prometheus-rules.yaml
kubectl -n monitoring get prometheusrule task-queue-alerts
```

The `PrometheusRule` carries the default kube-prometheus-stack selector labels
(`app`/`release: kube-prometheus-stack`); adjust if you changed the release
name. Rules included: queue depth, idle/overloaded workers, DLQ growth, failure
rate, SLA breach, webhook failure spikes, open circuit breakers, and missing
worker heartbeats.

### 9.2 Grafana dashboard

Load the shipped dashboard into the stack's Grafana via the dashboard sidecar
(a ConfigMap labeled `grafana_dashboard: "1"`):

```bash
kubectl -n monitoring create configmap task-queue-dashboard \
  --from-file=task-queue.json=deploy/grafana/dashboard.json -l grafana_dashboard="1"
kubectl -n monitoring rollout restart deployment/kube-prometheus-stack-grafana
kubectl -n monitoring port-forward svc/kube-prometheus-stack-grafana 3000:80
```

Open http://localhost:3000, log in (default `admin` / `prom-operator`), and open
the **Task Queue System** dashboard. Its panels use the default datasource,
which the stack provisions to its own Prometheus. (`deploy/grafana/provisioning/`
is used by the compose monitoring stack; `deploy/monitoring/prometheus.yml` is
compose-only and does **not** apply to Kubernetes.)

### 9.3 Verify the pipeline

```bash
kubectl -n monitoring port-forward svc/prometheus-operated 9090:9090 &

curl -s http://localhost:9090/api/v1/targets \
  | jq '.data.activeTargets[] | select(.labels.job | startswith("task-queue")) | {job: .labels.job, health, scrapeUrl}'

curl -s 'http://localhost:9090/api/v1/query?query=task_queue_length' | jq '.data.result'

curl -s 'http://localhost:9090/api/v1/rules' \
  | jq '.data.groups[] | select(.name=="task-queue.rules") | {name, rules: [.rules[].name]}'

curl -s 'http://localhost:9090/api/v1/alerts' \
  | jq '.data.alerts[] | select(.labels.alertname | startswith("TaskQueue")) | {name: .labels.alertname, state}'
```

All three services must be `health=up`. Enqueue a few jobs (§10) so the
`task_queue_*` series exist before judging the alerts or the HPA.

---

## 10. Post-deploy smoke tests

```bash
kubectl -n task-queue port-forward svc/task-queue-api 8080:8080 &
PF_PID=$!
```

Health checks:

```bash
curl -sf http://localhost:8080/healthz && echo HEALTHY
curl -sf http://localhost:8080/readyz && echo READY
```

Job round-trip (enqueue → completed). The worker pool processes `email` jobs by
default:

```bash
JOB_ID="$(curl -sf -X POST http://localhost:8080/jobs \
  -H "X-API-Key: ${TQ_API_KEY}" -H "Content-Type: application/json" \
  -d '{"type":"email","payload":{"to":"ops@example.com","subject":"smoke","body":"hello"},"priority":"high"}' \
  | jq -r .id)"
echo "enqueued ${JOB_ID}"

for i in $(seq 1 20); do
  STATUS="$(curl -s http://localhost:8080/jobs/${JOB_ID} | jq -r .status)"
  [ "${STATUS}" = "completed" ] && break
  sleep 1
done
echo "final status: ${STATUS}"
```

SSE stream — start a listener, then enqueue another job and confirm events flow:

```bash
curl -N -m 5 -s http://localhost:8080/events > /tmp/tq-sse.out &
curl -sf -X POST http://localhost:8080/jobs \
  -H "X-API-Key: ${TQ_API_KEY}" -H "Content-Type: application/json" \
  -d '{"type":"email","payload":{"to":"a@b.c","subject":"sse","body":"x"}}' > /dev/null
sleep 6
head -20 /tmp/tq-sse.out
```

Metrics endpoints (Prometheus text format on all three services):

```bash
kubectl -n task-queue port-forward svc/task-queue-worker 8081:8081 &
PF_PID2=$!
kubectl -n task-queue port-forward svc/task-queue-scheduler 8082:8082 &
PF_PID3=$!
curl -sf http://localhost:8080/metrics | grep -m1 task_queue_api_request_total
curl -sf http://localhost:8081/metrics | grep -m1 task_queue_length
curl -sf http://localhost:8082/metrics | grep -m1 go_goroutines
```

DLQ (should be empty on a clean run):

```bash
curl -s -H "X-API-Key: ${TQ_API_KEY}" http://localhost:8080/api/v1/dlq | jq .
```

Browser UI:

```bash
curl -sf -X POST http://localhost:8080/api/v1/login \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"admin\",\"password\":\"${TQ_ADMIN_PASSWORD}\"}" | jq .
```

Open http://localhost:8080/login (or https://${TQ_HOST}/login) and sign in with
`admin` / the generated admin password.

```bash
kill ${PF_PID} ${PF_PID2} ${PF_PID3} 2>/dev/null
```

---

## 11. Rollback and rotation

### 11.1 Application rollback

`rollout undo` reverts to the previous ReplicaSet (previous image + env). It
works because `imagePullPolicy: IfNotPresent` and locally-loaded/pulled images
remain available:

```bash
kubectl -n task-queue rollout undo deployment/task-queue-api
kubectl -n task-queue rollout status deployment/task-queue-api --timeout=180s
```

With immutable image tags, pin a specific version explicitly:

```bash
kubectl -n task-queue set image deployment/task-queue-api api=${REGISTRY}/task-queue-api:good-commit
```

### 11.2 Backend rollback

- **Schema** — roll back the last migration:

  ```bash
  kubectl -n task-queue port-forward statefulset/postgres 5432:5432 &
  POSTGRES_CONN_STR="postgres://taskqueue:${TQ_POSTGRES_PASSWORD}@localhost:5432/taskqueue?sslmode=disable" \
    go run ./cmd/cli migrate-down-schema --dir db/migrations
  ```

- **Data** — the Redis→Postgres data migration (`migrate-jobs`) is **one-way**;
  `cmd/cli migrate-down` intentionally performs no action and prints a warning.
  To revert, switch `STORE_BACKEND` back to `redis`; Postgres remains as a
  standby:

  ```bash
  kubectl -n task-queue set env deployment/task-queue-api STORE_BACKEND=redis
  kubectl -n task-queue set env deployment/task-queue-worker STORE_BACKEND=redis
  kubectl -n task-queue set env deployment/task-queue-scheduler STORE_BACKEND=redis
  ```

### 11.3 Key rotation

Secrets are read at pod start, so rotate the secret and restart the workloads:

```bash
export TQ_API_KEY="$(openssl rand -hex 32)"
kubectl -n task-queue create secret generic task-queue-secrets \
  --from-literal=api-key="${TQ_API_KEY}" \
  --from-literal=admin-password="${TQ_ADMIN_PASSWORD}" \
  --from-literal=postgres-conn-str="${TQ_POSTGRES_CONN_STR}" \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl -n task-queue rollout restart deployment/task-queue-api task-queue-worker task-queue-scheduler
kubectl -n task-queue rollout status deployment/task-queue-api --timeout=180s
```

### 11.4 Backups

```bash
# Postgres: logical dump (repeat on a schedule, off-cluster)
kubectl -n task-queue exec statefulset/postgres -- pg_dump -U taskqueue -d taskqueue > backup.sql

# Redis: snapshot to disk, then copy the RDB out
kubectl -n task-queue exec statefulset/redis -- redis-cli SAVE
kubectl -n task-queue exec statefulset/redis -- sh -c 'cp /data/dump.rdb /tmp/dump.rdb'
```

---

## 12. Post-deploy checklist

- [ ] `kubectl -n task-queue get pods` — all `Running`/`Ready`
- [ ] `curl -sf https://${TQ_HOST}/readyz` returns `200 OK`
- [ ] One job round-trips to `completed` (§10)
- [ ] SSE `/events` stream emits job events (§10)
- [ ] `kubectl -n task-queue get hpa` shows no `<unknown>` metric values (§8.3)
- [ ] Prometheus shows all three targets `up` and the `task-queue.rules` group
      is loaded (§9.3)
- [ ] Grafana **Task Queue System** dashboard renders panels with data (§9.2)
- [ ] DLQ empty, no `TaskQueue*` alerts firing under normal load

---

## 13. Known limitations / out of scope

- **Helm chart is not canonical.** `deploy/helm/task-queue/` has two defects:
  `api-deployment.yaml` declares `API_KEY` twice (the plaintext `values.yaml`
  value overrides the secret reference), and the worker readiness probe points
  at `/healthz/shutdown` (the drain endpoint) instead of `/readyz`. Use the raw
  `deploy/k8s/` manifests in this runbook.
- **Compose monitoring files are not cluster manifests.**
  `deploy/monitoring/prometheus.yml` and `deploy/monitoring/docker-compose.yml`
  are for the local compose stack only; Kubernetes scraping uses
  `deploy/monitoring/service-monitor.yaml`.
- **No HA for the data stores.** Redis and Postgres are single-replica
  StatefulSets; there is no replica count, failover, or backup tooling shipped.
- **Egress is restricted.** Default-deny egress means outbound calls (e.g. an
  OTLP collector outside the cluster, or an email/SMTP provider) require an
  additional NetworkPolicy rule.
- **Vault provider is not wired** into any binary; secrets come from Kubernetes
  Secrets (§3.3).
- **Single region / single cluster.** No multi-cluster or DR story is included.
- **Worker `preStop` was simplified** to `sleep 5`; draining is driven by the
  process catching SIGTERM (grace period 75s ≥ `DRAIN_TIMEOUT` 60s + 5s).

---

## Appendix A. Postgres Integration Tests

The Postgres store path is covered by a dedicated integration suite that runs
against a real PostgreSQL instance — no mocking, no build tags. The tests are
opt-in and gated on the `POSTGRES_CONN_STR` environment variable, so the fast
unit suite (`go test ./...`) never needs a database.

### A.1 Locally

The easiest way is the one-command Makefile target, which starts a Postgres
container from `deploy/test/docker-compose.yml`, runs the suite, and tears the
container down:

```bash
make test-postgres
```

Under the hood this runs:

```bash
POSTGRES_CONN_STR='postgres://tq:tq@localhost:5433/tq?sslmode=disable' \
POSTGRES_MIGRATIONS_DIR='<repo>/db/migrations' \
go test -count=1 ./test/... ./internal/storage/...
```

If you want the database to stay up between runs:

```bash
make test-postgres-up     # start the test DB only
make test-postgres        # run against the running container
make test-postgres-down   # stop it
```

Or point the suite at any Postgres you already run:

```bash
POSTGRES_CONN_STR='postgres://user:pass@host:5432/db?sslmode=disable' \
POSTGRES_MIGRATIONS_DIR='$(pwd)/db/migrations' \
go test -count=1 ./test/... ./internal/storage/...
```

`POSTGRES_MIGRATIONS_DIR` is optional when the suite can find `db/migrations`
from the repository root; it is always required to be set from CI because Go
runs each test package from its own directory.

What the suite covers against the real Postgres backend:

- job save/get/enqueue round-trips (payload, priority, dedup key, shard key,
  webhook config, DAG dependencies, progress)
- update status/progress/result, error history accumulation
- dequeue with SKIP LOCKED: priority tiers (high > medium > low), scheduled
  gating, tenant and shard filtering, DAG dependency blocking/release
- heartbeats, orphan recovery, completion/failure/requeue
- list/search filters and pagination, `GetByIDs`, dedup idempotency markers,
  queue length stats, job deletion and TTL cleanup

The `jobs` table is truncated before and after every test so runs are isolated
and repeatable.

### A.2 In CI

`test.yml` runs a dedicated `test-postgres` job that spins up a `postgres:16`
service container and executes the same suite with `-race`. The Postgres tests
are intentionally excluded from the fast test job so the default CI run needs
no database beyond Redis.

## Appendix B. Webhook Integration Tests

The webhook delivery suite lives in `internal/webhooks/` and is split in two:

- **Default-run delivery tests** (`delivery_integration_test.go`) — fully
  self-contained using `httptest` receivers and in-memory Redis, so they run
  as part of `go test ./...` with no external services:
  - HMAC-SHA256 signature verification on the receiving end (valid secret
    accepted, wrong secret rejected as a 4xx without retry)
  - payload integrity across the event types the system emits (`completed`,
    `failed`) — exact body, method, content-type, and timestamp
  - exponential backoff schedule with jitter bounds, give-up after `MaxRetries`
  - retry on 5xx and on client timeout; no retry on 4xx
  - 50-way concurrent deliveries with a goroutine-leak check
- **Redis-pipeline test** (`dispatcher_integration_test.go`) — gated behind
  `RUN_QUEUE_INTEGRATION=1` and drives the real stream → consumer-group →
  dispatcher path, verifying the delivered webhook signature end-to-end.

### B.1 Locally

```bash
make test-webhooks
```

This starts a dedicated Redis container (`task_queue_redis_test`) on host port
`:6380`, runs `RUN_QUEUE_INTEGRATION=1 go test -race ./internal/webhooks/`, and
removes the container afterwards. It uses a separate port so it never collides
with a running `make dev`/compose stack on `:6379`, and the gated test reads
`REDIS_ADDR` (default `localhost:6379`) to find Redis. If Redis is
unreachable the gated tests skip.

### B.2 In CI

`test.yml` runs a dedicated `test-webhooks` job with a `redis:7` service
container and `RUN_QUEUE_INTEGRATION=1`. The default-run delivery tests are
already covered by the fast `test` job.

## Appendix C. Vault Integration Tests

The Vault suite lives in `internal/secrets/vault_integration_test.go`. It tests
the `VaultSecretsProvider`'s TTL caching contract against a real Vault dev
server (AppRole auth + KV v2), using a counting HTTP transport so assertions are
made on actual Vault calls rather than mocks:

- login via AppRole (`role-id` + `secret-id`) and KV v2 secret retrieval
- value served from cache until the TTL expires (confirmed by changing the
  underlying secret and observing the old value is still returned, with only 1
  Vault read)
- re-fetch after TTL expiry picks up the new value
- stale-while-revalidate: a failed background refresh inside the pre-expiry
  window still serves the stale cached value; after full expiry the provider
  returns an error instead of stale data
- single-flight coalescing: 20 concurrent cold-miss readers cause exactly 1 Vault
  read, and 20 concurrent in-window readers trigger a single coalesced refresh
  (no stampede)

If no Vault server is reachable the tests skip, so `go test ./...` needs no
extra services.

### C.1 Locally

```bash
make test-vault
```

This starts a Vault dev server (`hashicorp/vault:1.18` in dev mode, root token
`root`) from `deploy/test/docker-compose.vault.yml` on host port `:8200`, runs
`go test -race ./internal/secrets/`, and tears the container down. Override the
endpoint with `VAULT_ADDR` / `VAULT_DEV_ROOT_TOKEN_ID` to run against an
existing dev server.

### C.2 In CI

`test.yml` runs a dedicated `test-vault` job with a `hashicorp/vault:1.18`
service container (dev mode, unsealed) on `:8200`.

## Appendix D. Chaos CLI

`cmd/chaos` is a standalone binary that runs failure-injection scenarios
against a **live** deployment and writes a structured JSON report with an
SLO-based pass/fail verdict. It complements the build-tagged in-process suite
in `chaos/` (which boots its own ephemeral stack via `go test -tags chaos`).

### D.1 Build

```sh
go build -o bin/chaos ./cmd/chaos/
```

### D.2 Scenarios

| Scenario         | Fault injected                                                   |
| ---------------- | ---------------------------------------------------------------- |
| `redis-crash`    | Stop (SIGTERM) or kill (SIGKILL) the Redis container, restart it after a fault window |
| `worker-kill`    | Stop a worker container mid-processing; the rest of the pool and the scheduler's in-flight reclaim finish the jobs |
| `redis-partition`| Isolate Redis from the compose network (or drop packets via iptables on a root host), restore after a fault window |
| `orphan-reclaim` | Forge stale (`now - 2h`) and fresh (`now + 30s`) in-flight visibility scores; the scheduler must reclaim only the stale job |

### D.3 Usage

```sh
chaos list                                   # list scenarios
chaos run <scenario> [flags]                 # run one scenario
chaos run <scenario> --dry-run               # validate config + reachability, no fault
chaos run <scenario> --output report.json    # write report to a file (default: stdout)
```

Every option is a flag with a `CHAOS_*` environment fallback; run
`chaos run --help` for the full list. Common ones:

| Flag                | Env                    | Default                     |
| ------------------- | ---------------------- | --------------------------- |
| `--api-url`         | `CHAOS_API_URL`        | `http://localhost:8080`     |
| `--api-key`         | `CHAOS_API_KEY`        | `secret-api-key`            |
| `--redis-addr`      | `CHAOS_REDIS_ADDR`     | `localhost:6379`            |
| `--redis-container` | `CHAOS_REDIS_CONTAINER`| `task_queue_redis` (auto-detect workers via compose labels) |
| `--worker-container`| `CHAOS_WORKER_CONTAINER`| empty (first compose worker auto-detected) |
| `--jobs`            |                        | `20`                        |
| `--fault-duration`  |                        | `5s`                        |
| `--sla-timeout`     |                        | `120s`                      |
| `--slo-success`     |                        | `1.0`                       |
| `--slo-max-dlq`     |                        | `0`                         |

Examples:

```sh
# sanity-check reachability against the deployed stack, no fault injected
bin/chaos run redis-crash --dry-run

# crash Redis hard for 10s and require 95% of jobs to survive
bin/chaos run redis-crash --crash-kind kill --fault-duration 10s \
  --slo-success 0.95 --output /tmp/redis-crash.json

# stop one worker and confirm the pool + reclaim finish the batch
bin/chaos run worker-kill --output /tmp/worker-kill.json

# forge an orphan and confirm the scheduler reclaims it within the SLO
bin/chaos run orphan-reclaim --output /tmp/reclaim.json
```

`--dry-run` probes the API, Redis, and container targets without injecting any
fault; run it before every scenario against a new deployment.

### D.4 Report format

The report is a single JSON object (indented, written to `--output` or stdout):

```json
{
  "scenario": "orphan-reclaim",
  "started_at": "2026-07-31T14:56:43Z",
  "ended_at": "2026-07-31T14:56:47Z",
  "duration_ms": 4369,
  "config": { "api_url": "...", "redis_addr": "...", "jobs": 20, "...": "..." },
  "fault": { "type": "forge-in-flight-visibility", "target": "...", "injected_at": "...", "cleared_at": "..." },
  "observations": { "jobs_enqueued": 20, "jobs_completed": 20, "recovery_time_ms": 3025, "...": "..." },
  "slo": { "job_success_ratio": 1, "recovery_timeout_ms": 120000, "max_dlq_growth": 0 },
  "verdict": "pass",
  "failures": ["..."]
}
```

Key fields:

- `config` — the flags/env that produced the run (reproducibility).
- `fault` — what was injected, the target container, and inject/clear timestamps.
- `observations` — enqueued/completed/failed/dropped counts, DLQ growth,
  recovery time, API unavailability, queue lengths before/after, stale
  reclaimed, worker endpoint health.
- `slo` — the thresholds the run was measured against.
- `verdict` — `pass` when no SLO check failed, `fail` otherwise.
- `failures` — human-readable SLO violations (present only on `fail`).

Notes:

- `redis-partition --partition-method iptables` requires a root host with
  `iptables`; the `docker` method (default) uses the compose network instead.
- The CLI never changes job payloads or queue keys beyond the scenario's own
  enqueue and the forged in-flight scores; each scenario waits for jobs to
  reach a terminal state before finishing.
- Exit code is `0` for completed runs (pass or fail) and non-zero only for
  execution errors (config, unreachable targets, docker failures). Use the
  report `verdict` to drive CI gates.

## Appendix E. Benchmarks

Committed baselines for the load-testing tooling live in
[`docs/benchmarks/`](docs/benchmarks/) and are captured by
[`scripts/bench/capture.sh`](scripts/bench/capture.sh) against a pinned
reference environment (`deploy/test/docker-compose.bench.yml`). Drift between a
new capture and the committed baseline is checked by
[`scripts/bench/compare.sh`](scripts/bench/compare.sh), and can be run on demand
from GitHub Actions via the `Benchmarks` workflow (manual dispatch only).

### E.1 Reference environment

The benchmark pins CPU/memory limits on the API and scheduler (and inherits the
worker limits from the base compose file) so numbers are reproducible:

```
docker-compose -p tq-bench -f docker-compose.yml \
  -f deploy/test/docker-compose.bench.yml up -d --build --scale worker=3
```

The stack uses the canonical ports (8080/6379), so stop any other stack
(including the dev `local` project) first. Container names are fixed
(`task_queue_api`, `task_queue_redis`, `task_queue_scheduler`,
`tq-bench-worker-1..3`), so only one stack can run at a time.

### E.2 Capturing a baseline

```
./scripts/bench/capture.sh
```

This enqueues `JOBS_PER_TIER` (default 5000) email jobs per priority tier
(high/medium/low/mixed) at `CONCURRENCY` (default 50), records enqueue latency
p50/p95/p99, end-to-end completion latency, sustained jobs/sec, error rate, DLQ
growth, and per-container CPU/mem, then writes
`docs/benchmarks/baseline-<date>.json` and `.md`. It cross-checks the enqueue
path with `scripts/load_test.sh` (1000 jobs @50).

Completion latency is measured as time-in-system: reconstructed from the Redis
`metrics:total` (arrival) and `metrics:completed` (completion) counter curves
sampled at ~50ms, assuming FIFO completion per priority queue. For jobs faster
than ~100ms the sub-sample values are approximate; the systematic polling skew
of black-box per-job polling is avoided entirely. See the "Methodology" section
of each generated report.

### E.3 Drift detection

```
./scripts/bench/compare.sh docs/benchmarks/baseline-2026-07-31.json   # vs newest committed baseline
./scripts/bench/compare.sh new.json docs/benchmarks/baseline-OLD.json # explicit baseline
./scripts/bench/compare.sh -t 10 --warn -v new.json baseline.json     # 10%, non-blocking, verbose
```

Because `capture.sh` reuses the same-date filename, compare the just-captured
report against the *committed* baseline explicitly (e.g.
`git show HEAD:docs/benchmarks/<baseline>.json > /tmp/base.json`) rather than
relying on the "newest file" default on the same day. Drift beyond `±20%` on
any latency percentile, sustained throughput, error rate, DLQ growth, or
unfinished count exits `1`; `--warn` reports without failing. The CI workflow
does this automatically against `HEAD`.

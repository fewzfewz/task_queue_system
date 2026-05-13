# Production Deployment Guide

This document describes the current production-oriented deployment path for the task queue system.

## What Gets Deployed

The system runs as four processes:

- API server
- worker pool
- scheduler
- Redis queue/backing store

PostgreSQL is optional, but required if you want durable job storage with `STORE_BACKEND=postgres` or `STORE_BACKEND=dual`.

## Operator Terminal

There is no in-app terminal. Day-to-day operator work happens from your shell:

- `kubectl` for cluster operations
- `make` for local tasks
- `go run ./cmd/cli ...` for migration and maintenance jobs
- `./scripts/chaos.sh ...` for chaos reporting

The browser UI is for job and DLQ operations, but it is not a shell replacement.

## Namespace

Create a dedicated namespace:

```bash
kubectl create namespace task-queue
```

Then apply manifests into that namespace or patch the YAML `metadata.namespace` fields to `task-queue`.

## Secrets

### API key

Store the API key in a Kubernetes Secret:

```bash
kubectl -n task-queue create secret generic task-queue-secrets \
  --from-literal=api-key='replace-me'
```

The API deployment reads it as `API_KEY`.

### Postgres connection string

Store the DSN in a separate secret:

```bash
kubectl -n task-queue create secret generic task-queue-postgres \
  --from-literal=conn-str='postgres://user:pass@host:5432/dbname?sslmode=require'
```

Then inject it as `POSTGRES_CONN_STR` for the API, worker, or any pod using Postgres.

### Vault

If you use Vault, store the AppRole values in Kubernetes Secrets and inject:

- `VAULT_ADDR`
- `VAULT_ROLE_ID`
- `VAULT_SECRET_ID`

The code reads tenant secrets from `secret/data/taskqueue/<tenant_id>`.

## Storage Initialization

### Redis

Apply:

- `deploy/k8s/redis-service.yaml`
- `deploy/k8s/redis-statefulset.yaml`

### Postgres schema

Before using `STORE_BACKEND=postgres` or `STORE_BACKEND=dual`, apply the schema:

```bash
make migrate-schema
```

This uses the built-in versioned SQL migration runner and tracks applied versions in `schema_migrations`.

## Application Deployments

Apply:

- `deploy/k8s/rbac.yaml`
- `deploy/k8s/api-deployment.yaml`
- `deploy/k8s/api-service.yaml`
- `deploy/k8s/worker-deployment.yaml`
- `deploy/k8s/scheduler-deployment.yaml`
- `deploy/k8s/hpa.yaml`
- `deploy/k8s/prometheus-adapter-configmap.yaml`
- `deploy/monitoring/prometheus-rules.yaml`
- `deploy/grafana/dashboard.json`
- `deploy/grafana/provisioning/dashboards/dashboards.yaml`
- `deploy/grafana/provisioning/datasources/datasource.yaml`

## Ingress and TLS

The repository does not ship a full ingress manifest yet. In production:

- terminate TLS at your ingress controller or load balancer
- route the public host to `task-queue-api`
- keep Redis and Postgres private inside the cluster

Typical setup:

- NGINX Ingress or Traefik
- cert-manager for TLS certificates
- a `ClusterIP` service for the API

## RBAC and Service Accounts

The current manifests do not include dedicated service accounts or RBAC rules.

Recommended production hardening:

- create a service account per workload
- give the API only the permissions it needs
- keep worker and scheduler permissions minimal
- mount secrets through Kubernetes Secrets or a CSI driver

## Readiness and Liveness

The services expose:

- `GET /healthz`
- `GET /readyz`

The worker also exposes:

- `POST /healthz/shutdown`
- `GET /metrics`

Use those endpoints for probes and graceful shutdown hooks.

## Deploy Order

1. Create namespace.
2. Create secrets.
3. Apply RBAC.
4. Apply Redis.
5. Apply Postgres schema if needed.
6. Apply API, worker, and scheduler deployments.
7. Apply service and ingress resources.
8. Confirm `/readyz` returns `200 OK`.

## What Is Still Missing

- a complete ingress manifest
- a production `values.yaml` if you want Helm
- full RBAC/service account definitions
- distributed tracing export

Tracing note:

- the repo now carries a lightweight trace ID hook in request logging
- OTEL/Jaeger can be added later without changing the handler call sites

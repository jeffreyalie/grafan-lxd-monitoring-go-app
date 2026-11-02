# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`grafan-lxd-monitoring` is a lightweight Go exporter that exposes the live state of LXD instances (VMs and containers) as Prometheus metrics. The Helm chart also ships a ServiceMonitor and a Grafana dashboard ConfigMap. There is **no web UI** (no `index.html`, no ingress).

Deployed on MicroK8s via Helm across three isolated environments, dev, staging and live, using a Gitea Actions CI/CD pipeline with sequential environment promotion. The Helm release name is always `grafan-lxd-monitoring`.

## Architecture Summary

```
Go app (main.go + metrics.go)
  - background poller: OpenBao AppRole login -> read LXD TLS cert/key -> LXD GetInstancesFull
  - caches the result in memory
  - serves /metrics (cached) and /healthz
        │
        ▼
Helm chart (helm-chart/)
  - deployment.yaml, service.yaml
  - servicemonitor.yaml        -> Prometheus Operator discovers the app
  - grafana-dashboard.yaml     -> ConfigMap (label grafana_dashboard=1), imported by Grafana sidecar
        │
        ▼
 dev  ->  staging  ->  live     (namespaces)
```

Prometheus and Grafana come from kube-prometheus-stack, which lives in a **separate** `monitoring` repo/namespace. This repo does not deploy them.

## Key Commands

### Build & Deploy

**Build and push the image:**
```bash
docker build -t 10.206.213.243:32000/grafan-lxd-monitoring:$TAG ./app
docker push 10.206.213.243:32000/grafan-lxd-monitoring:$TAG
```

**Deploy to an environment** (run from repo root, chart is local, release name fixed):
```bash
export KUBECONFIG=/path/to/kubeconfig
NS=dev   # dev | staging | live

helm upgrade --install grafan-lxd-monitoring ./helm-chart \
  --namespace $NS \
  --create-namespace \
  --values helm-chart/values-$NS.yaml \
  --set image.tag=$TAG \
  --wait --timeout 2m
```

In CI, the image tag is `pr-<run_number>` for both pipelines (see `IMAGE_TAG` in the workflow `env`).

### Development Workflow

**Local run** (needs real `VAULT_*` and `LXD_ADDR` env vars):
```bash
cd app
go run .
curl localhost:8080/metrics | grep ^lxd_
curl localhost:8080/healthz
```

**Lint / build via Docker** (no local Go toolchain needed):
```bash
make init    # first time only (go mod init grafan-lxd-monitoring)
make tidy    # after changing dependencies
make fmt     # gofmt -w
make check   # gofmt -l, go vet, go build
make all     # tidy + fmt + vet + build
```

**Tests** (none exist yet):
```bash
cd app && go test -v ./...
```

### Environment Setup

**MicroK8s namespaces and secrets:**
```bash
microk8s kubectl create namespace dev
microk8s kubectl create namespace staging
microk8s kubectl create namespace live

for NS in dev staging live; do
  microk8s kubectl create secret generic openbao-creds \
    --from-literal=VAULT_ADDR=https://<openbao-ip>:8200 \
    --from-literal=VAULT_ROLE_ID=<role-id> \
    --from-literal=VAULT_SECRET_ID=<secret-id> \
    --namespace=$NS \
    --dry-run=client -o yaml | microk8s kubectl apply -f -
done
```

Never commit real values for these. Use placeholders in docs.

### Gitea Workflow Secrets

Set at the **Gitea org level** (Infra org):

- `VAULT_ADDR`: OpenBao server address
- `VAULT_ROLE_ID`: AppRole Role ID (same for all workflows)
- `VAULT_SECRET_ID`: AppRole Secret ID

Workflows live in `.gitea/workflows/` at the repo root.

## Code Architecture Details

### Go Application (`app/`)

**Files:**

- `main.go`: HTTP server (`/metrics`, `/healthz`), `fetchInstances()`, `getSecrets()`
- `metrics.go`: `lxdCollector` (Prometheus collector) and the background poller

**Flow:**

1. `main()` registers `newLXDCollector(interval)`. `interval` comes from `POLL_INTERVAL` (default `30s`).
2. The collector runs `poll()` immediately and then on a ticker.
3. `poll()` calls `fetchInstances()`:
   - `getSecrets()`: AppRole login at `auth/approle/login`, then reads `homelab/data/lxd` (keys `client_cert`, `client_key`)
   - `lxd.ConnectLXD()` with `InsecureSkipVerify: true`
   - `GetInstancesFull()` on project `default`
4. On success the instances and `lastOK` are cached under a mutex. On failure the old cache is kept, `errCount` increments, and the error is logged.
5. `Collect()` reads the cache only. A Prometheus scrape never triggers an OpenBao or LXD call.

**Important rules when editing:**

- Do not call `fetchInstances()` or `getSecrets()` from the `/metrics` path. Scrapes must stay cheap.
- Only emit `lxd_last_successful_poll_timestamp_seconds` when `lastOK` is non-zero.
- Keep metric names stable. The dashboard JSON depends on them.
- Keep `/healthz` independent of OpenBao and LXD, otherwise an LXD outage restarts the pod.

**Environment variables:**

| Variable | Source | Default | Purpose |
|----------|--------|---------|---------|
| `VAULT_ADDR` | K8s Secret | required | OpenBao server URL |
| `VAULT_ROLE_ID` | K8s Secret | required | AppRole Role ID |
| `VAULT_SECRET_ID` | K8s Secret | required | AppRole Secret ID |
| `VAULT_SKIP_VERIFY` | deployment.yaml | `true` | Skip TLS verification for OpenBao (self-signed) |
| `LXD_ADDR` | values.yaml | required | LXD API endpoint |
| `POLL_INTERVAL` | optional | `30s` | LXD poll interval (Go duration) |
| `PORT` | optional | `8080` | HTTP listen port |

**Metrics exposed** (labels `name`, `type`, `project`, `status` on instance metrics):

| Metric | Type |
|--------|------|
| `lxd_instance_up` | gauge |
| `lxd_instance_cpu_usage_seconds_total` | counter |
| `lxd_instance_memory_usage_bytes` | gauge |
| `lxd_instance_processes` | gauge |
| `lxd_last_successful_poll_timestamp_seconds` | gauge |
| `lxd_poll_errors_total` | counter |

**Security considerations:**

- Credentials are never baked into the image. They come from K8s secrets at runtime.
- The container runs as a non-root user.
- LXD TLS credentials are fetched on each poll and not written to disk.
- `VAULT_SKIP_VERIFY=true` and `InsecureSkipVerify: true` for LXD exist because of self-signed certs in the homelab.

### Helm Chart (`helm-chart/`)

| File | Purpose |
|------|---------|
| `Chart.yaml` | Chart name `grafan-lxd-monitoring` |
| `values.yaml` | Base defaults: image, service (80 -> 8080), LXD address, `metrics.serviceMonitor.enabled`, `grafana.dashboard.enabled` |
| `values-dev.yaml` / `values-staging.yaml` / `values-live.yaml` | Per-environment overrides (`existingSecretName`, dashboard toggle) |
| `dashboards/lxd-dashboard.json` | Grafana dashboard source |
| `templates/deployment.yaml` | Pod spec, env from values and secret, probes on `/healthz` |
| `templates/service.yaml` | ClusterIP, port name `http` |
| `templates/servicemonitor.yaml` | Scrapes `/metrics` on port `http` every 30s |
| `templates/grafana-dashboard.yaml` | ConfigMap labelled `grafana_dashboard: "1"` |

**Naming contract:** everything is named from `{{ .Release.Name }}`. The Service selector, ServiceMonitor selector (`app: <release>`), Deployment labels and rollout checks in the workflows must all agree. Do not change the release name in one place only.

**The ServiceMonitor `port: http` must match the Service port name `http`.** If you rename the port in `service.yaml`, update `servicemonitor.yaml`.

### CI/CD Pipelines (`.gitea/workflows/`)

**PR Pipeline (`pr-pipeline.yml`, currently `workflow_dispatch`):**
1. `build-pr`: build and push image tagged `pr-<run_number>`
2. `deploy-dev-pr`: fetch kubeconfig from OpenBao, deploy to `dev`
3. `deploy-staging-pr`: deploy to `staging` after `dev`

**Push Pipeline (`push-pipeline.yml`, currently `workflow_dispatch`):**
1. `build`: build and push the image
2. `deploy-live`: deploy to `live`

**Destroy (`destroy.yaml`):** manual; uninstalls the release in the chosen namespace and deletes the `grafan-lxd-monitoring-grafana-dashboard` ConfigMap. Optional namespace deletion also removes `openbao-creds`.

All pipelines authenticate to OpenBao with AppRole and read the kubeconfig from `homelab/data/microk8s`. Each deploy job uses a Gitea `environment` (`dev`, `staging`, `live`) for approval gates. Keep the three-environment structure and promotion order unchanged.

## Common Development Tasks

### Verifying a deployment

```bash
microk8s kubectl get pods -n dev -l app=grafan-lxd-monitoring
microk8s kubectl logs deploy/grafan-lxd-monitoring -n dev
microk8s kubectl port-forward -n dev svc/grafan-lxd-monitoring 8080:80
curl -s localhost:8080/metrics | grep ^lxd_
microk8s kubectl get servicemonitor -n dev
microk8s kubectl get configmap -n dev -l grafana_dashboard=1
```

Prometheus target should be **UP**. If it is missing, check that the Prometheus `serviceMonitorSelector` and `serviceMonitorNamespaceSelector` are `{}` in the kube-prometheus-stack values.

### Debugging LXD connection issues

1. OpenBao login:
```bash
curl -sk -X POST https://<vault-addr>/v1/auth/approle/login \
  -H "Content-Type: application/json" \
  -d '{"role_id":"<role_id>","secret_id":"<secret_id>"}'
```

2. Read the LXD secret:
```bash
curl -sk -H "X-Vault-Token: <token>" https://<vault-addr>/v1/homelab/data/lxd
```

3. Check LXD directly:
```bash
curl -k https://<lxd-address>/1.0/instances?recursive=1 \
  --cert <client-cert> --key <client-key>
```

Symptoms in metrics: `lxd_poll_errors_total` rising and a growing `time() - lxd_last_successful_poll_timestamp_seconds` mean polling is failing. Check pod logs for `metrics poll failed:`.

### Dashboard doesn't appear in Grafana

- The ConfigMap needs the label `grafana_dashboard: "1"`.
- The Grafana sidecar needs `searchNamespace: ALL` to see ConfigMaps in `dev`, `staging` and `live`.
- Dashboard datasource uid is `prometheus` (the kube-prometheus-stack default). Change it in the JSON if your datasource uid differs.

### Changing the OpenBao secret path

Update `getSecrets()` in `app/main.go` (the `client.Logical().Read("homelab/data/lxd")` call), update the `gitea-runner` policy if needed, then rebuild and redeploy.

### Adding a metric

1. Add a `*prometheus.Desc` in `newLXDCollector` (`app/metrics.go`).
2. Emit it in `Collect()` from the cached data.
3. Add a panel in `helm-chart/dashboards/lxd-dashboard.json`.

## Important Notes

1. **Version compatibility:** LXD installed via snap on Ubuntu 24.04 host; Go version is pinned in `app/dockerfile` and `app/go.mod`.
2. **Secrets management:** all pipeline secrets are at Gitea org level (Infra org). K8s `openbao-creds` secrets are created manually per namespace.
3. **Deployment flow:** PR to main triggers build -> dev -> staging. Merge to main triggers build -> live.
4. **Workflow requirements:** workflows run from the repo root and use Gitea Actions with secrets from the org.
5. **Scope:** this repo is metrics-only. Do not reintroduce `index.html`, HTML templates or an Ingress here.
6. **Related repos:** `monitoring` (kube-prometheus-stack), `go-lxd-vm-dashboard` / `go-lxd-vm-dashboard-gha` (the original dashboard UI app).

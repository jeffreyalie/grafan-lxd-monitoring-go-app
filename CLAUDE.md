# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a lightweight Go exporter that exposes the live state of LXD instances (VMs and containers) as Prometheus metrics. The application serves `/metrics` and `/healthz` endpoints only, and is deployed on MicroK8s via Helm across three isolated environments: dev, staging, and live.

The application:
- Authenticates to OpenBao via AppRole
- Reads LXD TLS client certificate and key from OpenBao
- Polls the LXD API in the background (default every 30 seconds)
- Caches results in memory and serves them on `/metrics`
- Provides a Prometheus ServiceMonitor and Grafana dashboard via Helm chart

## Architecture

```
                 ┌──────────────────────────────────────────────┐
                 │        grafan-lxd-monitoring pod             │
                 │                                              │
                 │  background poller (every POLL_INTERVAL)     │
                 │    1. AppRole login to OpenBao               │
                 │    2. read homelab/data/lxd (cert + key)     │
                 │    3. GetInstancesFull() from LXD API        │
                 │    4. cache result in memory                 │
                 │                                              │
                 │  :8080/metrics  <- serves cached values      │
                 │  :8080/healthz  <- readiness / liveness      │
                 └──────────────┬───────────────────────────────┘
                                │ scrape every 30s
                                ▼
                 ┌──────────────────────────────┐
                 │ Prometheus (kube-prometheus- │
                 │ stack, monitoring namespace) │
                 │  picks up the ServiceMonitor │
                 └──────────────┬───────────────┘
                                │ query
                                ▼
                 ┌──────────────────────────────┐
                 │ Grafana                      │
                 │  sidecar imports ConfigMap   │
                 │  "LXD Instances" dashboard   │
                 └──────────────────────────────┘
```

## Code Structure

```
.
├── app/                    # Go application source code
│   ├── main.go             # HTTP server (/metrics, /healthz), LXD + OpenBao client logic
│   ├── metrics.go          # Prometheus collector, background LXD poller
│   ├── dockerfile          # Multi-stage Docker build
│   ├── go.mod
│   └── go.sum
├── helm-chart/             # Helm chart for deployment
│   ├── Chart.yaml
│   ├── values.yaml         # Base defaults
│   ├── values-dev.yaml     # dev overrides
│   ├── values-staging.yaml # staging overrides
│   ├── values-live.yaml    # live overrides
│   ├── dashboards/
│   │   └── lxd-dashboard.json
│   └── templates/          # Helm templates for deployment
│       ├── deployment.yaml
│       ├── service.yaml
│       ├── servicemonitor.yaml
│       └── grafana-dashboard.yaml
├── .gitea/                 # Gitea CI/CD workflows
│   └── workflows/
│       ├── pr-pipeline.yml   # PR flow: build -> dev -> staging
│       ├── push-pipeline.yml # Merge flow: build -> live
│       └── destroy.yaml      # Manual uninstall per environment
├── Makefile                # Docker-based go init/tidy/fmt/check
└── README.md               # Project documentation
```

## Development Commands

### Go Application
- `make init` - Initialize Go modules (first time only)
- `make tidy` - Update Go modules 
- `make fmt` - Format Go code
- `make check` - Run gofmt, go vet, and build checks
- `make all` - Run all checks and builds

### Testing and Verification
The application is a Prometheus metrics exporter that:
1. Exposes `/metrics` endpoint with LXD instance data
2. Has `/healthz` endpoint for readiness/liveness probes
3. Uses Docker containerized builds via Makefile commands

### Deployment
The application deploys to MicroK8s using Helm charts across dev, staging, and live environments. The CI/CD pipeline uses Gitea Actions with sequential environment promotion:
- PR Pipeline: build-pr -> deploy-dev-pr -> deploy-staging-pr  
- Push Pipeline: build -> deploy-live

## Environment Variables

| Variable | Source | Default | Purpose |
|---|---|---|---|
| `VAULT_ADDR` | K8s Secret `openbao-creds` | required | OpenBao server address |
| `VAULT_ROLE_ID` | K8s Secret `openbao-creds` | required | AppRole Role ID |
| `VAULT_SECRET_ID` | K8s Secret `openbao-creds` | required | AppRole Secret ID |
| `VAULT_SKIP_VERIFY` | `deployment.yaml` | `true` | Skip OpenBao TLS verification (self-signed cert) |
| `LXD_ADDR` | `values.yaml` via `deployment.yaml` | required | LXD API endpoint |
| `POLL_INTERVAL` | optional | `30s` | How often to poll LXD (Go duration) |
| `PORT` | optional | `8080` | HTTP listen port |

## Key Implementation Details

1. The Go application runs a background poller that fetches LXD instance data every POLL_INTERVAL
2. Secrets are fetched from OpenBao using AppRole authentication  
3. LXD TLS certificates and keys are read from OpenBao at `homelab/data/lxd`
4. Poll results are cached in memory for serving metrics
5. Prometheus metrics are collected and served without hitting LXD on each scrape
6. The Helm chart provides ServiceMonitor and Grafana dashboard ConfigMap
7. All deployment is orchestrated through Gitea Actions workflows

## CI/CD Pipeline

- PR Pipeline: build-pr -> deploy-dev-pr -> deploy-staging-pr  
- Push Pipeline: build -> deploy-live
- Destroy workflow: manual cleanup of releases and dashboards
- All workflows run from the root directory of the repository
- Workflows use Gitea Actions with predefined environment variables from secrets
- Environment approval gates can be added in Gitea repo settings
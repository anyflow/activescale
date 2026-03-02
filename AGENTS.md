# AGENTS Guide: activescale

This file is for coding agents working in this repository.
Follow repository behavior and conventions over generic preferences.

## Command Fidelity (Non-Negotiable)
- Do not reinterpret, downgrade, or replace an explicit user command with a different action.
- If a requested command is risky, destructive, or policy-blocked, stop and ask one clear question before taking an alternative.
- Never silently switch strategies for git history operations (`push -f`, rebase, reset, amend, etc.).
- If you cannot execute exactly what was requested, state why and ask for explicit confirmation of the fallback.

## Project Snapshot
- Module: `activescale` (Go 1.25)
- Purpose: Envoy `StreamMetrics` ingestion + Redis TTL storage + Custom Metrics API for HPA
- Entrypoint: `cmd/activescale/main.go`
- Key packages:
  - `internal/envoy` - gRPC metrics sink server
  - `internal/provider` - custom metrics provider for pods
  - `internal/redis` - Redis gauge store with TTL
- Deployment manifests: `manifest/base` + env overlays under `manifest/*`

## Quickstart Commands
- Build binary: `make build`
- Build all packages: `make build-pkgs`
- Run tests: `make test`
- Run locally: `make run`
- Docker local run: `make docker-run`

## Build, Test, Lint Reference

### Build
- `make build`
- `make build-pkgs`
- `go build ./...`

### Test (full)
- `make test`
- `go test ./...`

### Test (single package)
- `go test ./internal/redis -v`
- `go test ./internal/envoy -v`
- `go test ./internal/provider -v`

### Test (single test by regex)
- `go test ./internal/redis -run TestName -count=1 -v`
- `go test ./internal/envoy -run TestStreamMetrics -count=1 -v`
- `go test ./internal/provider -run TestGetMetricBySelector -count=1 -v`

### Optional deeper checks
- Race: `go test -race ./...`
- Coverage: `go test -cover ./...`

### Formatting and static analysis
- `gofmt -w .`
- `go vet ./...`

Notes:
- There is no dedicated lint target in `Makefile`.
- Current repo may show `[no test files]` in some packages; keep commands stable and add tests as you change behavior.

## Docker Targets
- `make docker-build` - local single-arch image build
- `make docker-push` - multi-arch image push
- `make docker-run` - run container with default env wiring

## Runtime Ports and Endpoints
- Envoy metrics gRPC sink: `GRPC_PORT` (default `9000`)
- Custom metrics secure port: `6443` (manifest uses `--secure-port=6443`)
- Health port: `18080`
  - Liveness: `/healthz`
  - Readiness: `/readyz`

## Configuration Rules
Configuration is env-driven with `pflag` support in `main.go`.
Use existing env names and semantics.

Key environment variables:
- `REDIS_ADDR`
- `REDIS_CONTEXT`
- `REDIS_TLS`
- `REDIS_TLS_INSECURE`
- `REDIS_CA_FILE`
- `REDIS_CLUSTER` (must be explicitly set true/false)
- `GRPC_PORT`
- `METRIC_TTL`
- `LOG_VERBOSITY`
- `LOG_METRICS_SUMMARY_INTERVAL`
- `METRIC_NAME`

Agent guidance:
- Do not rename env vars without updating manifests and docs together.
- Preserve startup behavior in `main.go` unless requested.
- Keep defaults safe and backwards compatible.

## Code Style

### Formatting and imports
- Always run `gofmt`.
- Keep import groups as: standard library, third-party, local (`activescale/...`).
- Remove unused imports and dead code.

### Naming and types
- Follow idiomatic Go naming.
- Package names: short lowercase.
- Exported names: clear PascalCase.
- Keep files/package boundaries coherent; avoid leaking `internal/*` details.
- Prefer explicit, clear types over clever shorthand in critical paths.

### Error handling
- In `internal/*`, return errors upward; do not call `os.Exit`/fatal.
- In `cmd/activescale/main.go`, `klog.Fatal/Fatalf` is acceptable for unrecoverable startup/config failures (existing pattern).
- Include useful context in error paths (namespace, pod, metric where relevant).

### Logging
- Use `k8s.io/klog/v2` consistently.
- Keep high-frequency logs behind verbosity (`klog.V(4)`).
- Use `klog.Infof` for lifecycle events (startup, listeners, mode selection).
- `LOG_VERBOSITY` controls runtime log level.

### Context, timeouts, concurrency
- Pass `context.Context` through Redis/Kubernetes boundaries.
- Add timeout boundaries for external checks/calls (`context.WithTimeout`).
- Use `sync/atomic` or `sync` primitives for shared mutable state.

## Domain-Specific Behavior
- Envoy stream handler extracts pod identity from Istio node ID format:
  `sidecar~<ip>~<pod>.<ns>~<ns>.svc.cluster.local`
- Matching metrics are written as `active_requests` into Redis with TTL.
- Provider serves pod metrics from Redis to custom metrics API.
- TTL expiration means metrics can be missing; treat missing vs zero carefully when changing scaling logic.

### Metric naming caveat
- `METRIC_NAME` can be empty. In that case, the server auto-detects inbound-scoped
  `downstream_rq_active` metrics instead of requiring one exact family name.
- When `METRIC_NAME` is set, matching is exact; non-matching families are dropped.
- Prometheus-style names (for example `envoy_http_downstream_rq_active`) may map to
  scoped Envoy stat names. Validate actual incoming family names before tuning HPA.

## Debugging and Verification
- Check API registration:
  - `kubectl get --raw '/apis/custom.metrics.k8s.io/v1beta2'`
- Query metric by selector:
  - `kubectl get --raw '/apis/custom.metrics.k8s.io/v1beta2/namespaces/<ns>/pods/*/active_requests?labelSelector=app=<app>'`
- Inspect logs:
  - `kubectl logs -n ns-observability deploy/activescale | rg -n "metrics batches received|stored active_requests|skipping metric name|missing pod identity"`
- Raise debug verbosity:
  - `kubectl -n ns-observability set env deploy/activescale LOG_VERBOSITY=4`
  - `kubectl -n ns-observability rollout restart deploy/activescale`

## Deployment and GitOps
- Base resources are in `manifest/base`.
- Overlays patch base config from `manifest/aic-*`, `manifest/eic-*`, `manifest/kic-*`.
- ArgoCD ApplicationSet: `manifest/argocd/applicationset.yaml`.

## Security and Ops Cautions
- `manifest/base/apiservice.yaml` currently contains `insecureSkipTLSVerify: true`.
  Treat as a known risk; do not change security posture unless explicitly requested.
- Some overlays set `REDIS_TLS: "false"`; verify transport security expectations per environment.

## TODO (Production OP)
- For OP environments (`manifest/*-op`), add/maintain a patch that removes
  `insecureSkipTLSVerify: true` from APIService and uses proper TLS trust
  (`caBundle` + matching serving cert SAN for the Service DNS).

## Cursor / Copilot Instructions
No repo-specific files were found:
- `.cursor/rules/**` absent
- `.cursorrules` absent
- `.github/copilot-instructions.md` absent

If these files are added later, treat them as higher-priority repo instructions and update this file.

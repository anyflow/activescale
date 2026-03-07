# AGENTS

## Project Overview
- Activescale is a Go-based gRPC sink + Kubernetes custom metrics adapter.
- It receives Envoy `StreamMetrics` batches, stores per-pod active request counts in Redis, and serves them through the Kubernetes Custom Metrics API for HPA decisions.
- It is stateless at process level and designed for HA deployments with Redis-backed shared state.
- Primary entrypoint: `cmd/activescale/main.go`.
- Module path: `activescale` (`go 1.25`).

## Repository Structure
- `cmd/activescale`: executable bootstrap (flags/env wiring, Redis client setup, server startup, readiness probes).
- `internal/envoy`: Envoy gRPC message handling and metric filtering/storing behavior.
- `internal/provider`: Kubernetes custom metrics provider implementation.
- `internal/redis`: Redis/Valkey storage abstraction and key formatting.
- `manifest/`: Kubernetes manifests (`base/` and overlays) and Argo CD/ApplicationSet assets.
- `test/`: integration/validation helper assets.
- `Dockerfile`, `MOTIVATION.md`, `README.md`: operational and architectural references.
- `Makefile`: canonical local workflow commands.
- `go.mod/go.sum`: dependencies and module metadata.

## Build / Test / Run Workflow
- `make build`  
  Builds binary to `./bin/activescale`.
- `make build-pkgs`  
  Runs `go build ./...` (compile-only across all packages).
- `make test`  
  Runs `go test ./...` for unit tests.
- `make run`  
  Runs local adapter via `go run` with env defaults:
  - `GRPC_PORT` (default `9000`)
  - `REDIS_ADDR` (default `localhost:6379`)
  - `METRIC_TTL` (default `20s`)
- `make docker-build`  
  Builds single-arch image locally.
- `make docker-push`  
  Builds and pushes multi-arch image (`linux/amd64,linux/arm64`).
- `make docker-run`  
  Starts container with required ports (`6443`, `9000`, `18080`) and env defaults.

Suggested validation checks after edits:
- `go test ./...`
- `make build`
- `go test ./internal/<pkg>` for changed package-level behavior.
- If manifest changes are made, verify generated YAML consistency in your local kustomize flow.

## Coding Conventions
- Go formatting: tabs and `gofmt`.
- Use explicit variable names (`podIdentity`, `metricTTL`, `redisAddress`) and avoid abbreviations for new identifiers.
- Keep package boundaries strict across `internal/*`.
- Logging: `k8s.io/klog/v2` for all new logs; use verbosity for noisy diagnostics.
- Import order: standard lib, third-party, then local module imports.
- Test files must be `*_test.go` and stay beside implementation.

## Configuration Reference
- Runtime configuration is env-driven with values also exposed as command flags where applicable:
  - `GRPC_PORT`, `REDIS_ADDR`, `REDIS_CONTEXT`, `REDIS_TLS`, `REDIS_TLS_INSECURE`, `REDIS_CA_FILE`, `REDIS_CLUSTER`, `METRIC_NAME`, `METRIC_TTL`, `LOG_VERBOSITY`, `LOG_METRICS_SUMMARY_INTERVAL`.
- `REDIS_CLUSTER` is required (`true|false`) and determines Cluster vs standalone client.
- `METRIC_NAME`:
  - unset/empty: auto-detect inbound scoped `downstream_rq_active` family.
  - set: strict exact-match behavior for stored metric samples.
- Defaults are in `cmd/activescale/main.go` and `Makefile`.

## Agent Operating Rules (for Codex)
- Treat `README.md` and `Makefile` as behavior/operational truth when implementing; align changes with them.
- Limit scope: prefer targeted changes in `cmd/activescale`, `internal/envoy`, `internal/provider`, and `internal/redis`.
- Preserve existing semantics unless explicitly asked to change behavior; document any behavior-impacting decisions in `README.md` or manifest comments.
- Add/adjust tests when changing parsing, matching, routing, storage keys, or metric filtering logic.
- Do not remove or bypass Redis/TLS/error-handling paths without explicit rationale.
- Keep commit messages in conventional style (`feat:`, `fix:`, `chore:`, etc.) if and when committing.

.PHONY: build build-pkgs test run deps-update docker-build docker-push docker-run update-image-refreshed-at all

IMAGE_REFRESHED_AT_FILE ?= manifest/base/deployment.yaml
IMAGE_REFRESHED_AT_KEY ?= activescale.thinq.io/image-refreshed-at

build:
	# Build binary into ./bin
	mkdir -p bin
	GOCACHE=$(CURDIR)/.gocache go build -o bin/activescale ./cmd/activescale

build-pkgs:
	# Compile all packages (no binary output)
	GOCACHE=$(CURDIR)/.gocache go build ./...

test:
	# Run unit tests
	go test ./...

run:
	# Run locally with env-based defaults
	go run ./cmd/activescale \
		--grpc-port=$${GRPC_PORT:-9000} \
		--redis-addr=$${REDIS_ADDR:-localhost:6379} \
		--ttl=$${METRIC_TTL:-20s}

IMAGE ?= ghcr.io/anyflow/activescale:latest
PLATFORMS ?= linux/amd64,linux/arm64
LOCAL_PLATFORM ?= linux/arm64

docker-build:
	# Build single-arch image locally
	docker buildx build --platform $(LOCAL_PLATFORM) -t $(IMAGE) --load .

update-image-refreshed-at:
	# Refresh rollout annotation in KST so GitOps sees a manifest diff after image push.
	TIMESTAMP=$$(TZ=Asia/Seoul date '+%Y-%m-%dT%H:%M:%S+09:00'); \
	awk -v ts="$$TIMESTAMP" -v key='$(IMAGE_REFRESHED_AT_KEY)' '{ if (index($$0, key)) print "        " key ": \"" ts "\""; else print }' $(IMAGE_REFRESHED_AT_FILE) > $(IMAGE_REFRESHED_AT_FILE).tmp && mv $(IMAGE_REFRESHED_AT_FILE).tmp $(IMAGE_REFRESHED_AT_FILE)

docker-push:
	# Build and push multi-arch image
	docker buildx build --platform $(PLATFORMS) -t $(IMAGE) --push .

docker-run:
	# Run container locally
	docker run --rm \
		-p 6443:6443 \
		-p 9000:9000 \
		-p 18080:18080 \
		-e REDIS_ADDR=$${REDIS_ADDR:-localhost:6379} \
		-e REDIS_CONTEXT=$${REDIS_CONTEXT:-activescale:tcn} \
		-e REDIS_TLS=$${REDIS_TLS:-false} \
		-e METRIC_TTL=$${METRIC_TTL:-20s} \
		-e LOG_VERBOSITY=$${LOG_VERBOSITY:-0} \
		-e LOG_METRICS_SUMMARY_INTERVAL=$${LOG_METRICS_SUMMARY_INTERVAL:-30s} \
		$(IMAGE)

all: build-pkgs test docker-push update-image-refreshed-at
	# Run build, test, push image and update deployment annotation

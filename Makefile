.PHONY: build build-pkgs test run deps-update

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

deps-update:
	# Update direct dependencies and tidy go.mod/go.sum
	go get -u ./...
	go mod tidy

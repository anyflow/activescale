.PHONY: build build-pkgs tidy test run

build:
	mkdir -p bin
	GOCACHE=$(CURDIR)/.gocache go build -o bin/activescale ./cmd/activescale

build-pkgs:
	GOCACHE=$(CURDIR)/.gocache go build ./...

tidy:
	go mod tidy

test:
	go test ./...

run:
	go run ./cmd/activescale \
		--grpc-port=$${GRPC_PORT:-9000} \
		--redis-addr=$${REDIS_ADDR:-localhost:6379} \
		--ttl=$${METRIC_TTL:-20s}

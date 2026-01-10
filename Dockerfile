FROM --platform=$BUILDPLATFORM golang:1.25 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/activescale ./cmd/activescale

FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /
COPY --from=builder /out/activescale /activescale

EXPOSE 6443 9000 18080
USER nonroot:nonroot
ENTRYPOINT ["/activescale"]

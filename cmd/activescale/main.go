// cmd/adapter/main.go
package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"activescale/internal/envoy"
	adapterprovider "activescale/internal/provider"
	redisstore "activescale/internal/redis"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"

	"k8s.io/client-go/kubernetes"

	basecmd "sigs.k8s.io/custom-metrics-apiserver/pkg/cmd"
)

func main() {
	var (
		redisAddr string
		ttl       time.Duration
		grpcAddr  string
	)
	cmd := &basecmd.AdapterBase{}
	defaultRedisAddr := envOr("REDIS_ADDR", "redis:6379")
	defaultGRPCAddr := envOr("GRPC_ADDR", ":9000")
	defaultTTL := 20 * time.Second
	if envTTL := os.Getenv("METRIC_TTL"); envTTL != "" {
		parsed, err := time.ParseDuration(envTTL)
		if err != nil {
			log.Fatalf("invalid METRIC_TTL: %v", err)
		}
		defaultTTL = parsed
	}

	cmd.Flags().StringVar(&redisAddr, "redis-addr", defaultRedisAddr, "redis address")
	cmd.Flags().DurationVar(&ttl, "ttl", defaultTTL, "metric TTL (e.g. 20s)")
	cmd.Flags().StringVar(&grpcAddr, "grpc-addr", defaultGRPCAddr, "envoy metrics gRPC listen addr")
	if err := cmd.Flags().Parse(os.Args); err != nil {
		log.Fatalf("parse flags: %v", err)
	}

	// redis
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	store := redisstore.New(rdb, ttl)

	// 1) gRPC sink server
	go func() {
		lis, err := net.Listen("tcp", grpcAddr)
		if err != nil {
			log.Fatalf("grpc listen: %v", err)
		}
		gs := grpc.NewServer()
		envoy.NewMetricsServer(store).Register(gs)
		log.Printf("envoy metrics gRPC listening on %s", grpcAddr)
		if err := gs.Serve(lis); err != nil {
			log.Fatalf("grpc serve: %v", err)
		}
	}()

	// 2) custom-metrics apiserver
	// framework는 보통 HTTPS + authn/authz + APIService 연동을 처리.
	// 여기서는 “provider만 주입”하는 최소 형태로 작성합니다.
	cfg, err := cmd.ClientConfig()
	if err != nil {
		log.Fatalf("kube config: %v", err)
	}
	kube, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("kube client: %v", err)
	}

	podsProvider := adapterprovider.NewPodsProvider(kube, store)
	cmd.WithCustomMetrics(podsProvider)

	go func() {
		if err := cmd.Run(context.Background()); err != nil {
			log.Fatalf("adapter run: %v", err)
		}
	}()

	// 프로세스 유지
	log.Fatal(http.ListenAndServe(":18080", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	})))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

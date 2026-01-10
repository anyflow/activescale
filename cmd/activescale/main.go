// cmd/adapter/main.go
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
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
		grpcPort  string
	)
	cmd := &basecmd.AdapterBase{}
	defaultRedisAddr := envOr("REDIS_ADDR", "redis:6379")
	defaultGRPCPort := envOr("GRPC_PORT", "9000")
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
	cmd.Flags().StringVar(&grpcPort, "grpc-port", defaultGRPCPort, "envoy metrics gRPC listen port")
	if err := cmd.Flags().Parse(os.Args); err != nil {
		log.Fatalf("parse flags: %v", err)
	}

	// redis
	redisOpts := &redis.Options{Addr: redisAddr}
	if envBool("REDIS_TLS", false) {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: envBool("REDIS_TLS_INSECURE", false),
		}
		if caFile := os.Getenv("REDIS_CA_FILE"); caFile != "" {
			caPEM, err := os.ReadFile(caFile)
			if err != nil {
				log.Fatalf("read REDIS_CA_FILE: %v", err)
			}
			certPool := x509.NewCertPool()
			if !certPool.AppendCertsFromPEM(caPEM) {
				log.Fatal("failed to parse REDIS_CA_FILE PEM")
			}
			tlsConfig.RootCAs = certPool
		}
		redisOpts.TLSConfig = tlsConfig
	}
	rdb := redis.NewClient(redisOpts)
	store := redisstore.New(rdb, ttl)

	// 1) gRPC sink server
	go func() {
		lis, err := net.Listen("tcp", ":"+grpcPort)
		if err != nil {
			log.Fatalf("grpc listen: %v", err)
		}
		gs := grpc.NewServer()
		envoy.NewMetricsServer(store).Register(gs)
		log.Printf("envoy metrics gRPC listening on %s", ":"+grpcPort)
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

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		log.Fatalf("invalid %s: %v", key, err)
	}
	return parsed
}

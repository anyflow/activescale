// cmd/activescale/main.go
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"activescale/internal/envoy"
	adapterprovider "activescale/internal/provider"
	redisstore "activescale/internal/redis"

	"github.com/redis/go-redis/v9"
	"github.com/spf13/pflag"
	"google.golang.org/grpc"

	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	basecmd "sigs.k8s.io/custom-metrics-apiserver/pkg/cmd"
)

func main() {
	var (
		redisAddr string
		ttl       time.Duration
		grpcPort  string
	)
	cmd := &basecmd.AdapterBase{FlagSet: pflag.CommandLine}
	klog.InitFlags(nil)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	defaultRedisAddr := envOr("REDIS_ADDR", "redis:6379")
	defaultRedisContext := envOr("REDIS_CONTEXT", "activescale:tcn")
	defaultGRPCPort := envOr("GRPC_PORT", "9000")
	defaultMetricName := envOr("METRIC_NAME", "envoy_http_downstream_rq_active")
	defaultTTL := 20 * time.Second
	defaultSummaryInterval := 30 * time.Second
	if envSummary := os.Getenv("LOG_METRICS_SUMMARY_INTERVAL"); envSummary != "" {
		parsed, err := time.ParseDuration(envSummary)
		if err != nil {
			klog.Fatalf("invalid LOG_METRICS_SUMMARY_INTERVAL: %v", err)
		}
		defaultSummaryInterval = parsed
	}
	if envTTL := os.Getenv("METRIC_TTL"); envTTL != "" {
		parsed, err := time.ParseDuration(envTTL)
		if err != nil {
			klog.Fatalf("invalid METRIC_TTL: %v", err)
		}
		defaultTTL = parsed
	}

	if envVerbosity := os.Getenv("LOG_VERBOSITY"); envVerbosity != "" {
		if err := pflag.CommandLine.Set("v", envVerbosity); err != nil {
			klog.Fatalf("invalid LOG_VERBOSITY: %v", err)
		}
	}

	pflag.CommandLine.StringVar(&redisAddr, "redis-addr", defaultRedisAddr, "redis address")
	redisContext := defaultRedisContext
	pflag.CommandLine.DurationVar(&ttl, "ttl", defaultTTL, "metric TTL (e.g. 20s)")
	pflag.CommandLine.StringVar(&grpcPort, "grpc-port", defaultGRPCPort, "envoy metrics gRPC listen port")
	if err := pflag.CommandLine.Parse(os.Args); err != nil {
		klog.Fatalf("parse flags: %v", err)
	}
	defer klog.Flush()
	klog.Infof("starting activescale init")
	klog.Infof("config grpc_port=%s redis_addr=%s redis_context=%s metric_name=%s ttl=%s log_verbosity=%s summary_interval=%s",
		grpcPort, redisAddr, redisContext, defaultMetricName, ttl, pflag.CommandLine.Lookup("v").Value.String(), defaultSummaryInterval)

	// redis
	klog.Infof("initializing redis client")
	redisOpts := &redis.Options{Addr: redisAddr}
	redisTLS := envBool("REDIS_TLS", false)
	if redisTLS {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: envBool("REDIS_TLS_INSECURE", false),
		}
		klog.Infof("redis tls enabled=%t insecure=%t ca_file_set=%t",
			redisTLS, tlsConfig.InsecureSkipVerify, os.Getenv("REDIS_CA_FILE") != "")
		if caFile := os.Getenv("REDIS_CA_FILE"); caFile != "" {
			caPEM, err := os.ReadFile(caFile)
			if err != nil {
				klog.Fatalf("read REDIS_CA_FILE: %v", err)
			}
			certPool := x509.NewCertPool()
			if !certPool.AppendCertsFromPEM(caPEM) {
				klog.Fatal("failed to parse REDIS_CA_FILE PEM")
			}
			tlsConfig.RootCAs = certPool
		}
		redisOpts.TLSConfig = tlsConfig
	}
	rdb := redis.NewClient(redisOpts)
	store := redisstore.New(rdb, ttl, redisContext)
	klog.Infof("redis client initialized")

	// 1) gRPC sink server
	klog.Infof("initializing envoy metrics gRPC server")
	go func() {
		lis, err := net.Listen("tcp", ":"+grpcPort)
		if err != nil {
			klog.Fatalf("grpc listen: %v", err)
		}
		klog.Infof("envoy metrics gRPC port bound addr=:%s", grpcPort)
		gs := grpc.NewServer()
		envoy.NewMetricsServer(store, defaultSummaryInterval, defaultMetricName).Register(gs)
		klog.Infof("envoy metrics gRPC listening on %s", ":"+grpcPort)
		if err := gs.Serve(lis); err != nil {
			klog.Fatalf("grpc serve: %v", err)
		}
	}()

	// 2) custom-metrics apiserver
	// framework는 보통 HTTPS + authn/authz + APIService 연동을 처리.
	// 여기서는 “provider만 주입”하는 최소 형태로 작성합니다.
	klog.Infof("initializing custom metrics provider")
	cfg, err := cmd.ClientConfig()
	if err != nil {
		klog.Fatalf("kube config: %v", err)
	}
	klog.Infof("kube config initialized")
	kube, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("kube client: %v", err)
	}
	klog.Infof("kube client initialized")

	podsProvider := adapterprovider.NewPodsProvider(kube, store, defaultSummaryInterval)
	cmd.WithCustomMetrics(podsProvider)

	go func() {
		if err := cmd.Run(context.Background()); err != nil {
			klog.Fatalf("adapter run: %v", err)
		}
	}()

	// 프로세스 유지
	klog.Fatalf("http server error: %v", http.ListenAndServe(":18080", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		klog.Fatalf("invalid %s: %v", key, err)
	}
	return parsed
}

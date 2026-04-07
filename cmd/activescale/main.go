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
	cmd.Flags()
	defaultRedisAddr := envOr("REDIS_ADDR", "redis:6379")
	defaultRedisContext := envOr("REDIS_CONTEXT", "activescale:tcn")
	defaultGRPCPort := envOr("GRPC_PORT", "9000")
	defaultTTL := envDuration("METRIC_TTL", 20*time.Second)
	defaultSummaryInterval := envDuration("LOG_METRICS_SUMMARY_INTERVAL", 30*time.Second)

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
	klog.Infof("config grpc_port=%s redis_addr=%s redis_context=%s ttl=%s log_verbosity=%s summary_interval=%s",
		grpcPort, redisAddr, redisContext, ttl, pflag.CommandLine.Lookup("v").Value.String(), defaultSummaryInterval)

	// redis
	klog.Infof("initializing redis client")
	redisTLS := envBool("REDIS_TLS", false)
	var tlsConfig *tls.Config
	if redisTLS {
		tlsConfig = &tls.Config{
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
	}
	// Redis Cluster returns MOVED replies unless the client supports cluster redirection.
	// Keep the config surface area small: require a single explicit env var.
	//
	// REDIS_CLUSTER=true  -> use ClusterClient
	// REDIS_CLUSTER=false -> use Client
	if _, ok := os.LookupEnv("REDIS_CLUSTER"); !ok {
		klog.Fatal("missing Redis mode config: set REDIS_CLUSTER=true|false")
	}
	redisCluster := envBool("REDIS_CLUSTER", false)

	var rdb redis.Cmdable
	if redisCluster {
		klog.Infof("redis mode=cluster addr=%s", redisAddr)
		rdb = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:     []string{redisAddr},
			TLSConfig: tlsConfig,
		})
	} else {
		klog.Infof("redis mode=standalone addr=%s", redisAddr)
		rdb = redis.NewClient(&redis.Options{
			Addr:      redisAddr,
			TLSConfig: tlsConfig,
		})
	}

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
		envoy.NewMetricsServer(store, defaultSummaryInterval).Register(gs)
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

	// 프로세스 유지 (liveness/readiness endpoints)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := rdb.Ping(ctx).Err(); err != nil {
			klog.Warningf("readiness check failed: %v", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not ready"))
			return
		}
		if _, err := kube.Discovery().ServerVersion(); err != nil {
			klog.Warningf("readiness check failed (kube): %v", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not ready"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})
	klog.Fatalf("http server error: %v", http.ListenAndServe(":18080", mux))
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

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		klog.Fatalf("invalid %s: %v", key, err)
	}
	return parsed
}

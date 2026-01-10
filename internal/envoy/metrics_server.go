// internal/envoy/metrics_server.go
package envoy

import (
	"sync"
	"sync/atomic"
	"time"

	redisstore "activescale/internal/redis"

	"google.golang.org/grpc"
	"k8s.io/klog/v2"

	// Envoy go-control-plane (예시 import)
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	metricsv3 "github.com/envoyproxy/go-control-plane/envoy/service/metrics/v3"
)

type MetricsServer struct {
	metricsv3.UnimplementedMetricsServiceServer
	store *redisstore.Store

	logOnce      sync.Once
	logEvery     time.Duration
	recvBatches  uint64
	metricName   string
}

func NewMetricsServer(store *redisstore.Store, logEvery time.Duration, metricName string) *MetricsServer {
	return &MetricsServer{
		store:      store,
		logEvery:   logEvery,
		metricName: metricName,
	}
}

func (s *MetricsServer) Register(grpcServer *grpc.Server) {
	metricsv3.RegisterMetricsServiceServer(grpcServer, s)
}

func (s *MetricsServer) StreamMetrics(stream metricsv3.MetricsService_StreamMetricsServer) error {
	ctx := stream.Context()
	s.logOnce.Do(func() {
		go s.logSummary()
	})

	for {
		msg, err := stream.Recv()
		if err != nil {
			klog.Warningf("metrics stream recv error: %v", err)
			return err
		}
		atomic.AddUint64(&s.recvBatches, 1)

		ns, pod := extractPodIdentity(msg.GetIdentifier().GetNode())
		if ns == "" || pod == "" {
			// 식별 불가면 그냥 무시(또는 metric name만 저장 등 정책 선택)
			klog.V(4).Info("missing pod identity in metrics stream")
			continue
		}

		// rq_active만 관심 (ProxyStatsMatcher로 이미 필터링된다고 가정)
		for _, mf := range msg.GetEnvoyMetrics() {
			// EnvoyMetrics에는 Counter/Gauge/Histogram이 섞여 있음
			name := mf.GetName()
			if name == "" {
				klog.V(4).Info("missing metric family name")
				continue
			}
			// Validate exact metric name to avoid accidentally ingesting other metrics.
			if name != s.metricName {
				klog.V(4).Infof("skipping metric name=%s", name)
				continue
			}
			for _, m := range mf.GetMetric() {
				if g := m.GetGauge(); g != nil {
					val := g.GetValue()
					if err := s.store.SetGauge(ctx, ns, pod, "active_requests", val); err != nil {
						klog.Warningf("redis set failed ns=%s pod=%s: %v", ns, pod, err)
						continue
					}
					klog.V(4).Infof("stored active_requests ns=%s pod=%s value=%.6f", ns, pod, val)
				}
			}
		}
	}
}

func (s *MetricsServer) logSummary() {
	ticker := time.NewTicker(s.logEvery)
	defer ticker.Stop()

	for range ticker.C {
		batches := atomic.SwapUint64(&s.recvBatches, 0)
		klog.Infof("envoy metrics batches received in last %s: %d", s.logEvery, batches)
	}
}

func extractPodIdentity(node *corev3.Node) (namespace, pod string) {
	if node == nil || node.Metadata == nil {
		return "", ""
	}
	// Istio 환경에서 metadata 키가 환경/버전마다 다를 수 있어 복수 키를 허용
	get := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := node.Metadata.Fields[k]; ok && v.GetKind() != nil {
				// string value만 처리
				if sv := v.GetStringValue(); sv != "" {
					return sv
				}
			}
		}
		return ""
	}

	// 사용자가 문서에 적어둔 키 우선
	pod = get("POD_NAME", "pod_name", "NAME", "POD", "pod")
	namespace = get("POD_NAMESPACE", "pod_namespace", "NAMESPACE", "ns", "namespace")
	return namespace, pod
}

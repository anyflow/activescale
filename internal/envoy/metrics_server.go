// internal/envoy/metrics_server.go
package envoy

import (
	"sync"
	"sync/atomic"
	"time"
	"strings"

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
	recvMessages uint64
	dropByID    uint64
	dropName     uint64
	storedGauges uint64
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

	var streamNS, streamPod string
	missingLogged := false
	for {
		msg, err := stream.Recv()
		if err != nil {
			klog.Warningf("metrics stream recv error: %v", err)
			return err
		}
		atomic.AddUint64(&s.recvMessages, 1)

		if streamNS == "" || streamPod == "" {
			streamNS, streamPod = extractPodIdentity(msg.GetIdentifier().GetNode())
		}
		if streamNS == "" || streamPod == "" {
			// 식별 불가면 그냥 무시(또는 metric name만 저장 등 정책 선택)
			if !missingLogged {
				klog.V(4).Info("missing pod identity in metrics stream")
				missingLogged = true
			}
			atomic.AddUint64(&s.dropByID, 1)
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
				atomic.AddUint64(&s.dropName, 1)
				continue
			}
			for _, m := range mf.GetMetric() {
				if g := m.GetGauge(); g != nil {
					val := g.GetValue()
					if err := s.store.SetGauge(ctx, streamNS, streamPod, "active_requests", val); err != nil {
						klog.Warningf("redis set failed ns=%s pod=%s: %v", streamNS, streamPod, err)
						continue
					}
					atomic.AddUint64(&s.storedGauges, 1)
					klog.V(4).Infof("stored active_requests ns=%s pod=%s value=%.6f", streamNS, streamPod, val)
				}
			}
		}
	}
}

func (s *MetricsServer) logSummary() {
	ticker := time.NewTicker(s.logEvery)
	defer ticker.Stop()

	for range ticker.C {
		messages := atomic.SwapUint64(&s.recvMessages, 0)
		dropByID := atomic.SwapUint64(&s.dropByID, 0)
		dropName := atomic.SwapUint64(&s.dropName, 0)
		stored := atomic.SwapUint64(&s.storedGauges, 0)
		klog.Infof("envoy metrics summary in last %s: messages=%d stored=%d dropped_by_ids=%d dropped_by_names=%d",
			s.logEvery, messages, stored, dropByID, dropName)
	}
}

func extractPodIdentity(node *corev3.Node) (namespace, pod string) {
	if node == nil {
		return "", ""
	}
	// Istio node.id 형식: sidecar~<ip>~<pod>.<namespace>~<namespace>.svc.cluster.local
	id := node.GetId()
	if id == "" {
		return "", ""
	}
	parts := strings.Split(id, "~")
	if len(parts) < 4 {
		return "", ""
	}
	podNS := parts[2]
	nsDomain := parts[3]
	if podNS == "" || nsDomain == "" {
		return "", ""
	}
	podParts := strings.SplitN(podNS, ".", 2)
	if len(podParts) != 2 {
		return "", ""
	}
	pod = podParts[0]
	namespace = strings.SplitN(nsDomain, ".", 2)[0]
	if pod == "" || namespace == "" {
		return "", ""
	}
	return namespace, pod
}

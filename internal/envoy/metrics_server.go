// internal/envoy/metrics_server.go
package envoy

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	podmetrics "activescale/internal/podmetrics"
	redisstore "activescale/internal/redis"

	dto "github.com/prometheus/client_model/go"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"

	// Envoy go-control-plane (예시 import)
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	metricsv3 "github.com/envoyproxy/go-control-plane/envoy/service/metrics/v3"
)

type MetricsServer struct {
	metricsv3.UnimplementedMetricsServiceServer
	store *redisstore.Store

	logOnce       sync.Once
	logEvery      time.Duration
	recvMessages  uint64
	dropByID      uint64
	dropName      uint64
	storedMetrics uint64
}

func NewMetricsServer(store *redisstore.Store, logEvery time.Duration) *MetricsServer {
	return &MetricsServer{
		store:    store,
		logEvery: logEvery,
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

		metricValues, seenValues, droppedByName := collectMetricValues(msg.GetEnvoyMetrics())
		if droppedByName > 0 {
			atomic.AddUint64(&s.dropName, uint64(droppedByName))
		}

		for _, metricName := range podmetrics.Names() {
			if !seenValues[metricName] {
				continue
			}
			if err := s.store.SetGauge(ctx, streamNS, streamPod, metricName, metricValues[metricName]); err != nil {
				klog.Warningf("redis set failed ns=%s pod=%s: %v", streamNS, streamPod, err)
			} else {
				atomic.AddUint64(&s.storedMetrics, 1)
				klog.V(4).Infof("stored %s ns=%s pod=%s value=%.6f", metricName, streamNS, streamPod, metricValues[metricName])
			}
		}
	}
}

func collectMetricValues(metricFamilies []*dto.MetricFamily) (map[string]float64, map[string]bool, int) {
	totals := make(map[string]float64, len(podmetrics.Names()))
	seen := make(map[string]bool, len(podmetrics.Names()))
	droppedByName := 0

	for _, metricFamily := range metricFamilies {
		name := metricFamily.GetName()
		if name == "" {
			klog.V(4).Info("missing metric family name")
			continue
		}

		metricName, ok := podmetrics.MatchMetricName(name)
		if !ok {
			klog.V(4).Infof("skipping metric name=%s", name)
			droppedByName++
			continue
		}

		for _, metric := range metricFamily.GetMetric() {
			if gauge := metric.GetGauge(); gauge != nil {
				totals[metricName] += gauge.GetValue()
				seen[metricName] = true
			}
		}
	}

	return totals, seen, droppedByName
}

func (s *MetricsServer) logSummary() {
	ticker := time.NewTicker(s.logEvery)
	defer ticker.Stop()

	for range ticker.C {
		messages := atomic.SwapUint64(&s.recvMessages, 0)
		dropByID := atomic.SwapUint64(&s.dropByID, 0)
		dropName := atomic.SwapUint64(&s.dropName, 0)
		stored := atomic.SwapUint64(&s.storedMetrics, 0)
		klog.Infof("envoy metrics summary in last %s: messages=%d stored_metrics=%d dropped_by_ids=%d dropped_by_names=%d",
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

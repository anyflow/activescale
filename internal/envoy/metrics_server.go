// internal/envoy/metrics_server.go
package envoy

import (
	"strings"

	redisstore "activescale/internal/redis"

	"google.golang.org/grpc"

	// Envoy go-control-plane (예시 import)
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	metricsv3 "github.com/envoyproxy/go-control-plane/envoy/service/metrics/v3"
)

type MetricsServer struct {
	metricsv3.UnimplementedMetricsServiceServer
	store *redisstore.Store
}

func NewMetricsServer(store *redisstore.Store) *MetricsServer {
	return &MetricsServer{store: store}
}

func (s *MetricsServer) Register(grpcServer *grpc.Server) {
	metricsv3.RegisterMetricsServiceServer(grpcServer, s)
}

func (s *MetricsServer) StreamMetrics(stream metricsv3.MetricsService_StreamMetricsServer) error {
	ctx := stream.Context()

	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}

		ns, pod := extractPodIdentity(msg.GetIdentifier().GetNode())
		if ns == "" || pod == "" {
			// 식별 불가면 그냥 무시(또는 metric name만 저장 등 정책 선택)
			continue
		}

		// rq_active만 관심 (ProxyStatsMatcher로 이미 필터링된다고 가정)
		for _, mf := range msg.GetEnvoyMetrics() {
			// EnvoyMetrics에는 Counter/Gauge/Histogram이 섞여 있음
			name := mf.GetName()
			// 보통 "envoy_http_downstream_rq_active" 포함 여부
			if !strings.Contains(name, "downstream_rq_active") {
				continue
			}
			for _, m := range mf.GetMetric() {
				if g := m.GetGauge(); g != nil {
					val := g.GetValue()
					_ = s.store.SetGauge(ctx, ns, pod, "active_requests", val)
				}
			}
		}
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

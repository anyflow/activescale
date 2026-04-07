package envoy

import (
	podmetrics "activescale/internal/podmetrics"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"google.golang.org/protobuf/proto"
)

func TestCollectMetricValues(t *testing.T) {
	t.Parallel()

	metricFamilies := []*dto.MetricFamily{
		{
			Name: proto.String("http.inbound_0.0.0.0_8080;.downstream_rq_active"),
			Metric: []*dto.Metric{
				{Gauge: &dto.Gauge{Value: proto.Float64(2)}},
				{Gauge: &dto.Gauge{Value: proto.Float64(3)}},
			},
		},
		{
			Name: proto.String("listener.0.0.0.0_8080.downstream_cx_active"),
			Metric: []*dto.Metric{
				{Gauge: &dto.Gauge{Value: proto.Float64(7)}},
			},
		},
		{
			Name: proto.String("listener.0.0.0.0_15090.downstream_cx_active"),
			Metric: []*dto.Metric{
				{Gauge: &dto.Gauge{Value: proto.Float64(11)}},
			},
		},
	}

	values, seen, droppedByName := collectMetricValues(metricFamilies)

	if got := values[podmetrics.ActiveRequests]; got != 5 {
		t.Fatalf("active_requests total = %v, want 5", got)
	}
	if !seen[podmetrics.ActiveRequests] {
		t.Fatal("active_requests should be marked as seen")
	}
	if got := values[podmetrics.ActiveConnections]; got != 7 {
		t.Fatalf("active_connections total = %v, want 7", got)
	}
	if !seen[podmetrics.ActiveConnections] {
		t.Fatal("active_connections should be marked as seen")
	}
	if droppedByName != 1 {
		t.Fatalf("droppedByName = %d, want 1", droppedByName)
	}
}

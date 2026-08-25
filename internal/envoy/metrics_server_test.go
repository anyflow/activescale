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
			Name: proto.String("http.outbound_0.0.0.0_8443;.downstream_rq_active"),
			Metric: []*dto.Metric{
				{Gauge: &dto.Gauge{Value: proto.Float64(4)}},
			},
		},
		{
			Name: proto.String("listener.0.0.0.0_8443.downstream_cx_active"),
			Metric: []*dto.Metric{
				{Gauge: &dto.Gauge{Value: proto.Float64(7)}},
			},
		},
		{
			Name: proto.String("listener.0.0.0.0_8080.downstream_cx_active"),
			Metric: []*dto.Metric{
				{Gauge: &dto.Gauge{Value: proto.Float64(5)}},
			},
		},
		{
			Name: proto.String("listener.0.0.0.0_8443.worker_0.downstream_cx_active"),
			Metric: []*dto.Metric{
				{Gauge: &dto.Gauge{Value: proto.Float64(13)}},
			},
		},
		{
			Name: proto.String("listener.admin.main_thread.downstream_cx_active"),
			Metric: []*dto.Metric{
				{Gauge: &dto.Gauge{Value: proto.Float64(9)}},
			},
		},
	}

	values, seen, droppedByName := collectMetricValues(metricFamilies)

	if got := values[podmetrics.InboundActiveRequests]; got != 5 {
		t.Fatalf("inbound_active_requests total = %v, want 5", got)
	}
	if !seen[podmetrics.InboundActiveRequests] {
		t.Fatal("inbound_active_requests should be marked as seen")
	}
	if got := values[podmetrics.OutboundActiveRequests]; got != 4 {
		t.Fatalf("outbound_active_requests total = %v, want 4", got)
	}
	if !seen[podmetrics.OutboundActiveRequests] {
		t.Fatal("outbound_active_requests should be marked as seen")
	}
	if got := values[podmetrics.ActiveConnections]; got != 12 {
		t.Fatalf("active_connections total = %v, want 12", got)
	}
	if !seen[podmetrics.ActiveConnections] {
		t.Fatal("active_connections should be marked as seen")
	}
	if droppedByName != 2 {
		t.Fatalf("droppedByName = %d, want 2", droppedByName)
	}
}

package podmetrics

import "testing"

func TestNames(t *testing.T) {
	got := Names()
	want := []string{InboundActiveRequests, OutboundActiveRequests, ActiveConnections}
	if len(got) != len(want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Names() = %v, want %v", got, want)
		}
	}
}

func TestMatchMetricNameForRequestMetrics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		envoyMetricName string
		wantMetricName  string
		wantMatched     bool
	}{
		{
			name:            "accepts inbound downstream active requests",
			envoyMetricName: "http.inbound_0.0.0.0_8080;.downstream_rq_active",
			wantMetricName:  InboundActiveRequests,
			wantMatched:     true,
		},
		{
			name:            "accepts outbound downstream active requests",
			envoyMetricName: "http.outbound_0.0.0.0_8443;.downstream_rq_active",
			wantMetricName:  OutboundActiveRequests,
			wantMatched:     true,
		},
		{
			name:            "rejects envoy prometheus alias",
			envoyMetricName: "envoy_http_downstream_rq_active",
			wantMatched:     false,
		},
		{
			name:            "rejects stats scope",
			envoyMetricName: "http.stats.downstream_rq_active",
			wantMatched:     false,
		},
		{
			name:            "rejects admin scope",
			envoyMetricName: "http.admin.downstream_rq_active",
			wantMatched:     false,
		},
		{
			name:            "rejects agent scope",
			envoyMetricName: "http.agent.downstream_rq_active",
			wantMatched:     false,
		},
		{
			name:            "rejects different metric family",
			envoyMetricName: "http.inbound_0.0.0.0_8080;.downstream_cx_active",
			wantMatched:     false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotMetricName, gotMatched := MatchMetricName(tt.envoyMetricName)
			if gotMatched != tt.wantMatched {
				t.Fatalf("MatchMetricName(%q) matched = %t, want %t", tt.envoyMetricName, gotMatched, tt.wantMatched)
			}
			if gotMetricName != tt.wantMetricName {
				t.Fatalf("MatchMetricName(%q) metric = %q, want %q", tt.envoyMetricName, gotMetricName, tt.wantMetricName)
			}
		})
	}
}

func TestMatchMetricNameForActiveConnections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		envoyMetricName string
		wantMetricName  string
		wantMatched     bool
	}{
		{
			name:            "accepts wildcard service listener",
			envoyMetricName: "listener.0.0.0.0_8443.downstream_cx_active",
			wantMetricName:  ActiveConnections,
			wantMatched:     true,
		},
		{
			name:            "accepts address-specific service listener on another port",
			envoyMetricName: "listener.172.20.10.15_8080.downstream_cx_active",
			wantMetricName:  ActiveConnections,
			wantMatched:     true,
		},
		{
			name:            "accepts outbound traffic listener",
			envoyMetricName: "listener.0.0.0.0_15001.downstream_cx_active",
			wantMetricName:  ActiveConnections,
			wantMatched:     true,
		},
		{
			name:            "accepts inbound traffic listener",
			envoyMetricName: "listener.0.0.0.0_15006.downstream_cx_active",
			wantMetricName:  ActiveConnections,
			wantMatched:     true,
		},
		{
			name:            "accepts hbone traffic listener",
			envoyMetricName: "listener.0.0.0.0_15008.downstream_cx_active",
			wantMetricName:  ActiveConnections,
			wantMatched:     true,
		},
		{
			name:            "rejects worker listener breakdown",
			envoyMetricName: "listener.0.0.0.0_8443.worker_0.downstream_cx_active",
			wantMatched:     false,
		},
		{
			name:            "rejects admin listener",
			envoyMetricName: "listener.admin.downstream_cx_active",
			wantMatched:     false,
		},
		{
			name:            "rejects admin main thread listener",
			envoyMetricName: "listener.admin.main_thread.downstream_cx_active",
			wantMatched:     false,
		},
		{
			name:            "rejects legacy admin main thread listener",
			envoyMetricName: "listener.admin_main_thread.downstream_cx_active",
			wantMatched:     false,
		},
		{
			name:            "rejects worker breakdown on infrastructure listener",
			envoyMetricName: "listener.0.0.0.0_15090.worker_6.downstream_cx_active",
			wantMatched:     false,
		},
		{
			name:            "rejects admin port",
			envoyMetricName: "listener.0.0.0.0_15000.downstream_cx_active",
			wantMatched:     false,
		},
		{
			name:            "rejects failure detection port",
			envoyMetricName: "listener.0.0.0.0_15002.downstream_cx_active",
			wantMatched:     false,
		},
		{
			name:            "rejects debug port",
			envoyMetricName: "listener.0.0.0.0_15004.downstream_cx_active",
			wantMatched:     false,
		},
		{
			name:            "rejects merged metrics port",
			envoyMetricName: "listener.0.0.0.0_15020.downstream_cx_active",
			wantMatched:     false,
		},
		{
			name:            "rejects health port",
			envoyMetricName: "listener.0.0.0.0_15021.downstream_cx_active",
			wantMatched:     false,
		},
		{
			name:            "rejects dns capture port",
			envoyMetricName: "listener.0.0.0.0_15053.downstream_cx_active",
			wantMatched:     false,
		},
		{
			name:            "rejects prometheus metrics port",
			envoyMetricName: "listener.0.0.0.0_15090.downstream_cx_active",
			wantMatched:     false,
		},
		{
			name:            "rejects different metric family",
			envoyMetricName: "listener.0.0.0.0_8443.downstream_rq_active",
			wantMatched:     false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotMetricName, gotMatched := MatchMetricName(tt.envoyMetricName)
			if gotMatched != tt.wantMatched {
				t.Fatalf("MatchMetricName(%q) matched = %t, want %t", tt.envoyMetricName, gotMatched, tt.wantMatched)
			}
			if gotMetricName != tt.wantMetricName {
				t.Fatalf("MatchMetricName(%q) metric = %q, want %q", tt.envoyMetricName, gotMetricName, tt.wantMetricName)
			}
		})
	}
}

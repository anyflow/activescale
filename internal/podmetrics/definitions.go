package podmetrics

import "strings"

const (
	InboundActiveRequests  = "inbound_active_requests"
	OutboundActiveRequests = "outbound_active_requests"
	ActiveConnections      = "active_connections"
)

type definition struct {
	name    string
	matches func(string) bool
}

var definitions = []definition{
	{
		name:    InboundActiveRequests,
		matches: isInboundActiveRequestMetric,
	},
	{
		name:    OutboundActiveRequests,
		matches: isOutboundActiveRequestMetric,
	},
	{
		name:    ActiveConnections,
		matches: isServiceActiveConnectionMetric,
	},
}

func Names() []string {
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.name)
	}
	return names
}

func MatchMetricName(envoyMetricName string) (string, bool) {
	for _, definition := range definitions {
		if definition.matches(envoyMetricName) {
			return definition.name, true
		}
	}
	return "", false
}

func isInboundActiveRequestMetric(name string) bool {
	return strings.HasPrefix(name, "http.inbound_") && strings.HasSuffix(name, ".downstream_rq_active")
}

func isOutboundActiveRequestMetric(name string) bool {
	return strings.HasPrefix(name, "http.outbound_") && strings.HasSuffix(name, ".downstream_rq_active")
}

func isServiceActiveConnectionMetric(name string) bool {
	if !strings.HasPrefix(name, "listener.") || !strings.HasSuffix(name, ".downstream_cx_active") {
		return false
	}

	listenerName := strings.TrimSuffix(strings.TrimPrefix(name, "listener."), ".downstream_cx_active")
	if listenerName == "" ||
		listenerName == "admin" ||
		strings.HasPrefix(listenerName, "admin.") ||
		strings.HasPrefix(listenerName, "admin_") ||
		strings.Contains(listenerName, ".worker_") {
		return false
	}

	separator := strings.LastIndex(listenerName, "_")
	if separator == -1 || separator == len(listenerName)-1 {
		return false
	}
	port := listenerName[separator+1:]
	// Ignore Istio-reserved non-traffic listeners. Traffic ports 15001, 15006, and 15008 remain eligible.
	switch port {
	case "15000": // Envoy admin commands and diagnostics.
		return false
	case "15002": // Istio failure-detection listener.
		return false
	case "15004": // Istio proxy debug endpoint.
		return false
	case "15020": // Merged Prometheus telemetry.
		return false
	case "15021": // Istio health checks.
		return false
	case "15053": // Istio DNS capture.
		return false
	case "15090": // Envoy Prometheus telemetry.
		return false
	default:
		return true
	}
}

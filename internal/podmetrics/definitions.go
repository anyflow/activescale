package podmetrics

import "strings"

const (
	ActiveRequests    = "active_requests"
	ActiveConnections = "active_connections"
)

type definition struct {
	name    string
	matches func(string) bool
}

var definitions = []definition{
	{
		name:    ActiveRequests,
		matches: isInboundActiveRequestMetric,
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
	return strings.HasPrefix(name, "http.inbound_") && strings.HasSuffix(name, "downstream_rq_active")
}

func isServiceActiveConnectionMetric(name string) bool {
	if !strings.HasPrefix(name, "listener.") || !strings.HasSuffix(name, ".downstream_cx_active") {
		return false
	}

	listenerName := strings.TrimPrefix(name, "listener.")
	listenerName = strings.TrimSuffix(listenerName, ".downstream_cx_active")
	if listenerName == "" {
		return false
	}
	if listenerName == "admin" || listenerName == "admin_main_thread" || strings.HasPrefix(listenerName, "worker_") {
		return false
	}

	separator := strings.LastIndex(listenerName, "_")
	if separator == -1 || separator == len(listenerName)-1 {
		return false
	}
	port := listenerName[separator+1:]
	switch port {
	case "15000", "15020", "15021", "15090":
		return false
	default:
		return true
	}
}

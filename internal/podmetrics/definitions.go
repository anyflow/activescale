package podmetrics

import (
	"fmt"
	"path"
	"strings"
	"sync"
)

const (
	InboundActiveRequests     = "inbound_active_requests"
	OutboundActiveRequests    = "outbound_active_requests"
	ActiveConnections         = "active_connections"
	AggregationSum            = "sum"
	FilterNone                = "none"
	FilterConnectionAggregate = "connection_aggregate"
)

type Definition struct {
	Name              string
	SourcePattern     string
	PerPodAggregation string
	Filter            string
}

type Catalog struct {
	definitions []Definition
}

type CatalogStore struct {
	mu      sync.RWMutex
	catalog *Catalog
}

func NewCatalogStore(catalog *Catalog) *CatalogStore {
	if catalog == nil {
		catalog = DefaultCatalog()
	}
	return &CatalogStore{catalog: catalog}
}

func (s *CatalogStore) Current() *Catalog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.catalog
}

func (s *CatalogStore) Replace(catalog *Catalog) {
	if catalog == nil {
		return
	}
	s.mu.Lock()
	s.catalog = catalog
	s.mu.Unlock()
}

func NewCatalog(definitions []Definition) (*Catalog, error) {
	if len(definitions) == 0 {
		return nil, fmt.Errorf("at least one metric definition is required")
	}

	seen := make(map[string]struct{}, len(definitions))
	validated := make([]Definition, len(definitions))
	for i, definition := range definitions {
		if definition.Name == "" {
			return nil, fmt.Errorf("metric definition %d has no name", i)
		}
		if _, exists := seen[definition.Name]; exists {
			return nil, fmt.Errorf("duplicate metric name %q", definition.Name)
		}
		seen[definition.Name] = struct{}{}

		if definition.SourcePattern == "" {
			return nil, fmt.Errorf("metric %q has no source pattern", definition.Name)
		}
		if _, err := path.Match(definition.SourcePattern, ""); err != nil {
			return nil, fmt.Errorf("metric %q has invalid source pattern: %w", definition.Name, err)
		}
		if definition.PerPodAggregation != AggregationSum {
			return nil, fmt.Errorf("metric %q has unsupported per-pod aggregation %q", definition.Name, definition.PerPodAggregation)
		}
		if definition.Filter != FilterNone && definition.Filter != FilterConnectionAggregate {
			return nil, fmt.Errorf("metric %q has unsupported filter %q", definition.Name, definition.Filter)
		}
		validated[i] = definition
	}

	return &Catalog{definitions: validated}, nil
}

func DefaultCatalog() *Catalog {
	catalog, err := NewCatalog([]Definition{
		{
			Name:              InboundActiveRequests,
			SourcePattern:     "http.inbound_*.downstream_rq_active",
			PerPodAggregation: AggregationSum,
			Filter:            FilterNone,
		},
		{
			Name:              OutboundActiveRequests,
			SourcePattern:     "http.outbound_*.downstream_rq_active",
			PerPodAggregation: AggregationSum,
			Filter:            FilterNone,
		},
		{
			Name:              ActiveConnections,
			SourcePattern:     "listener.*.downstream_cx_active",
			PerPodAggregation: AggregationSum,
			Filter:            FilterConnectionAggregate,
		},
	})
	if err != nil {
		panic(err)
	}
	return catalog
}

func (c *Catalog) Names() []string {
	names := make([]string, 0, len(c.definitions))
	for _, definition := range c.definitions {
		names = append(names, definition.Name)
	}
	return names
}

func (c *Catalog) MatchMetricName(envoyMetricName string) (string, bool) {
	for _, definition := range c.definitions {
		matches, err := path.Match(definition.SourcePattern, envoyMetricName)
		if err != nil || !matches {
			continue
		}
		if definition.Filter == FilterConnectionAggregate && !isServiceActiveConnectionMetric(envoyMetricName) {
			continue
		}
		return definition.Name, true
	}
	return "", false
}

var defaultCatalog = DefaultCatalog()

func Names() []string {
	return defaultCatalog.Names()
}

func MatchMetricName(envoyMetricName string) (string, bool) {
	return defaultCatalog.MatchMetricName(envoyMetricName)
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

package podmetrics

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestLoadCatalogFromKubernetes(t *testing.T) {
	client := newMetricFakeClient(&unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "activescale.anyflow.io/v1alpha1",
			"kind":       "ActivescaleMetric",
			"metadata": map[string]interface{}{
				"name": "backend-upstream-connections",
			},
			"spec": map[string]interface{}{
				"metricName":         "backend_upstream_connections",
				"envoyMetricPattern": "cluster.*.upstream_cx_active",
				"perPodAggregation":  "sum",
				"filter":             "none",
			},
		},
	})

	catalog, loadedFromCR, err := LoadCatalogFromKubernetes(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	if !loadedFromCR {
		t.Fatal("expected catalog to be loaded from a custom resource")
	}
	if got := catalog.Names(); len(got) != 1 || got[0] != "backend_upstream_connections" {
		t.Fatalf("catalog names = %v, want [backend_upstream_connections]", got)
	}
}

func TestLoadCatalogFromKubernetesUsesConfiguredMetricName(t *testing.T) {
	client := newMetricFakeClient(&unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "activescale.anyflow.io/v1alpha1",
			"kind":       "ActivescaleMetric",
			"metadata": map[string]interface{}{
				"name": "inbound-active-requests",
			},
			"spec": map[string]interface{}{
				"metricName":         "inbound_active_requests",
				"envoyMetricPattern": "http.inbound_*.downstream_rq_active",
				"perPodAggregation":  "sum",
				"filter":             "none",
			},
		},
	})

	catalog, _, err := LoadCatalogFromKubernetes(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := catalog.MatchMetricName("http.inbound_0.0.0.0_8080;.downstream_rq_active"); !ok || got != "inbound_active_requests" {
		t.Fatalf("MatchMetricName() = (%q, %t), want (inbound_active_requests, true)", got, ok)
	}
}

func TestLoadCatalogFromKubernetesRequiresMetricName(t *testing.T) {
	client := newMetricFakeClient(&unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "activescale.anyflow.io/v1alpha1",
			"kind":       "ActivescaleMetric",
			"metadata": map[string]interface{}{
				"name": "backend-upstream-connections",
			},
			"spec": map[string]interface{}{
				"envoyMetricPattern": "cluster.*.upstream_cx_active",
				"perPodAggregation":  "sum",
				"filter":             "none",
			},
		},
	})

	if _, _, err := LoadCatalogFromKubernetes(context.Background(), client); err == nil {
		t.Fatal("expected missing spec.metricName to fail")
	}
}

func TestLoadCatalogFromKubernetesFallsBackWhenEmpty(t *testing.T) {
	client := newMetricFakeClient()

	catalog, loadedFromCR, err := LoadCatalogFromKubernetes(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	if loadedFromCR {
		t.Fatal("empty custom resource list should use built-in catalog")
	}
	if got := catalog.Names(); len(got) != 3 {
		t.Fatalf("fallback catalog names = %v, want three built-in metrics", got)
	}
}

func newMetricFakeClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{MetricResource: "ActivescaleMetricList"},
		objects...,
	)
}

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/emicklei/go-restful/v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	genericapi "k8s.io/apiserver/pkg/endpoints"
	genericfilters "k8s.io/apiserver/pkg/endpoints/filters"
	genericserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/kubernetes/fake"
	metricsv1beta2 "k8s.io/metrics/pkg/apis/custom_metrics/v1beta2"
	"sigs.k8s.io/custom-metrics-apiserver/pkg/apiserver"
	"sigs.k8s.io/custom-metrics-apiserver/pkg/apiserver/installer"
	cmprovider "sigs.k8s.io/custom-metrics-apiserver/pkg/provider"
	metricstorage "sigs.k8s.io/custom-metrics-apiserver/pkg/registry/custom_metrics"

	"activescale/internal/podmetrics"
)

type fakeMetricStore map[string]float64

func (s fakeMetricStore) GetGauge(_ context.Context, _, pod, _ string) (float64, bool, error) {
	if pod == "storage-error" {
		return 0, false, errors.New("Redis is unavailable")
	}
	value, found := s[pod]
	return value, found, nil
}

func TestCustomMetricsHTTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		path   string
		status int
		values map[string]int64
	}{
		{"partial selector", "/namespaces/test/pods/*/" + podmetrics.InboundActiveRequests + "?labelSelector=app%3Dmixed", http.StatusOK, map[string]int64{"idle": 0, "busy": 7}},
		{"no samples for selected Pod", "/namespaces/test/pods/*/" + podmetrics.InboundActiveRequests + "?labelSelector=app%3Dmissing", http.StatusOK, map[string]int64{}},
		{"no selected Pods", "/namespaces/test/pods/*/" + podmetrics.InboundActiveRequests + "?labelSelector=app%3Dunmatched", http.StatusOK, map[string]int64{}},
		{"named zero", "/namespaces/test/pods/idle/" + podmetrics.InboundActiveRequests, http.StatusOK, map[string]int64{"idle": 0}},
		{"named missing", "/namespaces/test/pods/uncollected/" + podmetrics.InboundActiveRequests, http.StatusNotFound, nil},
		{"unsupported selector metric", "/namespaces/test/pods/*/unknown", http.StatusNotFound, nil},
		{"unsupported named metric", "/namespaces/test/pods/idle/unknown", http.StatusNotFound, nil},
		{"unsupported selector resource", "/namespaces/test/services/*/" + podmetrics.InboundActiveRequests, http.StatusNotFound, nil},
		{"unsupported named resource", "/namespaces/test/services/idle/" + podmetrics.InboundActiveRequests, http.StatusNotFound, nil},
		{"non-core Pod resource", "/namespaces/test/pods.example.org/idle/" + podmetrics.InboundActiveRequests, http.StatusNotFound, nil},
		{"selector storage error", "/namespaces/test/pods/*/" + podmetrics.InboundActiveRequests + "?labelSelector=app%3Dbroken", http.StatusInternalServerError, nil},
		{"named storage error", "/namespaces/test/pods/storage-error/" + podmetrics.InboundActiveRequests, http.StatusInternalServerError, nil},
	}

	kube := fake.NewSimpleClientset(
		testPod("idle", "mixed"),
		testPod("busy", "mixed"),
		testPod("expired", "mixed"),
		testPod("uncollected", "missing"),
		testPod("storage-error", "broken"),
	)
	provider := NewPodsProvider(kube, fakeMetricStore{"idle": 0, "busy": 7}, time.Hour)
	handler := customMetricsHandler(t, provider)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/apis/custom.metrics.k8s.io/v1beta2"+test.path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.status, response.Body.String())
			}
			if test.status != http.StatusOK {
				return
			}

			var list metricsv1beta2.MetricValueList
			if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil {
				t.Fatal(err)
			}
			if list.Items == nil {
				t.Fatalf("expected an items array: %s", response.Body.String())
			}
			values := make(map[string]int64, len(list.Items))
			for _, item := range list.Items {
				values[item.DescribedObject.Name] = item.Value.Value()
			}
			if !reflect.DeepEqual(values, test.values) || len(list.Items) != len(test.values) {
				t.Errorf("values = %v, want %v: %s", values, test.values, response.Body.String())
			}
		})
	}
}

func testPod(name, app string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace: "test",
		Name:      name,
		Labels:    map[string]string{"app": app},
	}}
}

func customMetricsHandler(t *testing.T, provider *PodsProvider) http.Handler {
	t.Helper()

	scheme := apiserver.Scheme
	group := installer.MetricsAPIGroupVersion{
		DynamicStorage: metricstorage.NewREST(provider),
		APIGroupVersion: &genericapi.APIGroupVersion{
			Root:            genericserver.APIGroupPrefix,
			GroupVersion:    metricsv1beta2.SchemeGroupVersion,
			ParameterCodec:  runtime.NewParameterCodec(scheme),
			Serializer:      apiserver.Codecs,
			Creater:         scheme,
			Convertor:       scheme,
			UnsafeConvertor: runtime.UnsafeObjectConvertor(scheme),
			Typer:           scheme,
			Namer:           runtime.Namer(meta.NewAccessor()),
		},
		ResourceLister: cmprovider.NewCustomMetricResourceLister(provider),
		Handlers:       &installer.CMHandlers{},
	}
	container := restful.NewContainer()
	container.Router(restful.CurlyRouter{})
	if err := group.InstallREST(container); err != nil {
		t.Fatal(err)
	}
	resolver := genericserver.NewRequestInfoResolver(&genericserver.Config{})
	return genericfilters.WithRequestInfo(container.ServeMux, resolver)
}

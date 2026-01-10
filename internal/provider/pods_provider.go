// internal/provider/pods_provider.go
package provider

import (
	"context"
	"fmt"
	"time"

	redisstore "activescale/internal/redis"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	custommetrics "k8s.io/metrics/pkg/apis/custom_metrics"
	cmprovider "sigs.k8s.io/custom-metrics-apiserver/pkg/provider"
)

type PodsProvider struct {
	kube  kubernetes.Interface
	store *redisstore.Store
}

func NewPodsProvider(kube kubernetes.Interface, store *redisstore.Store) *PodsProvider {
	return &PodsProvider{kube: kube, store: store}
}

func (p *PodsProvider) GetMetricBySelector(
	ctx context.Context,
	namespace string,
	selector labels.Selector,
	info cmprovider.CustomMetricInfo,
	metricSelector labels.Selector,
) (*custommetrics.MetricValueList, error) {
	_ = metricSelector

	// Pod 목록 조회
	pods, err := p.kube.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}

	now := metav1.NewTime(time.Now())
	out := &custommetrics.MetricValueList{
		Items: make([]custommetrics.MetricValue, 0, len(pods.Items)),
	}

	for _, pod := range pods.Items {
		val, ok, err := p.store.GetGauge(ctx, namespace, pod.Name, "active_requests")
		if err != nil {
			return nil, err
		}
		if !ok {
			// Skip pods with no value (including TTL-expired entries).
			continue
		}

		mv := custommetrics.MetricValue{
			DescribedObject: custommetrics.ObjectReference{
				APIVersion: "v1",
				Kind:       "Pod",
				Namespace:  namespace,
				Name:       pod.Name,
			},
			Metric: custommetrics.MetricIdentifier{
				Name: info.Metric,
			},
			Timestamp: now,
			Value:     *resource.NewQuantity(int64(val), resource.DecimalSI), // rq_active는 보통 정수 성격
		}
		out.Items = append(out.Items, mv)
	}

	if len(out.Items) == 0 {
		// Returning an error makes the API respond with 5xx, so HPA treats metrics as unavailable.
		return nil, fmt.Errorf("no metrics available for selector")
	}

	return out, nil
}

// GetMetricByName는 /pods/<pod>/metric 요청용. * 요청만 쓸 거면 없어도 되지만 구현 권장.
func (p *PodsProvider) GetMetricByName(
	ctx context.Context,
	name types.NamespacedName,
	info cmprovider.CustomMetricInfo,
	metricSelector labels.Selector,
) (*custommetrics.MetricValue, error) {
	_ = metricSelector
	if name.Namespace == "" {
		return nil, fmt.Errorf("namespace is required for pod metrics")
	}

	val, ok, err := p.store.GetGauge(ctx, name.Namespace, name.Name, "active_requests")
	if err != nil {
		return nil, err
	}
	if !ok {
		// Returning an error makes the API respond with 5xx, so HPA treats metrics as unavailable.
		return nil, fmt.Errorf("no metrics available for pod")
	}

	now := metav1.NewTime(time.Now())
	mv := &custommetrics.MetricValue{
		DescribedObject: custommetrics.ObjectReference{
			APIVersion: "v1",
			Kind:       "Pod",
			Namespace:  name.Namespace,
			Name:       name.Name,
		},
		Metric:    custommetrics.MetricIdentifier{Name: info.Metric},
		Timestamp: now,
		Value:     *resource.NewQuantity(int64(val), resource.DecimalSI),
	}
	return mv, nil
}

// Framework가 원하는 “지원 metric 선언”
func (p *PodsProvider) ListAllMetrics() []cmprovider.CustomMetricInfo {
	return []cmprovider.CustomMetricInfo{
		{
			GroupResource: schema.GroupResource{Group: "", Resource: "pods"},
			Metric:        "active_requests",
			Namespaced:    true,
		},
	}
}

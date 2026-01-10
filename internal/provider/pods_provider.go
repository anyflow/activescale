// internal/provider/pods_provider.go
package provider

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	redisstore "activescale/internal/redis"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	custommetrics "k8s.io/metrics/pkg/apis/custom_metrics"
	cmprovider "sigs.k8s.io/custom-metrics-apiserver/pkg/provider"
)

type PodsProvider struct {
	kube  kubernetes.Interface
	store *redisstore.Store

	logOnce     sync.Once
	logEvery    time.Duration
	queryCount  uint64
	resultCount uint64
}

func NewPodsProvider(kube kubernetes.Interface, store *redisstore.Store, logEvery time.Duration) *PodsProvider {
	return &PodsProvider{
		kube:     kube,
		store:    store,
		logEvery: logEvery,
	}
}

func (p *PodsProvider) GetMetricBySelector(
	ctx context.Context,
	namespace string,
	selector labels.Selector,
	info cmprovider.CustomMetricInfo,
	metricSelector labels.Selector,
) (*custommetrics.MetricValueList, error) {
	_ = metricSelector
	p.logOnce.Do(func() {
		go p.logSummary()
	})
	atomic.AddUint64(&p.queryCount, 1)

	// Pod 목록 조회
	klog.V(2).Infof("custom metrics query namespace=%s metric=%s selector=%s", namespace, info.Metric, selector.String())
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
		klog.V(2).Infof("custom metrics result empty namespace=%s metric=%s selector=%s", namespace, info.Metric, selector.String())
		return nil, fmt.Errorf("no metrics available for selector")
	}

	atomic.AddUint64(&p.resultCount, uint64(len(out.Items)))
	klog.V(2).Infof("custom metrics result count=%d namespace=%s metric=%s selector=%s", len(out.Items), namespace, info.Metric, selector.String())
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

	p.logOnce.Do(func() {
		go p.logSummary()
	})
	atomic.AddUint64(&p.queryCount, 1)
	klog.V(2).Infof("custom metrics query namespace=%s pod=%s metric=%s", name.Namespace, name.Name, info.Metric)
	val, ok, err := p.store.GetGauge(ctx, name.Namespace, name.Name, "active_requests")
	if err != nil {
		return nil, err
	}
	if !ok {
		// Returning an error makes the API respond with 5xx, so HPA treats metrics as unavailable.
		klog.V(2).Infof("custom metrics result empty namespace=%s pod=%s metric=%s", name.Namespace, name.Name, info.Metric)
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
	klog.V(2).Infof("custom metrics result namespace=%s pod=%s metric=%s", name.Namespace, name.Name, info.Metric)
	atomic.AddUint64(&p.resultCount, 1)
	return mv, nil
}

func (p *PodsProvider) logSummary() {
	ticker := time.NewTicker(p.logEvery)
	defer ticker.Stop()

	for range ticker.C {
		queries := atomic.SwapUint64(&p.queryCount, 0)
		results := atomic.SwapUint64(&p.resultCount, 0)
		klog.Infof("custom metrics queries in last %s: %d, returned items: %d", p.logEvery, queries, results)
	}
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

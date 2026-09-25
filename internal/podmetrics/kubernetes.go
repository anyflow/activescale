package podmetrics

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var MetricResource = schema.GroupVersionResource{
	Group:    "activescale.anyflow.io",
	Version:  "v1alpha1",
	Resource: "activescalemetrics",
}

// LoadCatalogFromKubernetes loads configured metrics when the CRD is installed.
// Existing deployments without the CRD keep the built-in metric contract.
func LoadCatalogFromKubernetes(ctx context.Context, client dynamic.Interface) (*Catalog, bool, error) {
	list, err := client.Resource(MetricResource).List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) || apierrors.IsForbidden(err) || apierrors.IsMethodNotSupported(err) {
			return DefaultCatalog(), false, nil
		}
		return nil, false, err
	}
	if len(list.Items) == 0 {
		return DefaultCatalog(), false, nil
	}

	definitions := make([]Definition, 0, len(list.Items))
	for _, item := range list.Items {
		definition, err := definitionFromResource(item)
		if err != nil {
			return nil, false, err
		}
		definitions = append(definitions, definition)
	}

	catalog, err := NewCatalog(definitions)
	if err != nil {
		return nil, false, err
	}
	return catalog, true, nil
}

func definitionFromResource(item unstructured.Unstructured) (Definition, error) {
	resourceName := item.GetName()
	metricName, found, err := unstructured.NestedString(item.Object, "spec", "metricName")
	if err != nil || !found || metricName == "" {
		if err == nil {
			err = fmt.Errorf("is required")
		}
		return Definition{}, fmt.Errorf("metric %q has invalid spec.metricName: %w", resourceName, err)
	}

	sourcePattern, found, err := unstructured.NestedString(item.Object, "spec", "envoyMetricPattern")
	if err != nil || !found || sourcePattern == "" {
		if err == nil {
			err = fmt.Errorf("is required")
		}
		return Definition{}, fmt.Errorf("metric %q has invalid spec.envoyMetricPattern: %w", metricName, err)
	}

	perPodAggregation, found, err := unstructured.NestedString(item.Object, "spec", "perPodAggregation")
	if err != nil || !found || perPodAggregation == "" {
		if err == nil {
			err = fmt.Errorf("is required")
		}
		return Definition{}, fmt.Errorf("metric %q has invalid spec.perPodAggregation: %w", metricName, err)
	}

	filter, found, err := unstructured.NestedString(item.Object, "spec", "filter")
	if err != nil || !found || filter == "" {
		if err == nil {
			err = fmt.Errorf("is required")
		}
		return Definition{}, fmt.Errorf("metric %q has invalid spec.filter: %w", metricName, err)
	}

	return Definition{
		Name:              metricName,
		SourcePattern:     sourcePattern,
		PerPodAggregation: perPodAggregation,
		Filter:            filter,
	}, nil
}

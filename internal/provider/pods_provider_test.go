package provider

import (
	"context"
	"testing"
)

type fakeMetricStore struct {
	fresh    bool
	value    float64
	hasValue bool
}

func (s *fakeMetricStore) GetGauge(context.Context, string, string, string) (float64, bool, error) {
	return s.value, s.hasValue, nil
}

func (s *fakeMetricStore) IsPodFresh(context.Context, string, string) (bool, error) {
	return s.fresh, nil
}

func TestMetricForPod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		store     fakeMetricStore
		wantValue float64
		wantFound bool
		wantZeros uint64
	}{
		{name: "fresh value", store: fakeMetricStore{fresh: true, value: 7, hasValue: true}, wantValue: 7, wantFound: true},
		{name: "fresh missing value", store: fakeMetricStore{fresh: true}, wantFound: true, wantZeros: 1},
		{name: "stale pod", store: fakeMetricStore{value: 7, hasValue: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &PodsProvider{store: &tt.store}
			value, found, err := provider.metricForPod(context.Background(), "ns", "pod", "metric")
			if err != nil {
				t.Fatal(err)
			}
			if value != tt.wantValue || found != tt.wantFound || provider.zeroCount != tt.wantZeros {
				t.Fatalf("metricForPod() = (%v, %v, zeros=%d), want (%v, %v, zeros=%d)", value, found, provider.zeroCount, tt.wantValue, tt.wantFound, tt.wantZeros)
			}
		})
	}
}

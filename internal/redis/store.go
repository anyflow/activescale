// internal/redis/store.go
package redisstore

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Store struct {
	rdb     *redis.Client
	ttl     time.Duration
	context string
}

func New(rdb *redis.Client, ttl time.Duration, context string) *Store {
	return &Store{rdb: rdb, ttl: ttl, context: context}
}

func (s *Store) key(ns, pod, metric string) string {
	return fmt.Sprintf("%s:%s:%s:%s", s.context, ns, pod, metric)
}

// SetGauge stores a single gauge value with TTL.
func (s *Store) SetGauge(ctx context.Context, ns, pod, metric string, val float64) error {
	return s.rdb.Set(ctx, s.key(ns, pod, metric), fmt.Sprintf("%.6f", val), s.ttl).Err()
}

// GetGauge returns (value, ok, err). ok=false means missing/expired.
func (s *Store) GetGauge(ctx context.Context, ns, pod, metric string) (float64, bool, error) {
	v, err := s.rdb.Get(ctx, s.key(ns, pod, metric)).Result()
	if err == redis.Nil {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	var f float64
	_, scanErr := fmt.Sscanf(v, "%f", &f)
	if scanErr != nil {
		return 0, false, scanErr
	}
	return f, true, nil
}

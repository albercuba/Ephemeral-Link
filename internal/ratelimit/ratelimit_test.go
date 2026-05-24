package ratelimit

import (
	"testing"
	"time"
)

func TestCleanupExpiredBuckets(t *testing.T) {
	limiter := New(60)
	limiter.buckets["expired"] = bucket{count: 1, reset: time.Now().Add(-time.Minute)}
	limiter.buckets["active"] = bucket{count: 1, reset: time.Now().Add(time.Minute)}
	limiter.lastCleanup = time.Now().Add(-2 * time.Minute)

	limiter.mu.Lock()
	limiter.cleanupExpiredLocked(time.Now())
	limiter.mu.Unlock()

	if _, ok := limiter.buckets["expired"]; ok {
		t.Fatal("expired bucket was not evicted")
	}
	if _, ok := limiter.buckets["active"]; !ok {
		t.Fatal("active bucket was evicted")
	}
}

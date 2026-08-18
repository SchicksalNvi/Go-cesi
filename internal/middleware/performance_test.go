package middleware

import (
	"testing"
	"time"
)

func TestResetPerformanceMetricsDoesNotDeadlock(t *testing.T) {
	recordMetrics("GET /before-reset", 25*time.Millisecond, 500)
	startedAt := time.Now()

	done := make(chan struct{})
	go func() {
		ResetPerformanceMetrics()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("ResetPerformanceMetrics deadlocked")
	}

	metrics := GetPerformanceMetrics()
	if metrics.RequestCount != 0 || metrics.ErrorCount != 0 {
		t.Fatalf("expected counters to be reset, got requests=%d errors=%d", metrics.RequestCount, metrics.ErrorCount)
	}
	if len(metrics.EndpointMetrics) != 0 || len(metrics.StatusCodeCounts) != 0 {
		t.Fatalf("expected metric maps to be reset, got endpoints=%d statuses=%d", len(metrics.EndpointMetrics), len(metrics.StatusCodeCounts))
	}
	if metrics.LastResetTime.Before(startedAt) {
		t.Fatalf("expected last reset time after %v, got %v", startedAt, metrics.LastResetTime)
	}
	if metrics.MemoryMetrics == nil || metrics.MemoryMetrics.LastUpdated.Before(startedAt) {
		t.Fatal("expected reset to publish a fresh memory metrics snapshot")
	}

	recordMetrics("GET /after-reset", 10*time.Millisecond, 200)
	metrics = GetPerformanceMetrics()
	if metrics.RequestCount != 1 {
		t.Fatalf("expected metrics recording to continue after reset, got %d requests", metrics.RequestCount)
	}
}

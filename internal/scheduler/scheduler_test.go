package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"
)

type mockBackend struct {
	mu              sync.Mutex
	promotedCount   int
	retriesCount    int
	stalledCount    int
	cronCount       int
	purgedCount     int
}

func (m *mockBackend) PromoteScheduled(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.promotedCount++
	return nil
}

func (m *mockBackend) PromoteRetries(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retriesCount++
	return nil
}

func (m *mockBackend) RequeueStalled(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stalledCount++
	return nil
}

func (m *mockBackend) FireCronJobs(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cronCount++
	return nil
}

func (m *mockBackend) PurgeExpired(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purgedCount++
	return nil
}

func TestSchedulerStartStop(t *testing.T) {
	mock := &mockBackend{}
	sched := New(mock)

	sched.Start()
	time.Sleep(600 * time.Millisecond)
	sched.Stop()

	mock.mu.Lock()
	defer mock.mu.Unlock()

	// Retry promoter runs every 200ms, so ~3 invocations in 600ms
	if mock.retriesCount < 1 {
		t.Error("expected PromoteRetries to be called at least once")
	}

	// Stalled reaper runs every 500ms, so ~1 invocation
	if mock.stalledCount < 1 {
		t.Error("expected RequeueStalled to be called at least once")
	}
}

func TestSchedulerDoubleStop(t *testing.T) {
	mock := &mockBackend{}
	sched := New(mock)

	sched.Start()
	time.Sleep(100 * time.Millisecond)

	// Should not panic
	sched.Stop()
	sched.Stop()
}

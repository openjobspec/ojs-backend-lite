package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/openjobspec/ojs-backend-lite/internal/core"
)

func BenchmarkPush(b *testing.B) {
	backend, _ := New()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		job := &core.Job{
			Type:  "bench",
			Queue: "default",
			Args:  json.RawMessage(`{"i":` + fmt.Sprint(i) + `}`),
		}
		_, _ = backend.Push(ctx, job)
	}
}

func BenchmarkFetch(b *testing.B) {
	backend, _ := New()
	ctx := context.Background()

	// Pre-populate with enough jobs
	for i := 0; i < b.N; i++ {
		job := &core.Job{
			Type:  "bench",
			Queue: "default",
			Args:  json.RawMessage(`{}`),
		}
		backend.Push(ctx, job)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = backend.Fetch(ctx, []string{"default"}, 1, fmt.Sprintf("w%d", i), 30000)
	}
}

func BenchmarkAck(b *testing.B) {
	backend, _ := New()
	ctx := context.Background()

	// Pre-populate and fetch jobs
	jobIDs := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		job := &core.Job{
			Type:  "bench",
			Queue: "default",
			Args:  json.RawMessage(`{}`),
		}
		backend.Push(ctx, job)
	}
	for i := 0; i < b.N; i++ {
		fetched, _ := backend.Fetch(ctx, []string{"default"}, 1, "worker-bench", 30000)
		if len(fetched) > 0 {
			jobIDs[i] = fetched[0].ID
		}
	}

	result := json.RawMessage(`{"status":"ok"}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if jobIDs[i] != "" {
			_, _ = backend.Ack(ctx, jobIDs[i], result)
		}
	}
}

func BenchmarkNack(b *testing.B) {
	backend, _ := New()
	ctx := context.Background()

	maxAttempts := 100
	jobIDs := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		job := &core.Job{
			Type:        "bench",
			Queue:       "default",
			Args:        json.RawMessage(`{}`),
			MaxAttempts: &maxAttempts,
		}
		backend.Push(ctx, job)
	}
	for i := 0; i < b.N; i++ {
		fetched, _ := backend.Fetch(ctx, []string{"default"}, 1, "worker-bench", 30000)
		if len(fetched) > 0 {
			jobIDs[i] = fetched[0].ID
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if jobIDs[i] != "" {
			_, _ = backend.Nack(ctx, jobIDs[i], &core.JobError{Message: "error"}, true)
		}
	}
}

func BenchmarkPushParallel(b *testing.B) {
	backend, _ := New()
	ctx := context.Background()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			job := &core.Job{
				Type:  "bench",
				Queue: "default",
				Args:  json.RawMessage(`{}`),
			}
			_, _ = backend.Push(ctx, job)
			i++
		}
	})
}

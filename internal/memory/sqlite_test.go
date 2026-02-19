package memory

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/openjobspec/ojs-backend-lite/internal/core"
)

func TestSQLitePersistence(t *testing.T) {
	tmpFile := "/tmp/ojs-test-" + core.NewUUIDv7() + ".db"
	defer os.Remove(tmpFile)

	ctx := context.Background()

	backend1, _ := New(WithPersist(tmpFile))

	job := &core.Job{
		Type:  "test-job",
		Queue: "default",
	}
	pushed, err := backend1.Push(ctx, job)
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	backend1.Close()

	backend2, _ := New(WithPersist(tmpFile))
	defer backend2.Close()

	retrieved, err := backend2.Info(ctx, pushed.ID)
	if err != nil {
		t.Fatalf("Info failed: %v", err)
	}

	if retrieved.Type != "test-job" {
		t.Errorf("Expected type 'test-job', got '%s'", retrieved.Type)
	}
}

func TestSQLiteCronPersistence(t *testing.T) {
	tmpFile := "/tmp/ojs-test-" + core.NewUUIDv7() + ".db"
	defer os.Remove(tmpFile)

	ctx := context.Background()

	backend1, _ := New(WithPersist(tmpFile))

	cronJob := &core.CronJob{
		Name:       "test-cron",
		Expression: "* * * * *",
		JobTemplate: &core.CronJobTemplate{
			Type: "cron-job",
		},
	}
	_, err := backend1.RegisterCron(ctx, cronJob)
	if err != nil {
		t.Fatalf("RegisterCron failed: %v", err)
	}

	backend1.Close()

	backend2, _ := New(WithPersist(tmpFile))
	defer backend2.Close()

	crons, err := backend2.ListCron(ctx)
	if err != nil {
		t.Fatalf("ListCron failed: %v", err)
	}

	if len(crons) != 1 {
		t.Fatalf("Expected 1 cron, got %d", len(crons))
	}

	if crons[0].Name != "test-cron" {
		t.Errorf("Expected cron name 'test-cron', got '%s'", crons[0].Name)
	}
}

func TestSQLiteQueueStatePersistence(t *testing.T) {
	tmpFile := "/tmp/ojs-test-" + core.NewUUIDv7() + ".db"
	defer os.Remove(tmpFile)

	ctx := context.Background()

	backend1, _ := New(WithPersist(tmpFile))

	job := &core.Job{
		Type:  "test",
		Queue: "paused-queue",
	}
	backend1.Push(ctx, job)

	if err := backend1.PauseQueue(ctx, "paused-queue"); err != nil {
		t.Fatalf("PauseQueue failed: %v", err)
	}

	backend1.Close()

	backend2, _ := New(WithPersist(tmpFile))
	defer backend2.Close()

	stats, err := backend2.QueueStats(ctx, "paused-queue")
	if err != nil {
		t.Fatalf("QueueStats failed: %v", err)
	}

	if stats.Status != "paused" {
		t.Errorf("Expected queue status 'paused', got '%s'", stats.Status)
	}
}

func TestSQLiteRetentionWithPersistence(t *testing.T) {
	tmpFile := "/tmp/ojs-test-" + core.NewUUIDv7() + ".db"
	defer os.Remove(tmpFile)

	ctx := context.Background()

	backend, _ := New(
		WithPersist(tmpFile),
		WithRetention(100*time.Millisecond, 100*time.Millisecond, 100*time.Millisecond),
	)
	defer backend.Close()

	job := &core.Job{
		Type:  "test",
		Queue: "default",
	}
	pushed, err := backend.Push(ctx, job)
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	jobs, err := backend.Fetch(ctx, []string{"default"}, 1, "worker-1", 30000)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("Fetch failed")
	}

	_, err = backend.Ack(ctx, pushed.ID, nil)
	if err != nil {
		t.Fatalf("Ack failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if err := backend.PurgeExpired(ctx); err != nil {
		t.Fatalf("PurgeExpired failed: %v", err)
	}

	_, err = backend.Info(ctx, pushed.ID)
	if err == nil {
		t.Errorf("Expected job to be purged")
	}
}

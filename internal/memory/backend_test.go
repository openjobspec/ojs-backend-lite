package memory

import (
"context"
"encoding/json"
"testing"

"github.com/openjobspec/ojs-backend-lite/internal/core"
)

func TestMemoryBackend_PushAndInfo(t *testing.T) {
b, _ := New()
ctx := context.Background()

job := &core.Job{
Type:  "test",
Queue: "default",
Args:  json.RawMessage(`{"foo":"bar"}`),
}

pushed, err := b.Push(ctx, job)
if err != nil {
t.Fatalf("Push failed: %v", err)
}

if pushed.ID == "" {
t.Fatal("Job ID should be generated")
}

if pushed.State != core.StateAvailable {
t.Errorf("Expected state %s, got %s", core.StateAvailable, pushed.State)
}

retrieved, err := b.Info(ctx, pushed.ID)
if err != nil {
t.Fatalf("Info failed: %v", err)
}

if retrieved.ID != pushed.ID {
t.Errorf("Expected ID %s, got %s", pushed.ID, retrieved.ID)
}
}

func TestMemoryBackend_FetchAndAck(t *testing.T) {
b, _ := New()
ctx := context.Background()

job := &core.Job{
Type:  "test",
Queue: "default",
Args:  json.RawMessage(`{"foo":"bar"}`),
}

pushed, err := b.Push(ctx, job)
if err != nil {
t.Fatalf("Push failed: %v", err)
}

fetched, err := b.Fetch(ctx, []string{"default"}, 1, "worker1", 30000)
if err != nil {
t.Fatalf("Fetch failed: %v", err)
}

if len(fetched) != 1 {
t.Fatalf("Expected 1 job, got %d", len(fetched))
}

if fetched[0].ID != pushed.ID {
t.Errorf("Expected job ID %s, got %s", pushed.ID, fetched[0].ID)
}

if fetched[0].State != core.StateActive {
t.Errorf("Expected state %s, got %s", core.StateActive, fetched[0].State)
}

result := json.RawMessage(`{"result":"success"}`)
ackResp, err := b.Ack(ctx, fetched[0].ID, result)
if err != nil {
t.Fatalf("Ack failed: %v", err)
}

if !ackResp.Acknowledged {
t.Error("Job should be acknowledged")
}

if ackResp.State != core.StateCompleted {
t.Errorf("Expected state %s, got %s", core.StateCompleted, ackResp.State)
}
}

func TestMemoryBackend_Health(t *testing.T) {
b, _ := New()
ctx := context.Background()

health, err := b.Health(ctx)
if err != nil {
t.Fatalf("Health failed: %v", err)
}

if health.Status != "ok" {
t.Errorf("Expected status ok, got %s", health.Status)
}

if health.Backend.Type != "memory" {
t.Errorf("Expected backend type memory, got %s", health.Backend.Type)
}
}

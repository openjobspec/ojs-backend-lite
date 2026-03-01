package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/openjobspec/ojs-backend-lite/internal/core"
)

type checkpointEntry struct {
	JobID     string
	State     json.RawMessage
	Sequence  int
	CreatedAt time.Time
}

type checkpointStore struct {
	mu     sync.RWMutex
	stores map[string]*checkpointEntry
}

func newCheckpointStore() *checkpointStore {
	return &checkpointStore{stores: make(map[string]*checkpointEntry)}
}

// SaveCheckpoint persists a checkpoint for a running job in memory.
func (b *MemoryBackend) SaveCheckpoint(ctx context.Context, jobID string, state json.RawMessage) error {
	b.checkpoints.mu.Lock()
	defer b.checkpoints.mu.Unlock()

	existing, ok := b.checkpoints.stores[jobID]
	seq := 1
	if ok {
		seq = existing.Sequence + 1
	}

	b.checkpoints.stores[jobID] = &checkpointEntry{
		JobID:     jobID,
		State:     state,
		Sequence:  seq,
		CreatedAt: time.Now(),
	}
	return nil
}

// GetCheckpoint retrieves the last checkpoint for a job.
func (b *MemoryBackend) GetCheckpoint(ctx context.Context, jobID string) (*core.Checkpoint, error) {
	b.checkpoints.mu.RLock()
	defer b.checkpoints.mu.RUnlock()

	entry, ok := b.checkpoints.stores[jobID]
	if !ok {
		return nil, fmt.Errorf("checkpoint for job '%s' not found", jobID)
	}

	return &core.Checkpoint{
		JobID:     entry.JobID,
		State:     entry.State,
		Sequence:  entry.Sequence,
		CreatedAt: entry.CreatedAt,
	}, nil
}

// DeleteCheckpoint removes a checkpoint for a job.
func (b *MemoryBackend) DeleteCheckpoint(ctx context.Context, jobID string) error {
	b.checkpoints.mu.Lock()
	defer b.checkpoints.mu.Unlock()
	delete(b.checkpoints.stores, jobID)
	return nil
}

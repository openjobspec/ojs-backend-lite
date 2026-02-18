package memory

import (
	"context"

	"github.com/openjobspec/ojs-backend-lite/internal/core"
)

func (b *MemoryBackend) ListQueues(ctx context.Context) ([]core.QueueInfo, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	queueMap := make(map[string]bool)
	for _, job := range b.jobs {
		queueMap[job.Queue] = true
	}

	queues := make([]core.QueueInfo, 0, len(queueMap))
	for name := range queueMap {
		status := "active"
		if b.queueState[name] == "paused" {
			status = "paused"
		}
		queues = append(queues, core.QueueInfo{
			Name:   name,
			Status: status,
		})
	}

	return queues, nil
}

func (b *MemoryBackend) QueueStats(ctx context.Context, name string) (*core.QueueStats, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	stats := core.Stats{}

	for _, job := range b.jobs {
		if job.Queue != name {
			continue
		}

		switch job.State {
		case core.StateAvailable:
			stats.Available++
		case core.StateActive:
			stats.Active++
		case core.StateCompleted:
			stats.Completed++
		case core.StateScheduled:
			stats.Scheduled++
		case core.StateRetryable:
			stats.Retryable++
		}
	}

	for _, job := range b.deadJobs {
		if job.Queue == name && job.State == core.StateDiscarded {
			stats.Dead++
		}
	}

	status := "active"
	if b.queueState[name] == "paused" {
		status = "paused"
	}

	return &core.QueueStats{
		Queue:  name,
		Status: status,
		Stats:  stats,
	}, nil
}

func (b *MemoryBackend) PauseQueue(ctx context.Context, name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.queueState[name] = "paused"

	if b.persist != nil {
		b.persist.SaveQueueState(name, "paused")
	}

	return nil
}

func (b *MemoryBackend) ResumeQueue(ctx context.Context, name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.queueState[name] = "active"

	if b.persist != nil {
		b.persist.SaveQueueState(name, "active")
	}

	return nil
}

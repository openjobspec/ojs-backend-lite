package memory

import (
	"context"
	"sort"
	"time"

	"github.com/openjobspec/ojs-backend-lite/internal/core"
)

func (b *MemoryBackend) ListDeadLetter(ctx context.Context, limit, offset int) ([]*core.Job, int, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	jobs := make([]*core.Job, 0, len(b.deadJobs))
	for _, job := range b.deadJobs {
		jobCopy := *job
		jobs = append(jobs, &jobCopy)
	}

	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CompletedAt > jobs[j].CompletedAt
	})

	total := len(jobs)

	if offset >= total {
		return []*core.Job{}, total, nil
	}

	end := offset + limit
	if end > total {
		end = total
	}

	return jobs[offset:end], total, nil
}

func (b *MemoryBackend) RetryDeadLetter(ctx context.Context, jobID string) (*core.Job, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	deadJob, exists := b.deadJobs[jobID]
	if !exists {
		return nil, core.NewNotFoundError("Dead letter job", jobID)
	}

	delete(b.deadJobs, jobID)

	deadJob.State = core.StateAvailable
	deadJob.Attempt = 0
	deadJob.Error = nil
	deadJob.Errors = nil
	deadJob.EnqueuedAt = core.FormatTime(time.Now())

	b.jobs[jobID] = deadJob
	b.addToQueueIndex(deadJob)

	if b.persist != nil {
		b.persist.DeleteDeadJob(jobID)
		b.persist.SaveJob(deadJob)
	}

	jobCopy := *deadJob
	return &jobCopy, nil
}

func (b *MemoryBackend) DeleteDeadLetter(ctx context.Context, jobID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.deadJobs[jobID]; !exists {
		return core.NewNotFoundError("Dead letter job", jobID)
	}

	delete(b.deadJobs, jobID)
	delete(b.jobs, jobID)

	if b.persist != nil {
		b.persist.DeleteDeadJob(jobID)
		b.persist.DeleteJob(jobID)
	}

	return nil
}

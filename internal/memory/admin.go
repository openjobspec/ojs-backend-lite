package memory

import (
	"context"
	"sort"

	"github.com/openjobspec/ojs-backend-lite/internal/core"
)

func (b *MemoryBackend) ListJobs(ctx context.Context, filters core.JobListFilters, limit, offset int) ([]*core.Job, int, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	jobs := make([]*core.Job, 0)
	for _, job := range b.jobs {
		if filters.State != "" && job.State != filters.State {
			continue
		}
		if filters.Queue != "" && job.Queue != filters.Queue {
			continue
		}
		if filters.Type != "" && job.Type != filters.Type {
			continue
		}
		if filters.WorkerID != "" && job.WorkerID != filters.WorkerID {
			continue
		}

		jobCopy := *job
		jobs = append(jobs, &jobCopy)
	}

	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt > jobs[j].CreatedAt
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

func (b *MemoryBackend) ListWorkers(ctx context.Context, limit, offset int) ([]*core.WorkerInfo, core.WorkerSummary, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	workers := make([]*core.WorkerInfo, 0, len(b.workers))
	for _, worker := range b.workers {
		info := &core.WorkerInfo{
			ID:            worker.ID,
			State:         worker.State,
			Directive:     worker.Directive,
			ActiveJobs:    len(worker.ActiveJobs),
			LastHeartbeat: worker.LastHeartbeat,
		}
		workers = append(workers, info)
	}

	sort.Slice(workers, func(i, j int) bool {
		return workers[i].ID < workers[j].ID
	})

	total := len(workers)
	summary := core.WorkerSummary{
		Total:   total,
		Running: 0,
		Quiet:   0,
		Stale:   0,
	}

	for _, worker := range workers {
		if worker.Directive == "quiet" {
			summary.Quiet++
		} else {
			summary.Running++
		}
	}

	if offset >= total {
		return []*core.WorkerInfo{}, summary, nil
	}

	end := offset + limit
	if end > total {
		end = total
	}

	return workers[offset:end], summary, nil
}

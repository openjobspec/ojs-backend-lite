package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/openjobspec/ojs-backend-lite/internal/core"
)

type persistStore interface {
	SaveJob(job *core.Job) error
	DeleteJob(id string) error
	SaveDeadJob(job *core.Job) error
	DeleteDeadJob(id string) error
	SaveCron(cron *core.CronJob) error
	DeleteCron(name string) error
	SaveWorkflow(id string, state *workflowState) error
	DeleteWorkflow(id string) error
	SaveUniqueKey(fingerprint, jobID string) error
	DeleteUniqueKey(fingerprint string) error
	SaveQueueState(name, status string) error
	LoadAll() (*persistedState, error)
	Close() error
}

type persistedState struct {
	Jobs       map[string]*core.Job
	DeadJobs   map[string]*core.Job
	Crons      map[string]*core.CronJob
	Workflows  map[string]*workflowState
	UniqueKeys map[string]string
	QueueState map[string]string
}

type queueIndex struct {
	available []*core.Job // sorted by (-priority, enqueued_at)
}

type workflowState struct {
	ID            string
	Name          string
	Type          string
	State         string
	Total         int
	Completed     int
	Failed        int
	CreatedAt     string
	CompletedAt   string
	JobDefs       []core.WorkflowJobRequest
	Callbacks     *core.WorkflowCallbacks
	JobIDs        []string
	Results       map[int][]byte
}

type workerState struct {
	ID            string
	State         string
	Directive     string
	ActiveJobs    []string
	LastHeartbeat string
}

type MemoryBackend struct {
	mu                 sync.RWMutex
	jobs               map[string]*core.Job
	queueState         map[string]string
	deadJobs           map[string]*core.Job
	crons              map[string]*core.CronJob
	workflows          map[string]*workflowState
	workers            map[string]*workerState
	uniqueKeys         map[string]string
	queueIdx           map[string]*queueIndex
	retentionCompleted time.Duration
	retentionCancelled time.Duration
	retentionDiscarded time.Duration
	persist            persistStore
	startTime          time.Time
	checkpoints        *checkpointStore
}

type Option func(*MemoryBackend) error

func WithRetention(completed, cancelled, discarded time.Duration) Option {
	return func(b *MemoryBackend) error {
		b.retentionCompleted = completed
		b.retentionCancelled = cancelled
		b.retentionDiscarded = discarded
		return nil
	}
}

func WithPersist(dbPath string) Option {
	return func(b *MemoryBackend) error {
		store, err := newSQLiteStore(dbPath)
		if err != nil {
			return fmt.Errorf("failed to open SQLite database at %q: %w", dbPath, err)
		}
		b.persist = store
		return nil
	}
}

func New(opts ...Option) (*MemoryBackend, error) {
	b := &MemoryBackend{
		jobs:               make(map[string]*core.Job),
		queueState:         make(map[string]string),
		deadJobs:           make(map[string]*core.Job),
		crons:              make(map[string]*core.CronJob),
		workflows:          make(map[string]*workflowState),
		workers:            make(map[string]*workerState),
		uniqueKeys:         make(map[string]string),
		queueIdx:           make(map[string]*queueIndex),
		retentionCompleted: 1 * time.Hour,
		retentionCancelled: 24 * time.Hour,
		retentionDiscarded: 24 * time.Hour,
		startTime:          time.Now(),
		checkpoints:        newCheckpointStore(),
	}

	for _, opt := range opts {
		if err := opt(b); err != nil {
			return nil, err
		}
	}

	if b.persist != nil {
		state, err := b.persist.LoadAll()
		if err == nil && state != nil {
			b.jobs = state.Jobs
			b.deadJobs = state.DeadJobs
			b.crons = state.Crons
			b.workflows = state.Workflows
			b.uniqueKeys = state.UniqueKeys
			b.queueState = state.QueueState

			for _, job := range b.jobs {
				if job.State == core.StateAvailable {
					b.addToQueueIndex(job)
				}
			}
		}
	}

	return b, nil
}

func (b *MemoryBackend) Health(ctx context.Context) (*core.HealthResponse, error) {
	return &core.HealthResponse{
		Status:        "ok",
		Version:       core.OJSVersion,
		UptimeSeconds: int64(time.Since(b.startTime).Seconds()),
		Backend: core.BackendHealth{
			Type:   "memory",
			Status: "ok",
		},
	}, nil
}

func (b *MemoryBackend) Close() error {
	if b.persist != nil {
		return b.persist.Close()
	}
	return nil
}

func (b *MemoryBackend) addToQueueIndex(job *core.Job) {
	if job.State != core.StateAvailable {
		return
	}

	idx, exists := b.queueIdx[job.Queue]
	if !exists {
		idx = &queueIndex{available: make([]*core.Job, 0)}
		b.queueIdx[job.Queue] = idx
	}

	priority := 0
	if job.Priority != nil {
		priority = *job.Priority
	}

	pos := sort.Search(len(idx.available), func(i int) bool {
		pi := 0
		if idx.available[i].Priority != nil {
			pi = *idx.available[i].Priority
		}
		if priority != pi {
			return priority > pi
		}
		return job.EnqueuedAt < idx.available[i].EnqueuedAt
	})

	idx.available = append(idx.available, nil)
	copy(idx.available[pos+1:], idx.available[pos:])
	idx.available[pos] = job
}

func (b *MemoryBackend) removeFromQueueIndex(job *core.Job) {
	idx, exists := b.queueIdx[job.Queue]
	if !exists {
		return
	}

	for i, j := range idx.available {
		if j.ID == job.ID {
			idx.available = append(idx.available[:i], idx.available[i+1:]...)
			return
		}
	}
}

package core

import (
	"context"
	"encoding/json"

	common "github.com/openjobspec/ojs-go-backend-common/core"
)

// Type aliases for shared types
type Job = common.Job
type OJSError = common.OJSError
type RetryPolicy = common.RetryPolicy
type UniquePolicy = common.UniquePolicy
type EnqueueRequest = common.EnqueueRequest
type EnqueueOptions = common.EnqueueOptions
type FetchRequest = common.FetchRequest
type AckRequest = common.AckRequest
type NackRequest = common.NackRequest
type JobError = common.JobError
type HeartbeatRequest = common.HeartbeatRequest
type BatchEnqueueRequest = common.BatchEnqueueRequest
type RateLimitPolicy = common.RateLimitPolicy

// Constant aliases
const (
	StateAvailable = common.StateAvailable
	StateScheduled = common.StateScheduled
	StatePending   = common.StatePending
	StateActive    = common.StateActive
	StateRetryable = common.StateRetryable
	StateCompleted = common.StateCompleted
	StateCancelled = common.StateCancelled
	StateDiscarded = common.StateDiscarded

	ErrCodeInvalidRequest  = common.ErrCodeInvalidRequest
	ErrCodeValidationError = common.ErrCodeValidationError
	ErrCodeNotFound        = common.ErrCodeNotFound
	ErrCodeConflict        = common.ErrCodeConflict
	ErrCodeDuplicate       = common.ErrCodeDuplicate
	ErrCodeInternalError   = common.ErrCodeInternalError
	ErrCodeUnsupported     = common.ErrCodeUnsupported
	ErrCodeQueuePaused     = common.ErrCodeQueuePaused

	OJSVersion                 = common.OJSVersion
	OJSMediaType               = common.OJSMediaType
	TimeFormat                 = common.TimeFormat
	DefaultVisibilityTimeoutMs = common.DefaultVisibilityTimeoutMs
)

// Function aliases
var (
	NewUUIDv7              = common.NewUUIDv7
	IsValidUUIDv7          = common.IsValidUUIDv7
	IsValidUUID            = common.IsValidUUID
	FormatTime             = common.FormatTime
	NowFormatted           = common.NowFormatted
	ParseEnqueueRequest    = common.ParseEnqueueRequest
	ValidateEnqueueRequest = common.ValidateEnqueueRequest
	CalculateBackoff       = common.CalculateBackoff
	DefaultRetryPolicy     = common.DefaultRetryPolicy
	ParseISO8601Duration   = common.ParseISO8601Duration
	FormatISO8601Duration  = common.FormatISO8601Duration
	IsValidTransition      = common.IsValidTransition
	IsTerminalState        = common.IsTerminalState
	IsCancellableState     = common.IsCancellableState
	NewInvalidRequestError = common.NewInvalidRequestError
	NewNotFoundError       = common.NewNotFoundError
	NewConflictError       = common.NewConflictError
	NewValidationError     = common.NewValidationError
	NewInternalError       = common.NewInternalError
)

// Backend interfaces — EXACT same as Redis backend
type JobManager interface {
	Push(ctx context.Context, job *Job) (*Job, error)
	PushBatch(ctx context.Context, jobs []*Job) ([]*Job, error)
	Info(ctx context.Context, jobID string) (*Job, error)
	Cancel(ctx context.Context, jobID string) (*Job, error)
}

type WorkerManager interface {
	Fetch(ctx context.Context, queues []string, count int, workerID string, visibilityTimeoutMs int) ([]*Job, error)
	Ack(ctx context.Context, jobID string, result []byte) (*AckResponse, error)
	Nack(ctx context.Context, jobID string, jobErr *JobError, requeue bool) (*NackResponse, error)
	Heartbeat(ctx context.Context, workerID string, activeJobs []string, visibilityTimeoutMs int) (*HeartbeatResponse, error)
	SetWorkerState(ctx context.Context, workerID string, state string) error
}

type QueueManager interface {
	ListQueues(ctx context.Context) ([]QueueInfo, error)
	QueueStats(ctx context.Context, name string) (*QueueStats, error)
	PauseQueue(ctx context.Context, name string) error
	ResumeQueue(ctx context.Context, name string) error
}

type DeadLetterManager interface {
	ListDeadLetter(ctx context.Context, limit, offset int) ([]*Job, int, error)
	RetryDeadLetter(ctx context.Context, jobID string) (*Job, error)
	DeleteDeadLetter(ctx context.Context, jobID string) error
}

type CronManager interface {
	RegisterCron(ctx context.Context, cron *CronJob) (*CronJob, error)
	ListCron(ctx context.Context) ([]*CronJob, error)
	DeleteCron(ctx context.Context, name string) (*CronJob, error)
}

type WorkflowManager interface {
	CreateWorkflow(ctx context.Context, req *WorkflowRequest) (*Workflow, error)
	GetWorkflow(ctx context.Context, id string) (*Workflow, error)
	CancelWorkflow(ctx context.Context, id string) (*Workflow, error)
	AdvanceWorkflow(ctx context.Context, workflowID string, jobID string, result json.RawMessage, failed bool) error
}

type AdminManager interface {
	ListJobs(ctx context.Context, filters JobListFilters, limit, offset int) ([]*Job, int, error)
	ListWorkers(ctx context.Context, limit, offset int) ([]*WorkerInfo, WorkerSummary, error)
}

type Backend interface {
	JobManager
	WorkerManager
	QueueManager
	DeadLetterManager
	CronManager
	WorkflowManager
	AdminManager
	Health(ctx context.Context) (*HealthResponse, error)
	Close() error
}

// Response/data types — EXACT same as Redis backend
type AckResponse struct {
	Acknowledged bool   `json:"acknowledged"`
	JobID        string `json:"job_id"`
	State        string `json:"state"`
	CompletedAt  string `json:"completed_at"`
	Job          *Job   `json:"job,omitempty"`
}

type NackResponse struct {
	JobID         string `json:"job_id"`
	State         string `json:"state"`
	Attempt       int    `json:"attempt"`
	MaxAttempts   int    `json:"max_attempts"`
	NextAttemptAt string `json:"next_attempt_at,omitempty"`
	DiscardedAt   string `json:"discarded_at,omitempty"`
	Job           *Job   `json:"job,omitempty"`
}

type QueueInfo struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at,omitempty"`
}

type QueueStats struct {
	Queue  string `json:"queue"`
	Status string `json:"status"`
	Stats  Stats  `json:"stats"`
}

type Stats struct {
	Available int `json:"available"`
	Active    int `json:"active"`
	Completed int `json:"completed"`
	Scheduled int `json:"scheduled"`
	Retryable int `json:"retryable"`
	Dead      int `json:"dead"`
}

type JobListFilters struct {
	State    string `json:"state,omitempty"`
	Queue    string `json:"queue,omitempty"`
	Type     string `json:"type,omitempty"`
	WorkerID string `json:"worker_id,omitempty"`
}

type WorkerInfo struct {
	ID            string `json:"id"`
	State         string `json:"state"`
	Directive     string `json:"directive"`
	ActiveJobs    int    `json:"active_jobs"`
	LastHeartbeat string `json:"last_heartbeat,omitempty"`
}

type WorkerSummary struct {
	Total   int `json:"total"`
	Running int `json:"running"`
	Quiet   int `json:"quiet"`
	Stale   int `json:"stale"`
}

type HealthResponse struct {
	Status        string        `json:"status"`
	Version       string        `json:"version"`
	UptimeSeconds int64         `json:"uptime_seconds"`
	Backend       BackendHealth `json:"backend"`
}

type BackendHealth struct {
	Type      string `json:"type"`
	Status    string `json:"status"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

type HeartbeatResponse struct {
	State        string   `json:"state"`
	Directive    string   `json:"directive"`
	JobsExtended []string `json:"jobs_extended,omitempty"`
	ServerTime   string   `json:"server_time"`
}

type CronJobTemplate struct {
	Type    string          `json:"type"`
	Args    json.RawMessage `json:"args,omitempty"`
	Options *EnqueueOptions `json:"options,omitempty"`
}

type CronJob struct {
	Name          string           `json:"name"`
	Expression    string           `json:"expression"`
	Timezone      string           `json:"timezone,omitempty"`
	OverlapPolicy string           `json:"overlap_policy,omitempty"`
	Enabled       bool             `json:"enabled"`
	JobTemplate   *CronJobTemplate `json:"job_template,omitempty"`
	CreatedAt     string           `json:"created_at,omitempty"`
	NextRunAt     string           `json:"next_run_at,omitempty"`
	LastRunAt     string           `json:"last_run_at,omitempty"`
	JobType       string           `json:"-"`
	Args          json.RawMessage  `json:"-"`
	Queue         string           `json:"-"`
	Meta          json.RawMessage  `json:"-"`
	Schedule      string           `json:"-"`
}

type WorkflowRequest struct {
	Type      string               `json:"type"`
	Name      string               `json:"name,omitempty"`
	Steps     []WorkflowJobRequest `json:"steps,omitempty"`
	Jobs      []WorkflowJobRequest `json:"jobs,omitempty"`
	Callbacks *WorkflowCallbacks   `json:"callbacks,omitempty"`
}

type WorkflowJobRequest struct {
	Name    string          `json:"name"`
	Type    string          `json:"type"`
	Args    json.RawMessage `json:"args"`
	Options *EnqueueOptions `json:"options,omitempty"`
}

type Workflow struct {
	ID             string             `json:"id"`
	Name           string             `json:"name,omitempty"`
	Type           string             `json:"type"`
	State          string             `json:"state"`
	StepsTotal     *int               `json:"steps_total,omitempty"`
	StepsCompleted *int               `json:"steps_completed,omitempty"`
	JobsTotal      *int               `json:"jobs_total,omitempty"`
	JobsCompleted  *int               `json:"jobs_completed,omitempty"`
	Callbacks      *WorkflowCallbacks `json:"callbacks,omitempty"`
	CreatedAt      string             `json:"created_at"`
	CompletedAt    string             `json:"completed_at,omitempty"`
}

type WorkflowCallbacks struct {
	OnSuccess  *WorkflowCallback `json:"on_success,omitempty"`
	OnFailure  *WorkflowCallback `json:"on_failure,omitempty"`
	OnComplete *WorkflowCallback `json:"on_complete,omitempty"`
}

type WorkflowCallback struct {
	Type    string          `json:"type"`
	Args    json.RawMessage `json:"args,omitempty"`
	Options *EnqueueOptions `json:"options,omitempty"`
}

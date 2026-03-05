package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	commonapi "github.com/openjobspec/ojs-go-backend-common/api"
	ojsotel "github.com/openjobspec/ojs-go-backend-common/otel"

	"github.com/openjobspec/ojs-backend-lite/internal/admin"
	"github.com/openjobspec/ojs-backend-lite/internal/api"
	"github.com/openjobspec/ojs-backend-lite/internal/core"
)

// NewRouter creates and configures the HTTP router with all OJS routes.
func NewRouter(backend core.Backend, cfg Config) http.Handler {
	return NewRouterWithRealtime(backend, cfg, nil, nil)
}

// NewRouterWithRealtime creates and configures the HTTP router with all OJS routes
// including real-time SSE and WebSocket endpoints.
func NewRouterWithRealtime(backend core.Backend, cfg Config, publisher core.EventPublisher, subscriber core.EventSubscriber) http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Recoverer)
	r.Use(ojsotel.HTTPMiddleware)
	r.Use(api.Metrics)
	r.Use(api.OJSHeaders)
	r.Use(api.LimitRequestBody)
	r.Use(api.ValidateContentType)

	// Optional API key authentication
	if cfg.APIKey != "" {
		r.Use(api.KeyAuth(cfg.APIKey, "/ojs/v1/health"))
	}

	// Create handlers
	jobHandler := api.NewJobHandler(backend)
	workerHandler := api.NewWorkerHandler(backend)
	systemHandler := api.NewSystemHandler(backend)
	queueHandler := api.NewQueueHandler(backend)
	deadLetterHandler := api.NewDeadLetterHandler(backend)
	cronHandler := api.NewCronHandler(backend)
	workflowHandler := api.NewWorkflowHandler(backend)
	batchHandler := api.NewBatchHandler(backend)
	adminHandler := api.NewAdminHandler(backend)
	healthDetailHandler := api.NewHealthDetailHandler(backend)

	// Wire event publisher into handlers
	if publisher != nil {
		jobHandler.SetEventPublisher(publisher)
		workerHandler.SetEventPublisher(publisher)
	}

	// System endpoints
	r.Get("/ojs/manifest", systemHandler.Manifest)
	r.Get("/ojs/v1/health", systemHandler.Health)
	r.Get("/ojs/v1/health/detail", healthDetailHandler.HealthDetail)

	// Metrics endpoint (Prometheus-compatible)
	r.Get("/metrics", api.MetricsHandler)

	// Job endpoints
	r.Post("/ojs/v1/jobs", jobHandler.Create)
	r.Get("/ojs/v1/jobs/{id}", jobHandler.Get)
	r.Delete("/ojs/v1/jobs/{id}", jobHandler.Cancel)

	// Batch enqueue
	r.Post("/ojs/v1/jobs/batch", batchHandler.Create)

	// Worker endpoints
	r.Post("/ojs/v1/workers/fetch", workerHandler.Fetch)
	r.Post("/ojs/v1/workers/ack", workerHandler.Ack)
	r.Post("/ojs/v1/workers/nack", workerHandler.Nack)
	r.Post("/ojs/v1/workers/heartbeat", workerHandler.Heartbeat)

	// Queue endpoints
	r.Get("/ojs/v1/queues", queueHandler.List)
	r.Get("/ojs/v1/queues/{name}/stats", queueHandler.Stats)
	r.Post("/ojs/v1/queues/{name}/pause", queueHandler.Pause)
	r.Post("/ojs/v1/queues/{name}/resume", queueHandler.Resume)

	// Dead letter endpoints
	r.Get("/ojs/v1/dead-letter", deadLetterHandler.List)
	r.Post("/ojs/v1/dead-letter/{id}/retry", deadLetterHandler.Retry)
	r.Delete("/ojs/v1/dead-letter/{id}", deadLetterHandler.Delete)

	// Cron endpoints
	r.Get("/ojs/v1/cron", cronHandler.List)
	r.Post("/ojs/v1/cron", cronHandler.Register)
	r.Delete("/ojs/v1/cron/{name}", cronHandler.Delete)

	// Workflow endpoints
	r.Post("/ojs/v1/workflows", workflowHandler.Create)
	r.Get("/ojs/v1/workflows/{id}", workflowHandler.Get)
	r.Delete("/ojs/v1/workflows/{id}", workflowHandler.Cancel)

	// Admin API endpoints (control plane)
	r.Get("/ojs/v1/admin/stats", adminHandler.Stats)
	r.Get("/ojs/v1/admin/queues", adminHandler.ListQueues)
	r.Get("/ojs/v1/admin/queues/{name}", adminHandler.GetQueue)
	r.Post("/ojs/v1/admin/queues/{name}/pause", adminHandler.PauseQueue)
	r.Post("/ojs/v1/admin/queues/{name}/resume", adminHandler.ResumeQueue)
	r.Get("/ojs/v1/admin/jobs", adminHandler.ListJobs)
	r.Get("/ojs/v1/admin/jobs/{id}", adminHandler.GetJob)
	r.Post("/ojs/v1/admin/jobs/{id}/retry", adminHandler.RetryJob)
	r.Post("/ojs/v1/admin/jobs/{id}/cancel", adminHandler.CancelJob)
	r.Post("/ojs/v1/admin/jobs/bulk/retry", adminHandler.BulkRetry)
	r.Get("/ojs/v1/admin/workers", adminHandler.ListWorkers)
	r.Post("/ojs/v1/admin/workers/{id}/quiet", adminHandler.QuietWorker)
	r.Get("/ojs/v1/admin/dead-letter", adminHandler.ListDeadLetter)
	r.Get("/ojs/v1/admin/dead-letter/stats", adminHandler.DeadLetterStats)
	r.Post("/ojs/v1/admin/dead-letter/{id}/retry", adminHandler.RetryDeadLetter)
	r.Delete("/ojs/v1/admin/dead-letter/{id}", adminHandler.DeleteDeadLetter)
	r.Post("/ojs/v1/admin/dead-letter/retry", adminHandler.BulkRetryDeadLetter)

	// Real-time endpoints (SSE and WebSocket)
	if subscriber != nil {
		sseHandler := api.NewSSEHandler(backend, subscriber)
		wsHandler := api.NewWSHandler(backend, subscriber)

		r.Get("/ojs/v1/jobs/{id}/events", sseHandler.JobEvents)
		r.Get("/ojs/v1/queues/{name}/events", sseHandler.QueueEvents)
		r.Get("/ojs/v1/ws", wsHandler.Handle)
	}

	// Admin UI (embedded SPA)
	r.Handle("/ojs/admin", http.RedirectHandler("/ojs/admin/", http.StatusMovedPermanently))
	r.Mount("/ojs/admin/", http.StripPrefix("/ojs/admin/", admin.Handler()))

	// API documentation (Swagger UI)
	commonapi.RegisterDocsRoutes(r, api.OpenAPISpec)

	return r
}

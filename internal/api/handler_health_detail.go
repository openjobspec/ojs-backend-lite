package api

import (
	"net/http"
	"runtime"
	"runtime/debug"

	"github.com/openjobspec/ojs-backend-lite/internal/core"
)

// HealthDetailHandler handles the detailed health check endpoint.
type HealthDetailHandler struct {
	backend core.Backend
}

// NewHealthDetailHandler creates a new HealthDetailHandler.
func NewHealthDetailHandler(backend core.Backend) *HealthDetailHandler {
	return &HealthDetailHandler{backend: backend}
}

// HealthDetail handles GET /ojs/v1/health/detail
func (h *HealthDetailHandler) HealthDetail(w http.ResponseWriter, r *http.Request) {
	resp, err := h.backend.Health(r.Context())
	if err != nil {
		WriteJSON(w, http.StatusServiceUnavailable, resp)
		return
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	goVersion := runtime.Version()
	buildVersion := ""
	if info, ok := debug.ReadBuildInfo(); ok {
		buildVersion = info.Main.Version
	}

	detail := map[string]any{
		"status":         resp.Status,
		"version":        resp.Version,
		"uptime_seconds": resp.UptimeSeconds,
		"backend":        resp.Backend,
		"runtime": map[string]any{
			"go_version":    goVersion,
			"build_version": buildVersion,
			"goroutines":    runtime.NumGoroutine(),
			"num_cpu":       runtime.NumCPU(),
			"goarch":        runtime.GOARCH,
			"goos":          runtime.GOOS,
		},
		"memory": map[string]any{
			"alloc_mb":        memStats.Alloc / 1024 / 1024,
			"total_alloc_mb":  memStats.TotalAlloc / 1024 / 1024,
			"sys_mb":          memStats.Sys / 1024 / 1024,
			"num_gc":          memStats.NumGC,
			"heap_objects":    memStats.HeapObjects,
			"heap_inuse_mb":   memStats.HeapInuse / 1024 / 1024,
			"stack_inuse_mb":  memStats.StackInuse / 1024 / 1024,
		},
		"metrics": map[string]any{
			"total_requests":  GlobalMetrics.TotalRequests.Load(),
			"total_errors":    GlobalMetrics.TotalErrors.Load(),
			"active_requests": GlobalMetrics.ActiveRequests.Load(),
		},
	}

	status := http.StatusOK
	if resp.Status != "ok" {
		status = http.StatusServiceUnavailable
	}
	WriteJSON(w, status, detail)
}

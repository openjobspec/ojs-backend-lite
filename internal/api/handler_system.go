package api

import (
	"net/http"

	"github.com/openjobspec/ojs-backend-lite/internal/core"
)

// SystemHandler handles system-related HTTP endpoints.
type SystemHandler struct {
	backend core.Backend
}

// NewSystemHandler creates a new SystemHandler.
func NewSystemHandler(backend core.Backend) *SystemHandler {
	return &SystemHandler{backend: backend}
}

// Manifest handles GET /ojs/manifest
func (h *SystemHandler) Manifest(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]any{
		"specversion": core.OJSVersion,
		"implementation": map[string]any{
			"name":    "ojs-backend-lite",
			"version": core.OJSVersion,
			"backend": "memory",
		},
		"conformance_level": 4,
		"protocols":         []string{"http", "grpc"},
		"backend":           "memory",
		"capabilities": map[string]any{
			"batch_enqueue":     true,
			"cron_jobs":         true,
			"dead_letter":       true,
			"delayed_jobs":      true,
			"job_ttl":           true,
			"priority_queues":   true,
			"rate_limiting":     false,
			"schema_validation": true,
			"unique_jobs":       true,
			"workflows":         true,
			"pause_resume":      true,
		},
		"extensions": map[string]any{
			"official": []map[string]any{
				{"name": "admin-api", "uri": "urn:ojs:ext:admin-api", "version": "1.0.0"},
				{"name": "dead-letter", "uri": "urn:ojs:ext:dead-letter", "version": "1.0.0"},
			},
		},
	})
}

// Health handles GET /ojs/v1/health
func (h *SystemHandler) Health(w http.ResponseWriter, r *http.Request) {
	resp, err := h.backend.Health(r.Context())
	if err != nil {
		WriteJSON(w, http.StatusServiceUnavailable, resp)
		return
	}

	status := http.StatusOK
	if resp.Status != "ok" {
		status = http.StatusServiceUnavailable
	}

	WriteJSON(w, status, resp)
}

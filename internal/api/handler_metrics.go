package api

import (
	"fmt"
	"net/http"
	"strings"
)

// MetricsHandler serves Prometheus-compatible metrics at /metrics.
func MetricsHandler(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder

	b.WriteString("# HELP ojs_http_requests_total Total number of HTTP requests.\n")
	b.WriteString("# TYPE ojs_http_requests_total counter\n")
	fmt.Fprintf(&b, "ojs_http_requests_total %d\n", GlobalMetrics.TotalRequests.Load())

	b.WriteString("# HELP ojs_http_errors_total Total number of HTTP 5xx errors.\n")
	b.WriteString("# TYPE ojs_http_errors_total counter\n")
	fmt.Fprintf(&b, "ojs_http_errors_total %d\n", GlobalMetrics.TotalErrors.Load())

	b.WriteString("# HELP ojs_http_active_requests Number of in-flight HTTP requests.\n")
	b.WriteString("# TYPE ojs_http_active_requests gauge\n")
	fmt.Fprintf(&b, "ojs_http_active_requests %d\n", GlobalMetrics.ActiveRequests.Load())

	b.WriteString("# HELP ojs_http_request_duration_ms_sum Sum of HTTP request durations in milliseconds.\n")
	b.WriteString("# TYPE ojs_http_request_duration_ms_sum counter\n")
	b.WriteString("# HELP ojs_http_request_duration_count Total count of HTTP requests by route.\n")
	b.WriteString("# TYPE ojs_http_request_duration_count counter\n")

	GlobalMetrics.RequestDuration.Range(func(key, value any) bool {
		bucket := value.(*durationBucket)
		labels := fmt.Sprintf(`method="%s",route="%s",status="%s"`, bucket.Method, bucket.Route, bucket.Status)
		fmt.Fprintf(&b, "ojs_http_request_duration_ms_sum{%s} %d\n", labels, bucket.SumMs.Load())
		fmt.Fprintf(&b, "ojs_http_request_duration_count{%s} %d\n", labels, bucket.Count.Load())
		return true
	})

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, b.String())
}

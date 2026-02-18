package api

import (
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// MetricsData holds aggregated HTTP metrics exposed via /metrics.
type MetricsData struct {
	TotalRequests   atomic.Int64
	TotalErrors     atomic.Int64
	ActiveRequests  atomic.Int64
	RequestDuration sync.Map // key: route+method+status → *durationBucket
}

type durationBucket struct {
	Count    atomic.Int64
	SumMs    atomic.Int64
	Route    string
	Method   string
	Status   string
}

// GlobalMetrics is the singleton metrics store used by the middleware.
var GlobalMetrics = &MetricsData{}

// Metrics returns a middleware that records HTTP request metrics.
func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		GlobalMetrics.ActiveRequests.Add(1)
		GlobalMetrics.TotalRequests.Add(1)

		start := time.Now()
		ww := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(ww, r)

		GlobalMetrics.ActiveRequests.Add(-1)
		duration := time.Since(start).Milliseconds()

		if ww.statusCode >= 500 {
			GlobalMetrics.TotalErrors.Add(1)
		}

		statusStr := strconv.Itoa(ww.statusCode)
		key := r.Method + " " + r.URL.Path + " " + statusStr

		val, loaded := GlobalMetrics.RequestDuration.LoadOrStore(key, &durationBucket{
			Route:  r.URL.Path,
			Method: r.Method,
			Status: statusStr,
		})
		bucket := val.(*durationBucket)
		if !loaded {
			bucket.Route = r.URL.Path
			bucket.Method = r.Method
			bucket.Status = statusStr
		}
		bucket.Count.Add(1)
		bucket.SumMs.Add(duration)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.written {
		r.statusCode = code
		r.written = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.written {
		r.written = true
	}
	return r.ResponseWriter.Write(b)
}

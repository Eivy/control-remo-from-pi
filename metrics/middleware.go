package metrics

import (
	"net/http"
	"strconv"
	"time"
)

// HTTPMetricsMiddleware wraps HTTP handlers to collect metrics
func HTTPMetricsMiddleware(collector *Collector, endpoint string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap the ResponseWriter to capture status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			// Call the next handler
			next(wrapped, r)

			// Record metrics
			duration := time.Since(start).Seconds()
			collector.UpdateAPIMetrics(endpoint, wrapped.statusCode, duration, nil)
		}
	}
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// ParseRateLimitHeaders extracts rate limit information from HTTP response headers
func ParseRateLimitHeaders(headers http.Header) *RateLimitInfo {
	limit := headers.Get("X-RateLimit-Limit")
	remaining := headers.Get("X-RateLimit-Remaining")
	reset := headers.Get("X-RateLimit-Reset")

	if limit == "" || remaining == "" || reset == "" {
		return nil
	}

	limitInt, _ := strconv.Atoi(limit)
	remainingInt, _ := strconv.Atoi(remaining)
	resetInt, _ := strconv.ParseInt(reset, 10, 64)

	return &RateLimitInfo{
		Limit:     limitInt,
		Remaining: remainingInt,
		Reset:     resetInt,
	}
}
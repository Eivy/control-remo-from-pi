package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cormoran/natureremo"
	"github.com/eivy/control-remo-from-pi/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestMetricsEndpoint(t *testing.T) {
	// Create a new registry to avoid conflicts
	registry := prometheus.NewRegistry()

	// Create a mock Nature Remo client (using test token)
	client := natureremo.NewClient("test-token")

	// Create metrics collector
	collector := metrics.NewCollector(client, 60*time.Second)
	registry.MustRegister(collector)

	// Update some test metrics
	collector.UpdateApplianceState("test-id", "test-light", "light", true)
	collector.UpdateAPIMetrics("GetAll", 200, 0.5, &metrics.RateLimitInfo{
		Limit:     1000,
		Remaining: 999,
		Reset:     time.Now().Unix() + 3600,
	})

	// Create HTTP server with metrics handler
	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	body := w.Body.String()

	// Check for expected metrics
	expectedMetrics := []string{
		"remo_appliance_power_state",
		"remo_appliance_state_changes_total",
		"remo_api_requests_total",
		"remo_api_request_duration_seconds",
		"remo_api_rate_limit_limit",
		"remo_api_rate_limit_remaining",
		"remo_last_update_timestamp",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(body, metric) {
			t.Errorf("Expected metric %s not found in output", metric)
		}
	}

	// Check for labels
	if !strings.Contains(body, `id="test-id"`) {
		t.Error("Expected appliance ID label not found")
	}

	if !strings.Contains(body, `name="test-light"`) {
		t.Error("Expected appliance name label not found")
	}

	if !strings.Contains(body, `type="light"`) {
		t.Error("Expected appliance type label not found")
	}

	t.Logf("Metrics output:\n%s", body)
}

package metrics

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/tenntenn/natureremo"
)

const (
	namespace = "remo"
)

type Collector struct {
	mu              sync.RWMutex
	remoClient      *natureremo.Client
	appliances      map[string]*ApplianceState
	apiMetrics      *APIMetrics
	lastUpdateTime  time.Time
	cacheDuration   time.Duration

	// Prometheus metric descriptors
	appliancePowerStateDesc     *prometheus.Desc
	applianceStateChangeDesc    *prometheus.Desc
	apiRequestTotalDesc         *prometheus.Desc
	apiRequestDurationDesc      *prometheus.Desc
	apiRateLimitLimitDesc       *prometheus.Desc
	apiRateLimitRemainingDesc   *prometheus.Desc
	apiRateLimitResetDesc       *prometheus.Desc
	lastUpdateTimestampDesc     *prometheus.Desc
}

type ApplianceState struct {
	ID               string
	Name             string
	Type             string
	PowerState       float64 // 1 for on, 0 for off
	LastStateChange  time.Time
	StateChangeCount float64
}

type APIMetrics struct {
	RequestCount     map[string]float64 // key: "endpoint:status_code"
	RequestDuration  map[string]float64 // key: "endpoint"
	RateLimitLimit   float64
	RateLimitRemain  float64
	RateLimitReset   float64
}

func NewCollector(client *natureremo.Client, cacheDuration time.Duration) *Collector {
	return &Collector{
		remoClient:     client,
		appliances:     make(map[string]*ApplianceState),
		apiMetrics:     &APIMetrics{
			RequestCount:    make(map[string]float64),
			RequestDuration: make(map[string]float64),
		},
		cacheDuration: cacheDuration,

		// Define metric descriptors
		appliancePowerStateDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "appliance", "power_state"),
			"Current power state of the appliance (1 = on, 0 = off)",
			[]string{"id", "name", "type"}, nil,
		),
		applianceStateChangeDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "appliance", "state_changes_total"),
			"Total number of state changes for the appliance",
			[]string{"id", "name", "type"}, nil,
		),
		apiRequestTotalDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "api", "requests_total"),
			"Total number of API requests",
			[]string{"endpoint", "status"}, nil,
		),
		apiRequestDurationDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "api", "request_duration_seconds"),
			"Duration of API requests in seconds",
			[]string{"endpoint"}, nil,
		),
		apiRateLimitLimitDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "api", "rate_limit_limit"),
			"API rate limit maximum",
			nil, nil,
		),
		apiRateLimitRemainingDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "api", "rate_limit_remaining"),
			"API rate limit remaining",
			nil, nil,
		),
		apiRateLimitResetDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "api", "rate_limit_reset_timestamp"),
			"API rate limit reset timestamp",
			nil, nil,
		),
		lastUpdateTimestampDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "last_update_timestamp"),
			"Timestamp of the last metrics update",
			nil, nil,
		),
	}
}

// Describe implements prometheus.Collector
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.appliancePowerStateDesc
	ch <- c.applianceStateChangeDesc
	ch <- c.apiRequestTotalDesc
	ch <- c.apiRequestDurationDesc
	ch <- c.apiRateLimitLimitDesc
	ch <- c.apiRateLimitRemainingDesc
	ch <- c.apiRateLimitResetDesc
	ch <- c.lastUpdateTimestampDesc
}

// Collect implements prometheus.Collector
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Appliance metrics
	for _, appliance := range c.appliances {
		ch <- prometheus.MustNewConstMetric(
			c.appliancePowerStateDesc,
			prometheus.GaugeValue,
			appliance.PowerState,
			appliance.ID,
			appliance.Name,
			appliance.Type,
		)
		ch <- prometheus.MustNewConstMetric(
			c.applianceStateChangeDesc,
			prometheus.CounterValue,
			appliance.StateChangeCount,
			appliance.ID,
			appliance.Name,
			appliance.Type,
		)
	}


	// API metrics
	for key, count := range c.apiMetrics.RequestCount {
		// Parse key format: "endpoint:status"
		parts := strings.Split(key, ":")
		endpoint := ""
		status := ""
		if len(parts) == 2 {
			endpoint = parts[0]
			status = parts[1]
		}
		ch <- prometheus.MustNewConstMetric(
			c.apiRequestTotalDesc,
			prometheus.CounterValue,
			count,
			endpoint,
			status,
		)
	}

	for endpoint, duration := range c.apiMetrics.RequestDuration {
		ch <- prometheus.MustNewConstMetric(
			c.apiRequestDurationDesc,
			prometheus.GaugeValue,
			duration,
			endpoint,
		)
	}

	ch <- prometheus.MustNewConstMetric(
		c.apiRateLimitLimitDesc,
		prometheus.GaugeValue,
		c.apiMetrics.RateLimitLimit,
	)
	ch <- prometheus.MustNewConstMetric(
		c.apiRateLimitRemainingDesc,
		prometheus.GaugeValue,
		c.apiMetrics.RateLimitRemain,
	)
	ch <- prometheus.MustNewConstMetric(
		c.apiRateLimitResetDesc,
		prometheus.GaugeValue,
		c.apiMetrics.RateLimitReset,
	)

	// Last update timestamp
	ch <- prometheus.MustNewConstMetric(
		c.lastUpdateTimestampDesc,
		prometheus.GaugeValue,
		float64(c.lastUpdateTime.Unix()),
	)
}

// UpdateApplianceState updates the state of an appliance
func (c *Collector) UpdateApplianceState(id, name, appType string, powerOn bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	newState := float64(0)
	if powerOn {
		newState = 1
	}

	if appliance, exists := c.appliances[id]; exists {
		if appliance.PowerState != newState {
			appliance.PowerState = newState
			appliance.LastStateChange = time.Now()
			appliance.StateChangeCount++
		}
	} else {
		c.appliances[id] = &ApplianceState{
			ID:               id,
			Name:             name,
			Type:             appType,
			PowerState:       newState,
			LastStateChange:  time.Now(),
			StateChangeCount: 0,
		}
	}
	c.lastUpdateTime = time.Now()
}


// UpdateAPIMetrics updates API-related metrics
func (c *Collector) UpdateAPIMetrics(endpoint string, statusCode int, duration float64, rateLimit *RateLimitInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := endpoint + ":" + fmt.Sprintf("%d", statusCode)
	c.apiMetrics.RequestCount[key]++
	c.apiMetrics.RequestDuration[endpoint] = duration

	if rateLimit != nil {
		c.apiMetrics.RateLimitLimit = float64(rateLimit.Limit)
		c.apiMetrics.RateLimitRemain = float64(rateLimit.Remaining)
		c.apiMetrics.RateLimitReset = float64(rateLimit.Reset)
	}
}

type RateLimitInfo struct {
	Limit     int
	Remaining int
	Reset     int64
}
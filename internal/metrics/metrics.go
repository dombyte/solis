// Package metrics provides Prometheus metrics for Solis inverter data.
// It uses the default Prometheus registry and serves metrics from the main HTTP server.
package metrics

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/dombyte/solis/internal/logging"
	"github.com/dombyte/solis/internal/solis"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// logger is the package-level logger for metrics operations.
var logger = logging.NewComponentLogger("metrics")

// ServiceInterface defines the methods needed from the service layer.
type ServiceInterface interface {
	GetKeys() []string
	GetValues(keys []string) (map[string]*solis.Value, error)
	GetAllCachedValues() map[string]*solis.Value
}

var (
	mu                   sync.RWMutex
	metrics              = make(map[string]*prometheus.GaugeVec)
	svc                  ServiceInterface
	initialized          = false
	stdMetricsRegistered = false
)

// sanitizeMetricName converts register key to Prometheus metric name.
func sanitizeMetricName(key string) string {
	key = strings.ReplaceAll(key, "-", "_")
	key = strings.ReplaceAll(key, ".", "_")
	return "solis_" + key
}

// Init initializes all Prometheus metrics with the default registry.
// It registers Go and Process collectors, and creates gauges for all Solis registers.
// This must be called once at application startup.
func Init(s ServiceInterface) {
	mu.Lock()
	defer mu.Unlock()

	if initialized {
		return
	}
	initialized = true

	svc = s

	// Register standard collectors with default registry (only once)
	if !stdMetricsRegistered {
		if err := prometheus.Register(prometheus.NewGoCollector()); err != nil {
			logger.Debug().Msgf("Go collector already registered: %v", err)
		}
		if err := prometheus.Register(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{})); err != nil {
			logger.Debug().Msgf("Process collector already registered: %v", err)
		}
		stdMetricsRegistered = true
	}

	// Create gauges for all registers (both stable and dynamic)
	keys := s.GetKeys()
	for _, key := range keys {
		name := sanitizeMetricName(key)
		gauge := prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: name,
				Help: "Current value of Solis register: " + key,
			},
			[]string{"key", "name", "unit"},
		)
		metrics[key] = gauge
		prometheus.MustRegister(gauge)
	}

	logger.Info().Msgf("Initialized %d Prometheus metrics", len(metrics))
}

// Update refreshes all gauge values from the service.
// This is called on each /metrics request to refresh values.
// It uses cached values for speed, falling back to storage if cache is not available.
func Update() error {
	mu.Lock()
	defer mu.Unlock()

	if svc == nil {
		return fmt.Errorf("metrics service not initialized")
	}

	// Try to get all values from cache first (fast path)
	var values map[string]*solis.Value
	if cachedValues := svc.GetAllCachedValues(); len(cachedValues) > 0 {
		values = cachedValues
		logger.Debug().Msgf("Using cached values for %d metrics", len(values))
	} else {
		// Cache not available or empty, fall back to storage
		keys := svc.GetKeys()
		var err error
		values, err = svc.GetValues(keys)
		if err != nil {
			return fmt.Errorf("failed to get values: %w", err)
		}
	}

	for key, value := range values {
		if gauge, ok := metrics[key]; ok {
			gauge.WithLabelValues(key, value.Name, value.Unit).Set(value.DecodedValue)
		}
	}

	return nil
}

// Handler returns an http.Handler that serves Prometheus metrics from the default registry.
// It updates metric values on each request before serving.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := Update(); err != nil {
			http.Error(w, "Failed to update metrics: "+err.Error(), http.StatusInternalServerError)
			logger.Error().Msgf("Metrics update failed: %v", err)
			return
		}

		// Serve metrics from the default registry
		promhttp.Handler().ServeHTTP(w, r)
	})
}

// Shutdown unregisters all metrics from the Prometheus default registry.
// This should be called during application shutdown for clean resource cleanup.
func Shutdown() {
	mu.Lock()
	defer mu.Unlock()

	// Unregister all Solis metrics from the default registry
	for _, gauge := range metrics {
		if gauge != nil {
			prometheus.Unregister(gauge)
		}
	}

	// Clear the metrics map
	metrics = make(map[string]*prometheus.GaugeVec)
	svc = nil
	initialized = false

	logger.Info().Msg("Prometheus metrics shutdown complete")
}

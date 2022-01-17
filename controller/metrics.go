package filter

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	EventHandleDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "event_handle_duration_milliseconds",
		Help:    "The latency of event handle",
		Buckets: []float64{0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10, 100, 500, 1000, 5000},
	}, []string{"name", "event_type"})
	EventHandleCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "event_handle_count",
		Help: "The count of event handle",
	}, []string{"name", "event_type"})
	EventHandleErrorCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "event_handle_error_count",
		Help: "The count of event handle error",
	}, []string{"name", "event_type", "error_type"})
	EventHandleRetryCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "event_handle_retry_count",
		Help: "The count of event handle retry",
	}, []string{"name"})
)

package main

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type prometheusMetrics struct {
	latencySpring *prometheus.HistogramVec
}

func outcomeFrom(status int, isError bool) string {
	if isError {
		return "SERVER_ERROR"
	}
	if isHTTPError(status) {
		if status >= 500 && status < 600 {
			return "SERVER_ERROR"
		}
		return "CLIENT_ERROR"
	}
	return "SUCCESS"
}

func (m *prometheusMetrics) recordLatency(method, status, uri, outcome string, elapsed time.Duration) {
	sec := float64(elapsed) / float64(time.Second)
	m.latencySpring.WithLabelValues(method, status, uri, outcome).Observe(sec)
}

var (
	dimensionsSpring = []string{"method", "status", "uri", "outcome"}
)

func newMetrics(registerer prometheus.Registerer, namespace string,
	latencyBucketsHTTP []float64) *prometheusMetrics {
	return &prometheusMetrics{

		latencySpring: newHistogramVec(
			registerer,
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "",
				Name:      "requests_seconds",
				Help:      "Spring-like server request duration in seconds.",
				Buckets:   latencyBucketsHTTP,
			},
			dimensionsSpring,
		),
	}
}

func newHistogramVec(registerer prometheus.Registerer,
	opts prometheus.HistogramOpts,
	labelValues []string) *prometheus.HistogramVec {
	return promauto.With(registerer).NewHistogramVec(opts, labelValues)
}

func (app *application) metricsHandler() http.Handler {
	registerer := app.registry
	gatherer := app.registry
	return promhttp.InstrumentMetricHandler(
		registerer, promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{}),
	)
}

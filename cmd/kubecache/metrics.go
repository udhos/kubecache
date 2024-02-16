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

func (m *prometheusMetrics) recordLatency(method, status, uri string, elapsed time.Duration) {
	sec := float64(elapsed) / float64(time.Second)
	m.latencySpring.WithLabelValues(method, status, uri).Observe(sec)
}

var (
	dimensionsSpring = []string{"method", "status", "uri"}
)

func newMetrics(registerer prometheus.Registerer, namespace string,
	latencyBucketsHTTP []float64) *prometheusMetrics {
	return &prometheusMetrics{

		latencySpring: newHistogramVec(
			registerer,
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "http_server",
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

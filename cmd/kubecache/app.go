package main

import (
	"net/http"

	"github.com/modernprogram/groupcache/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	"github.com/udhos/otelconfig/oteltrace"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"
)

type application struct {
	cfg              config
	tracer           trace.Tracer
	registry         *prometheus.Registry
	metrics          *prometheusMetrics
	serverMain       *http.Server
	serverHealth     *http.Server
	serverMetrics    *http.Server
	serverGroupCache *http.Server
	cache            *groupcache.Group
}

func newApplication(me string) *application {
	app := &application{
		registry: prometheus.NewRegistry(),
		cfg:      newConfig(me),
		tracer:   oteltrace.NewNoopTracer(),
	}

	initApplication(app)

	return app
}

func initApplication(app *application) {

	//
	// add basic/default instrumentation
	//
	app.registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	app.registry.MustRegister(prometheus.NewGoCollector())

	app.metrics = newMetrics(app.registry, app.cfg.metricsNamespace,
		app.cfg.metricsBucketsLatencyHTTP)

	//
	// start group cache
	//
	startGroupcache(app)

	//
	// register application route
	//

	mux := http.NewServeMux()
	app.serverMain = &http.Server{Addr: app.cfg.listenAddr, Handler: mux}

	const route = "/"

	log.Info().Msgf("registering route: %s %s", app.cfg.listenAddr, route)

	mux.Handle(route, otelhttp.NewHandler(app, "app.ServerHTTP"))
}

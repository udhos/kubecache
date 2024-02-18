package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

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

func (app *application) run() {
	log.Info().Msgf("application server: listening on %s", app.cfg.listenAddr)
	err := app.serverMain.ListenAndServe()
	log.Error().Msgf("application server: exited: %v", err)
}

func (app *application) stop() {
	const timeout = 5 * time.Second
	httpShutdown(app.serverHealth, "health", timeout)
	httpShutdown(app.serverMain, "main", timeout)
	httpShutdown(app.serverGroupCache, "groupcache", timeout)
	httpShutdown(app.serverMetrics, "metrics", timeout)
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

func httpShutdown(s *http.Server, label string, timeout time.Duration) {
	if s == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		log.Error().Msgf("http shutdown error: %s: %v", label, err)
	}
}

func (app *application) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	const me = "app.ServeHTTP"
	ctx, span := app.tracer.Start(r.Context(), me)
	defer span.End()

	begin := time.Now()

	uri := r.RequestURI

	method := r.Method

	key := method + " " + uri

	resp, errFetch := app.query(ctx, key)

	elap := time.Since(begin)

	app.metrics.recordLatency(r.Method, strconv.Itoa(resp.Status), uri, elap)

	//
	// log query status
	//
	{
		traceID := span.SpanContext().TraceID().String()
		status := resp.Status
		if errFetch == nil {
			if isHTTPError(status) {
				//
				// http error
				//
				log.Error().Str("traceID", traceID).Str("method", method).Str("uri", uri).Int("response_status", status).Dur("elapsed", elap).Msgf("ServeHTTP: traceID=%s method=%s url=%s response_status=%d elapsed=%v", traceID, method, uri, status, elap)
			} else {
				//
				// http success
				//
				log.Debug().Str("traceID", traceID).Str("method", method).Str("uri", uri).Int("response_status", status).Dur("elapsed", elap).Msgf("ServeHTTP: traceID=%s method=%s url=%s response_status=%d elapsed=%v", traceID, method, uri, status, elap)
			}
		} else {
			log.Error().Str("traceID", traceID).Str("method", method).Str("uri", uri).Int("response_status", status).Str("response_error", errFetch.Error()).Dur("elapsed", elap).Msgf("ServeHTTP: traceID=%s method=%s uri=%s response_status=%d elapsed=%v response_error:%v", traceID, method, uri, status, elap, errFetch)
		}
	}

	//
	// send response headers (1/3)
	//
	for k, v := range resp.Header {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}

	//
	// send response status (2/3)
	//
	if errFetch == nil {
		w.WriteHeader(resp.Status)
	} else {
		w.WriteHeader(500)
	}

	//
	// send response body (3/3)
	//
	if errFetch != nil {
		//
		// error
		//
		if len(resp.Body) > 0 {
			//
			// prefer received body
			//
			w.Write(resp.Body)
		} else {
			fmt.Fprint(w, errFetch.Error())
		}
	}
	w.Write(resp.Body)
}

func isHTTPError(status int) bool {
	return status < 200 || status > 299
}

func (app *application) query(c context.Context, key string) (response, error) {

	const me = "app.query"
	ctx, span := app.tracer.Start(c, me)
	defer span.End()

	var resp response

	var data []byte
	errGet := app.cache.Get(ctx, key, groupcache.AllocatingByteSliceSink(&data))

	if errGet != nil {
		log.Error().Msgf("key='%s' cache error:%v", key, errGet)
		resp.Status = 500
		return resp, errGet
	}

	if errJ := json.Unmarshal(data, &resp); errJ != nil {
		log.Error().Msgf("key='%s' json error:%v", key, errJ)
		resp.Status = 500
		return resp, errJ
	}

	return resp, nil
}

type response struct {
	Body   []byte      `json:"body"`
	Status int         `json:"status"`
	Header http.Header `json:"header"`
}

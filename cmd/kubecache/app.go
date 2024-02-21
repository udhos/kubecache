package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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
	restrictPrefix   []string
	backendURL       *url.URL
	httpClient       *http.Client
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

	{
		u, errURL := url.Parse(app.cfg.backendURL)
		if errURL != nil {
			log.Fatal().Msgf("backend URL: %v", errURL)
		}
		app.backendURL = u
	}

	errList := json.Unmarshal([]byte(app.cfg.restrictPrefix), &app.restrictPrefix)
	if errList != nil {
		log.Fatal().Msgf("restrict to prefix: '%s': %v", app.cfg.restrictPrefix, errList)
	}

	app.httpClient = &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
		Timeout:   app.cfg.backendTimeout,
	}

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

	uri := r.URL.String()

	method := r.Method

	key := method + " " + uri

	//
	// Restricted prefix list?
	//
	var useCache bool
	if len(app.restrictPrefix) == 0 {
		//
		// empty list, cache everything
		//
		useCache = true
	} else {
		//
		// check the path is in the restricted list
		//
		for _, p := range app.restrictPrefix {
			if strings.HasPrefix(r.URL.RequestURI(), p) {
				useCache = true
				break
			}
		}
	}

	resp, errFetch := app.query(ctx, key, useCache)

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
				bodyStr := string(resp.Body)
				log.Error().Str("traceID", traceID).Str("method", method).Str("uri", uri).Int("response_status", status).Dur("elapsed", elap).Str("response_body", bodyStr).Msgf("ServeHTTP: traceID=%s method=%s url=%s response_status=%d elapsed=%v response_body:%s", traceID, method, uri, status, elap, bodyStr)
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
	if errFetch == nil {
		w.Write(resp.Body)
	} else {
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
}

func isHTTPError(status int) bool {
	return status < 200 || status > 299
}

func (app *application) query(c context.Context, key string, useCache bool) (response, error) {

	const me = "app.query"
	ctx, span := app.tracer.Start(c, me)
	defer span.End()

	if useCache {
		var resp response
		var data []byte

		if errGet := app.cache.Get(ctx, key, groupcache.AllocatingByteSliceSink(&data)); errGet != nil {
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

	resp, _, errFetch := doFetch(ctx, app.tracer, app.httpClient, app.backendURL, key)
	if errFetch != nil {
		return resp, errFetch
	}

	return resp, nil
}

func parseKey(caller string, backendURL *url.URL, key string) (string, string, error) {
	method, uri, found := strings.Cut(key, " ")
	if !found {
		return "", "", fmt.Errorf("%s: parseKey: bad key: '%s'", caller, key)
	}

	reqURL, errParseURL := url.Parse(uri)
	if errParseURL != nil {
		return "", "", fmt.Errorf("%s: parse URL: '%s': %v", caller, uri, errParseURL)
	}

	reqURL.Scheme = backendURL.Scheme
	reqURL.Host = backendURL.Host

	u := reqURL.String()

	return method, u, nil
}

type response struct {
	Body   []byte      `json:"body"`
	Status int         `json:"status"`
	Header http.Header `json:"header"`
}

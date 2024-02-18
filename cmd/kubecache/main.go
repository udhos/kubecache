// Package main implements kubecache.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os/signal"
	"strconv"
	"syscall"

	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/modernprogram/groupcache/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/udhos/boilerplate/boilerplate"
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

func main() {
	//
	// initialize zerolog
	//
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	//
	// command-line
	//
	var showVersion bool
	flag.BoolVar(&showVersion, "version", showVersion, "show version")
	flag.Parse()

	me := filepath.Base(os.Args[0])

	//
	// version
	//
	{
		v := boilerplate.LongVersion(me + " version=" + version)
		if showVersion {
			fmt.Print(v)
			fmt.Println()
			return
		}
		log.Print(v)
	}

	app := &application{
		registry: prometheus.NewRegistry(),
	}

	//
	// config
	//

	app.cfg = newConfig(me)

	//
	// initialize tracing
	//

	{
		options := oteltrace.TraceOptions{
			DefaultService:     me,
			NoopTracerProvider: false,
			Debug:              true,
		}

		tracer, cancel, errTracer := oteltrace.TraceStart(options)
		if errTracer != nil {
			log.Fatal().Msgf("tracer: %v", errTracer)
		}

		defer cancel()

		app.tracer = tracer
	}

	//
	// init application
	//
	initApplication(app)

	//
	// start application server
	//

	go func() {
		log.Info().Msgf("application server: listening on %s", app.cfg.listenAddr)
		err := app.serverMain.ListenAndServe()
		log.Error().Msgf("application server: exited: %v", err)
	}()

	//
	// start health server
	//

	{
		log.Info().Msgf("registering health route: %s %s",
			app.cfg.healthAddr, app.cfg.healthPath)

		mux := http.NewServeMux()
		app.serverHealth = &http.Server{Addr: app.cfg.healthAddr, Handler: mux}
		mux.HandleFunc(app.cfg.healthPath, func(w http.ResponseWriter,
			_ /*r*/ *http.Request) {
			fmt.Fprintln(w, "health ok")
		})

		go func() {
			log.Info().Msgf("health server: listening on %s %s",
				app.cfg.healthAddr, app.cfg.healthPath)
			err := app.serverHealth.ListenAndServe()
			log.Info().Msgf("health server: exited: %v", err)
		}()
	}

	//
	// start metrics server
	//

	{
		log.Info().Msgf("registering metrics route: %s %s",
			app.cfg.metricsAddr, app.cfg.metricsPath)

		mux := http.NewServeMux()
		app.serverMetrics = &http.Server{Addr: app.cfg.metricsAddr, Handler: mux}
		mux.Handle(app.cfg.metricsPath, app.metricsHandler())

		go func() {
			log.Info().Msgf("metrics server: listening on %s %s",
				app.cfg.metricsAddr, app.cfg.metricsPath)
			err := app.serverMetrics.ListenAndServe()
			log.Error().Msgf("metrics server: exited: %v", err)
		}()
	}

	gracefulShutdown(app)
}

func gracefulShutdown(app *application) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	log.Info().Msgf("received signal '%v', initiating shutdown", sig)

	const timeout = 5 * time.Second

	httpShutdown(app.serverHealth, "health", timeout)
	httpShutdown(app.serverMain, "main", timeout)
	httpShutdown(app.serverGroupCache, "groupcache", timeout)
	httpShutdown(app.serverMetrics, "metrics", timeout)

	log.Info().Msgf("exiting")
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

func (app *application) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	const me = "app.ServeHTTP"
	ctx, span := app.tracer.Start(r.Context(), me)
	defer span.End()

	begin := time.Now()

	traceID := span.SpanContext().TraceID().String()

	uri := r.RequestURI

	key := r.Method + " " + uri

	resp, errFetch := app.query(ctx, key)

	elap := time.Since(begin)

	app.metrics.recordLatency(r.Method, strconv.Itoa(resp.Status), uri, elap)

	//
	// send response headers
	//
	for k, v := range resp.Header {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}

	if errFetch != nil {
		log.Error().Str("traceID", traceID).Msgf("traceID=%s key='%s' status=%d elapsed=%v error:%v",
			traceID, key, resp.Status, elap, errFetch)
		http.Error(w, errFetch.Error(), resp.Status)
		return
	}

	log.Info().Str("traceID", traceID).Msgf("traceID=%s key='%s' status=%d elapsed=%v",
		traceID, key, resp.Status, elap)

	if _, err := w.Write(resp.Body); err != nil {
		log.Error().Str("traceID", traceID).Msgf("traceID=%s key='%s' status=%d elapsed=%v write error:%v",
			traceID, key, resp.Status, elap, err)
	}
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

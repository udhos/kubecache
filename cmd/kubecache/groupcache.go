package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/modernprogram/groupcache/v2"
	"github.com/rs/zerolog/log"
	"github.com/udhos/groupcache_exporter"
	"github.com/udhos/groupcache_exporter/groupcache/modernprogram"
	"github.com/udhos/kubegroup/kubegroup"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func startGroupcache(app *application) func() {

	workspace := groupcache.NewWorkspace()

	//
	// create groupcache pool
	//

	myURL, errURL := kubegroup.FindMyURL(app.cfg.groupcachePort)
	if errURL != nil {
		log.Fatal().Msgf("groupcache my URL: %v", errURL)
	}
	log.Info().Msgf("groupcache my URL: %v", errURL)

	pool := groupcache.NewHTTPPoolOptsWithWorkspace(workspace, myURL, &groupcache.HTTPPoolOptions{})

	//
	// start groupcache server
	//

	app.serverGroupCache = &http.Server{Addr: app.cfg.groupcachePort, Handler: pool}

	go func() {
		log.Info().Msgf("groupcache server: listening on %s", app.cfg.groupcachePort)
		err := app.serverGroupCache.ListenAndServe()
		log.Error().Msgf("groupcache server: exited: %v", err)
	}()

	//
	// start watcher for addresses of peers
	//

	options := kubegroup.Options{
		Pool:           pool,
		GroupCachePort: app.cfg.groupcachePort,
		//PodLabelKey:    "app",         // default is "app"
		//PodLabelValue:  "my-app-name", // default is current PODs label value for label key
		MetricsRegisterer: app.registry,
		MetricsGatherer:   app.registry,
		MetricsNamespace:  app.cfg.kubegroupMetricsNamespace,
		Debug:             app.cfg.kubegroupDebug,
		ListerInterval:    app.cfg.kubegroupListerInterval,
	}

	kg, errKg := kubegroup.UpdatePeers(options)
	if errKg != nil {
		log.Fatal().Msgf("kubegroup error: %v", errKg)
	}

	//
	// create cache
	//

	httpClient := &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
		Timeout:   app.cfg.backendTimeout,
	}

	getter := groupcache.GetterFunc(
		func(c context.Context, key string, dest groupcache.Sink) error {

			const me = "groupcache.getter"
			ctx, span := app.tracer.Start(c, me)
			defer span.End()

			method, uri, found := strings.Cut(key, " ")
			if !found {
				return fmt.Errorf("getter: bad key: '%s'", key)
			}

			u, errURL := url.JoinPath(app.cfg.backendURL, uri)
			if errURL != nil {
				return fmt.Errorf("getter: bad URL: %v", errURL)
			}

			begin := time.Now()

			body, respHeaders, status, errFetch := fetch(ctx, httpClient, app.tracer,
				method, u)

			elap := time.Since(begin)

			isErrorStatus := isHTTPError(status)

			//
			// log fetch status
			//
			traceID := span.SpanContext().TraceID().String()
			if errFetch == nil {
				if isErrorStatus {
					//
					// http error
					//
					log.Error().Str("traceID", traceID).Str("method", method).Str("url", u).Int("response_status", status).Dur("elapsed", elap).Msgf("getter: traceID=%s method=%s url=%s response_status=%d elapsed=%v", traceID, method, u, status, elap)
				} else {
					//
					// http success
					//
					log.Debug().Str("traceID", traceID).Str("method", method).Str("url", u).Int("response_status", status).Dur("elapsed", elap).Msgf("getter: traceID=%s method=%s url=%s response_status=%d elapsed=%v", traceID, method, u, status, elap)
				}
			} else {
				log.Error().Str("traceID", traceID).Str("method", method).Str("url", u).Int("response_status", status).Str("response_error", errFetch.Error()).Dur("elapsed", elap).Msgf("getter: traceID=%s method=%s url=%s response_status=%d elapsed=%v response_error:%v", traceID, method, u, status, elap, errFetch)
			}

			if errFetch != nil {
				return errFetch
			}

			resp := response{
				Body:   body,
				Status: status,
				Header: respHeaders,
			}

			data, errJ := json.Marshal(resp)
			if errFetch != nil {
				return errJ
			}

			var ttl time.Duration
			if isErrorStatus {
				ttl = app.cfg.cacheErrorTTL
			} else {
				ttl = app.cfg.cacheTTL
			}
			expire := time.Now().Add(ttl)

			return dest.SetBytes(data, expire)
		},
	)

	// https://talks.golang.org/2013/oscon-dl.slide#46
	//
	// 64 MB max per-node memory usage
	app.cache = groupcache.NewGroupWithWorkspace(workspace, "path", app.cfg.groupcacheSizeBytes, getter)

	//
	// expose prometheus metrics for groupcache
	//

	g := modernprogram.New(app.cache)
	labels := map[string]string{
		//"app": appName,
	}
	namespace := ""
	collector := groupcache_exporter.NewExporter(namespace, labels, g)
	app.registry.MustRegister(collector)

	stop := func() {
		kg.Close()
	}

	return stop
}

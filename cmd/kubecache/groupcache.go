package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/modernprogram/groupcache/v2"
	"github.com/rs/zerolog/log"
	"github.com/udhos/groupcache_exporter"
	"github.com/udhos/groupcache_exporter/groupcache/modernprogram"
	"github.com/udhos/kubegroup/kubegroup"
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
		Pool:              pool,
		GroupCachePort:    app.cfg.groupcachePort,
		MetricsRegisterer: app.registry,
		MetricsGatherer:   app.registry,
		MetricsNamespace:  app.cfg.kubegroupMetricsNamespace,
		Debug:             app.cfg.kubegroupDebug,
		ListerInterval:    app.cfg.kubegroupListerInterval,
		//PodLabelKey:    "app",         // default is "app"
		//PodLabelValue:  "my-app-name", // default is current PODs label value for label key
	}

	kg, errKg := kubegroup.UpdatePeers(options)
	if errKg != nil {
		log.Fatal().Msgf("kubegroup error: %v", errKg)
	}

	//
	// create cache
	//

	getter := groupcache.GetterFunc(
		func(c context.Context, key string, dest groupcache.Sink) error {

			const me = "groupcache.getter"
			ctx, span := app.tracer.Start(c, me)
			defer span.End()

			resp, isErrorStatus, errFetch := doFetch(ctx, app.tracer, app.httpClient, app.backendURL, key)
			if errFetch != nil {
				return errFetch
			}

			data, errJ := json.Marshal(resp)
			if errJ != nil {
				return fmt.Errorf("%s: marshal json response: %v", me, errJ)
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
	labels := map[string]string{}
	namespace := ""
	collector := groupcache_exporter.NewExporter(namespace, labels, g)
	app.registry.MustRegister(collector)

	stop := func() {
		kg.Close()
	}

	return stop
}

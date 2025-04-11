package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/modernprogram/groupcache/v2"
	"github.com/rs/zerolog/log"
	"github.com/udhos/boilerplate/awsconfig"
	"github.com/udhos/ecs-task-discovery/groupcachediscovery"
	"github.com/udhos/groupcache_datadog/exporter"
	"github.com/udhos/groupcache_exporter"
	"github.com/udhos/groupcache_exporter/groupcache/modernprogram"
	"github.com/udhos/kube/kubeclient"
	"github.com/udhos/kubegroup/kubegroup"
)

func startGroupcache(app *application, forceNamespaceDefault bool) func() {

	workspace := groupcache.NewWorkspace()

	//
	// create groupcache pool
	//

	myURL, errURL := kubegroup.FindMyURL(app.cfg.groupcachePort)
	if errURL != nil {
		log.Fatal().Msgf("groupcache my URL: %v", errURL)
	}
	log.Info().Msgf("groupcache my URL: %s", myURL)

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

	var stopDisc func()

	metricsNamespace := app.cfg.metricsNamespace

	//
	// start watcher for addresses of peers
	//

	if app.cfg.compute == "ecs" {
		//
		// compute: amazon ecs
		//
		awsCfg, errCfg := awsconfig.AwsConfig(awsconfig.Options{})
		if errCfg != nil {
			log.Fatal().Msgf("startGroupcache: could not get aws config: %v", errCfg)
		}
		clientEcs := ecs.NewFromConfig(awsCfg.AwsConfig)
		discOptions := groupcachediscovery.Options{
			Pool:           pool,
			Client:         clientEcs,
			GroupCachePort: app.cfg.groupcachePort,
			ServiceName:    app.cfg.ecsTaskDiscoveryService, // self
			// ForceSingleTask: see below
			// MetricsRegisterer: see below
			MetricsNamespace: metricsNamespace,
			DogstatsdClient:  app.dogstatsdClient,
		}
		if app.cfg.prometheusEnable {
			discOptions.MetricsRegisterer = app.registry
		}
		if app.cfg.forceSingleTask {
			myAddr, errAddr := groupcachediscovery.FindMyAddr()
			if errAddr != nil {
				log.Fatal().Msgf("startGroupcache: groupcache my address: %v", errAddr)
			}
			discOptions.ForceSingleTask = myAddr
		}
		disc, errDisc := groupcachediscovery.New(discOptions)
		if errDisc != nil {
			log.Fatal().Msgf("startGroupcache: groupcache discovery error: %v", errDisc)
		}
		stopDisc = func() {
			disc.Stop()
		}
	} else {
		//
		// compute: kubernetes
		//
		clientsetOpt := kubeclient.Options{DebugLog: app.cfg.kubegroupDebug}
		clientset, errClientset := kubeclient.New(clientsetOpt)
		if errClientset != nil {
			log.Fatal().Msgf("startGroupcache: kubeclient: %v", errClientset)
		}
		options := kubegroup.Options{
			Client:                clientset,
			LabelSelector:         app.cfg.kubegroupLabelSelector,
			Pool:                  pool,
			GroupCachePort:        app.cfg.groupcachePort,
			MetricsNamespace:      app.cfg.kubegroupMetricsNamespace,
			Debug:                 app.cfg.kubegroupDebug,
			ForceNamespaceDefault: forceNamespaceDefault,
			DogstatsdClient:       app.dogstatsdClient,
			//MetricsRegisterer:   see below
			//MetricsGatherer:     see below
		}
		if app.cfg.prometheusEnable {
			options.MetricsRegisterer = app.registry
			options.MetricsGatherer = app.registry
		}
		kg, errKg := kubegroup.UpdatePeers(options)
		if errKg != nil {
			log.Fatal().Msgf("kubegroup error: %v", errKg)
		}
		stopDisc = func() {
			kg.Close()
		}
	}

	//
	// create cache
	//

	getter := groupcache.GetterFunc(
		func(c context.Context, key string, dest groupcache.Sink, _ *groupcache.Info) error {

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

	groupcacheOptions := groupcache.Options{
		Workspace:                   workspace,
		Name:                        "path",
		PurgeExpired:                !app.cfg.groupcacheDisablePurgeExpired,
		ExpiredKeysEvictionInterval: app.cfg.groupcacheExpiredKeysEvictionInterval,
		CacheBytesLimit:             app.cfg.groupcacheSizeBytes,
		Getter:                      getter,
	}

	// https://talks.golang.org/2013/oscon-dl.slide#46
	//
	// 64 MB max per-node memory usage
	app.cache = groupcache.NewGroupWithWorkspace(groupcacheOptions)

	extract := modernprogram.New(app.cache) // extract metrics from groupcache group

	unregister := func() {}

	if app.cfg.prometheusEnable {
		log.Info().Msgf("starting groupcache metrics exporter for Prometheus")
		labels := map[string]string{}
		collector := groupcache_exporter.NewExporter(metricsNamespace, labels, extract)
		app.registry.MustRegister(collector)
		unregister = func() { app.registry.Unregister(collector) }
	}

	closeExporterDogstatsd := func() {}

	if app.cfg.dogstatsdEnable {
		log.Info().Msgf("starting groupcache metrics exporter for Dogstatsd")
		exporter := exporter.New(exporter.Options{
			Client:         app.dogstatsdClient,
			Groups:         []groupcache_exporter.GroupStatistics{extract},
			ExportInterval: app.cfg.dogstatsdExportInterval,
		})
		closeExporterDogstatsd = func() { exporter.Close() }
	}

	return func() {
		stopDisc()
		unregister()
		closeExporterDogstatsd()
	}
}

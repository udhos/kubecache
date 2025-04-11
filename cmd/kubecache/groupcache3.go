package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/groupcache/groupcache-go/v3"
	"github.com/groupcache/groupcache-go/v3/transport"
	"github.com/rs/zerolog/log"
	"github.com/udhos/boilerplate/awsconfig"
	"github.com/udhos/ecs-task-discovery/groupcachediscovery"
	"github.com/udhos/kube/kubeclient"
	"github.com/udhos/kubegroup/kubegroup"
)

func startGroupcache3(app *application, forceNamespaceDefault bool) func() {

	ctx, cancel := context.WithCancel(context.Background())

	//
	// create groupcache instance
	//

	myIP, errAddr := kubegroup.FindMyAddress()
	if errAddr != nil {
		log.Fatal().Msgf("groupcache my address: %v", errAddr)
	}
	log.Info().Msgf("groupcache my address: %s", myIP)

	myAddr := myIP + app.cfg.groupcachePort

	daemon, errDaemon := groupcache.ListenAndServe(ctx, myAddr, groupcache.Options{})
	if errDaemon != nil {
		log.Fatal().Msgf("groupcache3 daemon: %v", errDaemon)
	}

	//
	// start watcher for addresses of peers
	//

	var stopDisc func()

	if app.cfg.compute == "ecs" {
		//
		// compute: amazon ecs
		//
		awsCfg, errCfg := awsconfig.AwsConfig(awsconfig.Options{})
		if errCfg != nil {
			log.Fatal().Msgf("startGroupcache3: could not get aws config: %v", errCfg)
		}
		clientEcs := ecs.NewFromConfig(awsCfg.AwsConfig)
		discOptions := groupcachediscovery.Options{
			Peers:          daemon,
			Client:         clientEcs,
			GroupCachePort: app.cfg.groupcachePort,
			ServiceName:    app.cfg.ecsTaskDiscoveryService, // self
			// ForceSingleTask: see below
		}
		if app.cfg.forceSingleTask {
			myAddr, errAddr := groupcachediscovery.FindMyAddr()
			if errAddr != nil {
				log.Fatal().Msgf("startGroupcache3: groupcache my address: %v", errAddr)
			}
			discOptions.ForceSingleTask = myAddr
		}
		disc, errDisc := groupcachediscovery.New(discOptions)
		if errDisc != nil {
			log.Fatal().Msgf("startGroupcache3: groupcache discovery error: %v", errDisc)
		}
		stopDisc = func() {
			disc.Stop()
			{
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := daemon.Shutdown(ctx); err != nil {
					log.Error().Msgf("groupcache3 daemon shutdown error: %v", err)
				}
			}
			cancel()
		}
	} else {
		//
		// compute: kubernetes
		//
		clientsetOpt := kubeclient.Options{DebugLog: app.cfg.kubegroupDebug}
		clientset, errClientset := kubeclient.New(clientsetOpt)
		if errClientset != nil {
			log.Fatal().Msgf("kubeclient: %v", errClientset)
		}
		options := kubegroup.Options{
			Client:                clientset,
			LabelSelector:         app.cfg.kubegroupLabelSelector,
			Peers:                 daemon,
			GroupCachePort:        app.cfg.groupcachePort,
			MetricsRegisterer:     app.registry,
			MetricsGatherer:       app.registry,
			MetricsNamespace:      app.cfg.kubegroupMetricsNamespace,
			Debug:                 app.cfg.kubegroupDebug,
			ForceNamespaceDefault: forceNamespaceDefault,
		}
		kg, errKg := kubegroup.UpdatePeers(options)
		if errKg != nil {
			log.Fatal().Msgf("kubegroup error: %v", errKg)
		}
		stopDisc = func() {
			{
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := daemon.Shutdown(ctx); err != nil {
					log.Error().Msgf("groupcache3 daemon shutdown error: %v", err)
				}
			}
			kg.Close()
			cancel()
		}
	}

	//
	// create cache
	//

	getter := groupcache.GetterFunc(
		func(c context.Context, key string, dest transport.Sink) error {

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

	cache, errGroup := daemon.NewGroup("files", app.cfg.groupcacheSizeBytes, getter)
	if errGroup != nil {
		log.Fatal().Msgf("new group error: %v", errGroup)
	}

	app.cache3 = cache

	//
	// expose prometheus metrics for groupcache
	//

	/*
		g := modernprogram.New(app.cache)
		labels := map[string]string{}
		namespace := ""
		collector := groupcache_exporter.NewExporter(namespace, labels, g)
		app.registry.MustRegister(collector)
	*/
	log.Error().Msgf("XXX TODO FIXME WRITEME groupcache3 expose prometheus metrics")

	return stopDisc
}

package main

import (
	"time"

	"github.com/udhos/boilerplate/envconfig"
)

type config struct {
	trace                                 bool
	debugLog                              bool
	listenAddr                            string
	backendURL                            string
	restrictRouteRegexp                   string
	restrictMethod                        string
	backendTimeout                        time.Duration
	cacheTTL                              time.Duration
	cacheErrorTTL                         time.Duration
	healthAddr                            string
	healthPath                            string
	metricsAddr                           string
	metricsPath                           string
	metricsNamespace                      string
	metricsBucketsLatencyHTTP             []float64
	emfSendLogs                           bool
	emfEnable                             bool
	prometheusEnable                      bool
	dogstatsdEnable                       bool
	dogstatsdExportInterval               time.Duration
	dogstatsdDebug                        bool
	dogstatsdClientTTL                    time.Duration
	groupcacheVersion                     int
	groupcachePort                        string
	groupcacheSizeBytes                   int64
	groupcacheDisablePurgeExpired         bool
	groupcacheExpiredKeysEvictionInterval time.Duration
	kubegroupMetricsNamespace             string
	kubegroupDebug                        bool
	kubegroupLabelSelector                string
	kubegroupForceNamespaceDefault        bool
	compute                               string
	forceSingleTask                       bool
	ecsTaskDiscoveryService               string // ecs service self discovery
}

func newConfig(roleSessionName string) config {

	env := envconfig.NewSimple(roleSessionName)

	return config{
		trace:      env.Bool("TRACE", true),
		debugLog:   env.Bool("DEBUG_LOG", true),
		listenAddr: env.String("LISTEN_ADDR", ":8080"),
		backendURL: env.String("BACKEND_URL", "http://config-server:9000"),
		//
		// only requests matching both RESTRICT_ROUTE_REGEXP and RESTRICT_METHOD are cached.
		// *empty* list means match *anything*.
		//
		restrictRouteRegexp: env.String("RESTRICT_ROUTE_REGEXP", `["^/develop", "^/homolog", "^/prod", "/develop/?$", "/homolog/?$", "/prod/?$"]`),
		restrictMethod:      env.String("RESTRICT_METHOD", `["GET", "HEAD"]`),
		//
		cacheTTL:         env.Duration("CACHE_TTL", 300*time.Second),
		cacheErrorTTL:    env.Duration("CACHE_ERROR_TTL", 60*time.Second),
		backendTimeout:   env.Duration("BACKEND_TIMEOUT", 300*time.Second),
		healthAddr:       env.String("HEALTH_ADDR", ":8888"),
		healthPath:       env.String("HEALTH_PATH", "/health"),
		metricsAddr:      env.String("METRICS_ADDR", ":3000"),
		metricsPath:      env.String("METRICS_PATH", "/metrics"),
		metricsNamespace: env.String("METRICS_NAMESPACE", ""),
		metricsBucketsLatencyHTTP: env.Float64Slice("METRICS_BUCKETS_LATENCY_HTTP",
			[]float64{0.00001, 0.000025, 0.00005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5, 10, 25, 50, 100, 250, 500, 1000}),
		emfSendLogs:                           env.Bool("EMF_SEND_LOGS", false),
		emfEnable:                             env.Bool("EMF_ENABLE", false),
		prometheusEnable:                      env.Bool("PROMETHEUS_ENABLE", true),
		dogstatsdEnable:                       env.Bool("DOGSTATSD_ENABLE", true),
		dogstatsdExportInterval:               env.Duration("DOGSTATSD_EXPORT_INTERVAL", 30*time.Second),
		dogstatsdDebug:                        env.Bool("DOGSTATSD_DEBUG", false),
		dogstatsdClientTTL:                    env.Duration("DOGSTATSD_CLIENT_TTL", time.Minute),
		groupcacheVersion:                     env.Int("GROUPCACHE_VERSION", 2),
		groupcachePort:                        env.String("GROUPCACHE_PORT", ":5000"),
		groupcacheSizeBytes:                   env.Int64("GROUPCACHE_SIZE_BYTES", 100_000_000),
		groupcacheDisablePurgeExpired:         env.Bool("GROUPCACHE_DISABLE_PURGE_EXPIRED", false),
		groupcacheExpiredKeysEvictionInterval: env.Duration("GROUPCACHE_EXPIRED_KEYS_EVICTION_INTERVAL", 30*time.Minute),
		kubegroupMetricsNamespace:             env.String("KUBEGROUP_METRICS_NAMESPACE", ""),
		kubegroupDebug:                        env.Bool("KUBEGROUP_DEBUG", true),
		kubegroupLabelSelector:                env.String("KUBEGROUP_LABEL_SELECTOR", "app=kubecache"),
		kubegroupForceNamespaceDefault:        env.Bool("KUBEGROUP_FORCE_NAMESPACE_DEFAULT", false),
		compute:                               env.String("COMPUTE", "kubernetes"), // "ecs", "kubernetes"
		forceSingleTask:                       env.Bool("FORCE_SINGLE_TASK", false),
		ecsTaskDiscoveryService:               env.String("ECS_TASK_DISCOVERY_SERVICE", "kubecache"), // ecs service self discovery
	}
}

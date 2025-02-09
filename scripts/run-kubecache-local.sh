#!/bin/bash

export KUBEGROUP_FORCE_NAMESPACE_DEFAULT=true
export GROUPCACHE_VERSION=2
export GROUPCACHE_SIZE_BYTES=2000             ;# default: 100,000,000
export BACKEND_URL=http://localhost:9000      ;# ADDR=:9000 miniapi -- curl localhost:8080/v1/hello
export RESTRICT_ROUTE_REGEXP='[]'             ;# *empty* list means match *anything*
export CACHE_TTL=60s                          ;# default: 300s
export TRACE=false
#export COMPUTE=ecs

kubecache

# while :; do curl -s localhost:3000/metrics | grep ^groupcache; echo; sleep 2; done

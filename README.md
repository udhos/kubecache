[![license](http://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/udhos/kubecache/blob/main/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/udhos/kubecache)](https://goreportcard.com/report/github.com/udhos/kubecache)
[![Go Reference](https://pkg.go.dev/badge/github.com/udhos/kubecache.svg)](https://pkg.go.dev/github.com/udhos/kubecache)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/kubecache)](https://artifacthub.io/packages/search?repo=kubecache)
[![Docker Pulls](https://img.shields.io/docker/pulls/udhos/kubecache)](https://hub.docker.com/r/udhos/kubecache)

# kubecache

[kubecache](https://github.com/udhos/kubecache) forwards HTTP GET requests to another service, cacheing responses in [groupcache](https://github.com/modernprogram/groupcache) for 5 minutes (by default).

# Build

```bash
git clone https://github.com/udhos/kubecache
cd kubecache
./build.sh
```

# Docker image

See: https://hub.docker.com/r/udhos/kubecache

# Helm chart

See: https://udhos.github.io/kubecache

# Configuration

See supported env vars in `configMapProperties` in [charts/kubecache/values.yaml](charts/kubecache/values.yaml).

# References

* (Portuguese) Blog post: https://udhos.github.io/blog/2024/02/18/groupcache.html

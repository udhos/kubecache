# Usage

[Helm](https://helm.sh) must be installed to use the charts.  Please refer to
Helm's [documentation](https://helm.sh/docs) to get started.

Once Helm has been set up correctly, add the repo as follows:

    helm repo add kubecache https://udhos.github.io/kubecache

Update files from repo:

    helm repo update

Search kubecache:

    $ helm search repo kubecache -l --version ">=0.0.0"
    NAME                CHART VERSION	APP VERSION	DESCRIPTION
    kubecache/kubecache	0.3.0        	0.3.0      	A Helm chart for Kubernetes
    kubecache/kubecache	0.2.0        	0.2.0      	A Helm chart for Kubernetes
    kubecache/kubecache	0.1.0        	0.1.0      	A Helm chart for Kubernetes

To install the charts:

    helm install my-kubecache kubecache/kubecache
    #            ^            ^         ^
    #            |            |          \__________ chart
    #            |            |
    #            |             \____________________ repo
    #            |
    #             \_________________________________ release (chart instance installed in cluster)

To uninstall the charts:

    helm uninstall my-kubecache

# Source

<https://github.com/udhos/kubecache>

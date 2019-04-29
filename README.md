# Gravitational Wormhole
Wormhole is a simple [CNI plugin](https://github.com/containernetworking/cni) designed to create an encrypted overlay network for [kubernetes](https://kubernetes.io) clusters.

[WireGuard](https://www.wireguard.com) is a fascinating Fast, Modern, Secure VPN tunnel, that has been gaining significant praise from security experts, and is currently proposed for inclusion within the linux kernel.

Wormhole uses WireGuard to create a simple and secure high performance encrypted overlay network for kubernetes clusters, that is easy to manage and troubleshoot.

Wormhole does not implement network policy, instead we recommend to use [calico](https://github.com/projectcalico/calico) or [kube-router](https://github.com/cloudnativelabs/kube-router) as network policy controllers.

## Notice
<aside class="warning">
The Gravitational Wormhole project is currently considered experimental, and has not undergone any external security audits. Use at your own risk.
</aside>


## Getting Started

### System Requirements
1. [WireGuard](https://www.wireguard.com/install/) is installed on each node in you're cluster.
2. A Kubernetes cluster with IPAM enabled (--pod-network-cidr= when using kubeadm based install)

### Install (Kubeadm Cluster)
```console
kubectl apply -f https://raw.githubusercontent.com/gravitational/wormhole/master/docs/kube-wormhole.yaml
```

Note: The kubeadm cluster must be initialized with (--pod-network-cidr / --service-cidr) to enable IPAM

### Install (Generic)
```console
kubectl apply -f https://raw.githubusercontent.com/gravitational/wormhole/master/docs/generic-wormhole.yaml
```

Note: Replace the --overlay-cidr flag in the daemonset with the overlay-cidr that matches you're network
Note: Kubernetes IPAM must be enabled (--cluster-cidr / --allocate-node-cidrs on kube-controller-manager)

## Build and Publish to a docker registry

```
WORM_REGISTRY_IMAGE="quay.io/gravitational/wormhole" go run mage.go build:publish
```

## Test

```
go run mage.go test:all
```


## More Information
- [Wormhole RFC](docs/rfcs/0001-spec.md)

## Contributing
The best way to contribute is to create issues or pull requests right here on Github. You can also reach the Gravitational team through their [website](https://gravitational.com)

## Resources
|Project Links| Description
|---|----
| [Blog](http://blog.gravitational.com) | Our blog, where we publish gravitational news |
| [Security Updates](https://groups.google.com/forum/#!forum/gravity-community-security) | Gravity Community Security Updates |
| [Community Forum](https://community.gravitational.com) | Gravitational Community Forum|

## Who Built Wormhole?
Wormhole was created by [Gravitational Inc.](https://gravitational.com) We have built wormhole by leveraging our experience automating and supporting hundreds of kubernetes clusters with [Gravity](https://gravitational.com/gravity/), our Kubernetes distribution optimized for deploying and remotely controlling complex applications into multiple environments at the same time:

- Multiple cloud regions
- Colocation
- Private enterprise clouds located behind firewalls
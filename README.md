# Gravitational Wormhole
> **Warning**
> 
> Wormhole was archived 2023-07-01, as Teleport no longer supports Gravity.
>
> Please see our [Gravitational is Teleport](https://goteleport.com/blog/gravitational-is-teleport/)
> blog post for more information.

Wormhole is a [CNI plugin](https://github.com/containernetworking/cni) that creates an encrypted overlay network for [kubernetes](https://kubernetes.io) clusters.

[WireGuard](https://www.wireguard.com) is a Fast, Modern, Secure VPN tunnel.

Wormhole uses WireGuard to create a simple and secure high performance encrypted overlay network for kubernetes clusters, that is easy to manage and troubleshoot.

Wormhole does not implement network policy, instead we recommend to use [calico](https://github.com/projectcalico/calico) or [kube-router](https://github.com/cloudnativelabs/kube-router) as network policy controllers.

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

## Troubleshooting
See [troubleshooting.md](docs/troubleshooting.md)

## Test

```
go run mage.go test:all
```


## More Information
- [Wormhole RFC](docs/rfcs/0001-spec.md)

# Troubleshooting Guide
## Connectivity
Gravitational Wormhole uses port 9806 by default for WireGuard connectivity between hosts. Ensure this port is allowed through any infrastructure firewalls or change the port by passing the --port command option to the process.

```console
kubectl -n wormhole edit ds/wormhole
...
    spec:
      containers:
      - args:
        - controller
        - --port=9000
        command:
        - /wormhole
...
```

## Logging
Gravitational Wormhole uses logrus for logging, which are available from the deployed kubernetes pods. Additional logging can be enabled by turning on debug logging.

**Warning:** Debug logging may leak shared secrets to the logs. See [Rotating Secrets](#rotating-secrets) on how to rotate secrets.

```console
kubectl -n wormhole edit ds/wormhole
...
    spec:
      containers:
      - args:
        - controller
        - --debug
        command:
        - /wormhole
...
```

## CRDs (Wormhole node object)
Wormhole uses a Wgnode object to advertise node configuration to other cluster members. Inspect the node object for problems.

```console
kubectl -n wormhole get wgnode -o yaml
apiVersion: v1
items:
- apiVersion: wormhole.gravitational.io/v1beta1
  kind: Wgnode
  metadata:
    name: kevin-test3
    namespace: wormhole
  status:
    endpoint: 10.162.0.5
    node_cidr: 10.20.2.0/24
    port: 9806
    public_key: zE4iLxHuYgRz+RmFHG2ePr1ma4hrSINg0INH5OItb0o=
- apiVersion: wormhole.gravitational.io/v1beta1
  kind: Wgnode
  metadata:
    name: kevin-test4
    namespace: wormhole
  status:
    endpoint: 10.162.0.4
    node_cidr: 10.20.1.0/24
    port: 9806
    public_key: auk1K9HFMsVBkyGoPjmViK//YMX+cdF/VK4I6alfyxM=
- apiVersion: wormhole.gravitational.io/v1beta1
  kind: Wgnode
  metadata:
    name: kevin-test5
    namespace: wormhole
  status:
    endpoint: 10.162.0.3
    node_cidr: 10.20.0.0/24
    port: 9806
    public_key: 8Vk+L/NJDvtRoLnfjxnTbFhEmWnbi2j3Rk+6xupXZSc=
kind: List
metadata:
  resourceVersion: ""
  selfLink: ""
```


## Rotating Secrets
All secrets are rotated automatically on each process start. Restart the wormhole controller on each node to rotate all secrets.

**Warning:** this may produce a short network interruption as the process is started and re-configures the network with new secrets. The node can optionally be drained before restarting the process.

```console
kubectl -n wormhole delete po -l k8s-app=wormhole
```




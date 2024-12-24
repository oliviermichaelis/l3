# L3

L3 (derived from its working title "Linkerd-Least-Latency") is an adaptive latency-aware load-balancing mechanism for multi-cluster service meshes.

This repository contains a reference implementation used in the [L3: Latency-aware Load Balancing in Multi-Cluster Service Mesh](https://dl.acm.org/doi/10.1145/3652892.3654793) paper.

## Concepts

### Service Mesh Interface (SMI)

L3 is built on top of the [Service Mesh Interface specification](https://smi-spec.io/), which was in the meantime superseded by the [Kubernetes Gateway API](https://kubernetes.io/blog/2023/08/29/gateway-api-v0-8/) starting from `v0.8.0`.

The SMI `TrafficSplit` is used by L3 to distribute traffic between multiple backends.

### Controller

A control-loop watches `TrafficSplits` which were labelled with a matching `least-latency.oliviermichaelis.dev/controlled-by` label.
For every matched `TrafficSplit`, L3 starts collecting data-plane metrics and updating the backend weights accordingly.

## Building

You can build and run the binary locally with

```sh
$ go build -o l3
$ l3 --controlled-by=least-latency
```

### Container image with Docker

To build the container image for L3, run

```sh
$ docker build -t l3:latest .
```

## Installation

Before invoking `make`, ensure that the correct Kubernetes context is set.

```sh
# To install the CustomResourceDefinitions for L3
make install

# To deploy L3 to Kubernetes
make deploy
```

### `TrafficSplit` example

The following is an example for a `TrafficSplit` that distributes traffic between two backends.
A L3 instance started with the `--controlled-by=least-latency` argument will start watching the `TrafficSplit` and attempt to change the request distribution to minimize overall latency.

```yaml
apiVersion: split.smi-spec.io/v1alpha1
kind: TrafficSplit
metadata:
  name: example
  namespace: example
  labels:
    least-latency.oliviermichaelis.dev/controlled-by: least-latency
spec:
  service: example
  backends:
  - service: example-east
    weight: 1
  - service: example-west
    weight: 1
```

## Infrastructure

We were limited to service mesh implementations supporting the SMI specification.
Therefore, L3's reference implementation is designed to work with [Linkerd](https://linkerd.io/).
However, the reference implementation currently doesn't support other service mesh implementations, as there has to be some service-mesh-specific code to collect data-plane metrics.

While L3 also works within a single cluster, its full potential is realized in a multi-cluster service mesh.
You can find Linkerd's documentation for multi-cluster setups [here](https://linkerd.io/2.11/tasks/installing-multicluster/).

### AWS benchmark environment

To evaluate L3's performance, we used a multi-region environment provisioned with AWS EKS and EC2.
You can find the Infrastructure-as-Code instructions in the [oliviermichaelis/l3-infrastructure](https://github.com/oliviermichaelis/l3-infrastructure) repository.

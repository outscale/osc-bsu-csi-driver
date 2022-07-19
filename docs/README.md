# Outscale Cloud Controller Manager (CCM)

The Outscale Cloud Controller Manager (cloud-provider-osc) provides the interface between a Kubernetes cluster and 3DS Outscale service APIs. 
This project is necessary for operating the cluster.


More details on [cloud-controller role](https://kubernetes.io/docs/concepts/architecture/cloud-controller/) in Kubernetes architecture.

# Features
- Node controller: provides Kubernetes details about nodes (Outscale Virtual Machines)
- Service controller: allows cluster user to expose Kubernetes Services using Outscale Load Balancer Unit (LBU) 

# Installation
See [deploy documentation](../deploy/README.md)

# Usage

Some example of services:
- [2048](../examples/2048)
- [simple-lb](../examples/simple-lb)
- [simple-internal](../examples/simple-internal)
- [advanced-lb](../examples/advanced-lb)

Services can be annotated to adapt behavior and configuration of Load Balancer Units.
Check [annotation documentation](../docs/annotations.md) for more details.

# Contributing

For new feature request or bug fixes, please [create an issue](https://github.com/outscale-dev/cloud-provider-osc/issues).

If you want to dig into cloud-provider-osc, check [development documentation](development.md).
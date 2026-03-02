# metal-operator
[![REUSE status](https://api.reuse.software/badge/github.com/ironcore-dev/metal-operator)](https://api.reuse.software/info/github.com/ironcore-dev/metal-operator)
[![Go Report Card](https://goreportcard.com/badge/github.com/ironcore-dev/metal-operator)](https://goreportcard.com/report/github.com/ironcore-dev/metal-operator)
[![GitHub License](https://img.shields.io/static/v1?label=License&message=Apache-2.0&color=blue)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](https://makeapullrequest.com)

`metal-operator` is a Kubernetes operator for automating bare metal server discovery and provisioning.

## Description

Metal-operator is a project built using Kubebuilder and controller-runtime to facilitate the discovery and provisioning 
of bare metal servers. It provides a robust and scalable solution for managing bare metal infrastructure, ensuring 
seamless integration and automation within Kubernetes environments.

## Getting Started

### Prerequisites
- go version v1.22.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/metal-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified. 
And it is required to have access to pull the image from the working environment. 
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/metal-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin 
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following are the steps to build the installer and distribute this project to users.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/metal-operator:tag
```

NOTE: The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

2. Using the installer

Users can just run kubectl apply -f <URL for YAML BUNDLE> to install the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/metal-operator/<tag or branch>/dist/install.yaml
```

## Monitoring

The metal-operator exposes custom Prometheus metrics for monitoring server state, power operations, and reconciliation performance. Metrics are available at the `/metrics` endpoint and include:

- **Server State Distribution** - Count of servers by state (Available, Reserved, Error, etc.)
- **Server Power State** - Count of servers by power state (On, Off, PoweringOn, etc.)
- **Server Conditions** - Health status of server conditions (Ready, Discovered, etc.)
- **Reconciliation Metrics** - Success/error counts for reconciliation operations

For detailed metrics documentation, example queries, and alerting rules, see [docs/metrics.md](docs/metrics.md).

### Quick Example

```bash
# Port-forward to metrics endpoint
kubectl -n metal-operator-system port-forward deployment/metal-operator-controller-manager 8443:8443

# Query server metrics
curl -k https://localhost:8443/metrics | grep metal_server
```

## Contributing

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## Licensing

Copyright 2025 SAP SE or an SAP affiliate company and IronCore contributors. Please see our [LICENSE](LICENSE) for
copyright and license information. Detailed information including third-party components and their licensing/copyright
information is available [via the REUSE tool](https://api.reuse.software/info/github.com/ironcore-dev/metal-operator).

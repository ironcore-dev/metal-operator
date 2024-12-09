# metalctl

## Installation

Install the `metalctl` CLI from source without cloning the repository. Requires [Go](https://go.dev) to be installed.

```bash
go install https://github.com/ironcore-dev/metal-operator/cmd/metalctl@latest
```

## Commands

### console

The `metalctl console` command allows you to access the serial console of a `Server`.

To open a connection to the `Servers` serial console run

```bash
metalctl console my-server
```

In order to authenticate against the API server you need either to provide a path to a `kubeconfig` via `--kubeconfig`
or set the `KUBECONFIG` environment variable by pointing to an effective `kubeconfig` file.

By default, the serial console on `ttyS1` will be opened. You can override this by setting `--serial-console-number`.

### move

The `metalctl move` command allows to move the metal Custom Resources, like e.g. `Endpoint`, `BMC`, `Server`, etc. from one
cluster to another.

> Warning!:
> Before running `metalctl move`, the user should take care of preparing the target cluster, including also installing
> all the required Custom Resources Definitions.

You can use:

```bash
metalctl move --source-kubeconfig="path-to-source-kubeconfig.yaml" --target-kubeconfig="path-to-target-kubeconfig.yaml"
```
to move the metal Custom Resources existing in all namespaces of the source cluster. In case you want to move the metal
Custom Resources defined in a single namespace, you can use the `--namespace` flag.

Status and ownership of a metal Custom Resource is also moved. If a metal Custom Resource present on the source cluster
exists on the target cluster with identical specification it won't be moved and no ownership of this object will be
set. In case of any errors during the process there will be performed a cleanup and the target cluster will be restored
to its previous state.

> Warning!: 
`metalctl move` has been designed and developed around the bootstrap use case described below, and currently this is
the only use case verified .
>
>If someone intends to use `metalctl move` outside of this scenario, it's recommended to set up a custom validation
pipeline of it before using the command on a production environment.
>
>Also, it is important to notice that move has not been designed for being used as a backup/restore solution and it has
several limitation for this scenario, like e.g. the implementation assumes the cluster must be stable while doing the
move operation, and possible race conditions happening while the cluster is upgrading, scaling up, remediating etc. has
never been investigated nor addressed.

#### Pivot

Pivoting is a process for moving the Custom Resources and install Custom Resource Definitions from a source cluster to
a target cluster.
 
This can now be achieved with the following procedure:

1. Use `make install` to install the metal Custom Resource Definitions into the target cluster
2. Use `metalctl move` to move the metal Custom Resources from a source cluster to a target cluster

#### Dry run

With `--dry-run` option you can dry-run the move action by only printing logs without taking any actual actions. Use
`--verbose` flag to enable verbose logging.

# metalctl

## Installation

Install the `metalctl` CLI from source without cloning the repository. Requires [Go](https://go.dev) to be installed.

```bash
go install https://github.com/ironcore-dev/metal-operator/cmd/metalctl@latest
```

## Commands

### visualizer, vis

The `metalctl visualizer` (or `metalctl viz`) command allows you to visualize the topology of your bare metal `Server`s.

To run the visualizer run

```bash
metalctl visualizer --kubeconfig="path-to-kubeconfig.yaml"
```

In order to access the 3D visualization, open your browser and navigate to `http://localhost:8080`.

You can configure the port by setting the `--port` flag.

#### Visualizer with Enrichment Data

When `ServerMetadata` resources contain enrichment data (populated by external controllers), the visualizer
automatically displays additional information. Enrichment is optional — servers without it are displayed
normally with only their base topology data.

**Location drill-down**: Use the Site, Building, and Room dropdowns in the top-right filter panel to
narrow the 3D view to a specific datacenter location. For example, select `DC1 > Building-A > Room-101`
to see only the servers in that room. Servers without location enrichment are always visible regardless
of the selected filters.

**Hardware details panel**: Click any server in the 3D view to open a side panel with detailed information:
- System info: manufacturer, model, serial number, UUID
- BIOS: vendor and version
- CPU: model, total sockets, cores, and threads
- Memory: total capacity and module count
- Storage: total capacity and device count
- Network: interface count, upstream switch and port (from enrichment)
- Location: full hierarchy path (site, building, room, rack, position)
- Asset info: asset tag, owner, purchase date
- External system: link back to the source system (e.g., Netbox device page)

**Enhanced tooltips**: Hover over any server to see a compact summary including hardware specs
(CPU model, memory, storage), the location breadcrumb (e.g., `DC1 > Building-A > Room-101`),
upstream network connectivity, and asset tag.

**Custom enrichment keys**: Any additional keys written to `ServerMetadata.Enrichment` beyond the
well-known keys are displayed in the details panel under a raw key-value listing. This allows
custom metadata to be visible without any visualizer changes.

Enrichment data is read from the `ServerMetadata.Enrichment` field using well-known keys defined in
`api/v1alpha1/constants.go`. See the [Server Enrichment](../concepts/server-enrichment.md) documentation
for details on populating enrichment data and building enrichment controllers.

**Example**: To test with manual enrichment data:

```bash
kubectl patch servermetadata my-server --type=merge -p '{
  "enrichment": {
    "datacenter.location/site": "DC1",
    "datacenter.location/building": "Building-A",
    "datacenter.location/room": "Room-101",
    "datacenter.location/rack": "Rack-5",
    "datacenter.location/position": "U12",
    "network.topology/upstream-switch": "sw-tor-01",
    "network.topology/upstream-port": "Ethernet1/1",
    "asset.management/asset-tag": "ASSET-12345"
  }
}'
```

After patching, refresh the visualizer at `http://localhost:8080`. The patched server should now
show location filters, enriched tooltips, and a full hardware details panel when clicked.

### console

The `metalctl console` command allows you to access the serial console of a `Server`.

To open a connection to the `Servers` serial console run

```bash
metalctl console my-server
```

In order to authenticate against the API server you need either to provide a path to a `kubeconfig` via `--kubeconfig`
or set the `KUBECONFIG` environment variable by pointing to an effective `kubeconfig` file.

By default, the serial console on `ttyS1` will be opened. You can override this by setting `--serial-console-number`.

Additionally, you can skip the host validation by providing the `--skip-host-key-validation=true` flag. If set to `false`
it is possible provide a custom `known_hosts` file via the `--known-hosts-file` flag.

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

# Server Enrichment

Server enrichment allows external systems to cache additional metadata on `ServerMetadata` resources. This enables
operators of tools like [Netbox](https://netbox.dev/), ServiceNow, or custom CMDBs to augment bare metal servers
with location, asset management, and network topology information — without modifying the metal-operator itself.

## Architecture

The metal-operator provides the structure; user-supplied controllers provide the data.

```
┌──────────────────────┐     ┌───────────────────────────┐
│   External Systems   │     │     metal-operator         │
│                      │     │                            │
│  ┌────────────────┐  │     │  ┌──────────────────────┐  │
│  │     Netbox     │  │     │  │       Server         │  │
│  └────────┬───────┘  │     │  └──────────┬───────────┘  │
│           │          │     │             │ same name    │
│  ┌────────┴───────┐  │     │  ┌──────────▼───────────┐  │
│  │  ServiceNow    │  │     │  │   ServerMetadata     │  │
│  └────────┬───────┘  │     │  │                      │  │
│           │          │     │  │  systemInfo: ...      │  │
│  ┌────────┴───────┐  │     │  │  cpu: [...]          │  │
│  │  Custom CMDB   │  │     │  │  memory: [...]       │  │
│  └────────┬───────┘  │     │  │  enrichment:         │  │
│           │          │     │  │    "location/site":   │  │
└───────────┼──────────┘     │  │      "DC1"           │  │
            │                │  └──────────┬───────────┘  │
   ┌────────▼───────────┐    │             │              │
   │  Your Enrichment   │    │  ┌──────────▼───────────┐  │
   │    Controller      ├────┤► │    Visualizer        │  │
   │ (watches Server,   │    │  │  (reads enrichment,  │  │
   │  patches metadata) │    │  │   filters by site,   │  │
   └────────────────────┘    │  │   shows hardware)    │  │
                             │  └──────────────────────┘  │
                             └───────────────────────────┘
```

Key points:

- **metal-operator** discovers hardware and populates `ServerMetadata` fields like `systemInfo`, `cpu`, `memory`, etc.
- **Enrichment controllers** (user-provided) watch `Server` resources and write to `ServerMetadata.Enrichment`.
- **Visualizer** reads both hardware and enrichment data to display a rich server topology.
- The `Server` and its `ServerMetadata` share the same name and are linked by an owner reference.

## Enrichment Field

The `Enrichment` field on `ServerMetadata` is a `map[string]string`:

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: ServerMetadata
metadata:
  name: my-server      # matches Server name
enrichment:
  datacenter.location/site: "DC1"
  datacenter.location/building: "Building-A"
  datacenter.location/room: "Room-101"
  datacenter.location/rack: "Rack-5"
  datacenter.location/position: "U12"
  network.topology/upstream-switch: "sw-tor-01"
  network.topology/upstream-port: "Ethernet1/1"
  asset.management/asset-tag: "ASSET-12345"
  external.system/name: "netbox"
  external.system/url: "https://netbox.example.com/dcim/devices/42/"
  external.system/sync-at: "2025-03-28T10:15:30Z"
```

Enrichment is optional. Servers without enrichment data continue to work normally — the visualizer gracefully
degrades to showing only hardware discovery data.

## Well-Known Enrichment Keys

Keys follow a hierarchical naming convention: `domain.category/attribute`. The metal-operator defines standard
keys in `api/v1alpha1/constants.go` for interoperability with the visualizer and other tools.

### Datacenter Location

| Constant | Key | Example | Visualizer |
|---|---|---|---|
| `EnrichmentLocationSite` | `datacenter.location/site` | `"DC1"` | Site filter dropdown |
| `EnrichmentLocationBuilding` | `datacenter.location/building` | `"Building-A"` | Building filter dropdown |
| `EnrichmentLocationRoom` | `datacenter.location/room` | `"Room-101"` | Room filter dropdown |
| `EnrichmentLocationRack` | `datacenter.location/rack` | `"Rack-5"` | Details panel |
| `EnrichmentLocationPosition` | `datacenter.location/position` | `"U12"` | Details panel |

### Network Topology

| Constant | Key | Example | Visualizer |
|---|---|---|---|
| `EnrichmentNetworkUpstreamSwitch` | `network.topology/upstream-switch` | `"sw-tor-01"` | Tooltip, details panel |
| `EnrichmentNetworkUpstreamPort` | `network.topology/upstream-port` | `"Ethernet1/1"` | Tooltip, details panel |

### Asset Management

| Constant | Key | Example | Visualizer |
|---|---|---|---|
| `EnrichmentAssetTag` | `asset.management/asset-tag` | `"ASSET-12345"` | Tooltip, details panel |
| `EnrichmentAssetPurchaseDate` | `asset.management/purchase-date` | `"2024-01-15"` | Details panel |
| `EnrichmentAssetOwner` | `asset.management/owner` | `"team-infra"` | Details panel |

### External System Metadata

| Constant | Key | Example | Visualizer |
|---|---|---|---|
| `EnrichmentExternalSystemID` | `external.system/id` | `"42"` | Details panel |
| `EnrichmentExternalSystemName` | `external.system/name` | `"netbox"` | Details panel |
| `EnrichmentExternalSystemURL` | `external.system/url` | `"https://netbox.example.com/..."` | Details panel (link) |
| `EnrichmentExternalSystemSyncAt` | `external.system/sync-at` | `"2025-03-28T10:15:30Z"` | Details panel |

## Creating an Enrichment Controller

An enrichment controller watches `Server` resources, fetches metadata from an external system, and patches the
corresponding `ServerMetadata.Enrichment` field. Below is a minimal example:

```go
package controller

import (
	"context"
	"fmt"
	"time"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type NetboxEnrichmentReconciler struct {
	client.Client
	NetboxURL string
	NetboxToken string
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermetadata,verbs=get;list;watch;update;patch

func (r *NetboxEnrichmentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the Server
	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, req.NamespacedName, server); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Only enrich servers that have completed discovery
	if server.Status.State != metalv1alpha1.ServerStateAvailable &&
		server.Status.State != metalv1alpha1.ServerStateReserved {
		return ctrl.Result{}, nil
	}

	// Fetch corresponding ServerMetadata
	metadata := &metalv1alpha1.ServerMetadata{}
	if err := r.Get(ctx, types.NamespacedName{Name: server.Name}, metadata); err != nil {
		if apierrors.IsNotFound(err) {
			// ServerMetadata not yet created — requeue
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get ServerMetadata: %w", err)
	}

	// Look up the server in Netbox using the serial number
	serial := metadata.SystemInfo.SystemInformation.SerialNumber
	netboxDevice, err := lookupDeviceBySerial(ctx, r.NetboxURL, r.NetboxToken, serial)
	if err != nil {
		log.Error(err, "Failed to look up device in Netbox", "serial", serial)
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	// Build the enrichment map
	enrichment := map[string]string{
		metalv1alpha1.EnrichmentLocationSite:     netboxDevice.Site,
		metalv1alpha1.EnrichmentLocationBuilding:  netboxDevice.Building,
		metalv1alpha1.EnrichmentLocationRoom:      netboxDevice.Room,
		metalv1alpha1.EnrichmentLocationRack:      netboxDevice.Rack,
		metalv1alpha1.EnrichmentLocationPosition:  netboxDevice.Position,
		metalv1alpha1.EnrichmentAssetTag:           netboxDevice.AssetTag,
		metalv1alpha1.EnrichmentExternalSystemID:   fmt.Sprintf("%d", netboxDevice.ID),
		metalv1alpha1.EnrichmentExternalSystemName: "netbox",
		metalv1alpha1.EnrichmentExternalSystemURL:  fmt.Sprintf("%s/dcim/devices/%d/", r.NetboxURL, netboxDevice.ID),
		metalv1alpha1.EnrichmentExternalSystemSyncAt: time.Now().UTC().Format(time.RFC3339),
	}

	// Patch the enrichment field
	patch := client.MergeFrom(metadata.DeepCopy())
	if metadata.Enrichment == nil {
		metadata.Enrichment = make(map[string]string)
	}
	for k, v := range enrichment {
		if v != "" {
			metadata.Enrichment[k] = v
		}
	}

	if err := r.Patch(ctx, metadata, patch); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch ServerMetadata: %w", err)
	}

	log.Info("Enriched server metadata from Netbox", "server", server.Name)
	return ctrl.Result{RequeueAfter: 1 * time.Hour}, nil
}

func (r *NetboxEnrichmentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.Server{}).
		Complete(r)
}
```

Replace `lookupDeviceBySerial` with your actual Netbox (or other CMDB) client logic. The pattern is the same
for any external data source.

## RBAC Requirements

An enrichment controller needs read access to `Server` resources and read/write access to `ServerMetadata`:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: enrichment-controller
rules:
  - apiGroups: ["metal.ironcore.dev"]
    resources: ["servers"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["metal.ironcore.dev"]
    resources: ["servermetadata"]
    verbs: ["get", "list", "watch", "update", "patch"]
```

`ServerMetadata` is a cluster-scoped resource, so use a `ClusterRole` and `ClusterRoleBinding`.

## Visualizer Integration

The [metalctl visualizer](../usage/metalctl.md) automatically reads enrichment data from `ServerMetadata` resources.
No configuration is needed — if enrichment data exists, the visualizer displays it.

**Location filters**: When servers have `datacenter.location/*` enrichment keys, the visualizer shows Site, Building,
and Room dropdown filters in the top-right panel. Selecting a location filters the 3D view to show only servers in
that location. Servers without location data are always visible.

**Hardware details panel**: Click any server in the 3D view to open a details panel. When `ServerMetadata` exists, the
panel shows system information (manufacturer, model, serial), BIOS details, CPU/memory/storage summaries, network
interface counts, and all enrichment key-value pairs.

**Enhanced tooltips**: Hover over a server to see a compact summary. When enrichment data is available, tooltips include
hardware specs, the location breadcrumb (e.g., `DC1 > Building-A > Room-101`), upstream switch/port information, and
the asset tag.

## Testing Without a Controller

You can manually patch enrichment data to test the visualizer integration:

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
    "asset.management/asset-tag": "ASSET-12345",
    "external.system/name": "netbox",
    "external.system/url": "https://netbox.example.com/dcim/devices/42/"
  }
}'
```

Then open the visualizer (`metalctl viz --port 8080`) and verify the location filters, details panel, and tooltips
reflect the enrichment data.

To remove enrichment:

```bash
kubectl patch servermetadata my-server --type=merge -p '{"enrichment": null}'
```

## Best Practices

1. **Use well-known keys** for data that the visualizer or other standard tools consume. Custom keys are supported
   but won't be recognized by built-in UI features like location filtering.

2. **Namespace custom keys** using a domain you own: `mycompany.example.com/custom-field`. This avoids collisions
   with future well-known keys.

3. **Handle errors gracefully**. If the external system is unreachable, requeue with a backoff rather than failing
   the reconciliation loop. Set `external.system/sync-at` so operators can detect stale data.

4. **Respect rate limits** of external APIs. Use `RequeueAfter` with reasonable intervals (e.g., 1 hour) rather
   than reconciling on every event.

5. **Skip empty values**. Only write enrichment keys that have meaningful data. The visualizer ignores empty strings.

6. **Use `MergeFrom` patches** to update only the enrichment field without overwriting hardware metadata populated
   by the metal-operator.

## FAQ

**Can multiple controllers enrich the same `ServerMetadata`?**

Yes. Each controller should use `MergeFrom` patches so that updates to different keys don't overwrite each other.
Use distinct key prefixes per controller (e.g., one controller writes `datacenter.location/*` keys, another writes
`asset.management/*` keys).

**What happens if `ServerMetadata` doesn't exist yet?**

`ServerMetadata` is created by the metal-operator during server discovery. If your enrichment controller runs
before discovery completes, it should handle `NotFound` errors and requeue.

**Which servers should I enrich?**

Typically only servers in the `Available` or `Reserved` state. Servers in `Initial` or `Discovery` may not have a
`ServerMetadata` resource yet.

**How do I remove enrichment data?**

Patch the `enrichment` field to `null` or remove individual keys. The visualizer gracefully degrades when enrichment
data is absent.

**Is there a size limit on enrichment data?**

The enrichment map is stored as part of the `ServerMetadata` resource. Kubernetes has a default 1.5 MB size limit
for etcd objects. In practice, keep enrichment data concise — a few dozen key-value pairs per server is typical.

**Is the Netbox integration built into metal-operator?**

No. The metal-operator provides the `Enrichment` field and well-known key constants. Integrations with Netbox,
ServiceNow, or any other system are implemented as separate controllers that you deploy alongside the metal-operator.

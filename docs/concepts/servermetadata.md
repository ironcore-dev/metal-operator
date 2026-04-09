# ServerMetadata

The `ServerMetadata` Custom Resource Definition (CRD) is a cluster-scoped resource that persists hardware discovery
data collected during the [Server](servers.md) Discovery phase. It stores information such as network interfaces,
LLDP neighbors, CPUs, storage devices, memory, NICs, and PCI devices.

`ServerMetadata` is created automatically by the `ServerReconciler` when a server completes Discovery. It is owned
by its corresponding `Server` via an owner reference, so it is garbage-collected when the `Server` is deleted.

## Purpose

Server discovery data lives in the `Server` status subresource, which can be lost when a `Server` resource is
recreated or its status is reset. `ServerMetadata` solves this by storing discovery data at the resource root
level (not in a status subresource), providing a durable record of hardware information.

When a `Server` has an empty status, the `ServerReconciler` checks for an existing `ServerMetadata` with the
same name. If found, the server's network interfaces are restored from the metadata and the server transitions
directly to `Available` (or `Reserved` if a `ServerClaimRef` is set), skipping the full Discovery cycle.

## Example ServerMetadata Resource

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: ServerMetadata
metadata:
  name: my-server
  ownerReferences:
    - apiVersion: metal.ironcore.dev/v1alpha1
      kind: Server
      name: my-server
      uid: <server-uid>
systemInfo:
  biosInformation:
    vendor: "AMI"
    version: "1.0.0"
    date: "01/01/2024"
  systemInformation:
    manufacturer: "Contoso"
    productName: "3500"
    serialNumber: "437XR1138R2"
  boardInformation:
    manufacturer: "Contoso"
    product: "Board-3500"
networkInterfaces:
  - name: eth0
    macAddress: "aa:bb:cc:dd:ee:ff"
    ipAddresses:
      - "192.168.1.100"
    carrierStatus: "up"
cpus:
  - architecture: "x86_64"
    modelName: "Intel Xeon E5-2680 v4"
    totalCores: 14
    totalThreads: 28
blockDevices:
  - name: "/dev/sda"
    type: "disk"
    size: 960197124096
```

## Lifecycle

- **Created**: Automatically by the `ServerReconciler` when Discovery completes, before the server transitions
  to `Available`.
- **Updated**: On each subsequent discovery, the `ServerMetadata` is updated with the latest hardware data.
- **Deleted**: Automatically via owner reference garbage collection when the parent `Server` is deleted, or
  explicitly when a `rediscover` annotation is applied (see [Rediscover](#rediscover)).

## Status Restoration

When the `ServerReconciler` encounters a `Server` with an empty `Status.State`:

1. It looks up a `ServerMetadata` resource with the same name as the server.
2. If found, it restores `Status.NetworkInterfaces` from the metadata.
3. If `Spec.ServerClaimRef` is set, the server transitions to `Reserved`; otherwise to `Available`.
4. The `ServerClaimReconciler` similarly detects bidirectional binding (`ServerClaim.Spec.ServerRef` ↔
   `Server.Spec.ServerClaimRef`) and restores the claim's phase to `Bound`.

This allows servers and their claims to resume normal operation without rediscovery.

## Rediscover

To force a full rediscovery of a server's hardware, apply the `rediscover` operation annotation:

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: Server
metadata:
  name: my-server
  annotations:
    metal.ironcore.dev/operation: rediscover
```

When the `ServerReconciler` processes this annotation, it:

1. Deletes the associated `ServerMetadata` resource.
2. Removes the annotation from the server.
3. Transitions the server back to the `Initial` state, triggering a full Discovery cycle.

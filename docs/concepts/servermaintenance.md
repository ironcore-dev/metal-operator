# ServerMaintenance

> **Deprecated:** `metal.ironcore.dev/v1alpha1 ServerMaintenance` is deprecated and will be removed in a future release.
> Use `servermaintenance.metal.ironcore.dev/v1alpha1 ServerMaintenance` instead, provided by `maintenance-operator`.
> See the [maintenance-operator documentation](../../maintenance-operator/docs/concepts/servermaintenance.md) for the current API.

`ServerMaintenance` represents a maintenance operation for a physical server. It transitions a `Server` from its
current operational state (e.g., Available/Reserved) into a Maintenance state. Each `ServerMaintenance` object tracks
the lifecycle of a maintenance task, ensuring servers are properly taken offline, updated, and restored.

## Migration

The `ServerMaintenance` CRD has moved from `metal.ironcore.dev` to `servermaintenance.metal.ironcore.dev`, owned by
`maintenance-operator`. During the migration window, the `metal-operator` reconciler behaves as follows:

- **Pending** objects are deleted immediately. Their owner controllers (e.g. `BIOSSettings`, `BMCSettings`) will
  reconcile and recreate them under the new group.
- **InMaintenance** objects continue to be fully served — power state, boot configuration, and locator LED are
  managed to completion.
- **Deletion** and finalizer cleanup run unchanged.

New objects must be created under `servermaintenance.metal.ironcore.dev/v1alpha1`. Any object created under
`metal.ironcore.dev/v1alpha1` with state `Pending` will be deleted by the reconciler.

## Key Points

- `ServerMaintenance` is namespaced and may represent various maintenance operations.
- Only one `ServerMaintenance` can be active per `Server` at a time. Others remain pending.
- When the active `ServerMaintenance` completes, the next pending one (if any) starts.
- If no more maintenance tasks are pending, the `Server` returns to its previous operational state.
- `policy` determines how maintenance starts:
    - **OwnerApproval:** Requires a label (e.g., `ok-to-maintenance: "true"`) on the `ServerClaim`.
    - **Enforced:** Does not require owner approval.
- `priority` determines which pending maintenance starts first for the same server:
    - Higher value wins.
    - On equal value, older `ServerMaintenance` wins.
    - If omitted, `priority` is treated as `0`.

## Workflow

1. A separate operator (e.g., `maintenance-operator`) or user creates a `ServerMaintenance` resource under
   `servermaintenance.metal.ironcore.dev/v1alpha1` referencing a specific `Server`.
2. If a `Server` is claimed, a label `metal.ironcore.dev/maintenance-needed: "true"` is added to the `ServerClaim`.
3. If `policy` is `OwnerApproval` and no approval label is set on the `ServerClaim`, the `ServerMaintenance`
   stays in `Pending`. The `Server` also remains unchanged.
4. If `policy` is `OwnerApproval` and the approval label is present, or if the policy is `Enforced`,
   `maintenance-operator` transitions the `Server` into `Maintenance` and updates the `ServerMaintenance` state.
5. The `ServerMaintenanceReconciler` in `maintenance-operator` creates a `ServerBootConfiguration` from the
   `ServerBootConfigurationTemplate` and applies it to the `Server`. The power state can be set via `serverPower`.
6. (optional) If no `ServerBootConfigurationTemplate` is provided, the server is powered off and maintenance proceeds
   without a boot configuration.
7. `metal-operator` transitions the `Server` back to its prior state. If additional `ServerMaintenance` objects are
   pending, the next one is processed.

## Example

```yaml
apiVersion: servermaintenance.metal.ironcore.dev/v1alpha1
kind: ServerMaintenance
metadata:
  name: bios-update
  namespace: ops
  annotations:
    metal.ironcore.dev/reason: "BIOS update"
spec:
  priority: 100
  policy: OwnerApproval
  serverRef:
    name: server-foo
  serverPower: On # or Off
  serverBootConfigurationTemplate:
    name: bios-update-config
    spec:
      image: "bios-update-image"
      serverRef:
        name: server-foo
      ignitionSecretRef:
        name: bios-update-ignition
```

If `policy: OwnerApproval` and no approval label exists on the `ServerClaim`, this `ServerMaintenance` remains
`Pending`, and the `Server` stays as is. Once the label is added, `maintenance-operator` transitions the `Server`
to `Maintenance` and performs the maintenance task.

## Deprecated API Example

The following uses the deprecated `metal.ironcore.dev/v1alpha1` group. Objects created this way with state `Pending`
will be automatically deleted by `metal-operator` and must be recreated under the new group.

```yaml
# Deprecated — use servermaintenance.metal.ironcore.dev/v1alpha1 instead
apiVersion: metal.ironcore.dev/v1alpha1
kind: ServerMaintenance
metadata:
  name: bios-update
  namespace: ops
spec:
  serverRef:
    name: server-foo
```

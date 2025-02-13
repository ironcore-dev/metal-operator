# ServerMaintenance

`ServerMaintenance` represents a maintenance operation for a physical server. It transitions a `Server` from its 
current operational state (e.g., Available/Reserved) into a Maintenance state. Each `ServerMaintenance` object tracks
the lifecycle of a maintenance task, ensuring servers are properly taken offline, updated, and restored.

## Key Points

- `ServerMaintenance` is namespaced and may represent various maintenance operations.
- Only one `ServerMaintenance` can be active per `Server` at a time. Others remain pending.
- When the active `ServerMaintenance` completes, the next pending one (if any) starts.
- If no more maintenance tasks are pending, the `Server` returns to its previous operational state.
- `policy` determines how maintenance starts:
    - **OwnerApproval:** Requires a label (e.g., `ok-to-maintenance: "true"`) on the `ServerClaim`.
    - **Enforced:** Does not require owner approval.

## Workflow

1. A separate operator (e.g., `foo-maintenance-operator`) or user creates a `ServerMaintenance` resource referencing a 
   specific `Server`.
2. If a `Server` is claimed, a label `metal.ironcore.dev/maintanence-needed: "true"` is added to the `ServerClaim`.
3. If `policy` is `OwnerApproval` and no `ok-to-maintenance` label is set on the `ServerClaim`, the `ServerMaintenance`
   stays in `Pending`. The `Server` also remains unchanged.
4. If `policy` is `OwnerApproval` and the `ok-to-maintenance` label is present (or if `alwaysPerformMaintenance` is 
   enabled), or if the policy is `Enforced`, the `metal-operator` transitions the `Server` into `Maintenance` and 
   updates the `ServerMaintenance` state accordingly.
5. The `ServerMaintenanceReconciler` creates a `ServerBootConfiguration` out of the `ServerMaintenance`'s 
   `ServerBootConfigurationTemplate` and applies it to the `Server`. The power state of the `Server` can set by providing the
   `ServerPower` field in the `ServerMaintenance` object. Once the maintenance task is complete, the maintenance operator
   sets the `ServerMaintenance` state to `Completed`.
6. (optional) In case no `ServerBootConfigurationTemplate` is provided, the maintenance operator powers off the `Server`, 
   applies a `ServerBootConfiguration` (if needed), performs the maintenance, and sets `ServerMaintenance` to `Completed`.
7. The `metal-operator` transitions the `Server` back to its prior state. If additional `ServerMaintenance` objects are
   pending, the next one is processed.

## Example

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: ServerMaintenance
metadata:
  name: bios-update
  namespace: ops
  annotations:
    metal.ironcore.dev/reason: "BIOS update"
spec:
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
status:
  state: Pending
```

If `policy: OwnerApproval` and no `ok-to-maintenance` label exists on the `ServerClaim`, this `ServerMaintenance` 
remains `Pending`, and the `Server` stays as is. Once the label is added (or if the operator setting 
`alwaysPerformMaintenance` is enabled), the `metal-operator` transitions the `Server` to `Maintenance`, and the 
maintenance operator performs the maintenance task.

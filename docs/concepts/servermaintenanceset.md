# ServerMaintenanceSet

The `ServerMaintenanceSet` represents a higher-level Kubernetes resource designed to coordinate and manage `ServerMaintenances` across a group of servers via label selection.
It enables users to declaratively specify maintenance actions for multiple servers, and provides status tracking for all associated `ServerMaintenances`.

## Key Points

- `ServerMaintenanceSet` is namespaced and may represent various maintenance operations.
- Selection: Supports selecting target servers via label selectors.
- `ServerMaintenanceSet` shows the number of server in maintenance, pending or failed.

## Workflow

1. A separate operator (e.g., `foo-maintenance-operator`) or user creates a `ServerMaintenanceSet` resource referencing a number of servers via the `ServerSelector`.
2. A `ServerMaintenance` is created for all selected servers.
3. Follows the flow of the `ServerMaintenance`

## Example
```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: ServerMaintenanceSet
metadata:
  name: bios-update
  namespace: ops
  annotations:
    metal.ironcore.dev/reason: "BIOS update"
spec:
  serverSelector:
    matchLabels:
    hardwareType: gpu-node
    location: datacenter-1
  template:
    policy: OwnerApproval
    serverPower: On # or Off
    serverBootConfigurationTemplate:
      name: bios-update-config
      spec:
        image: "bios-update-image"
      ignitionSecretRef:
        name: bios-update-ignition
```

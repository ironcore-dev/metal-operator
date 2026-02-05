# BIOSSettings

`BIOSSettings` represents a BIOS settings update operation for a physical server (compute system). It applies BIOS settings on a physical server's BIOS in a controlled, ordered manner.

## Key Points

- `BIOSSettings` uses a `settingsFlow` to apply BIOS settings in a defined order based on priority.
- Each flow item contains a name, a map of settings, and a priority (lower numbers are applied first).
- Only one `BIOSSettings` can be active per `Server` at a time.
- `BIOSSettings` changes are applied once the BIOS version matches the specified `version`.
- `BIOSSettings` handles server reboots (if required) using a `ServerMaintenance` resource.
- Once `BIOSSettings` moves to `Failed` state, it stays in this state unless manually moved out.

## Spec Fields

| Field | Description |
|-------|-------------|
| `serverRef` | Reference to the target `Server` (immutable once set) |
| `version` | The BIOS version this settings configuration applies to |
| `settingsFlow` | List of settings flow items to apply in priority order |
| `serverMaintenancePolicy` | Policy for maintenance: `OwnerApproval` or `Enforced` |
| `serverMaintenanceRef` | Optional reference to an existing `ServerMaintenance` resource |

### SettingsFlowItem Fields

| Field | Description |
|-------|-------------|
| `name` | Name identifier for this flow item (1-1000 characters) |
| `settings` | Map of BIOS setting key-value pairs to apply |
| `priority` | Execution order (1-2147483645); lower numbers are applied first |

## Status Fields

| Field | Description |
|-------|-------------|
| `state` | Overall state: `Pending`, `InProgress`, `Applied`, or `Failed` |
| `flowState` | List of individual flow item states with their conditions |
| `lastAppliedTime` | Timestamp when the last setting was successfully applied |
| `conditions` | Standard Kubernetes conditions for the resource |

## Workflow

1. A separate operator (e.g., `BIOSSettingsSet`) or user creates a `BIOSSettings` resource referencing a specific `Server`.
2. Settings from `settingsFlow` are processed in priority order (lowest priority number first).
3. For each flow item:
   - The provided settings are checked against the current BIOS settings.
   - If settings already match, the flow item state moves to `Applied`.
   - If settings need to be updated, `BIOSSettings` checks the BIOS version. If it does not match, it waits.
4. If `ServerMaintenance` is not already provided, it requests one and waits for the server to enter `Maintenance` state.
   - The `serverMaintenancePolicy` determines how maintenance is handled (`OwnerApproval` or `Enforced`).
5. `BIOSSettings` checks if the setting update requires a physical server reboot.
6. The setting update process is started and the server is rebooted if required.
7. `BIOSSettings` verifies the settings have been applied and transitions the flow item state to `Applied`.
8. Once all flow items are applied, the overall state moves to `Applied` and the `ServerMaintenance` resource is removed if it was created by this resource.
9. Any further update to the `BIOSSettings` spec will restart the process.
10. If `BIOSSettings` fails to apply any setting, it moves to `Failed` state until manually moved out.

## Example

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: BIOSSettings
metadata:
  name: biossettings-sample
spec:
  serverRef:
    name: endpoint-sample-system-0
  version: 2.10.3
  settingsFlow:
    - name: pxe-settings
      priority: 1
      settings:
        PxeDev1EnDis: Disabled
        PxeDev2EnDis: Enabled
    - name: other-settings
      priority: 2
      settings:
        OtherSetting: "123"
        AnotherSetting: Disabled
  serverMaintenancePolicy: OwnerApproval
```

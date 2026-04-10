# BMCSettings

`BMCSettings` applies BMC manager settings for one `BMC` resource.

Unlike BIOS settings on a single host, BMC settings may require maintenance for multiple servers managed by the same BMC.

## What It Does

- Targets one BMC through `spec.BMCRef`.
- Compares desired `spec.settings` against current manager settings.
- Waits for expected BMC firmware version (`spec.version`) before applying changes.
- Requests `ServerMaintenance` for related servers when needed.
- Applies settings, performs reset/reboot handling, then verifies convergence.

## Spec Reference

| Field | Required | Description |
|---|---|---|
| `spec.BMCRef.name` | Yes | Target BMC object. Immutable after creation. |
| `spec.version` | Yes | Required BMC firmware version gate for settings apply. |
| `spec.settings` | No | Map of BMC manager settings to enforce. |
| `spec.serverMaintenancePolicy` | No | Maintenance policy for affected servers. |
| `spec.serverMaintenanceRefs[]` | No | Existing maintenance refs, typically controller-managed. |

Note: The API field is `BMCRef` (capitalized) in this CRD schema.

## Status Fields In Detail

| Field | What it means | How to use it for debugging |
|---|---|---|
| `status.state` | Lifecycle state (`Pending`, `InProgress`, `Applied`, `Failed`). | Immediate indicator of blocked prerequisites vs execution failure. |
| `status.conditions[]` | Fine-grained checkpoints: version gate, maintenance waiting/progress, reset, issue/verify results. | Primary source for error reason and where in workflow it failed. Use alongside `spec.serverMaintenanceRefs[]` to diagnose prolonged maintenance waits. |

## Detailed State Machine

```mermaid
stateDiagram-v2
    [*] --> Pending

    state Pending {
      [*] --> CheckDiff
      CheckDiff --> NoDiff: settings already match
      CheckDiff --> CheckVersion: settings differ
      CheckVersion --> WaitVersion: firmware mismatch
      CheckVersion --> RequestMaintenance: firmware matches
      RequestMaintenance --> WaitMaintenance
      WaitMaintenance --> ReadyToApply: approved
    }

    Pending --> Applied: NoDiff
    Pending --> InProgress: ReadyToApply

    state InProgress {
      [*] --> ResetPreApply
      ResetPreApply --> IssueSettingsUpdate
      IssueSettingsUpdate --> WaitApplyComplete
      WaitApplyComplete --> RebootIfNeeded
      RebootIfNeeded --> VerifySettings
      VerifySettings --> Success: desired settings observed
      VerifySettings --> FailedVerify: mismatch/timeout
      IssueSettingsUpdate --> FailedIssue: request failed
    }

    InProgress --> Applied: Success
    InProgress --> Failed: FailedIssue or FailedVerify
    Failed --> Pending: retry annotation/manual recovery
```

## Detailed Workflow (All Main Cases)

1. Intake and ownership:
  - Resolve `BMCRef` and bind BMC-side reference.
  - Ensure finalizer and ownership links are in place.
2. Diff and version gate:
  - If no settings diff exists, transition to `Applied`.
  - If diff exists and version mismatches, remain `Pending`.
3. Maintenance orchestration:
  - Discover all servers associated with the BMC.
  - Request maintenance per server (policy driven) and wait for approval.
4. Apply path:
  - Optional BMC reset to establish stable state.
  - Issue settings update and track progress via conditions.
5. Reboot/verification path:
  - Perform reset/reboot when required by vendor behavior.
  - Verify settings from BMC readback.
6. Terminalization and cleanup:
  - On success set `Applied`, on failure set `Failed`.
  - Remove self-managed maintenance references where applicable.

## Troubleshooting Guide

| Symptom | Where to check | Likely cause | Action |
|---|---|---|---|
| `Pending` with no movement | `status.conditions[]` | Firmware version gate not satisfied | Run/complete `BMCVersion` to desired version first. |
| Stuck waiting for maintenance | `spec.serverMaintenanceRefs[]`, conditions | One or more server maintenances not approved | Approve each pending server maintenance resource. |
| `InProgress` too long | conditions + BMC health | BMC reset/apply did not converge | Check BMC reachability and vendor-specific settings endpoint health. |
| `Failed` after apply | verify condition message | Unsupported key/value or readback mismatch | Validate exact vendor key names and normalized values. |
| Deletion blocked | finalizer + in-progress state | Active reconciliation and pending cleanup refs | Resolve active operation first, then retry deletion. |

## Example

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMCSettings
metadata:
  name: bmcsettings-sample
spec:
  BMCRef:
    name: endpoint-sample
  version: 1.45.455b66-rev4
  settings:
    IPMILan.1.Enable: Enabled
    SNMP.1.AgentEnable: Disabled
  serverMaintenancePolicy: Enforced
```

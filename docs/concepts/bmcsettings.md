# BMCSettings

`BMCSettings` applies BMC manager settings for one `BMC` resource.

Unlike BIOS settings on a single host, BMC settings may require maintenance for multiple servers managed by the same BMC.

## What It Does

- Targets one BMC through `spec.BMCRef`.
- Compares desired `spec.settings` against current manager settings.
- Resolves `spec.variables` references against the BMCSettings object, ConfigMaps, and Secrets before applying.
- Waits for expected BMC firmware version (`spec.version`) before applying changes.
- Requests `ServerMaintenance` for related servers when needed.
- Applies settings, performs reset/reboot handling, then verifies convergence.

## Spec Reference

| Field | Required | Description |
|---|---|---|
| `spec.BMCRef.name` | Yes | Target BMC object. Immutable after creation. |
| `spec.version` | Yes | Required BMC firmware version gate for settings apply. |
| `spec.settings` | No | Map of BMC manager settings to enforce. Values may reference variables using `$(VarName)` syntax. |
| `spec.variables[]` | No | List of named variables resolved at apply time and substituted into `spec.settings` values. Max 64 items. |
| `spec.serverMaintenancePolicy` | No | Maintenance policy for affected servers. |
| `spec.serverMaintenanceRefs[]` | No | Existing maintenance refs, typically controller-managed. |

Note: The API field is `BMCRef` (capitalized) in this CRD schema.

### Variables

`spec.variables` allows setting values to be resolved dynamically at apply time.
Each variable has a `key` (referenced as `$(key)` in settings values) and exactly one source via `valueFrom`.

Variables are resolved in list order. A variable defined later in the list can reference any earlier-resolved variable by embedding `$(PreviousVarKey)` in its `configMapKeyRef.key`, `configMapKeyRef.name`, `secretKeyRef.key`, or `secretKeyRef.name` fields — the substitution is performed *before* the lookup.

**Escape sequence**: use `$$(KEY)` to produce a literal `$(KEY)` in the output without triggering variable substitution.

Each variable uses exactly one source type (enforced by validation):
- `fieldRef` — reads a field from the BMCSettings object itself.
- `configMapKeyRef` — reads a key from a ConfigMap. The `key`, `name`, and `namespace` fields may contain `$(VarName)` substitutions.
- `secretKeyRef` — reads a key from a Secret. The `key`, `name`, and `namespace` fields may contain `$(VarName)` substitutions.

After fetching the raw value from its source, already-resolved variables are also substituted into that value before it is stored. This means a ConfigMap value of `http://$(HOST)/api` will be fully expanded if `HOST` was resolved earlier in the list.

#### `valueFrom.fieldRef`

| Field | Required | Description |
|---|---|---|
| `fieldPath` | Yes | Field path on the BMCSettings object, e.g. `spec.BMCRef.name`. Min 1, max 256 chars. |

#### `valueFrom.configMapKeyRef` / `valueFrom.secretKeyRef`

| Field | Required | Description |
|---|---|---|
| `name` | Yes | Object name. Max 253 chars. |
| `namespace` | Yes | Object namespace. Max 63 chars. |
| `key` | Yes | Key within the object. May contain `$(VarName)` substitutions. Max 253 chars. |

Validation guarantees:
- Exactly one of `fieldRef`, `configMapKeyRef`, or `secretKeyRef` per variable.
- Variable `key` values must be unique within the list.
- Variable `key` is 1–63 characters.

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
      [*] --> CheckVersion
      CheckVersion --> WaitVersion: firmware mismatch
      CheckVersion --> Advancing: firmware matches
    }

    Pending --> InProgress: Advancing

    state InProgress {
      [*] --> ResolveAndDiff
      ResolveAndDiff --> NoDiff: settings already match
      ResolveAndDiff --> RequestMaintenance: settings differ
      RequestMaintenance --> WaitMaintenance
      WaitMaintenance --> ResetPreApply: approved
      ResetPreApply --> IssueSettingsUpdate
      IssueSettingsUpdate --> RebootIfNeeded: vendor reset required
      IssueSettingsUpdate --> VerifySettings: no reset needed
      RebootIfNeeded --> VerifySettings
      VerifySettings --> Success: desired settings observed
      VerifySettings --> StillInProgress: mismatch/not yet converged
      IssueSettingsUpdate --> FailedIssue: request failed
      ResolveAndDiff --> FailedResolve: missing source
    }

    InProgress --> Applied: NoDiff or Success
    InProgress --> Failed: FailedIssue or FailedResolve

    state Applied {
      [*] --> DriftCheck
      DriftCheck --> Drifted: settings differ from desired
      DriftCheck --> Stable: no change detected
    }

    Applied --> Pending: Drifted
    Failed --> Pending: retry annotation/manual recovery
```

## Detailed Workflow (All Main Cases)

1. **Intake and ownership** (`Pending`):
   - Resolve `BMCRef` and bind BMC-side reference.
   - Ensure finalizer and ownership links are in place.
2. **Version gate** (`Pending`):
   - If BMC firmware version mismatches `spec.version`, remain `Pending`.
   - Once version matches, transition unconditionally to `InProgress`.
3. **Diff and variable resolution** (`InProgress`):
   - Resolve `spec.variables` in list order, substituting already-resolved variables into each source selector before lookup.
   - Substitute `$(VarName)` placeholders into `spec.settings` values to build the effective settings map.
   - Compare effective settings against current BMC values. If no diff, transition to `Applied`.
   - If variable resolution fails (missing ConfigMap/Secret/field), transition to `Failed`.
4. **Maintenance orchestration** (`InProgress`):
   - Discover all servers associated with the BMC.
   - Request `ServerMaintenance` per server (policy-driven) and wait for approval.
5. **Apply path** (`InProgress`):
   - Pre-apply BMC reset to establish a stable state.
   - Issue settings update and track progress via conditions.
6. **Reboot/verification path** (`InProgress`):
   - Post-apply BMC reset/reboot when required by vendor behavior.
   - Re-run variable resolution and diff check to verify convergence.
   - Transition to `Applied` when diff is empty, otherwise stay `InProgress` and retry.
7. **Drift detection** (`Applied`):
   - On each reconcile, re-resolve variables and re-check diff against the BMC.
   - If drift is detected, transition back to `Pending` for a new apply cycle.
8. **Terminalisation and cleanup** (`Applied`):
   - Remove self-managed maintenance references.
   - On failure, set `Failed`; on retry annotation, reset to `Pending`.

## Troubleshooting Guide

| Symptom | Where to check | Likely cause | Action |
|---|---|---|---|
| `Pending` with no movement | `status.conditions[]` | Firmware version gate not satisfied | Run/complete `BMCVersion` to desired version first. |
| Stuck waiting for maintenance | `spec.serverMaintenanceRefs[]`, conditions | One or more server maintenances not approved | Approve each pending server maintenance resource. |
| `InProgress` too long | conditions + BMC health | BMC reset/apply did not converge | Check BMC reachability and vendor-specific settings endpoint health. |
| `Failed` after apply | verify condition message | Unsupported key/value or readback mismatch | Validate exact vendor key names and normalized values. |
| `Failed` on variable resolution | conditions | Missing ConfigMap/Secret or wrong key | Check that all referenced objects and keys exist in the correct namespace. |
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
  serverMaintenancePolicy: Enforced
  settings:
    BootMode: "UEFI"
    HyperThreading: "Enabled"
    LicenseKey: "$(LicenseKey)"
    FQDN: "$(BmcName).$(SearchDomain)"
  variables:
    - key: BmcName
      valueFrom:
        fieldRef:
          fieldPath: spec.BMCRef.name
    - key: SearchDomain
      valueFrom:
        configMapKeyRef:
          name: bmc-network-config
          namespace: metal-system
          key: search-domain
    - key: LicenseKey
      valueFrom:
        secretKeyRef:
          name: bmc-licenses
          namespace: metal-system
          key: $(BmcName)
```

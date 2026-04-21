# BMCSettings / BMCVersion Redesign — SettingsFlow, VersionSelector, ReadinessGates, List-Ref

<!-- SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

This document describes the full redesign of the `BMCSettings`, `BMCSettingsSet`, `BMCVersion`, and `BMCVersionSet` APIs introduced in metal-operator.

## Problem Statement

The original `BMCSettings` API has several structural limitations:

| Problem | Impact |
|---|---|
| Single `spec.version` string gate | Only one firmware version can be targeted; no way to have a fresh-install vs upgrade path |
| Flat `spec.settingsMap` | No phased application; all settings applied atomically in one maintenance window |
| `bmc.spec.bmcSettingRef` is a single pointer | If two `BMCSettingsSet` objects stamp the same BMC simultaneously, one overwrites the other (silent data loss) |
| Broken version comparison (`<` on opaque strings) | Vendor version strings are not semver-comparable; the `<` guard produces undefined ordering |
| No prerequisite sequencing across resources | A `BMCVersion` upgrade cannot wait for prerequisite `BMCSettings` to complete |
| `BMCSettingsSet` cannot filter by firmware version | All matching BMCs receive settings regardless of their current firmware |

---

## Design Decisions

### 1. `SettingsFlow` replaces `SettingsMap` + `Version`

`spec.settingsMap` and `spec.version` are replaced by `spec.settingsFlow: []BMCSettingsFlowItem`. Each item has a `name`, a `priority` (ordering within the flow), and its own `settings` map. Items are applied in ascending `priority` order, each in its own maintenance window.

**Why:** Enables phased application (e.g., safe defaults first, then aggressive tuning), and decouples the target version from individual setting groups. Version filtering moves to the Set level.

### 2. `VersionSelector` on `BMCSettingsSet`

`BMCSettingsSetSpec` gains a `versionSelector.versions: []string` field. When set, only BMCs whose `status.firmwareVersion` exactly matches one of the listed strings receive a stamped `BMCSettings` child.

**Why:** BMC firmware version strings are opaque, vendor-defined free-form strings (e.g., HPE `iLO 6 v1.70`, Dell `5.00.00.00`, Lenovo `TEI392M-2.50`). Semver parsing is impossible; exact-match is the only safe strategy. Filtering at the Set level is more maintainable than per-object gates.

### 3. `bmc.spec.bmcSettingRefs []` list replaces single pointer

`bmc.spec.bmcSettingRef *LocalObjectReference` is replaced by `bmc.spec.bmcSettingRefs []LocalObjectReference`. Each `BMCSettings` controller appends/removes its own entry idempotently.

**Why:** With multiple `BMCSettingsSet` objects targeting the same BMC, a single pointer causes a race: two controllers both write `bmcSettingRef`, and the last writer wins. A list eliminates the conflict entirely.

### 4. `ReadinessGates` on `BMCSettingsTemplate` and `BMCVersionTemplate`

Both templates gain `readinessGates: []ReadinessGate`. A gate specifies an external resource (`apiVersion/kind/name`) and a `conditionType` that must be `True` before the resource proceeds past `Pending`. Gate `scope: SetChild` is a special mode where the controller resolves the actual child object at runtime by scanning `ownerReferences` — enabling a `BMCVersionSet`-stamped gate to reference a `BMCSettingsSet` by name without knowing the generated child's name.

**Why:** Enables declarative prerequisite ordering across resource types (e.g., "apply these settings before updating firmware") without coupling controllers.

### 4a. `BMCVersion` drift detection and `Completed` re-evaluation

`BMCVersion` targets firmware at a specific version. Unlike software deployments, BMC firmware can regress (factory reset, emergency rollback). The controller must handle this gracefully.

**Terminal-state semantics for `Completed`:**

`Completed` is *conditionally* terminal. On every reconcile of a `Completed` `BMCVersion` the controller re-checks two conditions:

1. **Gates still blocked** (`any gate unsatisfied`) → stay `Completed`. The BMC is either already ahead of this hop or behind the point where this hop's self-parking gate triggers. No action.
2. **All gates satisfied AND `bmc.status.firmwareVersion != spec.version`** → reset to `Pending`. The BMC has regressed to this hop's exact activation point; re-run the upgrade.

No version string comparison is involved — the **self-parking gate** (a `fieldMatch` on `bmc.status.firmwareVersion == <source-version>`) acts as the "should this hop activate" predicate. Because firmware version strings are opaque vendor strings, ordering comparisons (`>=`, `<`) are not safe; only exact equality is used.

**Why this is safe:**

Each hop's self-parking gate checks for exactly the source version of that hop. When a BMC regresses to version X, only the one hop whose self-parking gate matches X will see all gates satisfied. Every other hop sees its self-parking gate as unsatisfied, so it stays `Completed` undisturbed. The result is that only the correct hop re-activates — all neighbours in the chain remain frozen.

**Example — full chain 4.x → 5.x → 6.x → 7.x, BMC factory-reset to 5.x:**

| Hop | Target | Self-parking gate | Gate satisfied? | BMC fw ≠ target? | Action |
|-----|--------|-------------------|-----------------|-----------------|--------|
| 1 (→5.x) | 5.x | `fw == 4.x` | ✗ (fw is 5.x) | — | stay `Completed` |
| 2 (→6.x) | 6.x | `fw == 5.x` | ✓ | 5.x ≠ 6.x → ✓ | reset → `Pending` → upgrades |
| 3 (→7.x) | 7.x | `fw == 6.x` | ✗ (fw is 5.x) | — | stay `Completed` |

After hop-2 upgrades BMC to 6.x, hop-3's self-parking gate becomes satisfied → it resets and runs, completing the cascade back to 7.x.

**Why not `bmc.firmwareVersion >= spec.version` to short-circuit?**

Firmware version strings are opaque (`"iLO 6 v1.70"`, `"5.00.00.00"`, `"TEI392M-2.50"`). Semantic version parsing is not guaranteed. The self-parking gate's exact-match predicate is the only safe comparator; the controller should not attempt its own version ordering.



When multiple `BMCSettings` target the same BMC, the controller serializes them using two layers — mirroring how `ServerMaintenance` works, but without a priority field:

1. **Active-slot lock** — if any peer `BMCSettings` for the same BMC is already `InProgress`, all others stay `Pending` unconditionally. An incumbent is never evicted.
2. **FIFO tiebreak among gate-clear candidates** — when the slot is free, the controller lists all `Pending` peers whose `readinessGates` are all satisfied, and picks the winner by `creationTimestamp → name`. Oldest creation timestamp wins; name is a stable alphabetical tiebreaker for objects created in the same second.

```go
func shouldBMCSettingsRunBefore(a, b *metalv1alpha1.BMCSettings) bool {
    if !a.CreationTimestamp.Equal(&b.CreationTimestamp) {
        return a.CreationTimestamp.Before(&b.CreationTimestamp)
    }
    return a.Name < b.Name
}
```

**Why no `priority` field?**

`priority` would only add value when two independently-authored Sets both have their gates satisfied at the same instant and you want non-FIFO ordering. In practice:

- If B *must* run after A, set a `readinessGate` on B → A. The gate enforces the order exactly; priority adds nothing.
- If there is *no dependency* between A and B (truly independent Sets), FIFO is deterministic and fair — the Set deployed earlier runs first. This matches user expectation and requires no coordination between Set authors.
- The only case priority would help is "I want to change the relative order without re-creating objects" — that is an exotic operational edge case that does not justify the API surface and implementation complexity.

`ServerMaintenance` has priority because there is no gate mechanism to express "this maintenance must wait for that one". `BMCSettings` already has `readinessGates` for exactly that purpose, making priority redundant.

---

## API Changes

### New shared type: `ReadinessGate`

```go
type ReadinessGateScope string

const (
    ReadinessGateScopeDirect ReadinessGateScope = "Direct"
    ReadinessGateScopeSetChild ReadinessGateScope = "PerBMC"
)

// ReadinessGate blocks a resource in Pending until the referenced
// object has the specified condition set to True.
type ReadinessGate struct {
    // APIVersion of the referenced object. e.g. "metal.ironcore.dev/v1alpha1"
    APIVersion string `json:"apiVersion"`
    // Kind of the referenced object. e.g. "BMCSettings"
    Kind string `json:"kind"`
    // Name of the referenced object (or owning Set when Scope is PerBMC).
    Name string `json:"name"`
    // ConditionType that must be True on the referenced object.
    // +optional
    ConditionType string `json:"conditionType,omitempty"`
    // Scope controls how Name is resolved.
    // Global (default): Name is an exact object name.
    // PerBMC: Name is treated as the owning Set name; the controller
    //   resolves the sibling child stamped for this BMC via ownerReferences.
    // +optional
    // +kubebuilder:default=Global
    Scope ReadinessGateScope `json:"scope,omitempty"`
}
```

### `api/v1alpha1/bmc_types.go`

```go
// Before:
BMCSettingRef *v1.LocalObjectReference `json:"bmcSettingRef,omitempty"`

// After:
BMCSettingRefs []v1.LocalObjectReference `json:"bmcSettingRefs,omitempty"`
```

### `api/v1alpha1/bmcsettings_types.go`

```go
// New types:

type BMCSettingsFlowItem struct {
    // Name uniquely identifies this flow step within the BMCSettings.
    Name string `json:"name"`
    // Priority controls application order within the flow.
    // Lower numbers are applied first. Must be unique within the flow.
    Priority int32 `json:"priority"`
    // Settings is the map of BMC manager key=value settings for this step.
    // +optional
    Settings map[string]string `json:"settings,omitempty"`
}

type BMCSettingsFlowStatus struct {
    Name  string            `json:"name"`
    State BMCSettingsState  `json:"state"`
}

// BMCSettingsTemplate replaces the old Version + SettingsMap fields:

type BMCSettingsTemplate struct {
    // SettingsFlow is the ordered list of setting groups to apply.
    // Items are applied in ascending Priority order.
    // +optional
    SettingsFlow []BMCSettingsFlowItem `json:"settingsFlow,omitempty"`

    // Priority controls ordering when multiple BMCSettings target the same BMC.
    // Higher value runs first. Mirrors ServerMaintenance.spec.priority.
    // +optional
    // +kubebuilder:default=0
    Priority int32 `json:"priority,omitempty"`

    // ReadinessGates blocks this BMCSettings in Pending until all gates are satisfied.
    // +optional
    ReadinessGates []ReadinessGate `json:"readinessGates,omitempty"`

    // ServerMaintenancePolicy controls maintenance behaviour for affected servers.
    // +optional
    ServerMaintenancePolicy ServerMaintenancePolicy `json:"serverMaintenancePolicy,omitempty"`
}

// BMCSettingsStatus adds FlowState and LastAppliedTime:

type BMCSettingsStatus struct {
    // State is the overall lifecycle state.
    State BMCSettingsState `json:"state,omitempty"`
    // FlowState tracks per-step state within settingsFlow.
    // +optional
    FlowState []BMCSettingsFlowStatus `json:"flowState,omitempty"`
    // LastAppliedTime is when the BMCSettings last transitioned to Applied.
    // +optional
    LastAppliedTime *metav1.Time `json:"lastAppliedTime,omitempty"`
    // Conditions holds fine-grained status conditions.
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

Fields removed: `Version string`, `SettingsMap map[string]string`.

### `api/v1alpha1/bmcsettingsset_types.go`

```go
// New type:

type VersionSelector struct {
    // Versions is the list of exact firmware version strings to match.
    // If empty or omitted, all firmware versions are included.
    // +optional
    Versions []string `json:"versions,omitempty"`
}

// Added to BMCSettingsSetSpec:

// VersionSelector limits stamping to BMCs whose status.firmwareVersion
// exactly matches one of the listed strings.
// When omitted, all BMCs matching the BMCSelector are included.
// +optional
VersionSelector *VersionSelector `json:"versionSelector,omitempty"`
```

### `api/v1alpha1/bmcversion_types.go`

```go
// Added to BMCVersionTemplate:

// ReadinessGates blocks this BMCVersion in Pending until all gates are satisfied.
// +optional
ReadinessGates []ReadinessGate `json:"readinessGates,omitempty"`
```

---

## Example CRDs

### BMCSettingsSet — version-gated, two-phase flow

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMCSettingsSet
metadata:
  name: ilo6-v170-baseline
spec:
  bmcSelector:
    matchLabels:
      metal.ironcore.dev/bmc-vendor: hpe
  versionSelector:
    versions:
      - "iLO 6 v1.70"
  template:
    metadata:
      labels:
        app.kubernetes.io/managed-by: ilo6-v170-baseline
    spec:
      readinessGates: []
      settingsFlow:
        - name: safe-defaults
          priority: 0
          settings:
            SyslogServer: "10.0.0.5"
            SNMPv3AuthProtocol: SHA256
        - name: performance-tuning
          priority: 10
          settings:
            HPManagementNetworkWorkloads: Enabled
            ProcHyperthreading: Enabled
      serverMaintenancePolicy: Enforced
```

### BMCVersionSet — waits for BMCSettingsSet to complete

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMCVersionSet
metadata:
  name: ilo6-v170-upgrade
spec:
  bmcSelector:
    matchLabels:
      metal.ironcore.dev/bmc-vendor: hpe
  template:
    metadata:
      labels:
        app.kubernetes.io/managed-by: ilo6-v170-upgrade
    spec:
      version: "iLO 6 v1.70"
      readinessGates:
        - apiVersion: metal.ironcore.dev/v1alpha1
          kind: BMCSettings
          name: ilo6-v170-baseline          # name of the BMCSettingsSet
          conditionType: Applied
          scope: SetChild                     # resolved to this BMC's child at runtime
      serverMaintenancePolicy: Enforced
```

### BMC status with multiple setting refs

```yaml
status:
  firmwareVersion: "iLO 6 v1.58"
  bmcSettingRefs:
    - name: ilo6-v170-baseline-abc12
    - name: corp-security-hardening-xyz89
```

---

## Status Examples

### BMCSettings mid-flow

```yaml
status:
  state: InProgress
  flowState:
    - name: safe-defaults
      state: Applied
    - name: performance-tuning
      state: InProgress
  conditions:
    - type: ReadinessGatesSatisfied
      status: "True"
    - type: MaintenanceInProgress
      status: "True"
    - type: ResetIssued
      status: "True"
```

### BMCVersion blocked on gate

```yaml
status:
  state: Pending
  conditions:
    - type: ReadinessGatesSatisfied
      status: "False"
      reason: GateNotSatisfied
      message: "BMCSettings ilo6-v170-baseline-abc12 condition Applied is False"
```

---

## Workflow Diagram

```mermaid
stateDiagram-v2
    [*] --> Pending

    state Pending {
      [*] --> CheckReadinessGates
      CheckReadinessGates --> GatesBlocked : gate not satisfied
      CheckReadinessGates --> CheckFlow : all gates pass
      GatesBlocked --> CheckReadinessGates : sibling watch triggers
      CheckFlow --> NoDiff : all flow items match BMC
      CheckFlow --> NextFlowItem : pending items exist
    }

    Pending --> Applied : NoDiff
    Pending --> InProgress : NextFlowItem (maintenance approved)

    state InProgress {
      [*] --> ApplyFlowItem
      ApplyFlowItem --> Verify
      Verify --> FlowItemApplied : settings converge
      Verify --> Failed : mismatch/timeout
      FlowItemApplied --> MoreItems : next flow item pending
      FlowItemApplied --> AllDone : last item
    }

    InProgress --> Pending : MoreItems (re-enter for next item)
    InProgress --> Applied : AllDone
    InProgress --> Failed : error
    Failed --> Pending : retry annotation
```

---

## Race Condition Analysis

| # | Scenario | Resolution |
|---|---|---|
| RC1 | Two Sets stamp same BMC simultaneously | `bmcSettingRefs` list — both append their own entry; no conflict |
| RC2 | v1.0 settings stamped before prereqs Applied | `readinessGates scope=SetChild` blocks in Pending; sibling watch re-triggers reactively |
| RC3 | BMCVersion starts before prerequisite settings Applied | Same gate mechanism blocks BMCVersion in Pending |
| RC4 | Two BMCSettings request maintenance simultaneously | Active-slot lock (any peer `InProgress`?) + FIFO tiebreak (`shouldBMCSettingsRunBefore`) serialises; incumbent `InProgress` is never evicted |
| RC5 | BMCVersionSet uses `GenerateName` — child name unknown at gate authoring time | Gates reference the Set name; `scope=SetChild` resolves child at runtime via ownerRef scan |
| RC6 | `versionSelector` stamps before firmware upgrade completes | `bmc_controller` writes `status.firmwareVersion` only after Redfish confirms; no gap |

---

## Serialization: how multiple BMCSettings on the same BMC are ordered

The controller uses the same two-layer pattern as `ServerMaintenance`:

**Layer 1 — active-slot lock.** Before doing anything else in `Pending` state, the controller lists all peers in `bmc.spec.bmcSettingRefs` and checks if any is `InProgress`. If one is, stay `Pending` and return. An incumbent is never evicted.

**Layer 2 — FIFO among gate-clear candidates.** When the slot is free, scan all `Pending` peers whose `readinessGates` are all satisfied. If any such peer would run before the current object by `shouldBMCSettingsRunBefore`, stay `Pending`. Otherwise claim the slot → transition to `InProgress`.

```go
// shouldBMCSettingsRunBefore returns true if a should claim the active slot
// before b. Uses creation time then name as a stable tiebreaker.
func shouldBMCSettingsRunBefore(a, b *metalv1alpha1.BMCSettings) bool {
    if !a.CreationTimestamp.Equal(&b.CreationTimestamp) {
        return a.CreationTimestamp.Before(&b.CreationTimestamp)
    }
    return a.Name < b.Name
}
```

### Why no `priority` field?

`ServerMaintenance` has a `priority` field because there is no mechanism to express "this maintenance must wait for that one" — the only ordering tool available is the integer rank. `BMCSettings` already has `readinessGates` for exactly that purpose:

| Ordering need | Without gates | With gates |
|---|---|---|
| B must run after A | Set `priority` on A higher than B | Set a gate on B → A's `Applied` condition |
| A and B are independent | `priority` determines order | FIFO (creationTimestamp) determines order — equally predictable |
| B high-priority but must wait for A | `priority: 100` on B causes confusion — high rank but blocked | Gate on B → A makes the dependency explicit and unambiguous |

The gate approach is strictly clearer: the dependency is declared on the object that has it, visible in its spec, and enforced as a hard prerequisite. `priority` would be an implicit, easily-misunderstood second ordering mechanism sitting alongside the explicit one.

### Gate cycle detection (future work)

A gates on B **and** B gates on A → both stay `Pending` forever. The controller should detect this and emit a `GateCycle` condition. Priority cannot cause cycles.

---

## Implementation Steps

### Phase 1 — API types

1. Add `ReadinessGate` / `ReadinessGateScope` to a new `api/v1alpha1/readinessgate_types.go`
2. `bmc_types.go`: rename `BMCSettingRef` → `BMCSettingRefs []`
3. `bmcsettings_types.go`: add `BMCSettingsFlowItem`, `BMCSettingsFlowStatus`; replace `Version`+`SettingsMap` with `SettingsFlow`, `ReadinessGates`; add `FlowState`+`LastAppliedTime` to status
4. `bmcsettingsset_types.go`: add `VersionSelector`
5. `bmcversion_types.go`: add `ReadinessGates` to `BMCVersionTemplate`
6. `make generate` — regenerate deepcopy + CRD manifests

### Phase 2 — BMCSettings controller

7. Add `addBMCSettingRef` / `removeBMCSettingRef` list-aware helpers (idempotent)
8. Remove broken `<` version string comparison; add `shouldBMCSettingsRunBefore` (FIFO: creationTimestamp → name)
9. In `Pending` state: check active-slot lock (any peer `InProgress`?); check gates; call `shouldBMCSettingsRunBefore` against gate-clear peers
10. Add `checkReadinessGates()` — `Name` mode does a direct Get; `OwnedBy` mode scans `bmcSettingRefs` and filters by ownerRef/field match
11. Thread flow-item iteration through `handleSettingInProgressState`: apply per-item, persist `flowState`, loop back to Pending for next item
12. Add sibling Watch on `BMCSettings` → re-enqueue all `BMCSettings` sharing the same `bmcSettingRefs` entry

### Phase 3 — BMCSettingsSet controller

12. In `createMissingBMCSettings`: skip BMC when `versionSelector.versions` is non-empty and `bmc.status.firmwareVersion` not in the list
13. Remove stale "BMC already has BMCSettingRef, skip" guard
14. Add Watch on `BMC` for `status.firmwareVersion` changes → re-enqueue owning `BMCSettingsSet`

### Phase 4 — BMCVersion controller

15. In Pending case: call `checkReadinessGates()` before `removeServerMaintenanceRefAndResetConditions`
16. Add `ReadinessGatesSatisfied` condition (`True`/`False`)
17. Add Watch on `BMCSettings` → enqueue `BMCVersions` whose gates reference that `BMCSettings`
18. In Completed case: re-evaluate gates; if **all gates satisfied AND `bmc.status.firmwareVersion != spec.version`** → reset state to Pending (drift recovery). Otherwise return early without changes.
19. Add Watch on `BMC` for `status.firmwareVersion` changes → re-enqueue owned `BMCVersion` resources for drift detection

### Phase 5 — BMCVersionSet controller

18. Deep-copy `ReadinessGates` verbatim in `createMissingBMCVersions` (follows existing template copy)

### Phase 6 — Manifests, samples, docs

19. `make manifests` — regenerate CRD YAML
20. Update/add `config/samples/metal_v1alpha1_bmcsettings.yaml`, `metal_v1alpha1_bmcsettingsset.yaml`, `metal_v1alpha1_bmcversionset.yaml`
21. Update `docs/concepts/bmcsettings.md`, `bmcsettingsset.md`, `bmcversion.md`

### Phase 7 — Tests

22. Update existing `bmcsettings_controller_test.go` for new API (flow items, list refs)
23. Add integration test: two `BMCSettingsSet` objects → same BMC → list ref populated; gate sequencing; `versionSelector` filtering

---

## Backward Compatibility

- `BMCSettings` objects without `settingsFlow` are no-ops (no diff, transition to `Applied`)
- `BMCSettingsSet` objects without `versionSelector` continue to target all matched BMCs
- `BMC` objects with no `bmcSettingRefs` are processed normally
- `BMCVersionTemplate` without `readinessGates` behaves identically to the current implementation
- The field rename `bmcSettingRef` → `bmcSettingRefs` requires a one-time migration of existing objects (controller handles both during a transition period using a compatibility shim)

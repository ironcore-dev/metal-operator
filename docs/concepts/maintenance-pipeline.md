# MaintenancePipeline — Draft Design

<!-- SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

> Status: DRAFT

---

## Problem Statement

The existing low-level APIs (`BMCSettings`, `BMCVersion`, `BIOSSettings`, `BIOSVersion`) require operators to:

1. Manually author multiple separate objects per cluster type and wire them together.
2. Know child object names ahead of time to express ordering dependencies.
3. Define BMC firmware version upgrade hops as separate objects per hop.
4. Separately manage BIOS settings/versions (Server-scoped) and BMC settings/versions (BMC-scoped) — with no unified view of the full lifecycle.

In addition, the 1 BMC → N Servers relationship is implicit and handled piecemeal:
- BMC-scoped resources (`BMCSettings`, `BMCVersion`) are stamped per BMC.
- Server-scoped resources (`BIOSSettings`, `BIOSVersion`) are stamped per Server.

---

## Goals

- Single authored resource expresses the full desired maintenance lifecycle for a fleet.
- Explicit stage ordering via list position — stages execute strictly one at a time in list order.
- Version upgrade hops are explicit pipeline stages — one `BMCVersion`/`BIOSVersion` stage per hop, each targeting exactly one firmware version.
- BMC-scoped stages run once per unique BMC (de-duplicated across servers sharing that BMC).
- Server-scoped stages run independently per Server, gated on their BMC's stage completion.
- Drift re-reconciliation is first-class: deviation from the desired state triggers re-execution of the affected stage.

---

## BMC : Server — The 1:N Solution

The pipeline controller creates **one auto-generated child object per unique BMC**:

| Child type | Granularity | Creates |
|---|---|---|
| `MaintenancePipelineRun` | one per **unique BMC** per Pipeline | `BMCSettings`, `BMCVersion` (once per BMC) AND `BIOSSettings`, `BIOSVersion` (once per Server in `serverRefs`) |

`MaintenancePipelineRun` owns and manages all child resources — both BMC-scoped and Server-scoped. There is no separate per-Server object. The Run tracks Server-scoped stage progress per-server in its status.

```
MaintenancePipeline
    ├── MaintenancePipelineRun/bmc-abc        (one Run per unique BMC)
    │       ├── child: BMCSettings/bmc-abc-baseline          (BMC-scoped, one per Run)
    │       ├── child: BMCVersion/bmc-abc-fw-v170            (BMC-scoped, one per hop per Run)
    │       ├── child: BIOSSettings/server-a-bios-pre        (Server-scoped, one per Server)
    │       ├── child: BIOSSettings/server-b-bios-pre
    │       ├── child: BIOSSettings/server-c-bios-pre
    │       ├── child: BIOSVersion/server-a-bios-v250        (Server-scoped, one per Server per hop)
    │       ├── child: BIOSVersion/server-b-bios-v250
    │       └── child: BIOSVersion/server-c-bios-v250
    └── MaintenancePipelineRun/bmc-xyz        (separate Run for a different BMC)
            └── ...
```

This means:
- BMC firmware upgrade happens exactly once per BMC regardless of how many Servers share it.
- BIOS settings/versions are stamped per Server by the same Run — no separate object needed.
- Server-scoped stages listed after BMC-scoped stages in the `stages` list wait for all preceding stages to complete before the Run stamps the Server children.
- `maxConcurrent` controls how many `MaintenancePipelineRun` objects are progressing simultaneously.

---

## API Design

### `MaintenancePipeline`

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: MaintenancePipeline
metadata:
  name: ilo6-upgrade-to-v180
spec:
  # Selects Servers. Controller resolves server.spec.bmcRef to get the BMC.
  serverSelector:
    matchLabels:
      kubernetes.metal.cloud.sap/bmc-vendor: hpe

  # Max number of MaintenancePipelineRun objects (unique BMCs) progressing simultaneously.
  # Controls fleet-level blast radius.
  maxConcurrent: 3

  # What to do when the system deviates from the desired state after completion.
  # Reconcile: re-run affected stages.
  # Observe: surface a condition but take no action.
  driftPolicy: Reconcile

  stages:
    # Stages execute strictly in list order. Each stage must reach Completed
    # before the next is started. BMC-scoped stages are listed first, followed
    # by Server-scoped stages.
    # Each BMCVersion/BIOSVersion stage targets exactly one firmware version.
    # If the hardware already reports the target version, the child completes immediately
    # without flashing. See config/samples/metal_v1alpha1_maintenancepipeline.yaml
    # for the full annotated example.

    - name: bmc-baseline
      kind: BMCSettings
      template:
        settingsFlow:
          - name: network-baseline
            priority: 1
            settings:
              SyslogServer: "10.0.0.5"
              DNSServer: "8.8.8.8"
              SNMPv3AuthProtocol: SHA256

    # BMC firmware upgrade — one explicit stage per version hop.
    - name: bmc-fw-v160
      kind: BMCVersion
      template:
        version: "iLO 6 v1.60"
        image:
          URI: "http://firmware-repo.internal/ilo6/v160/ilo6_160.bin"

    - name: bmc-fw-v170
      kind: BMCVersion
      template:
        version: "iLO 6 v1.70"
        image:
          URI: "http://firmware-repo.internal/ilo6/v170/ilo6_170.bin"

    - name: bmc-fw-v180
      kind: BMCVersion
      template:
        version: "iLO 6 v1.80"
        image:
          URI: "http://firmware-repo.internal/ilo6/v180/ilo6_180.bin"

    - name: bmc-post-upgrade
      kind: BMCSettings
      template:
        serverMaintenancePolicy: Enforced
        settingsFlow:
          - name: post-upgrade-tuning
            priority: 1
            settings:
              HPManagementNetworkWorkloads: Enabled
              ProcTurbo: Enabled

    - name: bios-pre-upgrade
      kind: BIOSSettings
      template:
        # version omitted — Run hydrates from server.status.biosVersion at stamp time
        serverMaintenancePolicy: Enforced
        settingsFlow:
          - name: pxe-prepare
            priority: 1
            settings:
              PxeBootToFirstInterface: Enabled

    # BIOS firmware upgrade — one explicit stage per version hop.
    - name: bios-fw-v240
      kind: BIOSVersion
      template:
        version: "U54 v2.40"
        image:
          URI: "http://firmware-repo.internal/bios/u54/v240/bios_v240.bin"

    - name: bios-fw-v250
      kind: BIOSVersion
      template:
        version: "U54 v2.50"
        image:
          URI: "http://firmware-repo.internal/bios/u54/v250/bios_v250.bin"

    - name: bios-post-upgrade
      kind: BIOSSettings
      template:
        version: "U54 v2.50"
        serverMaintenancePolicy: Enforced
        settingsFlow:
          - name: performance-tuning
            priority: 1
            settings:
              ProcHyperthreading: Enabled
              NUMAGroupSizeOpt: Clustered
```

---

### `MaintenancePipelineRun` (auto-generated, one per unique BMC)

The Run owns ALL child resources for both the BMC and all Servers that share it.

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: MaintenancePipelineRun
metadata:
  # Name is opaque — generated by the API server via generateName.
  # The controller never constructs a deterministic name; it looks up existing
  # Runs via label selectors (metal.ironcore.dev/pipeline + metal.ironcore.dev/bmc).
  name: ilo6-upgrade-to-v180-x7k9f        # generated suffix
  labels:
    metal.ironcore.dev/pipeline: ilo6-upgrade-to-v180
    metal.ironcore.dev/bmc:      bmc-abc123
  ownerReferences:
    - apiVersion: metal.ironcore.dev/v1alpha1
      kind: MaintenancePipeline
      name: ilo6-upgrade-to-v180
      uid: <pipeline-uid>           # filled by the controller
      controller: true
      blockOwnerDeletion: true
spec:
  bmcRef:
    name: bmc-abc123
  # All servers that share this BMC and are matched by the pipeline's serverSelector.
  serverRefs:
    - name: server-a
    - name: server-b
    - name: server-c
status:
  phase: InProgress       # Pending | InProgress | Completed | Failed

  # Stage-count summary (shown in `kubectl get mpr` columns).
  # Snapshot: stages 1–2 Completed, stage 3 (bmc-fw-v170) InProgress, stages 4–9 Pending.
  totalStages: 9
  pendingStages: 6
  activeStages: 1
  completedStages: 2
  failedStages: 0

  stages:
    - name: bmc-baseline
      phase: Completed
      childRef:
        kind: BMCSettings
        name: bmc-baseline-p4m2q
      completedAt: "2026-05-01T10:00:00Z"

    # Intermediate hop — driftPolicy: Suspend patched after completion
    # (hardware is expected to be ahead of v1.60; Observe would false-positive).
    - name: bmc-fw-v160
      phase: Completed
      childRef:
        kind: BMCVersion
        name: bmc-fw-v160-n8rt3
      completedAt: "2026-05-01T10:15:00Z"

    - name: bmc-fw-v170
      phase: InProgress
      childRef:
        kind: BMCVersion
        name: bmc-fw-v170-m3kx9

    - name: bmc-fw-v180
      phase: Pending

    - name: bmc-post-upgrade
      phase: Pending

    # Server-scoped stages list per-server sub-status even while Pending.
    - name: bios-pre-upgrade
      phase: Pending
      servers:
        - serverRef: {name: server-a}
          phase: Pending
        - serverRef: {name: server-b}
          phase: Pending
        - serverRef: {name: server-c}
          phase: Pending

    - name: bios-fw-v240
      phase: Pending
      servers:
        - serverRef: {name: server-a}
          phase: Pending
        - serverRef: {name: server-b}
          phase: Pending
        - serverRef: {name: server-c}
          phase: Pending

    - name: bios-fw-v250
      phase: Pending
      servers:
        - serverRef: {name: server-a}
          phase: Pending
        - serverRef: {name: server-b}
          phase: Pending
        - serverRef: {name: server-c}
          phase: Pending

    - name: bios-post-upgrade
      phase: Pending
      servers:
        - serverRef: {name: server-a}
          phase: Pending
        - serverRef: {name: server-b}
          phase: Pending
        - serverRef: {name: server-c}
          phase: Pending

  conditions:
    - type: Progressing
      status: "True"
    - type: DriftDetected
      status: "False"
```

---

## Stage Execution Order

Stages execute **strictly in list order**. The Run controller stamps the child resource for stage `N` only after stage `N-1` has reached `Completed`. There is no parallelism within a single Run.

```
bmc-baseline → bmc-fw-v160 → bmc-fw-v170 → bmc-fw-v180 → bmc-post-upgrade
    → bios-pre-upgrade → bios-fw-v240 → bios-fw-v250 → bios-post-upgrade
```

Each version hop is an **explicit stage** with its own `kind: BMCVersion` or `kind: BIOSVersion` entry. The controller stamps one child per stage; drift detection on completed stages uses the same `driftPolicyFor` rule as settings stages.

For Server-scoped stages (`BIOSSettings` / `BIOSVersion`), the Run stamps children **simultaneously for all servers** in `serverRefs`. Each server then advances through subsequent Server-scoped stages **independently** — a server moves to the next stage as soon as its own child completes, without waiting for other servers. The aggregate stage phase reflects the slowest server.

`maxConcurrent` caps **fleet-level** parallelism: how many `MaintenancePipelineRun` objects (unique BMCs) may be `InProgress` simultaneously. Stages within a single Run are always sequential.


---

## Drift Policy

`driftPolicy` is used at two levels with the same enum:

| `driftPolicy` value | On `MaintenancePipeline` | On child resource (`BMCSettings` etc.) |
|---|---|---|
| `Reconcile` | Re-run affected stage + all downstream on drift | — (not valid on children; children never self-recover) |
| `Observe` | Surface `DriftDetected` condition, take no action | Detects drift, updates status conditions, no hardware action, no maintenance window |
| `Suspend` | — (not valid at pipeline level) | Completely frozen: no reconciliation, no drift detection, no status updates |

Drift is detected by a watch on owned children transitioning to `Pending` (from `Completed`) → re-enqueue the owning `MaintenancePipelineRun`. The Observe-mode child is the source of truth for whether hardware has drifted; the Run does not need to watch raw BMC/Server status fields directly.

The child controller is the source of truth for whether its desired state is still satisfied. A completed `Observe`-mode child that detects hardware drift transitions to `Pending` (no maintenance window requested). The Run's watch fires and takes over coordination.

---

## Drift Recovery — Full Workflow

The `MaintenancePipelineRun` controller is the **sole authority** for resetting and re-executing child objects — child controllers must not self-recover from drift while owned by a Run.

### `spec.driftPolicy` Lifecycle on Child Objects

All child resources expose a `spec.driftPolicy` field with three modes:

| Value | Child controller behaviour |
|---|---|
| `` (empty, default) | Fully active — reconciles hardware, requests maintenance windows, applies changes |
| `Observe` | Read-only — reconciles hardware, detects drift, updates status conditions, but requests no maintenance windows and applies no changes |
| `Suspend` | Completely frozen — no reconciliation, no drift detection, no status updates, no hardware actions |

The `MaintenancePipelineRun` controller chooses which value to patch onto a completed child based on the **`driftPolicyFor` rule** (see below).

**Why two non-active values?**

The problem arises when the same `kind` appears multiple times in the stage list:

```
Stage 1: BMCVersion → v1.60   (completed, child desired: v1.60)
Stage 2: BMCSettings           (completed)
Stage 3: BMCVersion → v1.80   (completes → hardware now at v1.80)
```

Stage 1's child has `desired = v1.60`. With `Observe`, it would see `v1.80 ≠ v1.60` and report drift — a **false positive**. The hardware is *supposed* to be past v1.60. So Stage 1's child must be `Suspend` (completely inert). Stage 3's child must be `Observe` (regression below v1.80 is real drift).

Settings stages never have this problem — there is no "superseded" concept for settings; drift is always meaningful regardless of stage position.

**`driftPolicyFor` rule** (applied by the Run when patching a completed child):

```go
func driftPolicyFor(stages []PipelineStage, stageIndex int) DriftPolicy {
    current := stages[stageIndex]
    if current.Kind == BMCVersion || current.Kind == BIOSVersion {
        // Check whether a later stage with the same kind exists.
        for _, later := range stages[stageIndex+1:] {
            if later.Kind == current.Kind {
                return DriftPolicySuspend // superseded — hardware expected to be past this
            }
        }
    }
    // Terminal version stage, or any settings stage → Observe.
    return DriftPolicyObserve
}
```

**Normal lifecycle:**

```
Run creates child                  → driftPolicy: "" (active)
Child executes                     → Completed
Run calls driftPolicyFor(i):
  settings stage OR last version   → driftPolicy: Observe
  intermediate version stage       → driftPolicy: Suspend
Hardware drifts:
  Observe child sees mismatch      → transitions Completed → Pending
                                     (Observe: no maintenance window requested)
  Suspend child ignores all state  → stays Completed (or Pending — irrelevant, inert)
Run watch fires on Observe child   → recovery begins
```

**Recovery — halting the active child:**

Because stages execute strictly sequentially, at most **one** child is ever active at any given time. All completed children are already `Observe` or `Suspend`. Recovery only needs to halt the one currently active child:

```yaml
# Run detects drift (an Observe child transitioned to Pending)
# If a stage is currently InProgress, patch its active child:
spec:
  driftPolicy: Observe  # halts hardware actions; drift detection continues
# Then: find reset index, patch driftPolicy: "" from reset point forward—one stage at a time.
# For version-agnostic settings stages also re-hydrate spec.version from current hardware status.
```

---

### Drift Recovery Steps

```
1. Watch fires: an Observe-mode completed child transitioned to Pending
   (child detected hardware drift; Observe mode means no maintenance window requested)
2. Run reconciles → if a stage is currently InProgress, patches spec.driftPolicy: Observe on its child
   (halts hardware actions; all completed children are already Observe or Suspend)
3. Run evaluates each stage against live state to find the earliest dirty stage
   (Suspend children are skipped — intermediate version stages cannot be dirty by definition)
4. Patch `spec.driftPolicy: ""` on the existing child at the reset point.
   For version-agnostic settings stages also patch `spec.version` from current hardware status.
   All stages from the reset point forward are re-activated one at a time in list order.
5. Run resets `status.stages[]` to Pending from the reset point forward
6. Run advances through stages sequentially, waiting for each to reach Completed before patching the next
   (a new child is only created if none exists for a stage — e.g. a stage added to the pipeline after the initial run)
```

Patching the existing child re-uses its identity, labels, and ownerReferences. The child's controller re-executes against the existing spec — any new conditions are appended to the object's existing condition list, preserving the history of previous attempts. No objects are deleted or created during drift recovery.

---

### Example: Complete Factory Reset

**Before reset** — pipeline fully `Completed`:

```
bmc-abc:  firmwareVersion = "iLO 6 v1.80"    server-a/b/c: biosVersion = "U54 v2.50"
All BMCSettings  → Applied                   All BIOSSettings → Applied
```

**After factory reset**:

```
bmc-abc:  firmwareVersion = "iLO 6 v1.50"   ← not in templates list; factory default
server-a: biosVersion     = "U54 v2.10"     ← factory default
server-b: biosVersion     = "U54 v2.10"
server-c: biosVersion     = "U54 v2.10"
All settings wiped
```

**Step 1 — Hardware drifts; Monitor-mode completed children move to Pending**

```yaml
# All completed children were patched to Observe or Suspend by the Run.
# Observe children (terminal version stages + all settings stages) detect the
# hardware no longer matches their desired state and transition: Completed → Pending.
# Because Observe mode suppresses maintenance window requests, they wait for the Run.
# Suspend children (intermediate version hops) are completely inert — they are skipped.
# Children watching: BMCSettings/bmc-baseline (Observe), BMCVersion/bmc-v180 (Observe),
#   BMCSettings/bmc-post-upgrade (Observe), BIOSSettings/server-x-pre (Observe),
#   BIOSVersion/server-x-v250 (Observe), BIOSSettings/server-x-post (Observe) — × 3 servers
# The Run's watch fires when any Observe child transitions to Pending.
```

**Step 2 — Run evaluates drift per stage**

```
Stage            Desired state          Live state          Dirty?
──────────────── ────────────────────── ─────────────────── ──────
bmc-baseline     settings applied       wiped               YES  ← earliest dirty
bmc-fw-v160      v1.60 (Suspend)        —                   skipped (Suspend)
bmc-fw-v170      v1.70 (Suspend)        —                   skipped (Suspend)
bmc-fw-v180      v1.80                  v1.50               YES
bmc-post-upgrade settings applied       wiped               YES
bios-pre-upgrade settings applied       wiped               YES
bios-fw-v240     v2.40 (Suspend)        —                   skipped (Suspend)
bios-fw-v250     v2.50                  v2.10               YES
bios-post-upgrade settings applied      wiped               YES
```

Earliest dirty stage = `bmc-baseline` (index 0) → full reset.

**Step 3 — Run resets status**

```yaml
status:
  phase: InProgress
  totalStages: 9
  pendingStages: 9
  activeStages: 0
  completedStages: 0
  failedStages: 0
  stages:
    - name: bmc-baseline      phase: Pending   # stage[0] — starts immediately
    - name: bmc-fw-v160       phase: Pending   # stage[1] — waits for [0]
    - name: bmc-fw-v170       phase: Pending   # stage[2] — waits for [1]
    - name: bmc-fw-v180       phase: Pending   # stage[3] — waits for [2]
    - name: bmc-post-upgrade  phase: Pending   # stage[4] — waits for [3]
    - name: bios-pre-upgrade  phase: Pending   # stage[5] — waits for [4]
      servers: [{serverRef: {name: server-a}, phase: Pending}, ...]
    - name: bios-fw-v240      phase: Pending   # stage[6] — waits for [5]
      servers: [{serverRef: {name: server-a}, phase: Pending}, ...]
    - name: bios-fw-v250      phase: Pending   # stage[7] — waits for [6]
      servers: [{serverRef: {name: server-a}, phase: Pending}, ...]
    - name: bios-post-upgrade phase: Pending   # stage[8] — waits for [7]
      servers: [{serverRef: {name: server-a}, phase: Pending}, ...]
  conditions:
    - type: DriftDetected
      status: "True"
      reason: FullResetRequired
      message: "Factory reset detected on bmc-abc: firmwareVersion regressed to iLO 6 v1.50"
```

**Step 4 — Re-execution in order**

```
[T+0]   Run patches spec.driftPolicy: "" on BMCSettings/bmc-baseline
          → controller applies DNS, Syslog, SNMP → Applied
          → Run patches driftPolicy: Observe (settings stage → always Observe)

[T+1]   bmc-baseline Completed
          Run patches spec.driftPolicy: "" on existing bmc-fw-v160 child
            (current version ≠ v1.60 → child re-executes → requests maintenance window)

[T+2]   BMCVersion v1.60 Completed
          → driftPolicyFor: later same-kind stage exists → patch driftPolicy: Suspend
          Run patches spec.driftPolicy: "" on existing bmc-fw-v170 child

[T+3]   BMCVersion v1.70 Completed → driftPolicy: Suspend
          Run patches spec.driftPolicy: "" on existing bmc-fw-v180 child

[T+4]   BMCVersion v1.80 Completed
          → driftPolicyFor: no later same-kind stage → driftPolicy: Observe
          → bmc-fw-v180 Completed
          Run patches spec.driftPolicy: "" on existing bmc-post-upgrade child

[T+5]   BMCSettings/bmc-post-upgrade Applied → driftPolicy: Observe
          Run patches spec.driftPolicy: "" and re-hydrates spec.version: "U54 v2.10"
            (from server.status.biosVersion) on bios-pre-upgrade children (× 3)
            → BIOSSettings applies PxeBootToFirstInterface → Applied → driftPolicy: Observe

[T+5b]  bios-pre-upgrade Completed (all servers)
          Run patches spec.driftPolicy: "" on existing bios-fw-v240 children (× 3)
            (current version ≠ v2.40 → children re-execute → upgrade)

[T+6]   BIOSVersion v2.40 Completed per server → driftPolicy: Suspend
          Run patches spec.driftPolicy: "" on existing bios-fw-v250 children (× 3)

[T+7]   BIOSVersion v2.50 Completed per server → driftPolicy: Observe
          Run patches spec.driftPolicy: "" on existing bios-post-upgrade children (× 3)

[T+8]   BIOSSettings/bios-post-upgrade Applied on all servers → bios-post-upgrade Completed

[T+9]   All stages Completed → Run phase: Completed
          DriftDetected condition cleared
```

---

### Partial Drift Example — Only BIOS Regressed

If only `server-a`'s BIOS regressed (e.g. BIOS update failed and rolled back) while BMC is healthy:

```
bmc-abc:  firmwareVersion = "iLO 6 v1.80"  ← still correct, no BMC drift
server-a: biosVersion     = "U54 v2.40"    ← regressed from v2.50
server-b: biosVersion     = "U54 v2.50"    ← fine
server-c: biosVersion     = "U54 v2.50"    ← fine
```

The Run evaluates per-server:

```
Stage             server-a              server-b   server-c
───────────────── ──────────────────    ─────────  ─────────
bios-pre-upgrade  Applied (no drift)    Completed  Completed
bios-fw-v240      v2.40 (Suspend)       —          —          skipped (Suspend — superseded)
bios-fw-v250      v2.40 ≠ v2.50 DIRTY  Completed  Completed  ← earliest dirty for server-a
bios-post-upgrade settings may be gone  Completed  Completed
```

The Run:
1. Patches `driftPolicy: Observe` on `server-a`'s active child if one is `InProgress`
2. Patches `spec.driftPolicy: ""` on `server-a`'s `bios-fw-v250` child (existing object re-used)
3. Patches `spec.driftPolicy: ""` on `server-a`'s `bios-post-upgrade` child
4. Resets only `server-a`'s sub-status for `bios-fw-v250` and `bios-post-upgrade` to Pending
5. BMC stages and server-b/c stages are **not touched** — they remain Completed

The aggregate stage phase for `bios-fw-v250` transitions:
```yaml
- name: bios-fw-v250
  phase: InProgress   # aggregate: not all servers Completed
  servers:
    - serverRef:
        name: server-a
      phase: InProgress
    - serverRef:
        name: server-b
      phase: Completed
    - serverRef:
        name: server-c
      phase: Completed
```

---

## Version Hydration for Version-Agnostic Stages

`BIOSSettingsTemplate.version` is a required field in the existing API. Pipeline stages that apply settings regardless of the current firmware version (e.g. `bios-pre-upgrade`) should not need to hard-code a version.

When a stage `template` omits `version` (or sets it to `""`), the Run controller hydrates the field at child-creation time from the live hardware state — `server.status.biosVersion` for BIOS stages and `bmc.status.firmwareVersion` for BMC stages. Because stages execute sequentially, the hydrated value always matches the hardware at the moment the child is stamped. On drift recovery, the Run re-reads the current version and patches `spec.version` on the existing child before re-activating it.
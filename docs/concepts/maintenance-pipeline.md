# MaintenancePipeline — Draft Design

<!-- SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

> **Status: DRAFT .**

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
- There is no object that tracks "for this Server and its BMC, what is the full maintenance state?"

---

## Goals

- Single authored resource expresses the full desired maintenance lifecycle for a fleet.
- Explicit stage ordering via list position — stages execute strictly one at a time in list order.
- Version upgrade chains (hops) expressed as an ordered list of version templates — not as separate objects.
- BMC-scoped stages run once per unique BMC (de-duplicated across servers sharing that BMC).
- Server-scoped stages run independently per Server, gated on their BMC's stage completion.
- Drift re-reconciliation is first-class: deviation from the desired state triggers re-execution of the affected stage.

---

## Non-Goals (for this draft)

- Replacing the low-level `BMCSettings`, `BMCVersion`, `BIOSSettings`, `BIOSVersion` APIs — the pipeline generates these as child resources internally.
- Cross-pipeline dependencies.

---

## BMC vs Server — The 1:N Problem

```
BMC  ──┬── Server A  (BIOS settings/version scoped to Server A)
       ├── Server B  (BIOS settings/version scoped to Server B)
       └── Server C  (BIOS settings/version scoped to Server C)

       └── BMC settings/version scoped to the BMC (shared by A, B, C)
```

A naive "one instance per Server" model creates a race: if Servers A, B, and C all try to drive the BMC firmware upgrade independently, three competing `BMCVersion` objects are stamped for the same BMC.

### Proposed coordination model

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

## Reusing Existing Template Types

Rather than inventing new schema, the pipeline stages inline the **existing API template types** directly:

| Stage `kind` | Template type reused | Defined in |
|---|---|---|
| `BMCSettings` | `BMCSettingsTemplate` | `api/v1alpha1/bmcsettings_types.go` |
| `BMCVersion` | `BMCVersionTemplate` | `api/v1alpha1/bmcversion_types.go` |
| `BIOSSettings` | `BIOSSettingsTemplate` | `api/v1alpha1/biossettings_types.go` |
| `BIOSVersion` | `BIOSVersionTemplate` | `api/v1alpha1/biosversion_types.go` |

For `BMCVersion` and `BIOSVersion` stages (which may require multiple sequential hops), the stage uses a **`templates: []`** list — an ordered list of the existing template type. The controller applies them one at a time in order.

> **Note on target scope**: `target` is not a stage field — it is derived from `kind`:
> - `BMCSettings` / `BMCVersion` → BMC-scoped (stamped once per Run, against the Run's `bmcRef`)
> - `BIOSSettings` / `BIOSVersion` → Server-scoped (stamped per Server in `serverRefs`)

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

  # Maintenance policy applied to all server maintenance requests generated by this pipeline.
  serverMaintenancePolicy: OwnerApproval

  # What to do when the system deviates from the desired state after completion.
  # Reconcile: re-run affected stages.
  # Observe: surface a condition but take no action.
  # +kubebuilder:default=Reconcile
  driftPolicy: Reconcile

  stages:
    # Stages execute strictly in list order. Each stage must reach Completed
    # before the next is started. BMC-scoped stages are listed first, followed
    # by Server-scoped stages.

    - name: bmc-baseline
      kind: BMCSettings
      template:                         # BMCSettingsTemplate (api/v1alpha1/bmcsettings_types.go)
        settingsFlow:
          - name: network-baseline
            priority: 0
            settings:
              SyslogServer: "10.0.0.5"
              DNSServer: "8.8.8.8"
              SNMPv3AuthProtocol: SHA256
        serverMaintenancePolicy: OwnerApproval

    - name: bmc-firmware-upgrade
      kind: BMCVersion
      # templates: ordered list of BMCVersionTemplate (api/v1alpha1/bmcversion_types.go).
      # Applied one at a time in list order. Controller fast-forwards past versions
      # already matching bmc.status.firmwareVersion; stops at last entry.
      templates:
        - version: "iLO 6 v1.60"
          image:
            URI: "http://firmware-repo.internal/ilo6/v160/ilo6_160.bin"
          serverMaintenancePolicy: OwnerApproval
        - version: "iLO 6 v1.70"
          image:
            URI: "http://firmware-repo.internal/ilo6/v170/ilo6_170.bin"
          serverMaintenancePolicy: OwnerApproval
        - version: "iLO 6 v1.80"
          image:
            URI: "http://firmware-repo.internal/ilo6/v180/ilo6_180.bin"
          serverMaintenancePolicy: OwnerApproval

    - name: bmc-post-upgrade-settings
      kind: BMCSettings
      template:                         # BMCSettingsTemplate
        settingsFlow:
          - name: post-upgrade-tuning
            priority: 0
            settings:
              HPManagementNetworkWorkloads: Enabled
              ProcTurbo: Enabled
        serverMaintenancePolicy: OwnerApproval

    - name: bios-pre-upgrade
      kind: BIOSSettings
      template:                         # BIOSSettingsTemplate (api/v1alpha1/biossettings_types.go)
        # version omitted — Run hydrates from server.status.biosVersion at stamp time
        settingsFlow:
          - name: pxe-prepare
            priority: 0
            settings:
              PxeBootToFirstInterface: Enabled
        serverMaintenancePolicy: OwnerApproval

    - name: bios-version-upgrade
      kind: BIOSVersion
      # templates: ordered list of BIOSVersionTemplate (api/v1alpha1/biosversion_types.go).
      # Same hop semantics as BMCVersion above.
      templates:
        - version: "U54 v2.40"
          image:
            URI: "http://firmware-repo.internal/bios/u54/v240/bios_v240.bin"
          serverMaintenancePolicy: OwnerApproval
        - version: "U54 v2.50"
          image:
            URI: "http://firmware-repo.internal/bios/u54/v250/bios_v250.bin"
          serverMaintenancePolicy: OwnerApproval

    - name: bios-post-upgrade
      kind: BIOSSettings
      template:                         # BIOSSettingsTemplate
        version: "U54 v2.50"            # optional: only apply at this exact BIOS version
        settingsFlow:
          - name: performance-tuning
            priority: 0
            settings:
              ProcHyperthreading: Enabled
              NUMAGroupSizeOpt: Clustered
        serverMaintenancePolicy: OwnerApproval
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
    - kind: MaintenancePipeline
      name: ilo6-upgrade-to-v180
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
  stages:
    # BMC-scoped stage (kind: BMCSettings): one status entry, no per-server breakdown
    - name: bmc-baseline
      phase: Completed
      childRef:
        kind: BMCSettings
        name: bmc-baseline-p4m2q          # generated; Run uses childRef to track it
      completedAt: "2026-05-01T10:00:00Z"

    # BMCVersion stage with hop progress
    - name: bmc-firmware-upgrade
      phase: InProgress
      childRef:
        kind: BMCVersion
        name: bmc-firmware-upgrade-n8rt3   # generated
      versionProgress:
        completedVersions: ["iLO 6 v1.60"]
        activeVersion:     "iLO 6 v1.70"
        pendingVersions:   ["iLO 6 v1.80"]

    - name: bmc-post-upgrade-settings
      phase: Pending

    # Server-scoped stage (kind: BIOSSettings): per-server sub-status
    - name: bios-pre-upgrade
      phase: Pending            # aggregate: Pending until all servers Completed
      servers:
        - serverRef: server-a
          phase: Pending
        - serverRef: server-b
          phase: Pending
        - serverRef: server-c
          phase: Pending

    - name: bios-version-upgrade
      phase: Pending
      servers:
        - serverRef: server-a
          phase: Pending
        - serverRef: server-b
          phase: Pending
        - serverRef: server-c
          phase: Pending

    - name: bios-post-upgrade
      phase: Pending
      servers:
        - serverRef: server-a
          phase: Pending
        - serverRef: server-b
          phase: Pending
        - serverRef: server-c
          phase: Pending

  conditions:
    - type: Progressing
      status: "True"
    - type: DriftDetected
      status: "False"
```

---

## Version Template (Hop) Semantics

`BMCVersion` and `BIOSVersion` stages use a `templates: []` list — an **ordered list of the existing `BMCVersionTemplate` / `BIOSVersionTemplate` types**. No new schema is introduced.

### How the controller resolves the active hop

```
current bmc.status.firmwareVersion: "iLO 6 v1.60"

templates:
  [0] version: "iLO 6 v1.60"
  [1] version: "iLO 6 v1.70"
  [2] version: "iLO 6 v1.80"

Step 1: scan templates for current version → found at index 0 (already applied)
Step 2: advance to index 1 → stamp BMCVersion child with template[1]
Step 3: BMCVersion child reaches Completed → advance to index 2
Step 4: stamp BMCVersion child with template[2]
Step 5: BMCVersion child reaches Completed → no more templates → stage Completed
```

- If current version is **not in the list**: start from index 0.
- If current version equals the **last entry**: stage is immediately `Completed`.
- Only one `BMCVersion` / `BIOSVersion` child exists at a time; it is deleted and re-created for the next hop.
- No version string comparison (`>=`, `<`) is used — only exact-match position lookup.

### Drift recovery

On `bmc.status.firmwareVersion` change (watched by the Run controller):
1. Re-scan `templates` for the new current version.
2. If the new position is before `completedVersions`, reset `versionProgress` to that position and transition stage back to `InProgress`.
3. All stages after this stage in the list are also reset to `Pending`.

---

## Stage Execution Order

Stages execute **strictly in list order**. The Run controller stamps the child resource for stage `N` only after stage `N-1` has reached `Completed`. There is no parallelism within a single Run.

```
bmc-baseline → bmc-firmware-upgrade → bmc-post-upgrade-settings
    → bios-pre-upgrade → bios-version-upgrade → bios-post-upgrade
```

For Server-scoped stages (`BIOSSettings` / `BIOSVersion`), the Run stamps children **simultaneously for all servers** in `serverRefs` and waits until every server's child reaches `Completed` before advancing to the next stage.

`maxConcurrent` caps **fleet-level** parallelism: how many `MaintenancePipelineRun` objects (unique BMCs) may be in `InProgress` simultaneously. Stages within a single Run are always sequential.

> **Why sequential and not parallel?**
> BIOS settings and version operations are issued via Redfish, which routes through the BMC. Flashing BMC firmware while simultaneously issuing BIOS changes against the same BMC's Redfish API risks interfering with the flash in-flight. Sequential execution eliminates this race with no loss of practical throughput.

---

## Drift Policy

| `driftPolicy` | Behaviour on deviation |
|---|---|
| `Reconcile` | Re-run the affected stage (and all downstream stages that depend on it) |
| `Observe` | Set `DriftDetected: True` condition, surface in status, take no action |

Drift is detected by watches on:
- `bmc.status.firmwareVersion` → re-enqueue the owning `MaintenancePipelineRun` (version drift)
- `server.status.biosVersion` → re-enqueue the owning `MaintenancePipelineRun` (version drift, per-server)
- Any owned `BMCSettings` / `BIOSSettings` / `BMCVersion` / `BIOSVersion` child transitioning to `Pending` (from `Completed`) → re-enqueue the owning `MaintenancePipelineRun` (drift detected while suspended)

The child controller is the source of truth for whether its desired state is still satisfied. When a completed+suspended child detects hardware drift, it transitions to `Pending` (held there by `suspend: true`). The Run's watch fires and takes over coordination.

---

## Drift Recovery — Full Workflow

### The Core Problem Without Coordination

If existing child objects independently detect drift and self-recover, they race:

```
BMCVersion/bmc-post-upgrade  → detects v1.50 ≠ v1.80 → resets to Pending
BMCSettings/bmc-baseline     → detects settings gone  → resets to Pending
BIOSVersion/server-a         → detects v2.10 ≠ v2.50  → resets to Pending
```

All three fight for maintenance windows simultaneously — breaking the pipeline's ordering guarantees. The `MaintenancePipelineRun` controller is the **sole authority** for resetting and re-executing child objects. Child controllers must not self-recover from drift while owned by a Run.

---

### `spec.suspend` Lifecycle on Child Objects

All child resources expose a `spec.suspend` field. When `true`, the child controller holds in `Pending` — it detects drift and updates its status but requests no maintenance windows and takes no hardware action.

**Normal lifecycle:**

```
Run creates child        → suspend: false
Child executes           → Completed
Run patches child        → suspend: true   ← frozen immediately after Completed
Hardware drifts          → child detects mismatch → transitions to Pending
                           (suspend: true keeps it there — no maintenance window requested)
Run watch fires          → child left Completed → recovery begins
```

Freezing completed children (`suspend: true`) is what prevents the race: without it, a child could detect drift and race to re-acquire a maintenance window before the Run has a chance to evaluate the reset point and enforce stage ordering.

**Recovery — only the active child needs suspending:**

Because stages execute strictly sequentially, at most **one** child is ever `InProgress` at any given time. All other children are either not yet created, or already `Completed` + `suspend: true`. Recovery therefore only needs to suspend that one active child:

```yaml
# Run detects drift (a suspended completed child moved to Pending)
# If a stage is currently InProgress, suspend its child:
spec:
  suspend: true   # active child halts; no more hardware actions
# Then: find reset index, delete from reset index forward, re-create with suspend: false
```

---

### Drift Recovery Steps

```
1. Watch fires:
     (a) bmc.status.firmwareVersion or server.status.biosVersion changed, OR
     (b) a suspended completed child transitioned to Pending
         (child detected hardware drift but is held by suspend: true)
2. Run reconciles → if a stage is currently InProgress, sets spec.suspend: true on its child
   (all other children are already suspend: true from their completion freeze)
3. Run evaluates each stage against live state to find the earliest dirty stage
4. Run deletes all child objects from the reset point forward (clean slate)
5. Run resets status.stages[] to Pending from the reset point forward (all stages at index ≥ reset index)
6. Run re-executes stages sequentially — creating new children with suspend: false
```

Deletion (step 4) rather than in-place state reset avoids any race between the Run and a child's own reconcile loop. `ownerReferences` ensure garbage collection if the Run itself is deleted.

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

**Step 1 — Hardware drifts; suspended completed children move to Pending**

```yaml
# All completed children are already suspend: true (frozen after completion).
# Each child independently detects the hardware no longer matches its desired state
# and transitions: Completed → Pending.
# Because suspend: true, no maintenance window is requested — they just wait.
# Children: BMCSettings/bmc-baseline, BMCVersion/bmc-v180,
#           BMCSettings/bmc-post, BIOSSettings/server-a-pre,
#           BIOSVersion/server-a-v250, BIOSSettings/server-a-post  (× 3 servers)
# The Run's watch fires when children transition to Pending.
```

**Step 2 — Run evaluates drift per stage**

```
Stage               Desired state          Live state          Dirty?
─────────────────── ────────────────────── ─────────────────── ──────
bmc-baseline        settings applied       wiped               YES  ← earliest dirty
bmc-firmware-upgrade v1.80                v1.50               YES
bmc-post-upgrade    settings applied       wiped               YES
bios-pre-upgrade    settings applied       wiped               YES
bios-version-upgrade v2.50               v2.10               YES
bios-post-upgrade   settings applied       wiped               YES
```

Earliest dirty stage = `bmc-baseline` (index 0) → full reset.

**Step 3 — Run deletes all child objects**

```
Deleted: BMCSettings/...bmc-baseline
Deleted: BMCVersion/...bmc-v180
Deleted: BMCSettings/...bmc-post-upgrade
Deleted: BIOSSettings/...server-a-bios-pre   (× 3)
Deleted: BIOSVersion/...server-a-bios-v250   (× 3)
Deleted: BIOSSettings/...server-a-bios-post  (× 3)
```

**Step 4 — Run resets status**

```yaml
status:
  phase: InProgress
  stages:
    - name: bmc-baseline           phase: Pending   # stage[0] — starts immediately
    - name: bmc-firmware-upgrade   phase: Pending   # stage[1] — waits for [0]
    - name: bmc-post-upgrade       phase: Pending   # stage[2] — waits for [1]
    - name: bios-pre-upgrade       phase: Pending   # stage[3] — waits for [2]
      servers: [{serverRef: server-a, phase: Pending}, ...]
    - name: bios-version-upgrade   phase: Pending   # stage[4] — waits for [3]
      servers: [{serverRef: server-a, phase: Pending}, ...]
    - name: bios-post-upgrade      phase: Pending   # stage[5] — waits for [4]
      servers: [{serverRef: server-a, phase: Pending}, ...]
  conditions:
    - type: DriftDetected
      status: "True"
      reason: FullResetRequired
      message: "Factory reset detected on bmc-abc: firmwareVersion regressed to iLO 6 v1.50"
```

**Step 5 — DAG re-executes in order**

```
[T+0]   Run creates BMCSettings/bmc-baseline (suspend: false)
          → BMCSettings controller applies DNS, Syslog, SNMP → Applied
          → Run observes Completed → patches suspend: true on bmc-baseline child

[T+1]   bmc-baseline Completed
          → Run creates BMCVersion child for hop [0]: "iLO 6 v1.60"
            (current v1.50 not in list → start from index 0)

[T+2]   BMCVersion v1.60 Completed → Run deletes it, creates BMCVersion for hop [1]: "iLO 6 v1.70"

[T+3]   BMCVersion v1.70 Completed → Run deletes it, creates BMCVersion for hop [2]: "iLO 6 v1.80"

[T+4]   BMCVersion v1.80 Completed → bmc-firmware-upgrade stage Completed
          → Run creates BMCSettings/bmc-post-upgrade (unsuspended)

[T+5]   BMCSettings/bmc-post-upgrade Applied → bmc-post-upgrade-settings Completed
          → Run creates BIOSSettings/server-a/b/c-bios-pre
            → BIOSSettings applies PxeBootToFirstInterface → Applied on all servers

[T+5b]  bios-pre-upgrade Completed (all servers)
          → Run creates BIOSVersion children for server-a/b/c, hop [0]: "U54 v2.40"
            (current v2.10 not in list → start from index 0)

[T+6]   BIOSVersion v2.40 Completed per server → hop advances to "U54 v2.50"

[T+7]   BIOSVersion v2.50 Completed per server → bios-version-upgrade Completed
          → Run creates BIOSSettings/server-a/b/c-bios-post

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
Stage                server-a              server-b   server-c
────────────────     ──────────────────    ─────────  ─────────
bios-pre-upgrade     Applied (no drift)    Completed  Completed
bios-version-upgrade v2.40 ≠ v2.50 DIRTY  Completed  Completed  ← earliest dirty for server-a
bios-post-upgrade    settings may be gone  Completed  Completed
```

The Run:
1. Suspends only `server-a`'s active child if one is `InProgress` (all completed children are already `suspend: true`)
2. Deletes those two children
3. Resets only `server-a`'s sub-status for `bios-version-upgrade` and `bios-post-upgrade` to `Pending`
4. BMC stages and server-b/c stages are **not touched** — they remain `Completed`
5. Creates new `BIOSVersion/server-a` child starting from v2.40 (current position in list)
6. Once `BIOSVersion` reaches v2.50 → creates `BIOSSettings/server-a-post`

The aggregate stage phase for `bios-version-upgrade` transitions:
```yaml
- name: bios-version-upgrade
  phase: InProgress   # aggregate: not all servers Completed
  servers:
    - serverRef: server-a   phase: InProgress
    - serverRef: server-b   phase: Completed
    - serverRef: server-c   phase: Completed
```

---

## `MaintenancePipeline` Status

```yaml
status:
  # Summary across all MaintenancePipelineRun objects
  runs:
    total: 3
    pending: 0
    inProgress: 2
    completed: 1
    failed: 0
  conditions:
    - type: Progressing
      status: "True"
      reason: RunsInProgress
      message: "2 of 3 runs in progress"
    - type: Completed
      status: "False"
    - type: Degraded
      status: "False"
```

---

## Version Hydration for Version-Agnostic Stages

`BIOSSettingsTemplate.version` is a required field in the existing API. Pipeline stages that apply settings regardless of the current firmware version (e.g. `bios-pre-upgrade`) should not need to hard-code a version.

### Resolution

When a stage `template` omits `version` (or sets it to `""`), the Run controller **hydrates the field at child-creation time** from the live hardware state:

| Stage `kind` | Source of hydrated version |
|---|---|
| `BIOSSettings` | `server.status.biosVersion` |
| `BMCSettings` | `bmc.status.firmwareVersion` |

The Run reads the current version from the Server or BMC object at the moment it stamps the child, and injects it into `spec.version` on the child resource.

```yaml
# Pipeline stage — user omits version (version-agnostic intent)
- name: bios-pre-upgrade
  kind: BIOSSettings
  template:
    # version: omitted — "apply at whatever is running now"
    settingsFlow:
      - name: pxe-prepare
        priority: 0
        settings:
          PxeBootToFirstInterface: Enabled
    serverMaintenancePolicy: OwnerApproval
```

```yaml
# Child BIOSSettings stamped by the Run — version hydrated from server.status.biosVersion
apiVersion: metal.ironcore.dev/v1alpha1
kind: BIOSSettings
metadata:
  name: bios-pre-upgrade-w2kp7             # generated via generateName: bios-pre-upgrade-
  labels:
    metal.ironcore.dev/pipeline-run: ilo6-upgrade-to-v180-x7k9f
    metal.ironcore.dev/stage:        bios-pre-upgrade
    metal.ironcore.dev/server:       server-a
spec:
  serverRef:
    name: server-a
  version: "U54 v2.10"        # ← injected by Run from server.status.biosVersion at stamp time
  settingsFlow:
    - name: pxe-prepare
      priority: 0
      settings:
        PxeBootToFirstInterface: Enabled
  serverMaintenancePolicy: OwnerApproval
```

### Why this is safe

- The `BIOSSettings` controller uses `version` to confirm the settings are applied against the correct firmware. Since the Run stamps the child only after all preceding stages have completed (sequential execution), the hydrated version will always match the hardware at execution time.
- If drift causes the stage to re-run, the Run re-reads the current version fresh and hydrates a new child — so the stamped version always reflects reality at the time of application.
- No version string comparison is performed by the pipeline — the hydration is a simple field copy from `status`, not an ordering decision.

---

## Open Questions / TODOs

> These sections should be resolved before the design is finalised.

1. ~~**`version` field on `BIOSSettingsTemplate` is required**~~ — **Resolved.** When `version` is omitted in a stage `template`, the Run controller hydrates it at child-creation time from the live object: `server.status.biosVersion` for Server-scoped stages, `bmc.status.firmwareVersion` for BMC-scoped stages. The child `BIOSSettings` / `BMCSettings` is stamped with the version the hardware is actually running. See [Version Hydration](#version-hydration-for-version-agnostic-stages).

2. ~~**Per-hop pre/post settings**~~ — **Won't implement (no real-world precedent).** Vendor documentation (Dell iDRAC, HPE iLO) and operator community forums show no documented requirement to change BMC/BIOS settings differently before each individual version hop. Upgrade prerequisites are always about reaching a minimum firmware version before the next jump — which the ordered `templates: []` list already handles. The practical need ("apply settings before upgrading") is fully covered by placing a `BMCSettings` / `BIOSSettings` stage before the version stage in the list. Per-hop settings granularity would add significant schema complexity for a use case that has not been evidenced. Deferred as future work if a concrete vendor requirement emerges.

3. ~~**Admission webhook**~~ — **Implementation task, no open design questions.** The validation rules are fully defined and the project already has the `CustomValidator` webhook infrastructure in `internal/webhook/v1alpha1/`. Two checks on `spec.stages` at `ValidateCreate` / `ValidateUpdate`:
   1. Each stage `kind` must be one of `BMCSettings`, `BMCVersion`, `BIOSSettings`, `BIOSVersion`.
   2. `templates: []` is only valid on `BMCVersion` and `BIOSVersion` stages; `template:` (singular) is only valid on `BMCSettings` and `BIOSSettings` stages.

   Sequential execution eliminates cycle detection and cross-kind edge validation entirely.

4. ~~**Settings drift detection**~~ — **Resolved.** No hash fields required. Completed children are frozen (`suspend: true`) immediately after the Run observes their `Completed` state. The child controller continues evaluating the hardware even while suspended; when drift is detected, it transitions to `Pending` (held there by `suspend: true`, no maintenance window requested). The Run's watch fires on the `Pending` transition and triggers the recovery sequence. The child is the drift detector; `suspend: true` prevents it from acting autonomously; the Run is the recovery coordinator.

5. ~~**Stage execution parallelism and `maxConcurrent` semantics**~~ — **Resolved.** Stages execute strictly sequentially in list order. `maxConcurrent` caps fleet-level parallelism only (number of `MaintenancePipelineRun` objects in `InProgress` simultaneously). For Server-scoped stages, all servers in `serverRefs` are stamped simultaneously; the stage advances only when all servers reach `Completed`. Documented in [Stage Execution Order](#stage-execution-order).

6. ~~**`MaintenancePipelineRun` naming**~~ — **Resolved.** The controller uses `generateName` for all `MaintenancePipelineRun` and child resource creation. Names are opaque (API-server-generated suffix). Lookup is done exclusively via label selectors:
   - Run lookup: `metal.ironcore.dev/pipeline=<pipeline-name>` + `metal.ironcore.dev/bmc=<bmc-name>`
   - Child lookup: `metal.ironcore.dev/pipeline-run=<run-name>` + `metal.ironcore.dev/stage=<stage-name>` (+ `metal.ironcore.dev/server=<server-name>` for Server-scoped children)

   This is the same pattern used by `CronJob` → `Job`. No name length collision is possible.

7. ~~**Child resource lifecycle on pipeline deletion**~~ — **Resolved. No new field required.** The existing child webhook and finalizer protection already covers this:
   - The `BMCVersion` webhook `ValidateDelete` rejects any deletion attempt while `status.state == InProgress`, blocking the GC cascade at the API server level.
   - The `bmcversion` finalizer on the controller side holds the object in terminating state until the reconcile loop exits cleanly.
   - The same pattern applies to `BIOSVersion` and `BMCSettings` / `BIOSSettings` (each has its own finalizer and in-progress webhook guard).

   The deletion chain is therefore: `MaintenancePipelineRun` deleted → GC marks `BMCVersion` for deletion → webhook rejects → cascade blocked until flash completes → finalizer removed → GC proceeds. No `deletionPolicy` field needed.

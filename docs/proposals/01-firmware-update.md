---
title: Server BIOS/Firmware update
oep-number: 1
creation-date: 2024-09-02
status: under review
authors:
  - "@aobort"
reviewers:
  -
---

# OEP-0001: Server BIOS/Firmware update

## Table of Contents

- [Summary](#summary)
- [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals](#non-goals)
- [Proposal](#proposal)
    - [API Design](#api-design)
        - [ServerBIOS CRD](#serverbios-crd)
        - [ServerFirmware CRD](#serverfirmware-crd)
    - [Components](#components)
        - [ServerBIOS Controller](#serverbios-controller)
        - [ServerFirmware Controller](#serverfirmware-controller)
        - [FMI (Firmware Management Interface)](#fmi-firmware-management-interface)
    - [Workflow](#workflow)
      - [ServerBIOS state management](#serverbios-state-management)
      - [ServerFirmware state management](#serverfirmware-state-management)
    - [Safety considerations](#safety-considerations)
    - [Implementation Details](#implementation-details)
    - [Implementation Plan](#implementation-plan)
        - [Phase 1: BIOS Controller and Scan Functionality](#phase-1-bios-controller-and-scan-functionality)
        - [Phase 2: Maintenance Mode and Settings Update](#phase-2-maintenance-mode-and-settings-update)
        - [Phase 3: BIOS Version Update and Settings Compatibility](#phase-3-bios-version-update-and-settings-compatibility)
        - [Phase 4: Firmware Controller and Scan Functionality](#phase-4-firmware-controller-and-scan-functionality)
        - [Phase 5: Firmware Version Update and Settings Compatibility](#phase-5-firmware-version-update-and-settings-compatibility)
        - [Phase 6: Finalization](#phase-6-finalization)
    - [Testing Strategy](#testing-strategy)
- [Future Work](#future-work)

## Summary

Linked issue: [#99 BIOS/Firmware Update](https://github.com/ironcore-dev/metal-operator/issues/99)
PoC implementation: [#138 PoC: BIOS version & settings management](https://github.com/ironcore-dev/metal-operator/pull/138)

This proposal outlines the design for managing server BIOS configurations in the metal-operator, including version control and settings management through a dedicated ServerBIOS controller.

## Motivation

It is necessary to provide a robust and scalable solution to automate servers' BIOS management. It should provide a clear and concise API. It should provide the ability to override common settings in particular circumstances for particular servers. It should be capable to:

- Monitor current BIOS versions and settings;
- Update BIOS versions safely;
- Modify BIOS settings consistently;
- Handle these operations at scale through Kubernetes native patterns;

### Goals

- Provide declarative BIOS management through Kubernetes CRDs
- Enable automated BIOS version updates
- Support BIOS settings modifications
- Ensure safe operations with proper server state handling
- Implement vendor-agnostic BIOS management interface

### Non-Goals

- Vendor-specific BIOS feature management
- Direct low-level BIOS programming

## Proposal

### API Design

The following CRs aimed to represent the current state of a particular server:

- [ServerBIOS CRD](#serverbios-crd)
- [ServerFirmware CRD](#serverfirmware-crd)

Listed custom resources must be cluster-scoped.

#### ServerBIOS CRD

`ServerBIOS` CR represents the desired BIOS version and settings for concrete hardware server. The `.spec` of this type defines:

- the reference to the `Server` object;
- desired BIOS version and BIOS settings;

The `.status` of this type reflects:

- information about the BIOS version and settings which are actually applied;

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: ServerBIOS
spec:
  serverRef:
    name: string
  bios:
    version: string
    settings:
      setting1: value1
      setting2: value2
status:
  bios:
    version: string
    settings:
      setting1: value1
      setting2: value2
```

The target `Server` object MUST also contain the reference to the `ServerBIOS` object.

#### ServerFirmware CRD

`ServerFirmware` CR represents the desired firmware version for concrete hardware server. The `.spec` of this type defines:

- the reference to the `Server` object;
- desired firmware versions;

The `.status` of this type reflects:

- information about the firmware versions which are actually applied;

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: ServerFirmware
spec:
  serverRef:
    name: string
  firmwares:
    - version: string
      name: string
      manufacturer: string
    - version: string
      name: string
      manufacturer: string
    - version: string
      name: string
      manufacturer: string
status:
  firmwares:
    - version: string
      name: string
      manufacturer: string
    - version: string
      name: string
      manufacturer: string
    - version: string
      name: string
      manufacturer: string
```

The target `Server` object MUST also contain the reference to the `ServerFirmware` object.

### Components

#### ServerBIOS Controller

Responsible for:
- Reconciling desired vs actual BIOS state
- Managing BIOS version updates
- Applying BIOS settings changes
- Coordinating with server maintenance states

#### ServerFirmware Controller

Responsible for:
- Reconciling desired vs actual firmware state
- Managing firmware version updates
- Coordinating with server maintenance states

#### FMI (Firmware Management Interface)

Provides:
- Task runner interface for BIOS/Firmware operations
- Server interface for invoking task runner operations
- Client interface for requesting BIOS/Firmware operations

### Workflow

#### ServerBIOS state management

1. Pre-flight checks

    - Ensure reference to Server object exists
    - Ensure referred Server object exists
    - Ensure Server has mutual reference to ServerBIOS object

2. Discovery Phase

    - Controller initiates BIOS scan via FMI
    - Observed version and settings are recorded in status if (one of):
        * desired state is not specified
        * desired state matches observed state

3. Update Detection

    - Controller compares spec vs status
    - Determines if version or settings updates needed

4. Maintenance Mode

    - Server placed in maintenance state before updates to ensure safe BIOS modifications

5. Update Execution

    - Version updates performed if needed
    - Settings changes applied if needed
    - Reboot coordination if required

6. Post-flight

    - Server removed from maintenance state

#### ServerFirmware state management

1. Pre-flight checks

    - Ensure reference to Server object exists
    - Ensure referred Server object exists
    - Ensure Server has mutual reference to ServerFirmware object

2. Discovery Phase

    - Controller initiates firmware scan via FMI
    - Observed firmware versions are recorded in status if (one of):
        * desired state is not specified
        * desired state matches observed state

3. Update Detection

    - Controller compares spec vs status
    - Determines if version updates needed

4. Maintenance Mode

    - Server placed in maintenance state before updates to ensure safe firmware modifications

5. Update Execution

    - Firmware updates performed if needed
    - Reboot coordination if required

6. Post-flight

    - Server removed from maintenance state

### Safety Considerations

1. Server State Management

    - Updates only performed in maintenance mode
    - Proper finalizer handling for cleanup

2. Update Validation

    - Version compatibility checking
    - Settings validation before apply

3. Error Handling

    - Graceful failure recovery
    - Status condition updates
    - Requeue mechanism for retries

### Implementation Details

1. Reconciliation Loop

    ```go
    func (r *ServerBIOSReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
        // 1. Get ServerBIOS object
        // 2. Check if reconciliation required
        // 3. Perform scan operation
        // 4. Handle updates if needed
        // 5. Update status
    }
    ```

    ```go
    func (r *ServerFirmwareReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
        // 1. Get ServerFirmware object
        // 2. Check if reconciliation required
        // 3. Perform scan operation
        // 4. Handle updates if needed
        // 5. Update status
    }
    ```

2. Task Runner Interface

    ```go
    type TaskRunnerBIOS interface {
        ExecuteScan(ctx context.Context, serverBIOSRef string) (ScanResult, error)
        ExecuteSettingsApply(ctx context.Context, serverBIOSRef string) (SettingsApplyResult, error)
        ExecuteVersionUpdate(ctx context.Context, serverBIOSRef string) error
    }
    ```

    ```go
    type TaskRunnerFirmware interface {
        ExecuteScan(ctx context.Context, serverFirmwareRef string) (ScanResult, error)
        ExecuteVersionUpdate(ctx context.Context, serverFirmwareRef string) error
    }
    ```

3. Server Interface

    ```go
    type Server interface {
        TaskRunnerBIOS
        TaskRunnerFirmware
        Start(ctx context.Context) error
    }
    ```

   Server interface embeds TaskRunner interface to guarantee server is capable of running BIOS operations.

4. Client Interface

    ```go
    type TaskRunnerClient interface {
        ScanBIOS(ctx context.Context, serverBIOSRef string) (ScanResult, error)
        BIOSSettingsApply(ctx context.Context, serverBIOSRef string) (SettingsApplyResult, error)
        BIOSVersionUpdate(ctx context.Context, serverBIOSRef string) error
        ScanFirmware(ctx context.Context, serverFirmwareRef string) (ScanResult, error)
        FirmwareVersionUpdate(ctx context.Context, serverFirmwareRef string) error
    }
    ```

### Implementation Plan

#### Phase 1: BIOS Controller and Scan Functionality

- Basic ServerBIOS CRD implementation
- Core controller logic
- Scan functionality
- Documentation and testing

#### Phase 2: Maintenance Mode and Settings Update

- Maintenance mode integration
- Settings update functionality
- Documentation and testing

#### Phase 3: BIOS Version Update and Settings Compatibility

- Version update functionality
- Settings compatibility validation
- Documentation and testing

#### Phase 4: Firmware Controller and Scan Functionality

- Basic ServerFirmware CRD implementation
- Core controller logic
- Scan functionality
- Documentation and testing

#### Phase 5: Firmware Version Update and Settings Compatibility

- Version update functionality
- Documentation and testing

#### Phase 6: Finalization

- Error handling improvements
- Status reporting enhancements
- Documentation and testing

### Testing Strategy

1. Unit Tests

    - Controller logic
    - Task runner implementations
    - State transitions

2. Integration Tests

    - End-to-end BIOS updates
    - Settings modifications
    - Error scenarios

3. Performance Tests

    - Scale testing
    - Concurrent operations

## Future Work

- ServerBIOS group support based on built-in Kubernetes mechanisms like labels and selectors;
- Long-running tasks state tracking;
- ServerFirmware group support;

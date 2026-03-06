# BMC Task Tracking

> **Note:** The BMCTask controller is currently implemented but requires API changes to be functional.
> Specifically, the `BMC.Status.Tasks` field must be added to the BMC API (see PR #XXX).
> This controller can be merged independently and will become active once the API changes are merged.

## Overview

All BMC operations are tracked centrally in `BMC.Status.Tasks[]`. This provides a single source of truth for all BMC operations across multiple controllers.

## Architecture

### Dedicated Task Controller (New in v0.x.x) - Initial Rollout for ServerCleaning

The **BMCTask controller** is a dedicated controller responsible for monitoring BMC task progress. This separation of concerns provides:

- ✅ **Consistent polling** - All tasks polled at configurable intervals (default 30s)
- ✅ **Automatic monitoring** - Tasks update even when parent resources don't change
- ✅ **Better performance** - No task polling overhead on cleaning operations
- ✅ **Simplified controllers** - Controllers only create tasks, don't poll

**Current Implementation Status:**
- ✅ **ServerCleaning Controller** - Uses BMCTask controller for task monitoring
- 🔄 **Other Controllers** - Still use their own polling mechanisms (future enhancement)

```
┌─────────────────────────────────────────────────────────────┐
│                     BMC Resource                             │
│  ┌────────────────────────────────────────────────────────┐ │
│  │ Status:                                                 │ │
│  │   Tasks: []BMCTask  ← Single source of truth           │ │
│  │     - TaskURI, Type, State, Progress, Message          │ │
│  └────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
         ▲                                    ▲
         │ Creates tasks                      │ Polls & updates
         │                                    │
    ┌────┴─────┐                      ┌──────┴────────┐
    │SrvClean  │ ◄─────watches───────│   BMCTask     │
    │          │    task updates      │  Controller   │
    │          │                      │               │
    └──────────┘                      │ • Watches BMC │
                                      │ • Polls tasks │
                                      │ • Updates     │
                                      │   progress    │
                                      │ • Requeues    │
                                      └───────────────┘
```

### Controller Responsibilities

**BMCTask Controller (Dedicated Task Monitor):**
- Watches BMC resources that have tasks
- Polls BMC API for task status every 30s (configurable via `--task-poll-interval`)
- Updates `BMC.Status.Tasks` with latest State, PercentComplete, Message
- Automatically requeues when active tasks exist
- Stops polling when all tasks reach terminal states
- **Currently used by**: ServerCleaning controller

**Controllers Using BMCTask Controller:**
- **ServerCleaning Controller**: Creates tasks for cleaning operations, watches BMC for updates

**Controllers Using Own Polling (Future Migration):**
- **BMC Controller**: Still polls tasks during reconciliation (uses `updateBMCTaskStatus()`)
- **BMCVersion Controller**: Still has 2-minute polling via `ResyncInterval`
- **BMCSettings Controller**: Synchronous operations (no polling needed)

**Interaction Pattern (ServerCleaning):**
1. **Task Creation**: ServerCleaning adds task entry to `BMC.Status.Tasks` with initial state
2. **Automatic Monitoring**: BMCTask controller automatically detects new task and begins polling
3. **Progress Updates**: BMCTask controller updates task status every 30s
4. **Completion Detection**: BMCTask controller stops polling when task reaches terminal state
5. **Watch for Updates**: ServerCleaning controller watches BMC resources and reacts to task status changes

### Task Structure

Each `BMCTask` contains:

```go
type BMCTask struct {
    TaskURI         string      // Unique identifier for the task
    TaskType        BMCTaskType // Type of operation
    TargetID        string      // What the task operates on (e.g., "BMC", "BIOS", "Drive-1")
    State           string      // Current state (e.g., "New", "Running", "Completed", "Failed")
    PercentComplete int32       // Progress (0-100)
    Message         string      // Additional information
    LastUpdateTime  metav1.Time // When task was last updated
}
```

### Task Types

- **FirmwareUpdate**: BMC/BIOS firmware upgrades
- **ConfigurationChange**: BMC/BIOS attribute changes
- **DiskErase**: Disk wiping operations
- **BMCReset**: BMC reset operations
- **BIOSReset**: BIOS reset to defaults
- **NetworkClear**: Network configuration cleanup
- **AccountManagement**: User account operations
- **Other**: Other operations

## Task Lifecycle

### Automatic Task Monitoring (BMCTask Controller)

The **BMCTask controller** is a dedicated controller that automatically monitors all in-progress tasks:

**How it works:**
1. **Watches BMC resources** that have non-empty `Status.Tasks` arrays
2. **Runs every 30 seconds** (configurable via `--task-poll-interval` flag)
3. **Iterates through tasks** in `BMC.Status.Tasks`
4. **Skips terminal states**: `Completed`, `Failed`, `Killed`, `Exception`, `Cancelled`
5. **Polls the BMC** via `bmcClient.GetTaskStatus(taskURI)` for active tasks
6. **Updates task status** with latest `State`, `PercentComplete`, `Message`, and `LastUpdateTime`
7. **Persists changes** via `Status().Update()` if any tasks were updated
8. **Automatic requeue**: Continues polling as long as active tasks exist

**Key Benefits:**
- ✅ **Automatic monitoring** - Tasks update even if BMC resource doesn't change
- ✅ **Consistent frequency** - All tasks polled at same interval regardless of source
- ✅ **No event dependency** - Doesn't rely on BMC reconciliation to trigger updates
- ✅ **Works across restarts** - Tasks persisted in BMC status survive controller restarts
- ✅ **Simplified controllers** - BMCVersion/BMCSettings/ServerCleaning don't need polling logic

**Terminal States** (tasks that are no longer polled):
- `Completed` - Task finished successfully
- `Failed` - Task encountered an error
- `Killed` - Task was terminated
- `Exception` - Task threw an exception
- `Cancelled` - Task was cancelled

**Configuration:**
```bash
# Default 30 second polling interval
./manager

# Custom interval (e.g., 15 seconds)
./manager --task-poll-interval=15s

# Longer interval for less frequent updates (e.g., 1 minute)
./manager --task-poll-interval=1m
```

### 1. Synchronous Operations

For operations that complete immediately (e.g., BMC settings changes):

```go
task := metalv1alpha1.BMCTask{
    TaskURI:         fmt.Sprintf("config-change-%s-%s", name, time.Now().Format("20060102-150405")),
    TaskType:        metalv1alpha1.BMCTaskTypeConfigurationChange,
    TargetID:        "BMC",
    State:           "Completed",
    PercentComplete: 100,
    Message:         fmt.Sprintf("Applied %d BMC attributes", len(attributes)),
    LastUpdateTime:  metav1.Now(),
}
```

### 2. Asynchronous Operations

For long-running operations (e.g., firmware updates):

**Initial Creation:**
```go
task := metalv1alpha1.BMCTask{
    TaskURI:         taskMonitorURI, // From BMC client
    TaskType:        metalv1alpha1.BMCTaskTypeFirmwareUpdate,
    TargetID:        "BMC",
    State:           "New",
    PercentComplete: 0,
    Message:         fmt.Sprintf("Upgrading BMC firmware to %s", version),
    LastUpdateTime:  metav1.Now(),
}
```

**Progress Updates:**
```go
// Poll task status from BMC client
taskStatus, err := bmcClient.GetBMCUpgradeTask(ctx, manufacturer, taskURI)

// Update task in BMC status
updateBMCTask(ctx, bmcName, namespace, taskURI, func(bmcTask *metalv1alpha1.BMCTask) {
    bmcTask.State = string(taskStatus.TaskState)
    bmcTask.PercentComplete = int32(*taskStatus.PercentComplete)
    bmcTask.Message = fmt.Sprintf("Status: %s", taskStatus.TaskStatus)
})
```

## Controller-Specific Implementations

### BMCTask Controller (Dedicated Task Monitor)

**Responsibility:**
- Automatic monitoring of all BMC tasks across all controllers

**Operations:**
- Polls task status from BMC API
- Updates `BMC.Status.Tasks` with progress
- Manages requeue for active tasks

**Implementation Details:**
```go
// Only reconciles BMCs with tasks (via event filter)
func hasTasksPredicate() predicate.Predicate {
    return predicate.Funcs{
        CreateFunc: func(e event.CreateEvent) bool {
            bmc := e.Object.(*metalv1alpha1.BMC)
            return len(bmc.Status.Tasks) > 0
        },
        UpdateFunc: func(e event.UpdateEvent) bool {
            bmc := e.ObjectNew.(*metalv1alpha1.BMC)
            return len(bmc.Status.Tasks) > 0
        },
    }
}

// Polls tasks and updates status
func (r *BMCTaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Fetch BMC, skip if no tasks
    // Get BMC client
    // Iterate through tasks, poll non-terminal ones
    // Update BMC.Status.Tasks if changed
    // Requeue if active tasks exist
    return ctrl.Result{RequeueAfter: r.PollInterval}, nil
}
```

**Configuration:**
- `--task-poll-interval` flag controls polling frequency (default 30s)

### BMC Controller

**Operations Tracked:**
- BMC reset operations

**Helper Functions:**
- `addBMCTask(bmcObj, task)` - Add new task to BMC status
- `updateBMCTask(bmcObj, taskURI, updateFn)` - Update existing task
- `getBMCTask(bmcObj, taskURI)` - Retrieve task by URI

**Important:** The BMC controller **no longer polls tasks**. It only creates tasks for its operations. The BMCTask controller handles all polling automatically.

**Example Usage:**
```go
func (r *BMCReconciler) resetBMC(ctx context.Context, bmcObj *metalv1alpha1.BMC) error {
    // ... perform reset ...

    task := metalv1alpha1.BMCTask{
        TaskURI:         fmt.Sprintf("bmc-reset-%s", time.Now().Format("20060102-150405")),
        TaskType:        metalv1alpha1.BMCTaskTypeBMCReset,
        TargetID:        "BMC",
        State:           "Completed",
        PercentComplete: 100,
        Message:         "BMC reset initiated",
        LastUpdateTime:  metav1.Now(),
    }
    r.addBMCTask(bmcObj, task)

    return r.updateBMCState(ctx, bmcObj, metalv1alpha1.BMCStatePending)
}
```

### BMCVersion Controller

**Operations Tracked:**
- Firmware upgrade operations

**Helper Functions:**
- `addTaskToBMC(ctx, bmcName, namespace, task)` - Add task to referenced BMC

**Important:** The BMCVersion controller **no longer polls** for task progress. The BMCTask controller automatically monitors all in-progress tasks. The BMCVersion controller only needs to:
1. Create the task when starting a firmware upgrade
2. Watch the BMC resource for task status updates
3. React to task completion/failure

**Example Usage:**
```go
// When issuing upgrade
taskMonitor, _, err := bmcClient.UpgradeBMCVersion(ctx, manufacturer, params)
if taskMonitor != "" {
    r.addTaskToBMC(ctx, bmcVersion.Spec.BMCRef.Name, bmcVersion.Namespace, metalv1alpha1.BMCTask{
        TaskURI:         taskMonitor,
        TaskType:        metalv1alpha1.BMCTaskTypeFirmwareUpdate,
        TargetID:        "BMC",
        State:           "New",
        PercentComplete: 0,
        Message:         fmt.Sprintf("Upgrading BMC firmware to %s", bmcVersion.Spec.Version),
        LastUpdateTime:  metav1.Now(),
    })
}

// To check progress - read from BMC.Status.Tasks (BMCTask controller updates it automatically)
bmc := &metalv1alpha1.BMC{}
if err := r.Get(ctx, types.NamespacedName{Name: bmcName, Namespace: namespace}, bmc); err != nil {
    return err
}
for _, task := range bmc.Status.Tasks {
    if task.TaskURI == taskMonitor {
        // Task is automatically updated by BMCTask controller
        if task.State == "Completed" {
            // Firmware upgrade complete
        } else if task.State == "Failed" {
            // Firmware upgrade failed
        }
        break
    }
}
```

### BMCSettings Controller

**Operations Tracked:**
- BMC attribute configuration changes

**Helper Functions:**
- `addTaskToBMC(ctx, bmcName, namespace, task)` - Add task to referenced BMC

**Important:** For synchronous operations (immediate configuration changes), tasks are created with `State: "Completed"`. The BMCTask controller will not poll these since they're already in a terminal state.

**Example Usage:**
```go
err = bmcClient.SetBMCAttributesImmediately(ctx, BMC.Spec.BMCUUID, attributes)
if err != nil {
    return fmt.Errorf("failed to set BMC settings: %w", err)
}

// Record configuration change (synchronous operation - already completed)
taskURI := fmt.Sprintf("config-change-%s-%s", bmcSetting.Name, time.Now().Format("20060102-150405"))
r.addTaskToBMC(ctx, bmcSetting.Spec.BMCRef.Name, bmcSetting.Namespace, metalv1alpha1.BMCTask{
    TaskURI:         taskURI,
    TaskType:        metalv1alpha1.BMCTaskTypeConfigurationChange,
    TargetID:        "BMC",
    State:           "Completed",
    PercentComplete: 100,
    Message:         fmt.Sprintf("Applied %d BMC attributes", len(attributes)),
    LastUpdateTime:  metav1.Now(),
})
```

### ServerCleaning Controller

**Operations Tracked:**
- Disk erase operations
- BIOS reset operations
- Network configuration cleanup
- Account management operations

**Helper Functions:**
- `addTaskToBMC(ctx, bmcName, namespace, task)` - Add task to referenced BMC

**Important:** The ServerCleaning controller **no longer polls** for task progress. The BMCTask controller automatically monitors all in-progress tasks. The ServerCleaning controller only needs to:
1. Create tasks when starting cleaning operations
2. Watch the BMC resource for task status updates
3. React to task completion/failure to proceed with next cleaning steps

**Example Usage:**
```go
// Start disk erase operation
taskURI, err := bmcClient.ErasePhysicalDrive(ctx, driveURI)
if err != nil {
    return err
}

// Create task in BMC status
r.addTaskToBMC(ctx, bmcName, namespace, metalv1alpha1.BMCTask{
    TaskURI:         taskURI,
    TaskType:        metalv1alpha1.BMCTaskTypeDiskErase,
    TargetID:        driveURI,
    State:           "New",
    PercentComplete: 0,
    Message:         fmt.Sprintf("Erasing drive %s", driveURI),
    LastUpdateTime:  metav1.Now(),
})

// BMCTask controller will automatically poll and update this task
// ServerCleaning controller watches BMC and reacts to task completion
```

## Task Cleanup

Tasks are automatically pruned to prevent unbounded growth:
- Only the **last 10 tasks** are retained per BMC
- Older tasks are automatically removed when new tasks are added
- This happens transparently in `addBMCTask()` helper functions

## Querying Tasks

### From CLI

```bash
# List all tasks for a BMC
kubectl get bmc <bmc-name> -o jsonpath='{.status.tasks[*]}' | jq

# Get specific task type
kubectl get bmc <bmc-name> -o jsonpath='{.status.tasks[?(@.taskType=="FirmwareUpdate")]}' | jq

# Watch task progress
watch 'kubectl get bmc <bmc-name> -o jsonpath="{.status.tasks[0]}" | jq'

# Get tasks with specific state
kubectl get bmc <bmc-name> -o jsonpath='{.status.tasks[?(@.state=="Running")]}' | jq
```

### From Code

```go
// Get BMC object
bmc := &metalv1alpha1.BMC{}
err := client.Get(ctx, types.NamespacedName{Name: bmcName}, bmc)

// List all tasks
for _, task := range bmc.Status.Tasks {
    fmt.Printf("Task: %s, Type: %s, State: %s, Progress: %d%%\n",
        task.TaskURI, task.TaskType, task.State, task.PercentComplete)
}

// Find specific task
for _, task := range bmc.Status.Tasks {
    if task.TaskURI == targetURI {
        fmt.Printf("Found task: %s at %d%% complete\n", task.Message, task.PercentComplete)
        break
    }
}
```

## Benefits

### Single Source of Truth
- All BMC operations tracked in one place
- Eliminates duplication across controller status fields
- Simplifies operational monitoring

### Cross-Controller Awareness
- See all operations affecting a BMC regardless of source
- Better understanding of BMC state and activity
- Prevents conflicting operations

### Operational Transparency
- Complete audit trail of BMC operations
- Task history preserved (last 10 tasks)
- Clear progress indicators for async operations

### Better Failure Recovery
- Tasks persist in BMC status across controller restarts
- Can resume monitoring of long-running operations
- Clear indication of failed operations

## Migration Notes

### Backward Compatibility

**BMCVersion Controller:**
- Still maintains `Status.UpgradeTask` field (deprecated but updated)
- This allows existing monitoring/tooling to continue working
- Plan to remove in future version once consumers migrate

**BMCSettings Controller:**
- No previous task tracking existed
- Pure addition of functionality

**BMC Controller:**
- Tasks field was previously unused
- Now actively populated

### Architecture Changes (v0.x.x)

**What Changed:**

**Before (Old Architecture):**
- BMC controller polled tasks during every reconciliation (event-driven, inconsistent)
- BMCVersion controller had its own 2-minute polling loop
- ServerCleaning controller had its own 30-second polling loop
- Tasks only updated when reconciliation triggered
- Redundant BMC API calls from multiple controllers

**After (New Architecture):**
- Dedicated BMCTask controller handles ALL task polling
- Consistent 30-second polling interval (configurable)
- Tasks update automatically even without reconciliation events
- Single BMC API call per task per interval
- Other controllers only create tasks and watch for updates

**Migration Impact:**

✅ **No API changes** - `BMC.Status.Tasks` structure unchanged
✅ **No configuration changes** - Works with existing BMC resources
✅ **New flag available** - `--task-poll-interval` (default 30s maintains similar behavior)
✅ **Better consistency** - Tasks now update predictably every 30s
✅ **Improved performance** - Eliminates redundant polling overhead

**Deployment:**

1. Deploy new controller version with BMCTask controller
2. Verify task polling works as expected
3. Monitor logs for any issues
4. Roll back if needed (old architecture code preserved in git history)

**Testing:**

```bash
# Verify BMCTask controller is running
kubectl get pods -n metal-operator-system
kubectl logs -n metal-operator-system deployment/controller-manager | grep BMCTaskReconciler

# Test task polling
kubectl apply -f test-bmcversion.yaml

# Watch task progress (should update every 30s)
watch 'kubectl get bmc <bmc-name> -o jsonpath="{.status.tasks[0]}" | jq'

# Verify consistent updates
kubectl get bmc <bmc-name> -o jsonpath='{.status.tasks[0].lastUpdateTime}'
# Should update every ~30 seconds for active tasks
```

**Rollback Plan:**

If issues are found:
1. Revert to previous version
2. BMC controller will resume event-driven polling
3. No data loss - tasks persisted in BMC.Status.Tasks
4. Report issue with logs and reproduction steps

### Migrating Consumers

If you're consuming BMC operation status:

**Before:**
```go
// Old way - check specific controller status
bmcVersion := &metalv1alpha1.BMCVersion{}
client.Get(ctx, key, bmcVersion)
progress := bmcVersion.Status.UpgradeTask.PercentComplete
```

**After:**
```go
// New way - check BMC tasks
bmc := &metalv1alpha1.BMC{}
client.Get(ctx, key, bmc)
for _, task := range bmc.Status.Tasks {
    if task.TaskType == metalv1alpha1.BMCTaskTypeFirmwareUpdate {
        progress := task.PercentComplete
        break
    }
}
```

## Future Enhancements

Potential improvements:
- Task filtering by date range
- Task persistence to external storage for long-term audit
- Webhooks/events when tasks complete
- Task cancellation support
- Task priority/scheduling

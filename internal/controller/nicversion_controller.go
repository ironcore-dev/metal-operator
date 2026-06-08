// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
    "context"
    "errors"
    "fmt"
    "time"

    "github.com/ironcore-dev/controller-utils/clientutils"
    "github.com/ironcore-dev/controller-utils/conditionutils"
    metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
    "github.com/ironcore-dev/metal-operator/bmc"
    "github.com/ironcore-dev/metal-operator/internal/bmcutils"
    "github.com/stmcginnis/gofish"
    "github.com/stmcginnis/gofish/schemas"
    corev1 "k8s.io/api/core/v1"
    apierrors "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/apimachinery/pkg/types"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
    "sigs.k8s.io/controller-runtime/pkg/handler"
)

// NICVersionReconciler reconciles a NICVersion object
type NICVersionReconciler struct {
    client.Client
    ManagerNamespace string
    Insecure         bool
    Scheme           *runtime.Scheme
    BMCOptions       bmc.Options
    ResyncInterval   time.Duration
    Conditions       *conditionutils.Accessor
}

const (
    NICVersionFinalizer = "metal.ironcore.dev/nicversion"

    ConditionNICUpgradeIssued       = "NICUpgradeIssued"
    ConditionNICUpgradeCompleted    = "NICUpgradeCompleted"
    ConditionNICUpgradePowerOn      = "NICUpgradePowerOn"
    ConditionNICUpgradePowerOff     = "NICUpgradePowerOff"
    ConditionNICUpgradeVerification = "NICUpgradeVerification"

    ReasonNICUpgradeIssued           = "NICUpgradeIssued"
    ReasonNICUpgradeIssueFailed      = "NICUpgradeIssueFailed"
    ReasonNICRebootPowerOff          = "NICRebootPowerOff"
    ReasonNICRebootPowerOn           = "NICRebootPowerOn"
    ReasonNICVersionVerified         = "NICVersionVerified"
    ReasonNICVersionVerificationFail = "NICVersionVerificationFailed"
    ReasonNICUpgradeTaskFailed       = "NICUpgradeTaskFailed"
    ReasonNICUpgradeTaskCompleted    = "NICUpgradeTaskCompleted"
)

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=nicversions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=nicversions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=nicversions/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

func (r *NICVersionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)
    nicVersion := &metalv1alpha1.NICVersion{}
    if err := r.Get(ctx, req.NamespacedName, nicVersion); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    log.V(1).Info("Reconciling NICVersion")

    return r.reconcileExists(ctx, nicVersion)
}

func (r *NICVersionReconciler) reconcileExists(ctx context.Context, nicVersion *metalv1alpha1.NICVersion) (ctrl.Result, error) {
    if r.shouldDelete(ctx, nicVersion) {
        return r.delete(ctx, nicVersion)
    }
    return r.reconcile(ctx, nicVersion)
}

func (r *NICVersionReconciler) shouldDelete(ctx context.Context, nicVersion *metalv1alpha1.NICVersion) bool {
    log := ctrl.LoggerFrom(ctx)
    if nicVersion.DeletionTimestamp.IsZero() {
        return false
    }

    if controllerutil.ContainsFinalizer(nicVersion, NICVersionFinalizer) &&
        nicVersion.Status.State == metalv1alpha1.NICVersionStateInProgress {
        if _, err := GetServerByName(ctx, r.Client, nicVersion.Spec.ServerRef.Name); apierrors.IsNotFound(err) {
            log.V(1).Info("Server not found, proceeding with deletion", "Server", nicVersion.Spec.ServerRef.Name)
            return true
        }
        log.V(1).Info("Postponing deletion as NIC version update is in progress")
        return false
    }

    return true
}

func (r *NICVersionReconciler) delete(ctx context.Context, nicVersion *metalv1alpha1.NICVersion) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)
    log.V(1).Info("Deleting NICVersion")
    defer log.V(1).Info("Deleted NICVersion")

    if !controllerutil.ContainsFinalizer(nicVersion, NICVersionFinalizer) {
        return ctrl.Result{}, nil
    }

    if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, nicVersion, NICVersionFinalizer); err != nil || modified {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, nil
}

func (r *NICVersionReconciler) cleanupServerMaintenanceReferences(ctx context.Context, nicVersion *metalv1alpha1.NICVersion) error {
    log := ctrl.LoggerFrom(ctx)
    if nicVersion.Spec.ServerMaintenanceRef == nil {
        return nil
    }

    serverMaintenance, err := r.getServerMaintenanceForRef(ctx, nicVersion.Spec.ServerMaintenanceRef)
    if err != nil && !apierrors.IsNotFound(err) {
        return fmt.Errorf("failed to get referred ServerMaintenance: %w", err)
    }

    if serverMaintenance.DeletionTimestamp.IsZero() {
        if metav1.IsControlledBy(serverMaintenance, nicVersion) {
            log.V(1).Info("Deleting ServerMaintenance", "ServerMaintenance", client.ObjectKeyFromObject(serverMaintenance))
            if err := r.Delete(ctx, serverMaintenance); err != nil {
                return err
            }
        } else {
            log.V(1).Info("ServerMaintenance is controlled by somebody else", "ServerMaintenance", client.ObjectKeyFromObject(serverMaintenance))
        }
    }

    if apierrors.IsNotFound(err) || err == nil {
        log.V(1).Info("Cleaning up ServerMaintenance ref in NICVersion as the object is gone")
        if err := r.patchServerMaintenanceRef(ctx, nicVersion, nil); err != nil {
            return fmt.Errorf("failed to clean up serverMaintenance ref in NICVersionReconciler status: %w", err)
        }
    }
    return nil
}

func (r *NICVersionReconciler) reconcile(ctx context.Context, nicVersion *metalv1alpha1.NICVersion) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)
    if shouldIgnoreReconciliation(nicVersion) {
        log.V(1).Info("Skipped NICVersion reconciliation")
        return ctrl.Result{}, nil
    }

    if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, nicVersion, NICVersionFinalizer); err != nil || modified {
        return ctrl.Result{}, err
    }

    requeue, err := r.transitionState(ctx, nicVersion)
    if err != nil {
        return ctrl.Result{}, err
    }
    if requeue {
        return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
    }

    log.V(1).Info("Reconciled NICVersion")
    return ctrl.Result{}, nil
}

func (r *NICVersionReconciler) transitionState(ctx context.Context, nicVersion *metalv1alpha1.NICVersion) (bool, error) {
    log := ctrl.LoggerFrom(ctx)
    if nicVersion.Spec.ServerRef == nil {
        return false, fmt.Errorf("NICVersion does not have a ServerRef")
    }

    server, err := GetServerByName(ctx, r.Client, nicVersion.Spec.ServerRef.Name)
    if err != nil {
        return false, fmt.Errorf("failed to fetch server: %w", err)
    }

    bmcClient, err := bmcutils.GetBMCClientForServer(ctx, r.Client, server, r.Insecure, r.BMCOptions)
    if err != nil {
        if errors.As(err, &bmcutils.BMCUnAvailableError{}) {
            log.V(1).Info("BMC is not available, skipping", "BMC", server.Spec.BMCRef.Name, "Server", server.Name, "error", err)
            return true, nil
        }
        return false, fmt.Errorf("failed to get BMC client for server %s: %w", server.Name, err)
    }
    defer bmcClient.Logout()

    switch nicVersion.Status.State {
    case "", metalv1alpha1.NICVersionStatePending:
        return false, r.cleanup(ctx, bmcClient, nicVersion, server)
    case metalv1alpha1.NICVersionStateInProgress:
        if ok, err := r.handleServerMaintenance(ctx, bmcClient, nicVersion, server); err != nil || !ok {
            return false, err
        }
        return r.processInProgressState(ctx, bmcClient, nicVersion, server)
    case metalv1alpha1.NICVersionStateCompleted:
        return false, r.cleanup(ctx, bmcClient, nicVersion, server)
    case metalv1alpha1.NICVersionStateFailed:
        if shouldRetryReconciliation(nicVersion) {
            log.V(1).Info("Retrying ...")
            nicVersionBase := nicVersion.DeepCopy()
            nicVersion.Status.State = metalv1alpha1.NICVersionStatePending
            nicVersion.Status.Conditions = []metav1.Condition{}
            annotations := nicVersion.GetAnnotations()
            delete(annotations, metalv1alpha1.OperationAnnotation)
            nicVersion.SetAnnotations(annotations)
            if err := r.Status().Patch(ctx, nicVersion, client.MergeFrom(nicVersionBase)); err != nil {
                return true, fmt.Errorf("failed to patch NICVersion status for retrying: %w", err)
            }
            return true, nil
        }
        log.V(1).Info("Failed to upgrade NICVersion", "NICVersion", nicVersion, "Server", server.Name)
        return false, nil
    }

    log.V(1).Info("Unknown State found", "State", nicVersion.Status.State)
    return false, nil
}

func (r *NICVersionReconciler) handleServerMaintenance(ctx context.Context, bmcClient bmc.BMC, nicVersion *metalv1alpha1.NICVersion, server *metalv1alpha1.Server) (bool, error) {
    log := ctrl.LoggerFrom(ctx)
    if nicVersion.Spec.ServerMaintenanceRef == nil {
        if requeue, err := r.requestServerMaintenance(ctx, nicVersion, server); err != nil || requeue {
            return false, err
        }
    }

    condition, err := GetCondition(r.Conditions, nicVersion.Status.Conditions, ServerMaintenanceConditionWaiting)
    if err != nil {
        return false, err
    }

    if server.Status.State != metalv1alpha1.ServerStateMaintenance {
        log.V(1).Info("Server is not in maintenance. waiting...", "server State", server.Status.State, "server", server.Name)
        if condition.Status != metav1.ConditionTrue {
            if err := r.Conditions.Update(
                condition,
                conditionutils.UpdateStatus(corev1.ConditionTrue),
                conditionutils.UpdateReason(ServerMaintenanceReasonWaiting),
                conditionutils.UpdateMessage(fmt.Sprintf("Waiting for approval of %v", nicVersion.Spec.ServerMaintenanceRef.Name)),
            ); err != nil {
                return false, fmt.Errorf("failed to update creating ServerMaintenance condition: %w", err)
            }
            if err := r.updateStatus(ctx, nicVersion, nicVersion.Status.State, nicVersion.Status.UpgradeTask, condition); err != nil {
                return false, fmt.Errorf("failed to patch NICVersion ServerMaintenance waiting conditions: %w", err)
            }
        }
        return false, nil
    }

    if server.Spec.ServerMaintenanceRef == nil || server.Spec.ServerMaintenanceRef.UID != nicVersion.Spec.ServerMaintenanceRef.UID {
        log.V(1).Info("Server is already in maintenance", "Server", server.Name)
        if condition.Status != metav1.ConditionTrue {
            if err := r.Conditions.Update(
                condition,
                conditionutils.UpdateStatus(corev1.ConditionTrue),
                conditionutils.UpdateReason(ServerMaintenanceReasonWaiting),
                conditionutils.UpdateMessage(fmt.Sprintf("Waiting for approval of %v", nicVersion.Spec.ServerMaintenanceRef.Name)),
            ); err != nil {
                return false, fmt.Errorf("failed to update creating ServerMaintenance condition: %w", err)
            }
            if err := r.updateStatus(ctx, nicVersion, nicVersion.Status.State, nicVersion.Status.UpgradeTask, condition); err != nil {
                return false, fmt.Errorf("failed to patch NICVersion ServerMaintenance waiting conditions: %w", err)
            }
        }
        return false, nil
    }

    if condition.Reason != ServerMaintenanceReasonApproved {
        if err := r.Conditions.Update(
            condition,
            conditionutils.UpdateStatus(corev1.ConditionFalse),
            conditionutils.UpdateReason(ServerMaintenanceReasonApproved),
            conditionutils.UpdateMessage("Server is now in Maintenance mode"),
        ); err != nil {
            return false, fmt.Errorf("failed to update creating ServerMaintenance condition: %w", err)
        }
        if err := r.updateStatus(ctx, nicVersion, nicVersion.Status.State, nicVersion.Status.UpgradeTask, condition); err != nil {
            return false, fmt.Errorf("failed to patch NICVersion ServerMaintenance waiting conditions: %w", err)
        }
        return false, nil
    }

    if ok, err := r.handleBMCReset(ctx, bmcClient, nicVersion, server); !ok || err != nil {
        return false, err
    }
    return true, nil
}

func (r *NICVersionReconciler) handleBMCReset(
    ctx context.Context,
    bmcClient bmc.BMC,
    nicVersion *metalv1alpha1.NICVersion,
    server *metalv1alpha1.Server,
) (bool, error) {
    log := ctrl.LoggerFrom(ctx)
    resetBMC, err := GetCondition(r.Conditions, nicVersion.Status.Conditions, BMCConditionReset)
    if err != nil {
        return false, fmt.Errorf("failed to get condition for reset of BMC of server %v", err)
    }

    if resetBMC.Status != metav1.ConditionTrue {
        if resetBMC.Reason != BMCReasonReset {
            if err := resetBMCOfServer(ctx, r.Client, server, bmcClient); err == nil {
                if err := r.Conditions.Update(
                    resetBMC,
                    conditionutils.UpdateStatus(corev1.ConditionFalse),
                    conditionutils.UpdateReason(BMCReasonReset),
                    conditionutils.UpdateMessage("Issued BMC reset to stabilize BMC of the server"),
                ); err != nil {
                    return false, fmt.Errorf("failed to update reset BMC condition: %w", err)
                }
                return false, r.updateStatus(ctx, nicVersion, nicVersion.Status.State, nil, resetBMC)
            } else {
                log.Error(err, "failed to reset BMC of the server")
                return false, err
            }
        } else if server.Spec.BMCRef != nil {
            bmcObj := &metalv1alpha1.BMC{}
            if err := r.Get(ctx, client.ObjectKey{Name: server.Spec.BMCRef.Name}, bmcObj); err != nil {
                log.Error(err, "failed to get referred server's Manager")
                return false, err
            }
            annotations := bmcObj.GetAnnotations()
            if annotations != nil {
                if op, ok := annotations[metalv1alpha1.OperationAnnotation]; ok {
                    if op == metalv1alpha1.GracefulRestartBMC {
                        log.V(1).Info("Waiting for BMC reset as annotation on BMC object is set")
                        return false, nil
                    }
                }
            }
        }
        if err := r.Conditions.Update(
            resetBMC,
            conditionutils.UpdateStatus(corev1.ConditionTrue),
            conditionutils.UpdateReason(BMCReasonReset),
            conditionutils.UpdateMessage("BMC reset to stabilize BMC of the server is completed"),
        ); err != nil {
            return false, fmt.Errorf("failed to update power on server condition: %w", err)
        }
        return false, r.updateStatus(ctx, nicVersion, nicVersion.Status.State, nil, resetBMC)
    }
    return true, nil
}

func (r *NICVersionReconciler) processInProgressState(ctx context.Context, bmcClient bmc.BMC, nicVersion *metalv1alpha1.NICVersion, server *metalv1alpha1.Server) (bool, error) {
    log := ctrl.LoggerFrom(ctx)
    issuedCondition, err := GetCondition(r.Conditions, nicVersion.Status.Conditions, ConditionNICUpgradeIssued)
    if err != nil {
        return false, err
    }

    if issuedCondition.Status != metav1.ConditionTrue {
        log.V(1).Info("Processing NIC version upgrade ...")
        if server.Status.PowerState != metalv1alpha1.ServerOnPowerState {
            log.V(1).Info("Server in powered off state. Retrying ...", "Server", server.Name)
            return false, nil
        }
        hasPending, err := bmcClient.CheckBMCPendingComponentUpgrade(ctx, bmc.ComponentTypeNIC)
        if err != nil {
            log.V(1).Info("Failed to check pending component upgrade before NIC upgrade, proceeding anyway", "error", err)
        } else if hasPending {
            log.Info("Pending component upgrade detected, deferring NIC upgrade", "Server", server.Name)
            return true, nil
        }
        return false, r.upgradeNICVersion(ctx, bmcClient, nicVersion, server, issuedCondition)
    }

    completedCondition, err := GetCondition(r.Conditions, nicVersion.Status.Conditions, ConditionNICUpgradeCompleted)
    if err != nil {
        return false, err
    }

    if completedCondition.Status != metav1.ConditionTrue {
        log.V(1).Info("Check NIC version upgrade task status")
        requeue, err := r.checkUpdateNICUpgradeStatus(ctx, bmcClient, nicVersion, server, completedCondition)
        var TaskFetchFailed *BMCTaskFetchFailedError
        if errors.As(err, &TaskFetchFailed) {
            log.V(1).Info("Failed to fetch NIC upgrade task status from BMC.", "error", err)
            // Some vendors delete the task once upgrade is complete. Check version directly.
            currentVersion, errFetch := r.getNICVersionFromBMC(ctx, bmcClient, nicVersion)
            if errFetch != nil {
                log.Error(errors.Join(err, errFetch), "Failed to fetch current NIC version from BMC after upgrade task fetch failure.")
                return true, nil
            }
            if currentVersion == nicVersion.Spec.Version {
                log.V(1).Info("NIC version shows upgraded successfully even though task fetch failure", "Version", currentVersion)
                if err := r.Conditions.Update(
                    completedCondition,
                    conditionutils.UpdateStatus(corev1.ConditionTrue),
                    conditionutils.UpdateReason(ReasonNICUpgradeTaskCompleted),
                    conditionutils.UpdateMessage("Upgrade Task is missing. NIC version successfully upgraded to: "+nicVersion.Spec.Version),
                ); err != nil {
                    return false, fmt.Errorf("failed to update upgrade complete conditions: %w", err)
                }
                return false, r.updateStatus(ctx, nicVersion, nicVersion.Status.State, nicVersion.Status.UpgradeTask, completedCondition)
            }
            log.V(1).Info("NIC version not updated yet, need to wait for task details", "Version", currentVersion, "DesiredVersion", nicVersion.Spec.Version)
            return requeue, err
        }
        return requeue, err
    }

    rebootPowerOffCondition, err := GetCondition(r.Conditions, nicVersion.Status.Conditions, ConditionNICUpgradePowerOff)
    if err != nil {
        return false, err
    }

    if rebootPowerOffCondition.Status != metav1.ConditionTrue {
        log.V(1).Info("Ensuring server is powered off")
        if server.Status.PowerState != metalv1alpha1.ServerOffPowerState {
            return false, r.ensurePowerState(ctx, nicVersion, metalv1alpha1.PowerOff)
        }
        log.V(1).Info("Ensured server is powered off")
        if err := r.Conditions.Update(
            rebootPowerOffCondition,
            conditionutils.UpdateStatus(corev1.ConditionTrue),
            conditionutils.UpdateReason(ReasonNICRebootPowerOff),
            conditionutils.UpdateMessage("Powered off the server"),
        ); err != nil {
            return false, fmt.Errorf("failed to update reboot power off condition: %w", err)
        }
        return false, r.updateStatus(ctx, nicVersion, nicVersion.Status.State, nicVersion.Status.UpgradeTask, rebootPowerOffCondition)
    }

    rebootPowerOnCondition, err := GetCondition(r.Conditions, nicVersion.Status.Conditions, ConditionNICUpgradePowerOn)
    if err != nil {
        return false, err
    }

    if rebootPowerOnCondition.Status != metav1.ConditionTrue {
        log.V(1).Info("Ensuring server is powered on")
        if server.Status.PowerState != metalv1alpha1.ServerOnPowerState {
            return false, r.ensurePowerState(ctx, nicVersion, metalv1alpha1.PowerOn)
        }
        log.V(1).Info("Ensured server is powered on")
        if err := r.Conditions.Update(
            rebootPowerOnCondition,
            conditionutils.UpdateStatus(corev1.ConditionTrue),
            conditionutils.UpdateReason(ReasonNICRebootPowerOn),
            conditionutils.UpdateMessage("Powered on the server"),
        ); err != nil {
            return false, fmt.Errorf("failed to update reboot power on condition: %w", err)
        }
        return false, r.updateStatus(ctx, nicVersion, nicVersion.Status.State, nicVersion.Status.UpgradeTask, rebootPowerOnCondition)
    }

    verifyCondition, err := GetCondition(r.Conditions, nicVersion.Status.Conditions, ConditionNICUpgradeVerification)
    if err != nil {
        return false, err
    }

    if verifyCondition.Status != metav1.ConditionTrue {
        log.V(1).Info("Verifying NIC version update")
        currentVersion, err := r.getNICVersionFromBMC(ctx, bmcClient, nicVersion)
        if err != nil {
            return false, err
        }
        if currentVersion != nicVersion.Spec.Version {
            log.V(1).Info("NIC version not updated", "Version", currentVersion, "DesiredVersion", nicVersion.Spec.Version)
            if verifyCondition.Reason == "" {
                if err := r.Conditions.Update(
                    verifyCondition,
                    conditionutils.UpdateStatus(corev1.ConditionFalse),
                    conditionutils.UpdateReason(ReasonNICVersionVerificationFail),
                    conditionutils.UpdateMessage("waiting for NIC Version update"),
                ); err != nil {
                    return false, fmt.Errorf("failed to update the verification condition: %w", err)
                }
            }
            log.V(1).Info("Waiting for NIC version to reflect the new version")
            return true, nil
        }

        if err := r.Conditions.Update(
            verifyCondition,
            conditionutils.UpdateStatus(corev1.ConditionTrue),
            conditionutils.UpdateReason(ReasonNICVersionVerified),
            conditionutils.UpdateMessage("NIC Version updated"),
        ); err != nil {
            return false, fmt.Errorf("failed to update conditions: %w", err)
        }
        return false, r.updateStatus(ctx, nicVersion, metalv1alpha1.NICVersionStateCompleted, nicVersion.Status.UpgradeTask, verifyCondition)
    }

    log.V(1).Info("Unknown Conditions found", "Condition", verifyCondition.Type)
    return false, nil
}

// getNICVersionFromBMC reads firmware inventory and returns the version of the first matching NIC target.
func (r *NICVersionReconciler) getNICVersionFromBMC(ctx context.Context, bmcClient bmc.BMC, nicVersion *metalv1alpha1.NICVersion) (string, error) {
    entries, err := bmcClient.GetNICFirmwareInventory(ctx)
    if err != nil {
        return "", fmt.Errorf("failed to get NIC firmware inventory: %w", err)
    }

    targets := nicVersion.Status.DiscoveredTargets
    if len(nicVersion.Spec.Targets) > 0 {
        targets = nicVersion.Spec.Targets
    }

    for _, entry := range entries {
        for _, target := range targets {
            if entry.URI == target {
                return entry.Version, nil
            }
        }
    }

    // If no explicit targets, return first entry version
    if len(entries) > 0 {
        return entries[0].Version, nil
    }
    return "", fmt.Errorf("no NIC firmware inventory entries found")
}

func (r *NICVersionReconciler) cleanup(ctx context.Context, bmcClient bmc.BMC, nicVersion *metalv1alpha1.NICVersion, server *metalv1alpha1.Server) error {
    log := ctrl.LoggerFrom(ctx)

    // Auto-discover targets if not specified
    if len(nicVersion.Spec.Targets) == 0 && len(nicVersion.Status.DiscoveredTargets) == 0 {
        entries, err := bmcClient.GetNICFirmwareInventory(ctx)
        if err != nil {
            return fmt.Errorf("failed to get NIC firmware inventory: %w", err)
        }
        var discovered []string
        for _, e := range entries {
            if nicVersion.Spec.NICSelector == "" || containsSubstring(e.Name, nicVersion.Spec.NICSelector) {
                discovered = append(discovered, e.URI)
            }
        }
        if len(discovered) > 0 {
            nicVersionBase := nicVersion.DeepCopy()
            nicVersion.Status.DiscoveredTargets = discovered
            if err := r.Status().Patch(ctx, nicVersion, client.MergeFrom(nicVersionBase)); err != nil {
                return fmt.Errorf("failed to patch NICVersion discovered targets: %w", err)
            }
        }
    }

    currentVersion, err := r.getNICVersionFromBMC(ctx, bmcClient, nicVersion)
    if err != nil {
        return err
    }

    if currentVersion == nicVersion.Spec.Version {
        if err := r.cleanupServerMaintenanceReferences(ctx, nicVersion); err != nil {
            return err
        }
        log.V(1).Info("Upgraded NIC version", "Version", currentVersion, "Server", server.Name)
        return r.updateStatus(ctx, nicVersion, metalv1alpha1.NICVersionStateCompleted, nil, nil)
    }
    return r.updateStatus(ctx, nicVersion, metalv1alpha1.NICVersionStateInProgress, nil, nil)
}

func containsSubstring(s, substr string) bool {
    return len(substr) > 0 && len(s) >= len(substr) && (s == substr || len(s) > 0 && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
    for i := 0; i <= len(s)-len(substr); i++ {
        if s[i:i+len(substr)] == substr {
            return true
        }
    }
    return false
}

func (r *NICVersionReconciler) upgradeNICVersion(
    ctx context.Context,
    bmcClient bmc.BMC,
    nicVersion *metalv1alpha1.NICVersion,
    server *metalv1alpha1.Server,
    issuedCondition *metav1.Condition,
) error {
    log := ctrl.LoggerFrom(ctx)
    var username, password string
    if nicVersion.Spec.Image.SecretRef != nil {
        var err error
        password, username, err = GetImageCredentialsForSecretRef(ctx, r.Client, nicVersion.Spec.Image.SecretRef)
        if err != nil {
            return fmt.Errorf("failed to get image credentials ref for: %w", err)
        }
    }

    var forceUpdate bool
    if nicVersion.Spec.UpdatePolicy != nil && *nicVersion.Spec.UpdatePolicy == metalv1alpha1.UpdatePolicyForce {
        forceUpdate = true
    }

    parameters := &schemas.UpdateServiceSimpleUpdateParameters{
        ForceUpdate:      forceUpdate,
        ImageURI:         nicVersion.Spec.Image.URI,
        Password:         password,
        Username:         username,
        TransferProtocol: schemas.TransferProtocolType(nicVersion.Spec.Image.TransferProtocol),
    }

    // Override Targets with discovered targets if spec targets are empty
    if len(nicVersion.Spec.Targets) > 0 {
        parameters.Targets = nicVersion.Spec.Targets
    } else if len(nicVersion.Status.DiscoveredTargets) > 0 {
        parameters.Targets = nicVersion.Status.DiscoveredTargets
    }

    taskMonitor, isFatal, err := bmcClient.UpgradeNICVersion(ctx, server.Status.Manufacturer, parameters)
    upgradeCurrentTaskStatus := &metalv1alpha1.NICTask{URI: taskMonitor}

    if isFatal {
        log.Error(err, "failed to issue NIC upgrade", "Version", nicVersion.Spec.Version, "Server", server.Name)
        if errCond := r.Conditions.Update(
            issuedCondition,
            conditionutils.UpdateStatus(corev1.ConditionFalse),
            conditionutils.UpdateReason(ReasonNICUpgradeIssueFailed),
            conditionutils.UpdateMessage("Fatal error occurred. Upgrade might still go through on server."),
        ); errCond != nil {
            log.Error(errCond, "failed to update conditions")
            updateErr := r.updateStatus(ctx, nicVersion, metalv1alpha1.NICVersionStateFailed, upgradeCurrentTaskStatus, issuedCondition)
            return errors.Join(errCond, updateErr)
        }
        return r.updateStatus(ctx, nicVersion, metalv1alpha1.NICVersionStateFailed, upgradeCurrentTaskStatus, issuedCondition)
    }
    if err != nil {
        log.Error(err, "failed to issue NIC upgrade", "Version", nicVersion.Spec.Version, "Server", server.Name)
        return err
    }

    if errCond := r.Conditions.Update(
        issuedCondition,
        conditionutils.UpdateStatus(corev1.ConditionTrue),
        conditionutils.UpdateReason(ReasonNICUpgradeIssued),
        conditionutils.UpdateMessage(fmt.Sprintf("Task to upgrade has been created %v", taskMonitor)),
    ); errCond != nil {
        log.Error(errCond, "failed to update conditions")
        updateErr := r.updateStatus(ctx, nicVersion, metalv1alpha1.NICVersionStateFailed, upgradeCurrentTaskStatus, issuedCondition)
        return errors.Join(errCond, updateErr)
    }

    return r.updateStatus(ctx, nicVersion, nicVersion.Status.State, upgradeCurrentTaskStatus, issuedCondition)
}

func (r *NICVersionReconciler) checkUpdateNICUpgradeStatus(
    ctx context.Context,
    bmcClient bmc.BMC,
    nicVersion *metalv1alpha1.NICVersion,
    server *metalv1alpha1.Server,
    completedCondition *metav1.Condition,
) (bool, error) {
    log := ctrl.LoggerFrom(ctx)
    taskURI := nicVersion.Status.UpgradeTask.URI
    taskCurrentStatus, err := func() (*schemas.Task, error) {
        if taskURI == "" {
            return nil, fmt.Errorf("invalid task URI. uri provided: '%v'", taskURI)
        }
        return bmcClient.GetNICUpgradeTask(ctx, server.Status.Manufacturer, taskURI)
    }()
    if err != nil {
        return false, &BMCTaskFetchFailedError{
            TaskURI:  taskURI,
            Resource: "NICUpgrade",
            Err:      err,
        }
    }
    log.V(1).Info("NIC upgrade task current status", "TaskState", taskCurrentStatus.TaskState)

    upgradeCurrentTaskStatus := &metalv1alpha1.NICTask{
        URI:             nicVersion.Status.UpgradeTask.URI,
        State:           taskCurrentStatus.TaskState,
        Status:          taskCurrentStatus.TaskStatus,
        PercentComplete: int32(gofish.Deref(taskCurrentStatus.PercentComplete)),
    }

    transition := &conditionutils.FieldsTransition{
        IncludeStatus:  true,
        IncludeReason:  true,
        IncludeMessage: true,
    }
    checkpoint, err := transition.Checkpoint(r.Conditions, *completedCondition)
    if err != nil {
        return false, fmt.Errorf("failed to create checkpoint for Condition. %w", err)
    }

    if taskCurrentStatus.TaskState == schemas.KilledTaskState ||
        taskCurrentStatus.TaskState == schemas.ExceptionTaskState ||
        taskCurrentStatus.TaskState == schemas.CancelledTaskState ||
        (taskCurrentStatus.TaskStatus != schemas.OKHealth && taskCurrentStatus.TaskStatus != "") {
        message := fmt.Sprintf(
            "Upgrade NIC task has failed. with message %v check '%v' for details",
            taskCurrentStatus.Messages,
            taskURI,
        )
        if err := r.Conditions.Update(
            completedCondition,
            conditionutils.UpdateStatus(corev1.ConditionTrue),
            conditionutils.UpdateReason(ReasonNICUpgradeTaskFailed),
            conditionutils.UpdateMessage(message),
        ); err != nil {
            return false, fmt.Errorf("failed to update conditions: %w", err)
        }
        return false, r.updateStatus(ctx, nicVersion, metalv1alpha1.NICVersionStateFailed, upgradeCurrentTaskStatus, completedCondition)
    }

    if taskCurrentStatus.TaskState == schemas.CompletedTaskState {
        if err := r.Conditions.Update(
            completedCondition,
            conditionutils.UpdateStatus(corev1.ConditionTrue),
            conditionutils.UpdateReason(ReasonNICUpgradeTaskCompleted),
            conditionutils.UpdateMessage("NIC version successfully upgraded to: "+nicVersion.Spec.Version),
        ); err != nil {
            return false, fmt.Errorf("failed to update conditions: %w", err)
        }
        return false, r.updateStatus(ctx, nicVersion, nicVersion.Status.State, upgradeCurrentTaskStatus, completedCondition)
    }

    if err := r.Conditions.Update(
        completedCondition,
        conditionutils.UpdateStatus(corev1.ConditionFalse),
        conditionutils.UpdateReason(taskCurrentStatus.TaskState),
        conditionutils.UpdateMessage(
            fmt.Sprintf("NIC upgrade in state: %v: PercentageCompleted %d",
                taskCurrentStatus.TaskState,
                upgradeCurrentTaskStatus.PercentComplete),
        ),
    ); err != nil {
        return false, fmt.Errorf("failed to update conditions: %w", err)
    }

    ok, err := checkpoint.Transitioned(r.Conditions, *completedCondition)
    if !ok && err == nil {
        log.V(1).Info("NIC upgrade task has not progressed. retrying....")
        return true, nil
    }

    return false, r.updateStatus(ctx, nicVersion, nicVersion.Status.State, upgradeCurrentTaskStatus, completedCondition)
}

func (r *NICVersionReconciler) getServerMaintenanceForRef(ctx context.Context, serverMaintenanceRef *metalv1alpha1.ObjectReference) (*metalv1alpha1.ServerMaintenance, error) {
    if serverMaintenanceRef == nil {
        return nil, fmt.Errorf("server maintenance reference is nil")
    }
    serverMaintenance := &metalv1alpha1.ServerMaintenance{}
    if err := r.Get(ctx, client.ObjectKey{Name: serverMaintenanceRef.Name, Namespace: r.ManagerNamespace}, serverMaintenance); err != nil {
        return serverMaintenance, err
    }
    return serverMaintenance, nil
}

func (r *NICVersionReconciler) updateStatus(
    ctx context.Context,
    nicVersion *metalv1alpha1.NICVersion,
    state metalv1alpha1.NICVersionState,
    upgradeTask *metalv1alpha1.NICTask,
    condition *metav1.Condition,
) error {
    if nicVersion.Status.State == state && condition == nil && upgradeTask == nil {
        return nil
    }

    nicVersionBase := nicVersion.DeepCopy()
    nicVersion.Status.State = state

    if condition != nil {
        if err := r.Conditions.UpdateSlice(
            &nicVersion.Status.Conditions,
            condition.Type,
            conditionutils.UpdateStatus(condition.Status),
            conditionutils.UpdateReason(condition.Reason),
            conditionutils.UpdateMessage(condition.Message),
        ); err != nil {
            return fmt.Errorf("failed to patch NICVersion condition: %w", err)
        }
    } else {
        nicVersion.Status.Conditions = []metav1.Condition{}
    }

    nicVersion.Status.UpgradeTask = upgradeTask

    if err := r.Status().Patch(ctx, nicVersion, client.MergeFrom(nicVersionBase)); err != nil {
        return fmt.Errorf("failed to patch NICVersion status: %w", err)
    }

    return nil
}

func (r *NICVersionReconciler) patchServerMaintenanceRef(ctx context.Context, nicVersion *metalv1alpha1.NICVersion, serverMaintenance *metalv1alpha1.ServerMaintenance) error {
    nicVersionBase := nicVersion.DeepCopy()

    if serverMaintenance == nil {
        nicVersion.Spec.ServerMaintenanceRef = nil
    } else {
        nicVersion.Spec.ServerMaintenanceRef = &metalv1alpha1.ObjectReference{
            APIVersion: serverMaintenance.GroupVersionKind().GroupVersion().String(),
            Kind:       "ServerMaintenance",
            Namespace:  serverMaintenance.Namespace,
            Name:       serverMaintenance.Name,
            UID:        serverMaintenance.UID,
        }
    }

    if err := r.Patch(ctx, nicVersion, client.MergeFrom(nicVersionBase)); err != nil {
        return err
    }
    return nil
}

func (r *NICVersionReconciler) ensurePowerState(ctx context.Context, nicVersion *metalv1alpha1.NICVersion, powerState metalv1alpha1.Power) error {
    serverMaintenance, err := r.getServerMaintenanceForRef(ctx, nicVersion.Spec.ServerMaintenanceRef)
    if err != nil {
        return err
    }
    if serverMaintenance.Spec.ServerPower == powerState {
        return nil
    }
    serverMaintenanceBase := serverMaintenance.DeepCopy()
    serverMaintenance.Spec.ServerPower = powerState
    if err := r.Patch(ctx, serverMaintenance, client.MergeFrom(serverMaintenanceBase)); err != nil {
        return fmt.Errorf("failed to patch power state for ServerMaintenance: %w", err)
    }
    return nil
}

func (r *NICVersionReconciler) requestServerMaintenance(ctx context.Context, nicVersion *metalv1alpha1.NICVersion, server *metalv1alpha1.Server) (bool, error) {
    log := ctrl.LoggerFrom(ctx)
    if nicVersion.Spec.ServerMaintenanceRef != nil {
        if _, err := GetServerMaintenanceForObjectReference(ctx, r.Client, nicVersion.Spec.ServerMaintenanceRef); apierrors.IsNotFound(err) {
            log.V(1).Info("Referenced ServerMaintenance no longer exists, clearing ref to allow re-creation")
            if err = r.patchServerMaintenanceRef(ctx, nicVersion, nil); err != nil {
                return false, fmt.Errorf("failed to clear stale ServerMaintenance ref: %w", err)
            }
            return true, nil
        } else if err != nil {
            return false, fmt.Errorf("failed to verify ServerMaintenance existence: %w", err)
        }
        condition, err := GetCondition(r.Conditions, nicVersion.Status.Conditions, ServerMaintenanceConditionCreated)
        if err != nil {
            return false, err
        }
        if condition.Status == metav1.ConditionTrue {
            return false, nil
        }
        if err := r.Conditions.Update(
            condition,
            conditionutils.UpdateStatus(corev1.ConditionTrue),
            conditionutils.UpdateReason(ServerMaintenanceReasonCreated),
            conditionutils.UpdateMessage(fmt.Sprintf("Created/Present %v at %v", nicVersion.Spec.ServerMaintenanceRef.Name, time.Now())),
        ); err != nil {
            return false, fmt.Errorf("failed to update creating ServerMaintenance condition: %w", err)
        }
        if err := r.updateStatus(ctx, nicVersion, nicVersion.Status.State, nicVersion.Status.UpgradeTask, condition); err != nil {
            return false, fmt.Errorf("failed to patch NICVersion conditions: %w", err)
        }
        return true, nil
    }

    serverMaintenance := &metalv1alpha1.ServerMaintenance{
        ObjectMeta: metav1.ObjectMeta{
            Namespace: r.ManagerNamespace,
            Name:      nicVersion.Name,
        },
    }

    opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, serverMaintenance, func() error {
        serverMaintenance.Spec.Policy = nicVersion.Spec.ServerMaintenancePolicy
        serverMaintenance.Spec.ServerPower = metalv1alpha1.PowerOn
        serverMaintenance.Spec.ServerRef = &corev1.LocalObjectReference{Name: server.Name}
        if serverMaintenance.Status.State != metalv1alpha1.ServerMaintenanceStateInMaintenance && serverMaintenance.Status.State != "" {
            serverMaintenance.Status.State = ""
        }
        return controllerutil.SetControllerReference(nicVersion, serverMaintenance, r.Client.Scheme())
    })
    if err != nil {
        return false, fmt.Errorf("failed to create or patch serverMaintenance: %w", err)
    }
    log.V(1).Info("Created ServerMaintenance", "ServerMaintenance", client.ObjectKeyFromObject(serverMaintenance), "Operation", opResult)

    if err = r.patchServerMaintenanceRef(ctx, nicVersion, serverMaintenance); err != nil {
        return false, fmt.Errorf("failed to patch ServerMaintenance ref in NICVersion status: %w", err)
    }

    log.V(1).Info("Patched ServerMaintenance on NICVersion")
    return true, nil
}

func (r *NICVersionReconciler) enqueueNICVersionByServerRefs(ctx context.Context, obj client.Object) []ctrl.Request {
    log := ctrl.LoggerFrom(ctx)
    host := obj.(*metalv1alpha1.Server)

    if host.Status.State == metalv1alpha1.ServerStateDiscovery ||
        host.Status.State == metalv1alpha1.ServerStateError ||
        host.Status.State == metalv1alpha1.ServerStateInitial {
        return nil
    }

    if host.Spec.ServerMaintenanceRef == nil {
        return nil
    }

    nicVersionList := &metalv1alpha1.NICVersionList{}
    if err := r.List(ctx, nicVersionList); err != nil {
        log.Error(err, "failed to list nicVersionList")
        return nil
    }

    for _, nicVersion := range nicVersionList.Items {
        if nicVersion.Spec.ServerRef.Name == host.Name {
            if nicVersion.Spec.ServerMaintenanceRef == nil ||
                nicVersion.Status.State == metalv1alpha1.NICVersionStateCompleted ||
                nicVersion.Status.State == metalv1alpha1.NICVersionStateFailed {
                return nil
            }
            if nicVersion.Spec.ServerMaintenanceRef.Name != host.Spec.ServerMaintenanceRef.Name {
                return nil
            }
            return []ctrl.Request{{
                NamespacedName: types.NamespacedName{Namespace: nicVersion.Namespace, Name: nicVersion.Name},
            }}
        }
    }
    return nil
}

func (r *NICVersionReconciler) enqueueNICVersionByBMC(ctx context.Context, obj client.Object) []ctrl.Request {
    log := ctrl.LoggerFrom(ctx)
    bmcObj := obj.(*metalv1alpha1.BMC)

    serverList := &metalv1alpha1.ServerList{}
    if err := clientutils.ListAndFilter(ctx, r.Client, serverList, func(object client.Object) (bool, error) {
        server := object.(*metalv1alpha1.Server)
        return server.Spec.BMCRef != nil && server.Spec.BMCRef.Name == bmcObj.Name, nil
    }); err != nil {
        log.Error(err, "failed to list Server created by this BMC resources", "BMC", bmcObj.Name)
        return nil
    }

    serverMap := make(map[string]struct{})
    for _, server := range serverList.Items {
        serverMap[server.Name] = struct{}{}
    }

    nicVersionList := &metalv1alpha1.NICVersionList{}
    if err := clientutils.ListAndFilter(ctx, r.Client, nicVersionList, func(object client.Object) (bool, error) {
        nicVersion := object.(*metalv1alpha1.NICVersion)
        if _, exists := serverMap[nicVersion.Spec.ServerRef.Name]; !exists {
            return false, nil
        }
        return true, nil
    }); err != nil {
        log.Error(err, "failed to list NICVersion for BMC resources", "BMC", bmcObj.Name)
        return nil
    }

    reqs := make([]ctrl.Request, 0)
    for _, nicVersion := range nicVersionList.Items {
        if nicVersion.Status.State == metalv1alpha1.NICVersionStateInProgress {
            resetBMC, err := GetCondition(r.Conditions, nicVersion.Status.Conditions, BMCConditionReset)
            if err != nil {
                log.Error(err, "failed to get reset BMC condition")
                continue
            }
            if resetBMC.Status == metav1.ConditionTrue {
                continue
            }
            if resetBMC.Reason == BMCReasonReset {
                reqs = append(reqs, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: nicVersion.Namespace, Name: nicVersion.Name}})
            }
        }
    }
    return reqs
}

// SetupWithManager sets up the controller with the Manager.
func (r *NICVersionReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&metalv1alpha1.NICVersion{}).
        Owns(&metalv1alpha1.ServerMaintenance{}).
        Watches(&metalv1alpha1.Server{}, handler.EnqueueRequestsFromMapFunc(r.enqueueNICVersionByServerRefs)).
        Watches(&metalv1alpha1.BMC{}, handler.EnqueueRequestsFromMapFunc(r.enqueueNICVersionByBMC)).
        Complete(r)
}
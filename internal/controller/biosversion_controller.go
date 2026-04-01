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
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

const (
	BIOSVersionFinalizer = "metal.ironcore.dev/biosversion"

	ConditionBIOSUpgradeIssued       = "BIOSUpgradeIssued"
	ConditionBIOSUpgradeCompleted    = "BIOSUpgradeCompleted"
	ConditionBIOSUpgradePowerOn      = "BIOSUpgradePowerOn"
	ConditionBIOSUpgradePowerOff     = "BIOSUpgradePowerOff"
	ConditionBIOSUpgradeVerification = "BIOSUpgradeVerification"

	ReasonUpgradeIssued           = "UpgradeIssued"
	ReasonUpgradeIssueFailed      = "UpgradeIssueFailed"
	ReasonRebootPowerOff          = "RebootPowerOff"
	ReasonRebootPowerOn           = "RebootPowerOn"
	ReasonBIOSVersionVerified     = "BIOSVersionVerified"
	ReasonBIOSVersionVerification = "BIOSVersionVerificationFailed"
	ReasonUpgradeTaskFailed       = "UpgradeTaskFailed"
	ReasonUpgradeTaskCompleted    = "UpgradeTaskCompleted"
)

// BIOSVersionReconciler reconciles a BIOSVersion object
type BIOSVersionReconciler struct {
	client.Client
	ManagerNamespace string
	Insecure         bool
	Scheme           *runtime.Scheme
	BMCOptions       bmc.Options
	ResyncInterval   time.Duration
	Conditions       *conditionutils.Accessor
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biosversions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biosversions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biosversions/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="batch",resources=jobs,verbs=get;list;watch;create;update;patch;delete

func (r *BIOSVersionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	version := &metalv1alpha1.BIOSVersion{}
	if err := r.Get(ctx, req.NamespacedName, version); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.V(1).Info("Reconciling BIOSVersion")

	return r.reconcileExists(ctx, version)
}

func (r *BIOSVersionReconciler) reconcileExists(ctx context.Context, version *metalv1alpha1.BIOSVersion) (ctrl.Result, error) {
	if r.shouldDelete(ctx, version) {
		return r.delete(ctx, version)
	}
	return r.reconcile(ctx, version)
}

func (r *BIOSVersionReconciler) shouldDelete(ctx context.Context, version *metalv1alpha1.BIOSVersion) bool {
	log := ctrl.LoggerFrom(ctx)
	if version.DeletionTimestamp.IsZero() {
		return false
	}

	if controllerutil.ContainsFinalizer(version, BIOSVersionFinalizer) &&
		version.Status.State == metalv1alpha1.BIOSVersionStateInProgress {
		if _, err := GetServerByName(ctx, r.Client, version.Spec.ServerRef.Name); apierrors.IsNotFound(err) {
			log.V(1).Info("Server not found, proceeding with deletion", "Server", version.Spec.ServerRef.Name)
			return true
		}
		log.V(1).Info("Postponed deletion as BIOS version update is in progress")
		return false
	}

	return true
}

func (r *BIOSVersionReconciler) delete(ctx context.Context, version *metalv1alpha1.BIOSVersion) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Deleting BIOSVersion")
	defer log.V(1).Info("Deleted BIOSVersion")

	if !controllerutil.ContainsFinalizer(version, BIOSVersionFinalizer) {
		return ctrl.Result{}, nil
	}

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, version, BIOSVersionFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *BIOSVersionReconciler) cleanupServerMaintenanceReferences(ctx context.Context, version *metalv1alpha1.BIOSVersion) error {
	log := ctrl.LoggerFrom(ctx)
	if version.Spec.ServerMaintenanceRef == nil {
		return nil
	}

	serverMaintenance, err := r.getServerMaintenanceForRef(ctx, version.Spec.ServerMaintenanceRef)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get referred ServerMaintenance: %w", err)
	}

	if serverMaintenance.DeletionTimestamp.IsZero() {
		if metav1.IsControlledBy(serverMaintenance, version) {
			log.V(1).Info("Deleting ServerMaintenance", "ServerMaintenance", client.ObjectKeyFromObject(serverMaintenance))
			if err := r.Delete(ctx, serverMaintenance); err != nil {
				return err
			}
		} else {
			log.V(1).Info("ServerMaintenance is controlled by somebody else", "ServerMaintenance", client.ObjectKeyFromObject(serverMaintenance))
		}
	}

	// Remove the reference if the object is gone.
	if apierrors.IsNotFound(err) || err == nil {
		log.V(1).Info("Cleaned up ServerMaintenance ref in BIOSVersion")
		if err := r.patchServerMaintenanceRef(ctx, version, nil); err != nil {
			return fmt.Errorf("failed to clean up ServerMaintenance ref in BIOSVersion: %w", err)
		}
	}
	return nil
}

func (r *BIOSVersionReconciler) reconcile(ctx context.Context, version *metalv1alpha1.BIOSVersion) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	if shouldIgnoreReconciliation(version) {
		log.V(1).Info("Skipped BIOSVersion reconciliation")
		return ctrl.Result{}, nil
	}

	base := version.DeepCopy()
	if version.Spec.ServerMaintenanceRef != nil && clearDeprecatedObjectRefFields(version.Spec.ServerMaintenanceRef) {
		if err := r.Patch(ctx, version, client.MergeFrom(base)); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to clear deprecated ObjectReference fields on BIOSVersion: %w", err)
		}
		return ctrl.Result{}, nil
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, version, BIOSVersionFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	requeue, err := r.transitionState(ctx, version)
	if err != nil {
		return ctrl.Result{}, err
	}
	if requeue {
		return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
	}

	log.V(1).Info("Reconciled BIOSVersion")
	return ctrl.Result{}, nil
}

func (r *BIOSVersionReconciler) transitionState(ctx context.Context, version *metalv1alpha1.BIOSVersion) (bool, error) {
	log := ctrl.LoggerFrom(ctx)
	if version.Spec.ServerRef == nil {
		return false, fmt.Errorf("BIOSVersion does not have a ServerRef")
	}

	server, err := GetServerByName(ctx, r.Client, version.Spec.ServerRef.Name)
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

	switch version.Status.State {
	case "", metalv1alpha1.BIOSVersionStatePending:
		return false, r.cleanup(ctx, bmcClient, version, server)
	case metalv1alpha1.BIOSVersionStateInProgress:
		if ok, err := r.handleServerMaintenance(ctx, bmcClient, version, server); err != nil || !ok {
			return false, err
		}

		return r.processInProgressState(ctx, bmcClient, version, server)
	case metalv1alpha1.BIOSVersionStateCompleted:
		return false, r.cleanup(ctx, bmcClient, version, server)
	case metalv1alpha1.BIOSVersionStateFailed:
		if shouldRetryReconciliation(version) {
			log.V(1).Info("Retrying BIOSVersion reconciliation")
			versionBase := version.DeepCopy()

			annotations := version.GetAnnotations()
			delete(annotations, metalv1alpha1.OperationAnnotation)
			version.SetAnnotations(annotations)

			if err := r.Patch(ctx, version, client.MergeFrom(versionBase)); err != nil {
				return true, fmt.Errorf("failed to patch BIOSVersion metadata for retrying: %w", err)
			}

			versionBase = version.DeepCopy()
			version.Status.State = metalv1alpha1.BIOSVersionStatePending
			version.Status.Conditions = []metav1.Condition{}

			if err := r.Status().Patch(ctx, version, client.MergeFrom(versionBase)); err != nil {
				return true, fmt.Errorf("failed to patch BIOSVersion status for retrying: %w", err)
			}
			return true, nil
		}
		log.V(1).Info("Failed to upgrade BIOSVersion", "BIOSVersion", version.Name, "Server", server.Name)
		return false, nil
	}

	log.V(1).Info("Unknown state found", "state", version.Status.State)
	return false, nil
}

func (r *BIOSVersionReconciler) handleServerMaintenance(ctx context.Context, bmcClient bmc.BMC, version *metalv1alpha1.BIOSVersion, server *metalv1alpha1.Server) (bool, error) {
	log := ctrl.LoggerFrom(ctx)
	if requeue, err := r.requestServerMaintenance(ctx, version, server); err != nil || requeue {
		return false, err
	}

	condition, err := GetCondition(r.Conditions, version.Status.Conditions, ServerMaintenanceConditionWaiting)
	if err != nil {
		return false, err
	}

	if server.Status.State != metalv1alpha1.ServerStateMaintenance {
		log.V(1).Info("Server not in maintenance, waiting", "serverState", server.Status.State, "server", server.Name)
		return false, r.ensureMaintenanceWaitingCondition(ctx, version, condition)
	}

	if server.Spec.ServerMaintenanceRef == nil || server.Spec.ServerMaintenanceRef.Name != version.Spec.ServerMaintenanceRef.Name || server.Spec.ServerMaintenanceRef.Namespace != version.Spec.ServerMaintenanceRef.Namespace {
		log.V(1).Info("Server already in maintenance for another request", "Server", server.Name)
		return false, r.ensureMaintenanceWaitingCondition(ctx, version, condition)
	}

	if condition.Reason != ServerMaintenanceReasonApproved {
		if err := r.Conditions.Update(
			condition,
			conditionutils.UpdateStatus(corev1.ConditionFalse),
			conditionutils.UpdateReason(ServerMaintenanceReasonApproved),
			conditionutils.UpdateMessage("Server is now in Maintenance mode"),
		); err != nil {
			return false, fmt.Errorf("failed to update ServerMaintenance approved condition: %w", err)
		}
		if err := r.updateStatus(ctx, version, version.Status.State, version.Status.UpgradeTask, condition); err != nil {
			return false, fmt.Errorf("failed to patch BIOSVersion ServerMaintenance approved conditions: %w", err)
		}
		return false, nil
	}

	if ok, err := r.handleBMCReset(ctx, bmcClient, version, server); !ok || err != nil {
		return false, err
	}
	return true, nil
}

func (r *BIOSVersionReconciler) ensureMaintenanceWaitingCondition(ctx context.Context, version *metalv1alpha1.BIOSVersion, condition *metav1.Condition) error {
	if condition.Status == metav1.ConditionTrue {
		return nil
	}
	if err := r.Conditions.Update(
		condition,
		conditionutils.UpdateStatus(corev1.ConditionTrue),
		conditionutils.UpdateReason(ServerMaintenanceReasonWaiting),
		conditionutils.UpdateMessage(fmt.Sprintf("Waiting for approval of %v", version.Spec.ServerMaintenanceRef.Name)),
	); err != nil {
		return fmt.Errorf("failed to update ServerMaintenance waiting condition: %w", err)
	}
	return r.updateStatus(ctx, version, version.Status.State, version.Status.UpgradeTask, condition)
}

func (r *BIOSVersionReconciler) processInProgressState(ctx context.Context, bmcClient bmc.BMC, version *metalv1alpha1.BIOSVersion, server *metalv1alpha1.Server) (bool, error) {
	type stepFunc func() (done bool, requeue bool, err error)
	steps := []stepFunc{
		func() (bool, bool, error) { return r.handleUpgradeIssue(ctx, bmcClient, version, server) },
		func() (bool, bool, error) { return r.handleUpgradeCompletion(ctx, bmcClient, version, server) },
		func() (bool, bool, error) { return r.handlePowerOff(ctx, version, server) },
		func() (bool, bool, error) { return r.handlePowerOn(ctx, version, server) },
		func() (bool, bool, error) { return r.handleVerification(ctx, bmcClient, version, server) },
	}

	for _, step := range steps {
		done, requeue, err := step()
		if !done || err != nil {
			return requeue, err
		}
	}

	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("All upgrade conditions completed")
	return false, nil
}

func (r *BIOSVersionReconciler) handleUpgradeIssue(ctx context.Context, bmcClient bmc.BMC, version *metalv1alpha1.BIOSVersion, server *metalv1alpha1.Server) (bool, bool, error) {
	log := ctrl.LoggerFrom(ctx)
	issuedCondition, err := GetCondition(r.Conditions, version.Status.Conditions, ConditionBIOSUpgradeIssued)
	if err != nil {
		return false, false, err
	}
	if issuedCondition.Status == metav1.ConditionTrue {
		return true, false, nil
	}

	log.V(1).Info("Processing BIOS version upgrade")
	if server.Status.PowerState != metalv1alpha1.ServerOnPowerState {
		log.V(1).Info("Server powered off, waiting for power on", "Server", server.Name)
		return false, false, nil
	}
	hasPending, err := bmcClient.CheckBMCPendingComponentUpgrade(ctx, bmc.ComponentTypeBIOS)
	if err != nil {
		log.V(1).Info("Failed to check pending component upgrade, proceeding with BIOS upgrade", "error", err)
	} else if hasPending {
		log.Info("Pending component upgrade detected, deferring BIOS upgrade to avoid interruption", "Server", server.Name)
		return false, true, nil
	}
	return false, false, r.upgradeBIOSVersion(ctx, bmcClient, version, server, issuedCondition)
}

func (r *BIOSVersionReconciler) handleUpgradeCompletion(ctx context.Context, bmcClient bmc.BMC, version *metalv1alpha1.BIOSVersion, server *metalv1alpha1.Server) (bool, bool, error) {
	log := ctrl.LoggerFrom(ctx)
	completedCondition, err := GetCondition(r.Conditions, version.Status.Conditions, ConditionBIOSUpgradeCompleted)
	if err != nil {
		return false, false, err
	}
	if completedCondition.Status == metav1.ConditionTrue {
		return true, false, nil
	}

	log.V(1).Info("Checking BIOS version upgrade task status")
	requeue, err := r.checkUpdateBiosUpgradeStatus(ctx, bmcClient, version, server, completedCondition)
	var taskFetchFailed *BMCTaskFetchFailedError
	if !errors.As(err, &taskFetchFailed) {
		return false, requeue, err
	}

	// Some vendors delete task details once upgrade is completed.
	// Check the current version and proceed if it matches spec.
	log.V(1).Info("Failed to fetch BIOS upgrade task status from BMC", "error", err)
	currentVersion, errVersionFetch := r.getBIOSVersionFromBMC(ctx, bmcClient, server)
	if errVersionFetch != nil {
		log.Error(errors.Join(err, errVersionFetch), "Failed to fetch current BIOS version from BMC after upgrade task fetch failure")
		return false, true, nil
	}
	if currentVersion != version.Spec.Version {
		log.V(1).Info("BIOS version not updated yet, need to wait for task details", "Version", currentVersion, "DesiredVersion", version.Spec.Version)
		return false, requeue, err
	}

	log.V(1).Info("BIOS version matched spec despite task fetch failure", "Version", currentVersion)
	if err := r.Conditions.Update(
		completedCondition,
		conditionutils.UpdateStatus(corev1.ConditionTrue),
		conditionutils.UpdateReason(ReasonUpgradeTaskCompleted),
		conditionutils.UpdateMessage("Upgrade Task is missing. BIOS version successfully upgraded to: "+version.Spec.Version),
	); err != nil {
		return false, false, fmt.Errorf("failed to update upgrade complete conditions: %w", err)
	}
	return false, false, r.updateStatus(ctx, version, version.Status.State, version.Status.UpgradeTask, completedCondition)
}

func (r *BIOSVersionReconciler) handlePowerOff(ctx context.Context, version *metalv1alpha1.BIOSVersion, server *metalv1alpha1.Server) (bool, bool, error) {
	log := ctrl.LoggerFrom(ctx)
	condition, err := GetCondition(r.Conditions, version.Status.Conditions, ConditionBIOSUpgradePowerOff)
	if err != nil {
		return false, false, err
	}
	if condition.Status == metav1.ConditionTrue {
		return true, false, nil
	}

	log.V(1).Info("Ensuring server is powered off")
	if server.Status.PowerState != metalv1alpha1.ServerOffPowerState {
		return false, false, r.ensurePowerState(ctx, version, metalv1alpha1.PowerOff)
	}
	log.V(1).Info("Ensured server is powered off")

	if err := r.Conditions.Update(
		condition,
		conditionutils.UpdateStatus(corev1.ConditionTrue),
		conditionutils.UpdateReason(ReasonRebootPowerOff),
		conditionutils.UpdateMessage("Powered off the server"),
	); err != nil {
		return false, false, fmt.Errorf("failed to update reboot power off condition: %w", err)
	}
	return false, false, r.updateStatus(ctx, version, version.Status.State, version.Status.UpgradeTask, condition)
}

func (r *BIOSVersionReconciler) handlePowerOn(ctx context.Context, version *metalv1alpha1.BIOSVersion, server *metalv1alpha1.Server) (bool, bool, error) {
	log := ctrl.LoggerFrom(ctx)
	condition, err := GetCondition(r.Conditions, version.Status.Conditions, ConditionBIOSUpgradePowerOn)
	if err != nil {
		return false, false, err
	}
	if condition.Status == metav1.ConditionTrue {
		return true, false, nil
	}

	log.V(1).Info("Ensuring server is powered on")
	if server.Status.PowerState != metalv1alpha1.ServerOnPowerState {
		return false, false, r.ensurePowerState(ctx, version, metalv1alpha1.PowerOn)
	}
	log.V(1).Info("Ensured server is powered on")

	if err := r.Conditions.Update(
		condition,
		conditionutils.UpdateStatus(corev1.ConditionTrue),
		conditionutils.UpdateReason(ReasonRebootPowerOn),
		conditionutils.UpdateMessage("Powered on the server"),
	); err != nil {
		return false, false, fmt.Errorf("failed to update reboot power on condition: %w", err)
	}
	return false, false, r.updateStatus(ctx, version, version.Status.State, version.Status.UpgradeTask, condition)
}

func (r *BIOSVersionReconciler) handleVerification(ctx context.Context, bmcClient bmc.BMC, version *metalv1alpha1.BIOSVersion, server *metalv1alpha1.Server) (bool, bool, error) {
	log := ctrl.LoggerFrom(ctx)
	condition, err := GetCondition(r.Conditions, version.Status.Conditions, ConditionBIOSUpgradeVerification)
	if err != nil {
		return false, false, err
	}
	if condition.Status == metav1.ConditionTrue {
		return true, false, nil
	}

	log.V(1).Info("Verifying BIOS version update")
	currentVersion, err := r.getBIOSVersionFromBMC(ctx, bmcClient, server)
	if err != nil {
		return false, false, err
	}
	if currentVersion != version.Spec.Version {
		log.V(1).Info("BIOS version not updated", "Version", currentVersion, "DesiredVersion", version.Spec.Version)
		if condition.Reason == "" {
			if err := r.Conditions.Update(
				condition,
				conditionutils.UpdateStatus(corev1.ConditionFalse),
				conditionutils.UpdateReason(ReasonBIOSVersionVerification),
				conditionutils.UpdateMessage("Waiting for BIOS version update"),
			); err != nil {
				return false, false, fmt.Errorf("failed to update the verification condition: %w", err)
			}
		}
		log.V(1).Info("Waiting for BIOS version to be updated on server")
		return false, true, nil
	}

	if err := r.Conditions.Update(
		condition,
		conditionutils.UpdateStatus(corev1.ConditionTrue),
		conditionutils.UpdateReason(ReasonBIOSVersionVerified),
		conditionutils.UpdateMessage("BIOS Version updated"),
	); err != nil {
		return false, false, fmt.Errorf("failed to update conditions: %w", err)
	}
	return false, false, r.updateStatus(ctx, version, metalv1alpha1.BIOSVersionStateCompleted, version.Status.UpgradeTask, condition)
}

func (r *BIOSVersionReconciler) handleBMCReset(ctx context.Context, bmcClient bmc.BMC, version *metalv1alpha1.BIOSVersion, server *metalv1alpha1.Server) (bool, error) {
	log := ctrl.LoggerFrom(ctx)
	resetBMC, err := GetCondition(r.Conditions, version.Status.Conditions, BMCConditionReset)
	if err != nil {
		return false, fmt.Errorf("failed to get BMC reset condition: %w", err)
	}

	// Already completed
	if resetBMC.Status == metav1.ConditionTrue {
		return true, nil
	}

	// Phase 1: Issue the reset if not yet issued
	if resetBMC.Reason != BMCReasonReset {
		if err := resetBMCOfServer(ctx, r.Client, server, bmcClient); err != nil {
			log.Error(err, "Failed to reset BMC of the server")
			return false, err
		}
		if err := r.Conditions.Update(
			resetBMC,
			conditionutils.UpdateStatus(corev1.ConditionFalse),
			conditionutils.UpdateReason(BMCReasonReset),
			conditionutils.UpdateMessage("Issued BMC reset to stabilize the BMC"),
		); err != nil {
			return false, fmt.Errorf("failed to update reset BMC condition: %w", err)
		}
		return false, r.updateStatus(ctx, version, version.Status.State, nil, resetBMC)
	}

	// Phase 2: Wait for reset completion (annotation removal on BMC object)
	if server.Spec.BMCRef != nil {
		bmcObj := &metalv1alpha1.BMC{}
		if err := r.Get(ctx, client.ObjectKey{Name: server.Spec.BMCRef.Name}, bmcObj); err != nil {
			log.Error(err, "Failed to get referred server's Manager")
			return false, err
		}
		if op := bmcObj.GetAnnotations()[metalv1alpha1.OperationAnnotation]; op == metalv1alpha1.GracefulRestartBMC {
			log.V(1).Info("Waiting for BMC reset as annotation on BMC object is set")
			return false, nil
		}
	}

	// Phase 3: Mark reset as completed
	if err := r.Conditions.Update(
		resetBMC,
		conditionutils.UpdateStatus(corev1.ConditionTrue),
		conditionutils.UpdateReason(BMCReasonReset),
		conditionutils.UpdateMessage("BMC reset completed"),
	); err != nil {
		return false, fmt.Errorf("failed to update BMC reset completed condition: %w", err)
	}
	return false, r.updateStatus(ctx, version, version.Status.State, nil, resetBMC)
}

func (r *BIOSVersionReconciler) getBIOSVersionFromBMC(ctx context.Context, bmcClient bmc.BMC, server *metalv1alpha1.Server) (string, error) {
	currentVersion, err := bmcClient.GetBiosVersion(ctx, server.Spec.SystemURI)
	if err != nil {
		return "", fmt.Errorf("failed to get BIOS version: %w", err)
	}

	return currentVersion, nil
}

func (r *BIOSVersionReconciler) cleanup(ctx context.Context, bmcClient bmc.BMC, version *metalv1alpha1.BIOSVersion, server *metalv1alpha1.Server) error {
	log := ctrl.LoggerFrom(ctx)
	currentVersion, err := r.getBIOSVersionFromBMC(ctx, bmcClient, server)
	if err != nil {
		return err
	}

	if currentVersion == version.Spec.Version {
		if err := r.cleanupServerMaintenanceReferences(ctx, version); err != nil {
			return err
		}

		log.V(1).Info("Upgraded BIOS version", "Version", currentVersion, "Server", server.Name)
		return r.updateStatus(ctx, version, metalv1alpha1.BIOSVersionStateCompleted, nil, nil)
	}
	return r.updateStatus(ctx, version, metalv1alpha1.BIOSVersionStateInProgress, nil, nil)
}

func (r *BIOSVersionReconciler) getServerMaintenanceForRef(ctx context.Context, serverMaintenanceRef *metalv1alpha1.ObjectReference) (*metalv1alpha1.ServerMaintenance, error) {
	if serverMaintenanceRef == nil {
		return nil, fmt.Errorf("server maintenance reference is nil")
	}

	serverMaintenance := &metalv1alpha1.ServerMaintenance{}
	if err := r.Get(ctx, client.ObjectKey{Name: serverMaintenanceRef.Name, Namespace: r.ManagerNamespace}, serverMaintenance); err != nil {
		return serverMaintenance, err
	}

	return serverMaintenance, nil
}

func (r *BIOSVersionReconciler) updateStatus(ctx context.Context, version *metalv1alpha1.BIOSVersion, state metalv1alpha1.BIOSVersionState, upgradeTask *metalv1alpha1.Task, condition *metav1.Condition) error {
	versionBase := version.DeepCopy()

	version.Status.State = state

	if condition != nil {
		if err := r.Conditions.UpdateSlice(
			&version.Status.Conditions,
			condition.Type,
			conditionutils.UpdateStatus(condition.Status),
			conditionutils.UpdateReason(condition.Reason),
			conditionutils.UpdateMessage(condition.Message),
		); err != nil {
			return fmt.Errorf("failed to patch BIOSVersion condition: %w", err)
		}
	}

	version.Status.UpgradeTask = upgradeTask

	if equality.Semantic.DeepEqual(version.Status, versionBase.Status) {
		return nil
	}

	if err := r.Status().Patch(ctx, version, client.MergeFrom(versionBase)); err != nil {
		return fmt.Errorf("failed to patch BIOSVersion status: %w", err)
	}

	return nil
}

func (r *BIOSVersionReconciler) patchServerMaintenanceRef(ctx context.Context, version *metalv1alpha1.BIOSVersion, serverMaintenance *metalv1alpha1.ServerMaintenance) error {
	versionBase := version.DeepCopy()

	if serverMaintenance == nil {
		version.Spec.ServerMaintenanceRef = nil
	} else {
		version.Spec.ServerMaintenanceRef = &metalv1alpha1.ObjectReference{
			Namespace: serverMaintenance.Namespace,
			Name:      serverMaintenance.Name,
		}
	}

	if err := r.Patch(ctx, version, client.MergeFrom(versionBase)); err != nil {
		return err
	}

	return nil
}

func (r *BIOSVersionReconciler) ensurePowerState(ctx context.Context, version *metalv1alpha1.BIOSVersion, powerState metalv1alpha1.Power) error {
	serverMaintenance, err := r.getServerMaintenanceForRef(ctx, version.Spec.ServerMaintenanceRef)
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

func (r *BIOSVersionReconciler) requestServerMaintenance(ctx context.Context, version *metalv1alpha1.BIOSVersion, server *metalv1alpha1.Server) (bool, error) {
	log := ctrl.LoggerFrom(ctx)
	if version.Spec.ServerMaintenanceRef != nil {
		if _, err := GetServerMaintenanceForObjectReference(ctx, r.Client, version.Spec.ServerMaintenanceRef); apierrors.IsNotFound(err) {
			log.V(1).Info("Referenced ServerMaintenance no longer exists, clearing ref to allow re-creation")
			if err = r.patchServerMaintenanceRef(ctx, version, nil); err != nil {
				return false, fmt.Errorf("failed to clear stale ServerMaintenance ref: %w", err)
			}
			return true, nil // requeue to re-create
		} else if err != nil {
			return false, fmt.Errorf("failed to verify ServerMaintenance existence: %w", err)
		}
		condition, err := GetCondition(r.Conditions, version.Status.Conditions, ServerMaintenanceConditionCreated)
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
			conditionutils.UpdateMessage(fmt.Sprintf("Created/Present %v at %v", version.Spec.ServerMaintenanceRef.Name, time.Now())),
		); err != nil {
			return false, fmt.Errorf("failed to update creating ServerMaintenance condition: %w", err)
		}
		if err := r.updateStatus(ctx, version, version.Status.State, version.Status.UpgradeTask, condition); err != nil {
			return false, fmt.Errorf("failed to patch BIOSVersion conditions: %w", err)
		}
		return true, nil
	}

	serverMaintenance := &metalv1alpha1.ServerMaintenance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.ManagerNamespace,
			Name:      version.Name,
		},
	}

	opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, serverMaintenance, func() error {
		serverMaintenance.Spec.Policy = version.Spec.ServerMaintenancePolicy
		serverMaintenance.Spec.ServerPower = metalv1alpha1.PowerOn
		serverMaintenance.Spec.ServerRef = &corev1.LocalObjectReference{Name: server.Name}
		if serverMaintenance.Status.State != metalv1alpha1.ServerMaintenanceStateInMaintenance && serverMaintenance.Status.State != "" {
			serverMaintenance.Status.State = ""
		}
		return controllerutil.SetControllerReference(version, serverMaintenance, r.Client.Scheme())
	})
	if err != nil {
		return false, fmt.Errorf("failed to create or patch ServerMaintenance: %w", err)
	}
	log.V(1).Info("Created ServerMaintenance", "ServerMaintenance", client.ObjectKeyFromObject(serverMaintenance), "Operation", opResult)

	if err = r.patchServerMaintenanceRef(ctx, version, serverMaintenance); err != nil {
		return false, fmt.Errorf("failed to patch ServerMaintenance ref in BIOSVersion: %w", err)
	}

	log.V(1).Info("Patched ServerMaintenance on BIOSVersion")
	return true, nil
}

func (r *BIOSVersionReconciler) checkUpdateBiosUpgradeStatus(ctx context.Context, bmcClient bmc.BMC, version *metalv1alpha1.BIOSVersion, server *metalv1alpha1.Server, completedCondition *metav1.Condition) (bool, error) {
	log := ctrl.LoggerFrom(ctx)
	taskURI := version.Status.UpgradeTask.URI
	taskCurrentStatus, err := func() (*schemas.Task, error) {
		if taskURI == "" {
			return nil, fmt.Errorf("invalid task URI: '%v'", taskURI)
		}
		return bmcClient.GetBiosUpgradeTask(ctx, server.Status.Manufacturer, taskURI)
	}()
	if err != nil {
		return false, &BMCTaskFetchFailedError{
			TaskURI:  taskURI,
			Resource: "BIOSUpgrade",
			Err:      err,
		}
	}
	log.V(1).Info("BIOS upgrade task current status", "TaskState", taskCurrentStatus.TaskState)

	upgradeCurrentTaskStatus := &metalv1alpha1.Task{
		URI:             version.Status.UpgradeTask.URI,
		State:           taskCurrentStatus.TaskState,
		Status:          taskCurrentStatus.TaskStatus,
		PercentComplete: int32(gofish.Deref(taskCurrentStatus.PercentComplete)),
	}

	// Use checkpoint in case the job has stalled and we need to requeue
	transition := &conditionutils.FieldsTransition{
		IncludeStatus:  true,
		IncludeReason:  true,
		IncludeMessage: true,
	}
	checkpoint, err := transition.Checkpoint(r.Conditions, *completedCondition)
	if err != nil {
		return false, fmt.Errorf("failed to create checkpoint for condition: %w", err)
	}

	if taskCurrentStatus.TaskState == schemas.KilledTaskState ||
		taskCurrentStatus.TaskState == schemas.ExceptionTaskState ||
		taskCurrentStatus.TaskState == schemas.CancelledTaskState ||
		(taskCurrentStatus.TaskStatus != schemas.OKHealth && taskCurrentStatus.TaskStatus != "") {
		message := fmt.Sprintf(
			"BIOS upgrade task failed with message %v, check '%v' for details",
			taskCurrentStatus.Messages,
			taskURI,
		)
		if err := r.Conditions.Update(
			completedCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(ReasonUpgradeTaskFailed),
			conditionutils.UpdateMessage(message),
		); err != nil {
			return false, fmt.Errorf("failed to update conditions: %w", err)
		}

		return false, r.updateStatus(ctx, version, metalv1alpha1.BIOSVersionStateFailed, upgradeCurrentTaskStatus, completedCondition)
	}

	if taskCurrentStatus.TaskState == schemas.CompletedTaskState {
		if err := r.Conditions.Update(
			completedCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(ReasonUpgradeTaskCompleted),
			conditionutils.UpdateMessage("BIOS version successfully upgraded to: "+version.Spec.Version),
		); err != nil {
			return false, fmt.Errorf("failed to update conditions: %w", err)
		}

		return false, r.updateStatus(ctx, version, version.Status.State, upgradeCurrentTaskStatus, completedCondition)
	}

	// In-progress task states
	if err := r.Conditions.Update(
		completedCondition,
		conditionutils.UpdateStatus(corev1.ConditionFalse),
		conditionutils.UpdateReason(taskCurrentStatus.TaskState),
		conditionutils.UpdateMessage(
			fmt.Sprintf("BIOS upgrade in state: %v: PercentageCompleted %d",
				taskCurrentStatus.TaskState,
				upgradeCurrentTaskStatus.PercentComplete),
		),
	); err != nil {
		return false, fmt.Errorf("failed to update conditions: %w", err)
	}

	ok, err := checkpoint.Transitioned(r.Conditions, *completedCondition)
	if !ok && err == nil {
		log.V(1).Info("BIOS upgrade task stalled, requeueing")
		// The upgrade job has stalled or is too slow. We need to requeue with exponential backoff.
		return true, nil
	}

	// TODO: Fail the state after certain timeout
	return false, r.updateStatus(ctx, version, version.Status.State, upgradeCurrentTaskStatus, completedCondition)
}

func (r *BIOSVersionReconciler) upgradeBIOSVersion(ctx context.Context, bmcClient bmc.BMC, version *metalv1alpha1.BIOSVersion, server *metalv1alpha1.Server, issuedCondition *metav1.Condition) error {
	log := ctrl.LoggerFrom(ctx)
	var username, password string
	if version.Spec.Image.SecretRef != nil {
		var err error
		username, password, err = GetImageCredentialsForSecretRef(ctx, r.Client, version.Spec.Image.SecretRef)
		if err != nil {
			return fmt.Errorf("failed to get image credentials: %w", err)
		}
	}

	var forceUpdate bool
	if version.Spec.UpdatePolicy != nil && *version.Spec.UpdatePolicy == metalv1alpha1.UpdatePolicyForce {
		forceUpdate = true
	}

	parameters := &schemas.UpdateServiceSimpleUpdateParameters{
		ForceUpdate:      forceUpdate,
		ImageURI:         version.Spec.Image.URI,
		Password:         password,
		Username:         username,
		TransferProtocol: schemas.TransferProtocolType(version.Spec.Image.TransferProtocol),
	}

	taskMonitor, isFatal, err := func() (string, bool, error) {
		return bmcClient.UpgradeBiosVersion(ctx, server.Status.Manufacturer, parameters)
	}()

	upgradeCurrentTaskStatus := &metalv1alpha1.Task{URI: taskMonitor}

	if isFatal {
		log.Error(err, "Failed to issue BIOS upgrade", "Version", version.Spec.Version, "Server", server.Name)
		if err := r.Conditions.Update(
			issuedCondition,
			conditionutils.UpdateStatus(corev1.ConditionFalse),
			conditionutils.UpdateReason(ReasonUpgradeIssueFailed),
			conditionutils.UpdateMessage("Fatal error occurred. Upgrade might still go through on server."),
		); err != nil {
			log.Error(err, "Failed to update conditions")
			return errors.Join(err, r.updateStatus(ctx, version, metalv1alpha1.BIOSVersionStateFailed, upgradeCurrentTaskStatus, issuedCondition))
		}

		return r.updateStatus(ctx, version, metalv1alpha1.BIOSVersionStateFailed, upgradeCurrentTaskStatus, issuedCondition)
	}
	if err != nil {
		log.Error(err, "Failed to issue BIOS upgrade", "Version", version.Spec.Version, "Server", server.Name)
		return err
	}
	if err := r.Conditions.Update(
		issuedCondition,
		conditionutils.UpdateStatus(corev1.ConditionTrue),
		conditionutils.UpdateReason(ReasonUpgradeIssued),
		conditionutils.UpdateMessage(fmt.Sprintf("Upgrade task created: %v", taskMonitor)),
	); err != nil {
		return fmt.Errorf("failed to update issued condition after successful upgrade: %w", err)
	}

	return r.updateStatus(ctx, version, version.Status.State, upgradeCurrentTaskStatus, issuedCondition)
}

func (r *BIOSVersionReconciler) enqueueBiosVersionByServerRefs(ctx context.Context, obj client.Object) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)
	server := obj.(*metalv1alpha1.Server)

	// Skip servers in states that don't require requeue
	if server.Status.State == metalv1alpha1.ServerStateDiscovery ||
		server.Status.State == metalv1alpha1.ServerStateError ||
		server.Status.State == metalv1alpha1.ServerStateInitial {
		return nil
	}

	// Skip servers without maintenance
	if server.Spec.ServerMaintenanceRef == nil {
		return nil
	}

	biosVersionList := &metalv1alpha1.BIOSVersionList{}
	if err := r.List(ctx, biosVersionList); err != nil {
		log.Error(err, "Failed to list BIOSVersions")
		return nil
	}

	for _, version := range biosVersionList.Items {
		if version.Spec.ServerRef.Name == server.Name {
			// Skip completed or failed BIOSVersions
			if version.Spec.ServerMaintenanceRef == nil ||
				version.Status.State == metalv1alpha1.BIOSVersionStateCompleted ||
				version.Status.State == metalv1alpha1.BIOSVersionStateFailed {
				return nil
			}
			if version.Spec.ServerMaintenanceRef.Name != server.Spec.ServerMaintenanceRef.Name {
				return nil
			}
			return []ctrl.Request{{
				NamespacedName: types.NamespacedName{Namespace: version.Namespace, Name: version.Name},
			}}
		}
	}
	return nil
}

func (r *BIOSVersionReconciler) enqueueBiosSettingsByBMC(ctx context.Context, obj client.Object) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)
	bmcObj := obj.(*metalv1alpha1.BMC)

	serverList := &metalv1alpha1.ServerList{}
	if err := clientutils.ListAndFilter(ctx, r.Client, serverList, func(object client.Object) (bool, error) {
		server := object.(*metalv1alpha1.Server)
		return server.Spec.BMCRef != nil && server.Spec.BMCRef.Name == bmcObj.Name, nil
	}); err != nil {
		log.Error(err, "Failed to list Servers for BMC", "BMC", bmcObj.Name)
		return nil
	}

	serverMap := make(map[string]struct{})
	for _, server := range serverList.Items {
		serverMap[server.Name] = struct{}{}
	}

	biosVersionList := &metalv1alpha1.BIOSVersionList{}
	if err := clientutils.ListAndFilter(ctx, r.Client, biosVersionList, func(object client.Object) (bool, error) {
		version := object.(*metalv1alpha1.BIOSVersion)
		if _, exists := serverMap[version.Spec.ServerRef.Name]; !exists {
			return false, nil
		}
		return true, nil
	}); err != nil {
		log.Error(err, "Failed to list BIOSVersions for BMC", "BMC", bmcObj.Name)
		return nil
	}

	reqs := make([]ctrl.Request, 0)
	for _, version := range biosVersionList.Items {
		if version.Status.State == metalv1alpha1.BIOSVersionStateInProgress {
			resetBMC, err := GetCondition(r.Conditions, version.Status.Conditions, BMCConditionReset)
			if err != nil {
				log.Error(err, "Failed to get BMC reset condition")
				continue
			}
			if resetBMC.Status == metav1.ConditionTrue {
				continue
			}
			// Enqueue only if the BMC reset was requested
			if resetBMC.Reason == BMCReasonReset {
				reqs = append(reqs, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: version.Namespace, Name: version.Name}})
			}
		}
	}
	return reqs
}

// SetupWithManager sets up the controller with the Manager.
func (r *BIOSVersionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BIOSVersion{}).
		Owns(&metalv1alpha1.ServerMaintenance{}).
		Watches(&metalv1alpha1.Server{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBiosVersionByServerRefs)).
		Watches(&metalv1alpha1.BMC{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBiosSettingsByBMC)).
		Complete(r)
}

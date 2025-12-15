// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/controller-utils/clientutils"
	"github.com/ironcore-dev/controller-utils/conditionutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	"github.com/stmcginnis/gofish/common"
	"github.com/stmcginnis/gofish/redfish"
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
	biosVersion := &metalv1alpha1.BIOSVersion{}
	if err := r.Get(ctx, req.NamespacedName, biosVersion); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.V(1).Info("Reconciling BIOSVersion")

	return r.reconcileExists(ctx, log, biosVersion)
}

func (r *BIOSVersionReconciler) reconcileExists(ctx context.Context, log logr.Logger, biosVersion *metalv1alpha1.BIOSVersion) (ctrl.Result, error) {
	if r.shouldDelete(log, biosVersion) {
		return r.delete(ctx, log, biosVersion)
	}
	return r.reconcile(ctx, log, biosVersion)
}

func (r *BIOSVersionReconciler) shouldDelete(log logr.Logger, biosVersion *metalv1alpha1.BIOSVersion) bool {
	if biosVersion.DeletionTimestamp.IsZero() {
		return false
	}

	if controllerutil.ContainsFinalizer(biosVersion, BIOSVersionFinalizer) &&
		biosVersion.Status.State == metalv1alpha1.BIOSVersionStateInProgress {
		log.V(1).Info("Postponing deletion as BIOS version update is in progress")
		return false
	}

	return true
}

func (r *BIOSVersionReconciler) delete(ctx context.Context, log logr.Logger, biosVersion *metalv1alpha1.BIOSVersion) (ctrl.Result, error) {
	log.V(1).Info("Deleting BIOSVersion")
	defer log.V(1).Info("Deleted BIOSVersion")

	if !controllerutil.ContainsFinalizer(biosVersion, BIOSVersionFinalizer) {
		return ctrl.Result{}, nil
	}

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, biosVersion, BIOSVersionFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *BIOSVersionReconciler) cleanupServerMaintenanceReferences(ctx context.Context, log logr.Logger, biosVersion *metalv1alpha1.BIOSVersion) error {
	if biosVersion.Spec.ServerMaintenanceRef == nil {
		return nil
	}

	serverMaintenance, err := r.getServerMaintenanceForRef(ctx, biosVersion.Spec.ServerMaintenanceRef)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get referred ServerMaintenance: %w", err)
	}

	if serverMaintenance.DeletionTimestamp.IsZero() {
		if metav1.IsControlledBy(serverMaintenance, biosVersion) {
			log.V(1).Info("Deleting ServerMaintenance", "ServerMaintenance", client.ObjectKeyFromObject(serverMaintenance))
			if err := r.Delete(ctx, serverMaintenance); err != nil {
				return err
			}
		} else {
			log.V(1).Info("ServerMaintenance is controlled by somebody else", "ServerMaintenance", client.ObjectKeyFromObject(serverMaintenance))
		}
	}

	// Remove the reference if the object is gone.
	if apierrors.IsNotFound(err) {
		if err := r.patchServerMaintenanceRef(ctx, biosVersion, nil); err != nil {
			return fmt.Errorf("failed to clean up serverMaintenance ref in BIOSVersionReconciler status: %w", err)
		}
	}
	return nil
}

func (r *BIOSVersionReconciler) reconcile(ctx context.Context, log logr.Logger, biosVersion *metalv1alpha1.BIOSVersion) (ctrl.Result, error) {
	if shouldIgnoreReconciliation(biosVersion) {
		log.V(1).Info("Skipped BIOSVersion reconciliation")
		return ctrl.Result{}, nil
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, biosVersion, BIOSVersionFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	requeue, err := r.transitionState(ctx, log, biosVersion)
	if err != nil {
		return ctrl.Result{}, err
	}
	if requeue {
		return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
	}

	log.V(1).Info("Reconciled BIOSVersion")
	return ctrl.Result{}, nil
}

func (r *BIOSVersionReconciler) transitionState(ctx context.Context, log logr.Logger, biosVersion *metalv1alpha1.BIOSVersion) (bool, error) {
	if biosVersion.Spec.ServerRef == nil {
		return false, fmt.Errorf("BIOSVersion does not have a ServerRef")
	}

	server, err := GetServerByName(ctx, r.Client, biosVersion.Spec.ServerRef.Name)
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

	switch biosVersion.Status.State {
	case "", metalv1alpha1.BIOSVersionStatePending:
		return false, r.cleanup(ctx, log, bmcClient, biosVersion, server)
	case metalv1alpha1.BIOSVersionStateInProgress:
		if biosVersion.Spec.ServerMaintenanceRef == nil {
			if requeue, err := r.requestServerMaintenance(ctx, log, biosVersion, server); err != nil || requeue {
				return false, err
			}
		}

		if server.Status.State != metalv1alpha1.ServerStateMaintenance {
			log.V(1).Info("Server is not in maintenance. waiting...", "server State", server.Status.State, "server", server.Name)
			return false, nil
		}

		if server.Spec.ServerMaintenanceRef == nil || server.Spec.ServerMaintenanceRef.UID != biosVersion.Spec.ServerMaintenanceRef.UID {
			log.V(1).Info("Server is already in maintenance", "Server", server.Name)
			return false, nil
		}

		if ok, err := r.handleBMCReset(ctx, log, bmcClient, biosVersion, server); !ok || err != nil {
			return false, err
		}

		return r.processInProgressState(ctx, log, bmcClient, biosVersion, server)
	case metalv1alpha1.BIOSVersionStateCompleted:
		return false, r.cleanup(ctx, log, bmcClient, biosVersion, server)
	case metalv1alpha1.BIOSVersionStateFailed:
		if shouldRetryReconciliation(biosVersion) {
			log.V(1).Info("Retrying ...")
			biosVersionBase := biosVersion.DeepCopy()
			biosVersion.Status.State = metalv1alpha1.BIOSVersionStatePending
			biosVersion.Status.Conditions = nil
			annotations := biosVersion.GetAnnotations()
			delete(annotations, metalv1alpha1.OperationAnnotation)
			biosVersion.SetAnnotations(annotations)

			if err := r.Status().Patch(ctx, biosVersion, client.MergeFrom(biosVersionBase)); err != nil {
				return true, fmt.Errorf("failed to patch BIOSVersion status for retrying: %w", err)
			}
			return true, nil
		}
		log.V(1).Info("Failed to upgrade BIOSVersion", "BIOSVersion", biosVersion, "Server", server.Name)
		return false, nil
	}

	log.V(1).Info("Unknown State found", "State", biosVersion.Status.State)
	return false, nil
}

func (r *BIOSVersionReconciler) processInProgressState(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, biosVersion *metalv1alpha1.BIOSVersion, server *metalv1alpha1.Server) (bool, error) {
	issuedCondition, err := GetCondition(r.Conditions, biosVersion.Status.Conditions, ConditionBIOSUpgradeIssued)
	if err != nil {
		return false, err
	}

	if issuedCondition.Status != metav1.ConditionTrue {
		log.V(1).Info("Processing BIOS version upgrade ...")
		if server.Status.PowerState != metalv1alpha1.ServerOnPowerState {
			log.V(1).Info("Server in powered off state. Retrying ...", "Server", server.Name)
			return false, nil
		}
		return false, r.upgradeBIOSVersion(ctx, log, bmcClient, biosVersion, server, issuedCondition)
	}

	completedCondition, err := GetCondition(r.Conditions, biosVersion.Status.Conditions, ConditionBIOSUpgradeCompleted)
	if err != nil {
		return false, err
	}

	if completedCondition.Status != metav1.ConditionTrue {
		log.V(1).Info("Check BIOS version upgrade task status")
		return r.checkUpdateBiosUpgradeStatus(ctx, log, bmcClient, biosVersion, server, completedCondition)
	}

	rebootPowerOffCondition, err := GetCondition(r.Conditions, biosVersion.Status.Conditions, ConditionBIOSUpgradePowerOff)
	if err != nil {
		return false, err
	}

	if rebootPowerOffCondition.Status != metav1.ConditionTrue {
		log.V(1).Info("Ensuring server is powered off")
		if server.Status.PowerState != metalv1alpha1.ServerOffPowerState {
			return false, r.ensurePowerState(ctx, biosVersion, metalv1alpha1.PowerOff)
		}
		log.V(1).Info("Ensured server is powered off")

		if err := r.Conditions.Update(
			rebootPowerOffCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(ReasonRebootPowerOff),
			conditionutils.UpdateMessage("Powered off the server"),
		); err != nil {
			return false, fmt.Errorf("failed to update reboot power off condition: %w", err)
		}

		return false, r.updateStatus(ctx, biosVersion, biosVersion.Status.State, biosVersion.Status.UpgradeTask, rebootPowerOffCondition)
	}

	rebootPowerOnCondition, err := GetCondition(r.Conditions, biosVersion.Status.Conditions, ConditionBIOSUpgradePowerOn)
	if err != nil {
		return false, err
	}

	if rebootPowerOnCondition.Status != metav1.ConditionTrue {
		log.V(1).Info("Ensuring server is powered on")
		if server.Status.PowerState != metalv1alpha1.ServerOnPowerState {
			return false, r.ensurePowerState(ctx, biosVersion, metalv1alpha1.PowerOn)
		}
		log.V(1).Info("Ensured server is powered on")

		if err := r.Conditions.Update(
			rebootPowerOnCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(ReasonRebootPowerOn),
			conditionutils.UpdateMessage("Powered on the server"),
		); err != nil {
			return false, fmt.Errorf("failed to update reboot power on condition: %w", err)
		}

		return false, r.updateStatus(ctx, biosVersion, biosVersion.Status.State, biosVersion.Status.UpgradeTask, rebootPowerOnCondition)
	}

	condition, err := GetCondition(r.Conditions, biosVersion.Status.Conditions, ConditionBIOSUpgradeVerification)
	if err != nil {
		return false, err
	}

	if condition.Status != metav1.ConditionTrue {
		log.V(1).Info("Verifying BIOS version update")

		currentBiosVersion, err := r.getBIOSVersionFromBMC(ctx, bmcClient, server)
		if err != nil {
			return false, err
		}
		if currentBiosVersion != biosVersion.Spec.Version {
			// todo: add timeout
			log.V(1).Info("BIOS version not updated", "Version", currentBiosVersion, "DesiredVersion", biosVersion.Spec.Version)
			if condition.Reason == "" {
				if err := r.Conditions.Update(
					condition,
					conditionutils.UpdateStatus(corev1.ConditionFalse),
					conditionutils.UpdateReason(ReasonBIOSVersionVerification),
					conditionutils.UpdateMessage("waiting for BIOS Version update"),
				); err != nil {
					return false, fmt.Errorf("failed to update the verification condition: %w", err)
				}
			}
			log.V(1).Info("Waiting for BIOS version to reflect the new version")
			return true, nil
		}

		if err := r.Conditions.Update(
			condition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(ReasonBIOSVersionVerified),
			conditionutils.UpdateMessage("BIOS Version updated"),
		); err != nil {
			return false, fmt.Errorf("failed to update conditions: %w", err)
		}

		return false, r.updateStatus(ctx, biosVersion, metalv1alpha1.BIOSVersionStateCompleted, biosVersion.Status.UpgradeTask, condition)
	}

	log.V(1).Info("Unknown Conditions found", "Condition", condition.Type)
	return false, nil
}

func (r *BIOSVersionReconciler) handleBMCReset(
	ctx context.Context,
	log logr.Logger,
	bmcClient bmc.BMC,
	biosVersion *metalv1alpha1.BIOSVersion,
	server *metalv1alpha1.Server,
) (bool, error) {
	// reset BMC if not already done
	resetBMC, err := GetCondition(r.Conditions, biosVersion.Status.Conditions, BMCConditionReset)
	if err != nil {
		return false, fmt.Errorf("failed to get condition for reset of BMC of server %v", err)
	}

	if resetBMC.Status != metav1.ConditionTrue {
		// once the server is powered on, reset the BMC to make sure its in stable state
		// this avoids problems with some BMCs that hang up in subsequent operations
		if resetBMC.Reason != BMCReasonReset {
			if err := resetBMCOfServer(ctx, log, r.Client, server, bmcClient); err == nil {
				// mark reset to be issued, wait for next reconcile
				if err := r.Conditions.Update(
					resetBMC,
					conditionutils.UpdateStatus(corev1.ConditionFalse),
					conditionutils.UpdateReason(BMCReasonReset),
					conditionutils.UpdateMessage("Issued BMC reset to stabilize BMC of the server"),
				); err != nil {
					return false, fmt.Errorf("failed to update reset BMC condition: %w", err)
				}
				return false, r.updateStatus(ctx, biosVersion, biosVersion.Status.State, nil, resetBMC)
			} else {
				log.V(1).Error(err, "failed to reset BMC of the server")
				return false, err
			}
		} else if server.Spec.BMCRef != nil {
			// we need to wait until the BMC resource annotation is removed
			bmcObj := &metalv1alpha1.BMC{}
			if err := r.Get(ctx, client.ObjectKey{Name: server.Spec.BMCRef.Name}, bmcObj); err != nil {
				log.V(1).Error(err, "failed to get referred server's Manager")
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
		return false, r.updateStatus(ctx, biosVersion, biosVersion.Status.State, nil, resetBMC)
	}
	return true, nil
}

func (r *BIOSVersionReconciler) getBIOSVersionFromBMC(ctx context.Context, bmcClient bmc.BMC, server *metalv1alpha1.Server) (string, error) {
	currentBiosVersion, err := bmcClient.GetBiosVersion(ctx, server.Spec.SystemURI)
	if err != nil {
		return "", fmt.Errorf("failed to get BIOS version: %w", err)
	}

	return currentBiosVersion, nil
}

func (r *BIOSVersionReconciler) cleanup(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, biosVersion *metalv1alpha1.BIOSVersion, server *metalv1alpha1.Server) error {
	currentBiosVersion, err := r.getBIOSVersionFromBMC(ctx, bmcClient, server)
	if err != nil {
		return err
	}

	if currentBiosVersion == biosVersion.Spec.Version {
		if err := r.cleanupServerMaintenanceReferences(ctx, log, biosVersion); err != nil {
			return err
		}

		log.V(1).Info("Upgraded BIOS version", "Version", currentBiosVersion, "Server", server.Name)
		return r.updateStatus(ctx, biosVersion, metalv1alpha1.BIOSVersionStateCompleted, nil, nil)
	}
	return r.updateStatus(ctx, biosVersion, metalv1alpha1.BIOSVersionStateInProgress, nil, nil)
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

func (r *BIOSVersionReconciler) updateStatus(
	ctx context.Context,
	biosVersion *metalv1alpha1.BIOSVersion,
	state metalv1alpha1.BIOSVersionState,
	upgradeTask *metalv1alpha1.Task,
	condition *metav1.Condition,
) error {
	if biosVersion.Status.State == state && condition == nil && upgradeTask == nil {
		return nil
	}

	biosVersionBase := biosVersion.DeepCopy()
	biosVersion.Status.State = state

	if condition != nil {
		if err := r.Conditions.UpdateSlice(
			&biosVersion.Status.Conditions,
			condition.Type,
			conditionutils.UpdateStatus(condition.Status),
			conditionutils.UpdateReason(condition.Reason),
			conditionutils.UpdateMessage(condition.Message),
		); err != nil {
			return fmt.Errorf("failed to patch BIOSVersion condition: %w", err)
		}
	} else {
		biosVersion.Status.Conditions = nil
	}

	biosVersion.Status.UpgradeTask = upgradeTask

	if err := r.Status().Patch(ctx, biosVersion, client.MergeFrom(biosVersionBase)); err != nil {
		return fmt.Errorf("failed to patch BIOSVersion status: %w", err)
	}

	return nil
}

func (r *BIOSVersionReconciler) patchServerMaintenanceRef(ctx context.Context, biosVersion *metalv1alpha1.BIOSVersion, serverMaintenance *metalv1alpha1.ServerMaintenance) error {
	biosVersionsBase := biosVersion.DeepCopy()

	if serverMaintenance == nil {
		biosVersion.Spec.ServerMaintenanceRef = nil
	} else {
		biosVersion.Spec.ServerMaintenanceRef = &metalv1alpha1.ObjectReference{
			APIVersion: serverMaintenance.GroupVersionKind().GroupVersion().String(),
			Kind:       "ServerMaintenance",
			Namespace:  serverMaintenance.Namespace,
			Name:       serverMaintenance.Name,
			UID:        serverMaintenance.UID,
		}
	}

	if err := r.Patch(ctx, biosVersion, client.MergeFrom(biosVersionsBase)); err != nil {
		return err
	}

	return nil
}

func (r *BIOSVersionReconciler) ensurePowerState(ctx context.Context, biosVersion *metalv1alpha1.BIOSVersion, powerState metalv1alpha1.Power) error {
	serverMaintenance, err := r.getServerMaintenanceForRef(ctx, biosVersion.Spec.ServerMaintenanceRef)
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

func (r *BIOSVersionReconciler) requestServerMaintenance(ctx context.Context, log logr.Logger, biosVersion *metalv1alpha1.BIOSVersion, server *metalv1alpha1.Server) (bool, error) {
	if biosVersion.Spec.ServerMaintenanceRef != nil {
		return false, nil
	}

	serverMaintenance := &metalv1alpha1.ServerMaintenance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.ManagerNamespace,
			Name:      biosVersion.Name,
		},
	}

	opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, serverMaintenance, func() error {
		serverMaintenance.Spec.Policy = biosVersion.Spec.ServerMaintenancePolicy
		serverMaintenance.Spec.ServerPower = metalv1alpha1.PowerOn
		serverMaintenance.Spec.ServerRef = &corev1.LocalObjectReference{Name: server.Name}
		if serverMaintenance.Status.State != metalv1alpha1.ServerMaintenanceStateInMaintenance && serverMaintenance.Status.State != "" {
			serverMaintenance.Status.State = ""
		}
		return controllerutil.SetControllerReference(biosVersion, serverMaintenance, r.Client.Scheme())
	})
	if err != nil {
		return false, fmt.Errorf("failed to create or patch serverMaintenance: %w", err)
	}
	log.V(1).Info("Created ServerMaintenance", "ServerMaintenance", client.ObjectKeyFromObject(serverMaintenance), "Operation", opResult)

	if err = r.patchServerMaintenanceRef(ctx, biosVersion, serverMaintenance); err != nil {
		return false, fmt.Errorf("failed to patch ServerMaintenance ref in BIOSVersion status: %w", err)
	}

	log.V(1).Info("Patched ServerMaintenance on BIOSVersion")
	return true, nil
}

func (r *BIOSVersionReconciler) checkUpdateBiosUpgradeStatus(
	ctx context.Context,
	log logr.Logger,
	bmcClient bmc.BMC,
	biosVersion *metalv1alpha1.BIOSVersion,
	server *metalv1alpha1.Server,
	completedCondition *metav1.Condition,
) (bool, error) {
	taskURI := biosVersion.Status.UpgradeTask.URI
	taskCurrentStatus, err := func() (*redfish.Task, error) {
		if taskURI == "" {
			return nil, fmt.Errorf("invalid task URI. uri provided: '%v'", taskURI)
		}
		return bmcClient.GetBiosUpgradeTask(ctx, server.Status.Manufacturer, taskURI)
	}()
	if err != nil {
		return false, fmt.Errorf("failed to get the task details of bios upgrade task %s: %w", taskURI, err)
	}
	log.V(1).Info("BIOS upgrade task current status", "TaskState", taskCurrentStatus.TaskState)

	upgradeCurrentTaskStatus := &metalv1alpha1.Task{
		URI:             biosVersion.Status.UpgradeTask.URI,
		State:           taskCurrentStatus.TaskState,
		Status:          taskCurrentStatus.TaskStatus,
		PercentComplete: int32(taskCurrentStatus.PercentComplete),
	}

	// use checkpoint in case the job has stalled and we need to requeue
	transition := &conditionutils.FieldsTransition{
		IncludeStatus:  true,
		IncludeReason:  true,
		IncludeMessage: true,
	}
	checkpoint, err := transition.Checkpoint(r.Conditions, *completedCondition)
	if err != nil {
		return false, fmt.Errorf("failed to create checkpoint for Condition. %w", err)
	}

	if taskCurrentStatus.TaskState == redfish.KilledTaskState ||
		taskCurrentStatus.TaskState == redfish.ExceptionTaskState ||
		taskCurrentStatus.TaskState == redfish.CancelledTaskState ||
		(taskCurrentStatus.TaskStatus != common.OKHealth && taskCurrentStatus.TaskStatus != "") {
		message := fmt.Sprintf(
			"Upgrade Bios task has failed. with message %v check '%v' for details",
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

		return false, r.updateStatus(ctx, biosVersion, metalv1alpha1.BIOSVersionStateFailed, upgradeCurrentTaskStatus, completedCondition)
	}

	if taskCurrentStatus.TaskState == redfish.CompletedTaskState {
		if err := r.Conditions.Update(
			completedCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(ReasonUpgradeTaskCompleted),
			conditionutils.UpdateMessage("BIOS version successfully upgraded to: "+biosVersion.Spec.Version),
		); err != nil {
			return false, fmt.Errorf("failed to update conditions: %w", err)
		}

		return false, r.updateStatus(ctx, biosVersion, biosVersion.Status.State, upgradeCurrentTaskStatus, completedCondition)
	}

	// in-progress task states
	if err := r.Conditions.Update(
		completedCondition,
		conditionutils.UpdateStatus(corev1.ConditionFalse),
		conditionutils.UpdateReason(taskCurrentStatus.TaskState),
		conditionutils.UpdateMessage(
			fmt.Sprintf("BIOS upgrade in state: %v: PercentageCompleted %v",
				taskCurrentStatus.TaskState,
				taskCurrentStatus.PercentComplete),
		),
	); err != nil {
		return false, fmt.Errorf("failed to update conditions: %w", err)
	}

	ok, err := checkpoint.Transitioned(r.Conditions, *completedCondition)
	if !ok && err == nil {
		log.V(1).Info("BIOS upgrade task has not progressed. retrying....")
		// The upgrade job has stalled or is too slow. We need to requeue with exponential backoff.
		return true, nil
	}

	// todo: Fail the state after certain timeout
	return false, r.updateStatus(ctx, biosVersion, biosVersion.Status.State, upgradeCurrentTaskStatus, completedCondition)
}

func (r *BIOSVersionReconciler) upgradeBIOSVersion(
	ctx context.Context,
	log logr.Logger,
	bmcClient bmc.BMC,
	biosVersion *metalv1alpha1.BIOSVersion,
	server *metalv1alpha1.Server,
	issuedCondition *metav1.Condition,
) error {
	var username, password string
	if biosVersion.Spec.Image.SecretRef != nil {
		var err error
		password, username, err = GetImageCredentialsForSecretRef(ctx, r.Client, biosVersion.Spec.Image.SecretRef)
		if err != nil {
			return fmt.Errorf("failed to get image credentials ref for: %w", err)
		}
	}

	var forceUpdate bool
	if biosVersion.Spec.UpdatePolicy != nil && *biosVersion.Spec.UpdatePolicy == metalv1alpha1.UpdatePolicyForce {
		forceUpdate = true
	}

	parameters := &redfish.SimpleUpdateParameters{
		ForceUpdate:      forceUpdate,
		ImageURI:         biosVersion.Spec.Image.URI,
		Passord:          password,
		Username:         username,
		TransferProtocol: redfish.TransferProtocolType(biosVersion.Spec.Image.TransferProtocol),
	}

	taskMonitor, isFatal, err := func() (string, bool, error) {
		return bmcClient.UpgradeBiosVersion(ctx, server.Status.Manufacturer, parameters)
	}()

	upgradeCurrentTaskStatus := &metalv1alpha1.Task{URI: taskMonitor}

	if isFatal {
		log.V(1).Error(err, "failed to issue bios upgrade", "Version", biosVersion.Spec.Version, "Server", server.Name)
		if errCond := r.Conditions.Update(
			issuedCondition,
			conditionutils.UpdateStatus(corev1.ConditionFalse),
			conditionutils.UpdateReason(ReasonUpgradeIssueFailed),
			conditionutils.UpdateMessage("Fatal error occurred. Upgrade might still go through on server."),
		); errCond != nil {
			log.V(1).Error(errCond, "failed to update conditions")
			err := r.updateStatus(ctx, biosVersion, metalv1alpha1.BIOSVersionStateFailed, upgradeCurrentTaskStatus, issuedCondition)
			return errors.Join(errCond, err)
		}

		return r.updateStatus(ctx, biosVersion, metalv1alpha1.BIOSVersionStateFailed, upgradeCurrentTaskStatus, issuedCondition)
	}
	if err != nil {
		log.V(1).Error(err, "failed to issue bios upgrade", "Version", biosVersion.Spec.Version, "Server", server.Name)
		return err
	}
	if errCond := r.Conditions.Update(
		issuedCondition,
		conditionutils.UpdateStatus(corev1.ConditionTrue),
		conditionutils.UpdateReason(ReasonUpgradeIssued),
		conditionutils.UpdateMessage(fmt.Sprintf("Task to upgrade has been created %v", taskMonitor)),
	); errCond != nil {
		log.V(1).Error(errCond, "failed to update conditions")
		if errCond := r.Conditions.Update(
			issuedCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(ReasonUpgradeIssued),
			conditionutils.UpdateMessage(fmt.Sprintf("Task to upgrade has been created %v", taskMonitor)),
		); errCond != nil {
			log.V(1).Error(errCond, "failed to update conditions")
			err := r.updateStatus(ctx, biosVersion, metalv1alpha1.BIOSVersionStateFailed, upgradeCurrentTaskStatus, issuedCondition)
			return errors.Join(errCond, err)
		}
	}

	return r.updateStatus(ctx, biosVersion, biosVersion.Status.State, upgradeCurrentTaskStatus, issuedCondition)
}

func (r *BIOSVersionReconciler) enqueueBiosVersionByServerRefs(ctx context.Context, obj client.Object) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)
	host := obj.(*metalv1alpha1.Server)

	// don't requeue if host in wrong state
	if host.Status.State == metalv1alpha1.ServerStateDiscovery ||
		host.Status.State == metalv1alpha1.ServerStateError ||
		host.Status.State == metalv1alpha1.ServerStateInitial {
		return nil
	}

	// don't requeue if host does not have Maintenance
	if host.Spec.ServerMaintenanceRef == nil {
		return nil
	}

	biosVersionList := &metalv1alpha1.BIOSVersionList{}
	if err := r.List(ctx, biosVersionList); err != nil {
		log.Error(err, "failed to list biosVersionList")
		return nil
	}

	for _, biosVersion := range biosVersionList.Items {
		if biosVersion.Spec.ServerRef.Name == host.Name {
			// states where we do not need to requeue for host changes
			if biosVersion.Spec.ServerMaintenanceRef == nil ||
				biosVersion.Status.State == metalv1alpha1.BIOSVersionStateCompleted ||
				biosVersion.Status.State == metalv1alpha1.BIOSVersionStateFailed {
				return nil
			}
			if biosVersion.Spec.ServerMaintenanceRef.Name != host.Spec.ServerMaintenanceRef.Name {
				return nil
			}
			return []ctrl.Request{{
				NamespacedName: types.NamespacedName{Namespace: biosVersion.Namespace, Name: biosVersion.Name},
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
		log.V(1).Error(err, "failed to list Server created by this BMC resources", "BMC", bmcObj.Name)
		return nil
	}

	serverMap := make(map[string]struct{})
	for _, server := range serverList.Items {
		serverMap[server.Name] = struct{}{}
	}

	biosVersionList := &metalv1alpha1.BIOSVersionList{}
	if err := clientutils.ListAndFilter(ctx, r.Client, biosVersionList, func(object client.Object) (bool, error) {
		biosVersion := object.(*metalv1alpha1.BIOSVersion)
		if _, exists := serverMap[biosVersion.Spec.ServerRef.Name]; !exists {
			return false, nil
		}
		return true, nil
	}); err != nil {
		log.V(1).Error(err, "failed to list Server created by this BMC resources", "BMC", bmcObj.Name)
		return nil
	}

	reqs := make([]ctrl.Request, 0)
	for _, biosVersion := range biosVersionList.Items {
		if biosVersion.Status.State == metalv1alpha1.BIOSVersionStateInProgress {
			resetBMC, err := GetCondition(r.Conditions, biosVersion.Status.Conditions, BMCConditionReset)
			if err != nil {
				log.V(1).Error(err, "failed to get reset BMC condition")
				continue
			}
			if resetBMC.Status == metav1.ConditionTrue {
				continue
			}
			// enqueue only if the BMC reset is requested for this BMC
			if resetBMC.Reason == BMCReasonReset {
				reqs = append(reqs, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: biosVersion.Namespace, Name: biosVersion.Name}})
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

// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/controller-utils/clientutils"
	"github.com/ironcore-dev/controller-utils/conditionutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	"github.com/stmcginnis/gofish/common"
	"github.com/stmcginnis/gofish/redfish"
)

// BIOSVersionReconciler reconciles a BIOSVersion object
type BIOSVersionReconciler struct {
	client.Client
	ManagerNamespace string
	Insecure         bool
	Scheme           *runtime.Scheme
	BMCOptions       bmc.Options
	ResyncInterval   time.Duration
}

const (
	BIOSVersionFinalizer                   = "metal.ironcore.dev/biosversion"
	biosVersionUpgradeIssued               = "BIOSVersionUpgradeIssued"
	biosVersionUpgradeCompleted            = "BIOSVersionUpgradeCompleted"
	biosVersionUpgradeRebootServerPowerOn  = "BIOSVersionUpgradePowerOn"
	biosVersionUpgradeRebootServerPowerOff = "BIOSVersionUpgradePowerOff"
	biosVersionUpgradeVerficationCondition = "BIOSVersionUpgradeVerification"
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
	_ = log.FromContext(ctx)
	log := ctrl.LoggerFrom(ctx)
	biosVersion := &metalv1alpha1.BIOSVersion{}
	if err := r.Get(ctx, req.NamespacedName, biosVersion); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.V(1).Info("Reconciling BIOSVersion")

	return r.reconcileExists(ctx, log, biosVersion)
}

// Determine whether reconciliation is required. It's not required if:
// - object is being deleted;
func (r *BIOSVersionReconciler) reconcileExists(
	ctx context.Context,
	log logr.Logger,
	biosVersion *metalv1alpha1.BIOSVersion,
) (ctrl.Result, error) {
	// if object is being deleted - reconcile deletion
	if delete := r.shouldDelete(log, biosVersion); delete {
		log.V(1).Info("Object is being deleted")
		return r.delete(ctx, log, biosVersion)
	}

	return r.reconcile(ctx, log, biosVersion)
}

func (r *BIOSVersionReconciler) shouldDelete(
	log logr.Logger,
	biosVersion *metalv1alpha1.BIOSVersion,
) bool {
	if biosVersion.DeletionTimestamp.IsZero() {
		return false
	}

	if controllerutil.ContainsFinalizer(biosVersion, BIOSVersionFinalizer) &&
		biosVersion.Status.State == metalv1alpha1.BIOSVersionStateInProgress {
		log.V(1).Info("postponing delete as Version update is in progress")
		return false
	}
	return true
}

func (r *BIOSVersionReconciler) delete(
	ctx context.Context,
	log logr.Logger,
	biosVersion *metalv1alpha1.BIOSVersion,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(biosVersion, BIOSVersionFinalizer) {
		return ctrl.Result{}, nil
	}

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, biosVersion, BIOSVersionFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	log.V(1).Info("BIOSVersion is deleted")
	return ctrl.Result{}, nil
}

func (r *BIOSVersionReconciler) cleanupServerMaintenanceReferences(
	ctx context.Context,
	log logr.Logger,
	biosVersion *metalv1alpha1.BIOSVersion,
) error {
	if biosVersion.Spec.ServerMaintenanceRef == nil {
		return nil
	}
	// try to get the serverMaintaince created
	serverMaintenance, err := r.getReferredServerMaintenance(ctx, log, biosVersion.Spec.ServerMaintenanceRef)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get referred serverMaintenance obj from BIOSVersion: %w", err)
	}

	// if we got the server ref. by and its not being deleted
	if err == nil && serverMaintenance.DeletionTimestamp.IsZero() {
		// created by the controller
		if metav1.IsControlledBy(serverMaintenance, biosVersion) {
			// if the BIOSVersion is not being deleted, update the
			log.V(1).Info("Deleting server maintenance", "serverMaintenance Name", serverMaintenance.Name, "state", serverMaintenance.Status.State)
			if err := r.Delete(ctx, serverMaintenance); err != nil {
				return err
			}
		} else { // not created by controller
			log.V(1).Info("Server maintenance status not updated as its provided by user", "serverMaintenance Name", serverMaintenance.Name, "state", serverMaintenance.Status.State)
		}
	}
	// if already deleted, or we deleted it, or it's created by a user, remove reference
	if apierrors.IsNotFound(err) || err == nil {
		err = r.patchMaintenanceRequestRefOnBiosVersion(ctx, log, biosVersion, nil)
		if err != nil {
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

	requeue, err := r.ensureBiosVersionStateTransition(ctx, log, biosVersion)
	if err != nil {
		return ctrl.Result{}, err
	}
	if requeue {
		return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
	}
	return ctrl.Result{}, nil
}

func (r *BIOSVersionReconciler) ensureBiosVersionStateTransition(
	ctx context.Context,
	log logr.Logger,
	biosVersion *metalv1alpha1.BIOSVersion,
) (bool, error) {
	server, err := r.getReferredServer(ctx, log, biosVersion.Spec.ServerRef)
	if err != nil {
		log.V(1).Info("Referred server object could not be fetched")
		return false, err
	}

	bmcClient, err := bmcutils.GetBMCClientForServer(ctx, r.Client, server, r.Insecure, r.BMCOptions)
	if err != nil {
		return false, fmt.Errorf("failed to get BMC client for server: %w", err)
	}
	defer bmcClient.Logout()

	switch biosVersion.Status.State {
	case "", metalv1alpha1.BIOSVersionStatePending:
		return false, r.checkVersionAndTransistionState(ctx, log, bmcClient, biosVersion, server)
	case metalv1alpha1.BIOSVersionStateInProgress:
		if biosVersion.Spec.ServerMaintenanceRef == nil {
			if requeue, err := r.requestMaintenanceOnServer(ctx, log, biosVersion, server); err != nil || requeue {
				return false, err
			}
		}

		if server.Status.State != metalv1alpha1.ServerStateMaintenance {
			log.V(1).Info("Server is not in maintenance. waiting...", "server State", server.Status.State, "server", server.Name)
			return false, nil
		}

		if server.Spec.ServerMaintenanceRef == nil || server.Spec.ServerMaintenanceRef.UID != biosVersion.Spec.ServerMaintenanceRef.UID {
			// server in maintenance for other tasks. or
			// server maintenance ref is wrong in either server or biosSettings
			// wait for update on the server obj
			log.V(1).Info("Server is already in maintenance for other tasks", "Server", server.Name, "serverMaintenanceRef", server.Spec.ServerMaintenanceRef)
			return false, nil
		}

		return r.handleUpgradeInProgressState(ctx, log, bmcClient, biosVersion, server)
	case metalv1alpha1.BIOSVersionStateCompleted:
		// clean up maintenance crd and references and mark completed if version matches.
		return false, r.checkVersionAndTransistionState(ctx, log, bmcClient, biosVersion, server)
	case metalv1alpha1.BIOSVersionStateFailed:
		log.V(1).Info("Failed to upgrade BIOSVersion", "ctx", ctx, "BIOSVersion", biosVersion, "server", server)
		return false, nil
	}
	log.V(1).Info("Unknown State found", "BIOSVersion state", biosVersion.Status.State)
	return false, nil
}

func (r *BIOSVersionReconciler) handleUpgradeInProgressState(
	ctx context.Context,
	log logr.Logger,
	bmcClient bmc.BMC,
	biosVersion *metalv1alpha1.BIOSVersion,
	server *metalv1alpha1.Server,
) (bool, error) {
	acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
	issuedCondition, err := r.getCondition(acc, biosVersion.Status.Conditions, biosVersionUpgradeIssued)
	if err != nil {
		return false, err
	}

	if issuedCondition.Status != metav1.ConditionTrue {
		log.V(1).Info("Issuing Upgrade of BIOS version")
		if server.Status.PowerState != metalv1alpha1.ServerOnPowerState {
			log.V(1).Info("Server is still powered off. waiting", "Server", server.Name, "Server power state", server.Status.PowerState)
			return false, nil
		}
		return false, r.issueBiosUpgrade(ctx, log, bmcClient, biosVersion, server, issuedCondition, acc)
	}

	completedCondition, err := r.getCondition(acc, biosVersion.Status.Conditions, biosVersionUpgradeCompleted)
	if err != nil {
		return false, err
	}

	if completedCondition.Status != metav1.ConditionTrue {
		log.V(1).Info("Check Upgrade task of Bios")
		return r.checkUpdateBiosUpgradeStatus(ctx, log, bmcClient, biosVersion, server, completedCondition, acc)
	}

	rebootPowerOffCondition, err := r.getCondition(acc, biosVersion.Status.Conditions, biosVersionUpgradeRebootServerPowerOff)
	if err != nil {
		return false, err
	}

	if rebootPowerOffCondition.Status != metav1.ConditionTrue {
		log.V(1).Info("Turn server power Off")
		if server.Status.PowerState != metalv1alpha1.ServerOffPowerState {
			return false, r.patchServerMaintenancePowerState(ctx, log, biosVersion, metalv1alpha1.PowerOff)
		}
		if err := acc.Update(
			rebootPowerOffCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason("RebootPowerOff"),
			conditionutils.UpdateMessage("Powered off the server"),
		); err != nil {
			return false, fmt.Errorf("failed to update reboot power off condition: %w", err)
		}
		err = r.updateBiosVersionStatus(
			ctx,
			log,
			biosVersion,
			biosVersion.Status.State,
			biosVersion.Status.UpgradeTask,
			rebootPowerOffCondition,
			acc,
		)
		return false, err
	}

	rebootPowerOnCondition, err := r.getCondition(acc, biosVersion.Status.Conditions, biosVersionUpgradeRebootServerPowerOn)
	if err != nil {
		return false, err
	}

	if rebootPowerOnCondition.Status != metav1.ConditionTrue {
		log.V(1).Info("Turn server power On")
		if server.Status.PowerState != metalv1alpha1.ServerOnPowerState {
			return false, r.patchServerMaintenancePowerState(ctx, log, biosVersion, metalv1alpha1.PowerOn)
		}

		if err := acc.Update(
			rebootPowerOnCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason("RebootPowerOn"),
			conditionutils.UpdateMessage("Powered on the server"),
		); err != nil {
			return false, fmt.Errorf("failed to update reboot power on condition: %w", err)
		}
		err = r.updateBiosVersionStatus(
			ctx,
			log,
			biosVersion,
			biosVersion.Status.State,
			biosVersion.Status.UpgradeTask,
			rebootPowerOnCondition,
			acc,
		)
		return false, err
	}

	VerificationCondition, err := r.getCondition(acc, biosVersion.Status.Conditions, biosVersionUpgradeVerficationCondition)
	if err != nil {
		return false, err
	}

	if VerificationCondition.Status != metav1.ConditionTrue {
		log.V(1).Info("Verify Bios Version update")

		currentBiosVersion, err := r.getBiosVersionFromBMC(ctx, log, bmcClient, server)
		if err != nil {
			return false, err
		}
		if currentBiosVersion != biosVersion.Spec.Version {
			// todo: add timeout
			log.V(1).Info("BIOS version not updated", "Current Bios Version", currentBiosVersion, "Required Version", biosVersion.Spec.Version)
			if VerificationCondition.Reason == "" {
				if err := acc.Update(
					VerificationCondition,
					conditionutils.UpdateStatus(corev1.ConditionFalse),
					conditionutils.UpdateReason("VerifyBIOSVersionUpdate"),
					conditionutils.UpdateMessage("waiting for BIOS Version update"),
				); err != nil {
					return false, fmt.Errorf("failed to update the verification condition: %w", err)
				}
			}
			log.V(1).Info("Waiting for bios version to reflect the new version")
			return true, nil
		}

		if err := acc.Update(
			VerificationCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason("VerifedBIOSVersionUpdate"),
			conditionutils.UpdateMessage("BIOS Version updated"),
		); err != nil {
			log.V(1).Error(err, "failed to update the conditions status. retrying...")
			return false, err
		}
		err = r.updateBiosVersionStatus(
			ctx,
			log,
			biosVersion,
			metalv1alpha1.BIOSVersionStateCompleted,
			biosVersion.Status.UpgradeTask,
			VerificationCondition,
			acc,
		)
		return false, err
	}

	log.V(1).Info("Unknown Conditions found", "BIOSVersion Conditions", biosVersion.Status.Conditions)
	return false, nil
}

func (r *BIOSVersionReconciler) getBiosVersionFromBMC(
	ctx context.Context,
	log logr.Logger,
	bmcClient bmc.BMC,
	server *metalv1alpha1.Server,
) (string, error) {
	currentBiosVersion, err := bmcClient.GetBiosVersion(ctx, server.Spec.SystemURI)
	if err != nil {
		log.V(1).Error(err, "failed to get current BIOS version", "server", server.Name)
		return "", err
	}

	return currentBiosVersion, nil
}

func (r *BIOSVersionReconciler) checkVersionAndTransistionState(
	ctx context.Context,
	log logr.Logger,
	bmcClient bmc.BMC,
	biosVersion *metalv1alpha1.BIOSVersion,
	server *metalv1alpha1.Server,
) error {
	currentBiosVersion, err := r.getBiosVersionFromBMC(ctx, log, bmcClient, server)
	if err != nil {
		return err
	}
	if currentBiosVersion == biosVersion.Spec.Version {
		if err := r.cleanupServerMaintenanceReferences(ctx, log, biosVersion); err != nil {
			return err
		}
		log.V(1).Info("Done with bios version upgrade", "ctx", ctx, "Current BIOS Version", currentBiosVersion, "Server", server.Name)
		err := r.updateBiosVersionStatus(ctx, log, biosVersion, metalv1alpha1.BIOSVersionStateCompleted, nil, nil, nil)
		return err
	}
	err = r.updateBiosVersionStatus(ctx, log, biosVersion, metalv1alpha1.BIOSVersionStateInProgress, nil, nil, nil)
	return err
}

func (r *BIOSVersionReconciler) getCondition(acc *conditionutils.Accessor, conditions []metav1.Condition, conditionType string) (*metav1.Condition, error) {
	condition := &metav1.Condition{}
	condFound, err := acc.FindSlice(conditions, conditionType, condition)

	if err != nil {
		return nil, fmt.Errorf("failed to find Condition %v. error: %v", conditionType, err)
	}
	if !condFound {
		condition.Type = conditionType
		if err := acc.Update(
			condition,
			conditionutils.UpdateStatus(corev1.ConditionFalse),
		); err != nil {
			return condition, fmt.Errorf("failed to create/update new Condition %v. error: %v", conditionType, err)
		}
	}

	return condition, nil
}

func (r *BIOSVersionReconciler) getReferredServerMaintenance(
	ctx context.Context,
	log logr.Logger,
	serverMaintenanceRef *corev1.ObjectReference,
) (*metalv1alpha1.ServerMaintenance, error) {
	key := client.ObjectKey{Name: serverMaintenanceRef.Name, Namespace: r.ManagerNamespace}
	serverMaintenance := &metalv1alpha1.ServerMaintenance{}
	if err := r.Get(ctx, key, serverMaintenance); err != nil {
		log.V(1).Error(err, "failed to get referred serverMaintenance obj")
		return serverMaintenance, err
	}

	return serverMaintenance, nil
}

func (r *BIOSVersionReconciler) getReferredServer(
	ctx context.Context,
	log logr.Logger,
	serverRef *corev1.LocalObjectReference,
) (*metalv1alpha1.Server, error) {
	key := client.ObjectKey{Name: serverRef.Name}
	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, key, server); err != nil {
		log.V(1).Error(err, "failed to get referred server")
		return server, err
	}
	return server, nil
}

func (r *BIOSVersionReconciler) getReferredSecret(
	ctx context.Context,
	log logr.Logger,
	secretRef *corev1.LocalObjectReference,
) (string, string, error) {
	if secretRef == nil {
		return "", "", nil
	}
	key := client.ObjectKey{Name: secretRef.Name}
	secret := &corev1.Secret{}
	if err := r.Get(ctx, key, secret); err != nil {
		log.V(1).Error(err, "failed to get referred Secret obj", "secret name", secretRef.Name)
		return "", "", err
	}

	return secret.StringData[metalv1alpha1.BMCSecretUsernameKeyName], secret.StringData[metalv1alpha1.BMCSecretPasswordKeyName], nil
}

func (r *BIOSVersionReconciler) updateBiosVersionStatus(
	ctx context.Context,
	log logr.Logger,
	biosVersion *metalv1alpha1.BIOSVersion,
	state metalv1alpha1.BIOSVersionState,
	upgradeTask *metalv1alpha1.Task,
	condition *metav1.Condition,
	acc *conditionutils.Accessor,
) error {
	if biosVersion.Status.State == state && condition == nil && upgradeTask == nil {
		return nil
	}

	biosVersionBase := biosVersion.DeepCopy()
	biosVersion.Status.State = state

	if condition != nil {
		if err := acc.UpdateSlice(
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

	log.V(1).Info("Updated BIOSVersion state ",
		"new state", state,
		"new conditions", biosVersion.Status.Conditions,
		"Upgrade Task status", biosVersion.Status.UpgradeTask,
	)

	return nil
}

func (r *BIOSVersionReconciler) patchMaintenanceRequestRefOnBiosVersion(
	ctx context.Context,
	log logr.Logger,
	biosVersion *metalv1alpha1.BIOSVersion,
	serverMaintenance *metalv1alpha1.ServerMaintenance,
) error {
	biosVersionsBase := biosVersion.DeepCopy()

	if serverMaintenance == nil {
		biosVersion.Spec.ServerMaintenanceRef = nil
	} else {
		biosVersion.Spec.ServerMaintenanceRef = &corev1.ObjectReference{
			APIVersion: serverMaintenance.GroupVersionKind().GroupVersion().String(),
			Kind:       "ServerMaintenance",
			Namespace:  serverMaintenance.Namespace,
			Name:       serverMaintenance.Name,
			UID:        serverMaintenance.UID,
		}
	}

	if err := r.Patch(ctx, biosVersion, client.MergeFrom(biosVersionsBase)); err != nil {
		log.V(1).Error(err, "failed to patch BIOSVersion serverMaintenance ref")
		return err
	}

	return nil
}

func (r *BIOSVersionReconciler) patchServerMaintenancePowerState(
	ctx context.Context,
	log logr.Logger,
	biosVersion *metalv1alpha1.BIOSVersion,
	powerState metalv1alpha1.Power,
) error {
	serverMaintenance, err := r.getReferredServerMaintenance(ctx, log, biosVersion.Spec.ServerMaintenanceRef)
	if err != nil {
		return err
	}
	if serverMaintenance.Spec.ServerPower == powerState {
		return nil
	}

	serverMaintenanceBase := serverMaintenance.DeepCopy()
	serverMaintenance.Spec.ServerPower = powerState
	if err := r.Patch(ctx, serverMaintenance, client.MergeFrom(serverMaintenanceBase)); err != nil {
		return fmt.Errorf("failed to patch power for serverMaintenance: %w", err)
	}

	log.V(1).Info("Patched desired Power of the ServerMaintenance", "Server", serverMaintenance.Spec.ServerRef.Name, "state", powerState)
	return nil
}

func (r *BIOSVersionReconciler) requestMaintenanceOnServer(
	ctx context.Context,
	log logr.Logger,
	biosVersion *metalv1alpha1.BIOSVersion,
	server *metalv1alpha1.Server,
) (bool, error) {
	// if Server maintenance ref is already given. no further action required.
	if biosVersion.Spec.ServerMaintenanceRef != nil {
		return false, nil
	}

	serverMaintenance := &metalv1alpha1.ServerMaintenance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.ManagerNamespace,
			Name:      biosVersion.Name,
		}}

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
	log.V(1).Info("Created serverMaintenance", "serverMaintenance", serverMaintenance.Name, "serverMaintenance label", serverMaintenance.Labels, "Operation", opResult)

	err = r.patchMaintenanceRequestRefOnBiosVersion(ctx, log, biosVersion, serverMaintenance)
	if err != nil {
		return false, fmt.Errorf("failed to patch serverMaintenance ref in BIOSVersion status: %w", err)
	}

	log.V(1).Info("Patched serverMaintenance on BIOSVersion")

	return true, nil
}

func (r *BIOSVersionReconciler) checkUpdateBiosUpgradeStatus(
	ctx context.Context,
	log logr.Logger,
	bmcClient bmc.BMC,
	biosVersion *metalv1alpha1.BIOSVersion,
	server *metalv1alpha1.Server,
	completedCondition *metav1.Condition,
	acc *conditionutils.Accessor,
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
	log.V(1).Info("BIOS upgrade task current status", "Task status", taskCurrentStatus)

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
	checkpoint, err := transition.Checkpoint(acc, *completedCondition)
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
		if err := acc.Update(
			completedCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason("BiosUpgradeTaskFailed"),
			conditionutils.UpdateMessage(message),
		); err != nil {
			return false, fmt.Errorf("failed to update the conditions status: %w", err)
		}
		err = r.updateBiosVersionStatus(
			ctx,
			log,
			biosVersion,
			metalv1alpha1.BIOSVersionStateFailed,
			upgradeCurrentTaskStatus,
			completedCondition,
			acc,
		)
		return false, err
	}

	if taskCurrentStatus.TaskState == redfish.CompletedTaskState {
		if err := acc.Update(
			completedCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason("taskCompleted"),
			conditionutils.UpdateMessage("Bios successfully upgraded to: "+biosVersion.Spec.Version),
		); err != nil {
			log.V(1).Error(err, "failed to update the conditions status. reconcile again")
			return false, err
		}
		err = r.updateBiosVersionStatus(
			ctx,
			log,
			biosVersion,
			biosVersion.Status.State,
			upgradeCurrentTaskStatus,
			completedCondition,
			acc,
		)
		return false, err
	}

	// in-progress task states
	if err := acc.Update(
		completedCondition,
		conditionutils.UpdateStatus(corev1.ConditionFalse),
		conditionutils.UpdateReason(string(taskCurrentStatus.TaskState)),
		conditionutils.UpdateMessage(
			fmt.Sprintf("Bios upgrade in state: %v: PercentageCompleted %v",
				taskCurrentStatus.TaskState,
				taskCurrentStatus.PercentComplete),
		),
	); err != nil {
		return false, fmt.Errorf("failed to update the conditions status: %w", err)
	}
	ok, err := checkpoint.Transitioned(acc, *completedCondition)
	if !ok && err == nil {
		log.V(1).Info("BIOS upgrade task has not progressed. retrying....")
		// the job has stalled or slow, we need to requeue with exponential backoff
		return true, nil
	}
	// todo: Fail the state after certain timeout
	err = r.updateBiosVersionStatus(
		ctx,
		log,
		biosVersion,
		biosVersion.Status.State,
		upgradeCurrentTaskStatus,
		completedCondition,
		acc,
	)
	return false, err
}

func (r *BIOSVersionReconciler) issueBiosUpgrade(
	ctx context.Context,
	log logr.Logger,
	bmcClient bmc.BMC,
	biosVersion *metalv1alpha1.BIOSVersion,
	server *metalv1alpha1.Server,
	issuedCondition *metav1.Condition,
	acc *conditionutils.Accessor,
) error {
	password, username, err := r.getReferredSecret(ctx, log, biosVersion.Spec.Image.SecretRef)
	if err != nil {
		log.V(1).Error(err, "failed to get secret ref for", "secretRef", biosVersion.Spec.Image.SecretRef.Name)
		return err
	}
	var forceUpdate bool
	if biosVersion.Spec.UpdatePolicy != nil {
		switch *biosVersion.Spec.UpdatePolicy {
		case metalv1alpha1.UpdatePolicyForce:
			forceUpdate = true
		default:
			forceUpdate = false
		}
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
		log.V(1).Error(err, "failed to issue bios upgrade", "requested bios version", biosVersion.Spec.Version, "server", server.Name)
		if errCond := acc.Update(
			issuedCondition,
			conditionutils.UpdateStatus(corev1.ConditionFalse),
			conditionutils.UpdateReason("IssueBIOSUpgradeFailed"),
			conditionutils.UpdateMessage("Fatal error occurred. Upgrade might still go through on server."),
		); errCond != nil {
			log.V(1).Error(errCond, "failed to update the conditions status")
			err := r.updateBiosVersionStatus(
				ctx,
				log,
				biosVersion,
				metalv1alpha1.BIOSVersionStateFailed,
				upgradeCurrentTaskStatus,
				issuedCondition,
				acc,
			)
			return errors.Join(errCond, err)
		}
		err := r.updateBiosVersionStatus(
			ctx,
			log,
			biosVersion,
			metalv1alpha1.BIOSVersionStateFailed,
			upgradeCurrentTaskStatus,
			issuedCondition,
			acc,
		)
		return err
	}
	if err != nil {
		log.V(1).Error(err, "failed to issue bios upgrade", "bios version", biosVersion.Spec.Version, "server", server.Name)
		return err
	}
	if errCond := acc.Update(
		issuedCondition,
		conditionutils.UpdateStatus(corev1.ConditionTrue),
		conditionutils.UpdateReason("UpgradeIssued"),
		conditionutils.UpdateMessage(fmt.Sprintf("Task to upgrade has been created %v", taskMonitor)),
	); errCond != nil {
		log.V(1).Error(errCond, "failed to update the conditions status... retrying")
		if errCond := acc.Update(
			issuedCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason("UpgradeIssued"),
			conditionutils.UpdateMessage(fmt.Sprintf("Task to upgrade has been created %v", taskMonitor)),
		); errCond != nil {
			log.V(1).Error(errCond, "failed to update the conditions status, failing the upgrade process! BIOS might still be updated to new version")
			err := r.updateBiosVersionStatus(
				ctx,
				log,
				biosVersion,
				metalv1alpha1.BIOSVersionStateFailed,
				upgradeCurrentTaskStatus,
				issuedCondition,
				acc,
			)
			return errors.Join(errCond, err)
		}
	}

	err = r.updateBiosVersionStatus(
		ctx,
		log,
		biosVersion,
		biosVersion.Status.State,
		upgradeCurrentTaskStatus,
		issuedCondition,
		acc,
	)
	return err
}

func (r *BIOSVersionReconciler) enqueueBiosVersionByRefs(
	ctx context.Context,
	obj client.Object,
) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)
	host := obj.(*metalv1alpha1.Server)

	// dont requeue if host in wrong state
	if host.Status.State == metalv1alpha1.ServerStateDiscovery ||
		host.Status.State == metalv1alpha1.ServerStateError ||
		host.Status.State == metalv1alpha1.ServerStateInitial {
		return nil
	}

	// dont requeue if host does not have Maintenance
	if host.Spec.ServerMaintenanceRef == nil {
		return nil
	}

	BIOSVersionList := &metalv1alpha1.BIOSVersionList{}
	if err := r.List(ctx, BIOSVersionList); err != nil {
		log.Error(err, "failed to list biosVersionList")
		return nil
	}

	for _, biosVersion := range BIOSVersionList.Items {
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

// SetupWithManager sets up the controller with the Manager.
func (r *BIOSVersionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BIOSVersion{}).
		Owns(&metalv1alpha1.ServerMaintenance{}).
		Watches(&metalv1alpha1.Server{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBiosVersionByRefs)).
		Complete(r)
}

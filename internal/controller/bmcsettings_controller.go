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

	"github.com/ironcore-dev/controller-utils/clientutils"
	"github.com/ironcore-dev/controller-utils/conditionutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	"github.com/stmcginnis/gofish/schemas"
)

// BMCSettingsReconciler reconciles a BMCSettings object
type BMCSettingsReconciler struct {
	client.Client
	ManagerNamespace string
	ResyncInterval   time.Duration
	Insecure         bool
	Scheme           *runtime.Scheme
	BMCOptions       bmc.Options
	Conditions       *conditionutils.Accessor
}

const (
	BMCSettingFinalizer               = "metal.ironcore.dev/bmcsettings"
	BMCResetPostSettingApplyCondition = "ResetPostSettingApply"
	BMCPoweredOffCondition            = "PoweredOff"
	BMCPoweredOffReason               = "PoweredOff"
	BMCVersionUpdatePendingCondition  = "VersionUpdatePending"
	BMCVersionUpgradePendingReason    = "VersionUpgradePending"
	BMCVersionMatchingReason          = "VersionMatching"

	BMCSettingsChangesIssuedCondition      = "ChangesIssued"
	BMCSettingsChangesIssuedReason         = "ChangesIssued"
	BMCSettingsChangesVerifiedCondition    = "ChangesVerified"
	BMCSettingsChangesVerifiedReason       = "ChangesVerified"
	BMCSettingsChangesNotYetVerifiedReason = "ChangesNotYetVerified"
)

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcsettings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcsettings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcsettings/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="batch",resources=jobs,verbs=get;list;watch;create;update;patch;delete

func (r *BMCSettingsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	settings := &metalv1alpha1.BMCSettings{}
	if err := r.Get(ctx, req.NamespacedName, settings); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Reconciling BMCSettings")

	return r.reconcileExists(ctx, settings)
}

// Determine whether reconciliation is required. It's not required if:
// - object is being deleted
// - object does not contain reference to a BMC
// - the referred BMC references another BMCSettings object with a lower version
func (r *BMCSettingsReconciler) reconcileExists(ctx context.Context, settings *metalv1alpha1.BMCSettings) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	if r.shouldDelete(ctx, settings) {
		log.V(1).Info("Object is being deleted")
		return r.delete(ctx, settings)
	}

	return r.reconcile(ctx, settings)
}

func (r *BMCSettingsReconciler) shouldDelete(ctx context.Context, bmcSetting *metalv1alpha1.BMCSettings) bool {
	log := ctrl.LoggerFrom(ctx)
	if bmcSetting.DeletionTimestamp.IsZero() {
		return false
	}

	if controllerutil.ContainsFinalizer(bmcSetting, BMCSettingFinalizer) &&
		bmcSetting.Status.State == metalv1alpha1.BMCSettingsStateInProgress {
		if _, err := r.getBMC(ctx, bmcSetting); apierrors.IsNotFound(err) {
			log.V(1).Info("BMC not found, proceeding with deletion", "BMC", bmcSetting.Spec.BMCRef.Name)
			return true
		}
		log.V(1).Info("postponing delete as Settings update is in progress")
		return false
	}
	return true
}

func (r *BMCSettingsReconciler) delete(ctx context.Context, settings *metalv1alpha1.BMCSettings) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	if err := r.cleanupReferences(ctx, settings); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to cleanup references: %w", err)
	}
	log.V(1).Info("Ensured references were cleaned up")

	if err := r.cleanupServerMaintenanceReferences(ctx, settings); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to cleanup server maintenance references: %w", err)
	}
	log.V(1).Info("Ensured server maintenance references were cleaned up")

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, settings, BMCSettingFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	log.V(1).Info("Deleted BMCSettings")
	return ctrl.Result{}, nil
}

func (r *BMCSettingsReconciler) cleanupServerMaintenanceReferences(ctx context.Context, settings *metalv1alpha1.BMCSettings) error {
	log := ctrl.LoggerFrom(ctx)
	if settings.Spec.ServerMaintenanceRefs == nil {
		return nil
	}
	serverMaintenances, errs := r.getReferredServerMaintenances(ctx, settings.Spec.ServerMaintenanceRefs)

	var finalErr []error
	var missingServerMaintenanceRef []error

	if len(errs) > 0 {
		for _, err := range errs {
			if apierrors.IsNotFound(err) {
				missingServerMaintenanceRef = append(missingServerMaintenanceRef, err)
			} else {
				finalErr = append(finalErr, err)
			}
		}
	}

	if len(missingServerMaintenanceRef) != len(settings.Spec.ServerMaintenanceRefs) {
		for _, serverMaintenance := range serverMaintenances {
			if serverMaintenance.DeletionTimestamp.IsZero() && metav1.IsControlledBy(serverMaintenance, settings) {
				log.V(1).Info("Deleting server maintenance", "ServerMaintenance Name", serverMaintenance.Name, "State", serverMaintenance.Status.State)
				if err := r.Delete(ctx, serverMaintenance); err != nil {
					log.V(1).Info("Failed to delete server maintenance", "ServerMaintenance Name", serverMaintenance.Name)
					finalErr = append(finalErr, err)
				}
			} else {
				log.V(1).Info(
					"Server maintenance not deleted",
					"ServerMaintenance Name", serverMaintenance.Name,
					"State", serverMaintenance.Status.State,
					"Owner", serverMaintenance.OwnerReferences,
				)
			}
		}
	}

	if len(finalErr) == 0 {
		if err := r.patchMaintenanceRequestRefOnBMCSettings(ctx, settings, nil); err != nil {
			return fmt.Errorf("failed to clean up serverMaintenance ref in settings status: %w", err)
		}
		log.V(1).Info("ServerMaintenance refs cleaned up")
	}
	return errors.Join(finalErr...)
}

func (r *BMCSettingsReconciler) cleanupReferences(ctx context.Context, settings *metalv1alpha1.BMCSettings) error {
	if settings.Spec.BMCRef == nil {
		return nil
	}

	bmcObj, err := r.getBMC(ctx, settings)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	if bmcObj.Spec.BMCSettingRef != nil && bmcObj.Spec.BMCSettingRef.Name == settings.Name {
		return r.patchBMCSettingsRefOnBMC(ctx, bmcObj, nil)
	}
	return nil
}

func (r *BMCSettingsReconciler) reconcile(ctx context.Context, settings *metalv1alpha1.BMCSettings) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	if shouldIgnoreReconciliation(settings) {
		log.V(1).Info("Skipped BMCSettings reconciliation")
		return ctrl.Result{}, nil
	}

	base := settings.DeepCopy()
	changed := false
	for i := range settings.Spec.ServerMaintenanceRefs {
		changed = clearDeprecatedObjectRefFields(settings.Spec.ServerMaintenanceRefs[i].ServerMaintenanceRef) || changed
	}
	if changed {
		if err := r.Patch(ctx, settings, client.MergeFrom(base)); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to clear deprecated ObjectReference fields on BMCSettings: %w", err)
		}
		return ctrl.Result{}, nil
	}

	if settings.Spec.BMCRef == nil {
		log.V(1).Info("Object does not refer to BMC object")
		return ctrl.Result{}, nil
	}

	bmcObj, err := r.getBMC(ctx, settings)
	if err != nil {
		log.V(1).Info("Failed to fetch referred BMC object")
		return ctrl.Result{}, err
	}
	if bmcObj.Spec.BMCSettingRef == nil {
		if err := r.patchBMCSettingsRefOnBMC(ctx, bmcObj, &corev1.LocalObjectReference{Name: settings.Name}); err != nil {
			return ctrl.Result{}, err
		}
	} else if bmcObj.Spec.BMCSettingRef.Name != settings.Name {
		referredBMCSettings, err := r.getReferredBMCSettings(ctx, bmcObj.Spec.BMCSettingRef)
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.V(1).Info("Referred BMC contains reference to non-existing BMCSettings, updating reference")
				if err := r.patchBMCSettingsRefOnBMC(ctx, bmcObj, &corev1.LocalObjectReference{Name: settings.Name}); err != nil {
					return ctrl.Result{}, err
				}
				// Requeue since updating the BMC object does not trigger reconciliation here
				return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
			}
			log.V(1).Info("Referred BMC contains reference to different BMCSettings, unable to fetch the referenced BMCSettings")
			return ctrl.Result{}, err
		}
		// TODO: Handle version checks correctly
		if referredBMCSettings.Spec.Version < settings.Spec.Version {
			log.V(1).Info("Updating BMCSettings reference to the latest BMC version")
			if err := r.patchBMCSettingsRefOnBMC(ctx, bmcObj, &corev1.LocalObjectReference{Name: settings.Name}); err != nil {
				return ctrl.Result{}, err
			}
			// Requeue to reconcile with the updated BMC reference
			return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
		}
		// This BMCSettings does not own the BMC — stop reconciliation
		log.V(1).Info("BMC is owned by a newer or equal version BMCSettings, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, settings, BMCSettingFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	return r.ensureBMCSettingsMaintenanceStateTransition(ctx, settings, bmcObj)
}

func (r *BMCSettingsReconciler) ensureBMCSettingsMaintenanceStateTransition(ctx context.Context, settings *metalv1alpha1.BMCSettings, bmcObj *metalv1alpha1.BMC) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, r.Client, bmcObj, r.Insecure, r.BMCOptions)
	if err != nil {
		if errors.As(err, &bmcutils.BMCUnAvailableError{}) {
			log.V(1).Info("BMC is not available, skipping", "BMC", bmcObj.Name, "error", err)
			return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to create BMC client: %w", err)
	}
	defer bmcClient.Logout()
	switch settings.Status.State {
	case "", metalv1alpha1.BMCSettingsStatePending:
		var state = metalv1alpha1.BMCSettingsStateInProgress
		versionCheckCondition, err := GetCondition(r.Conditions, settings.Status.Conditions, BMCVersionUpdatePendingCondition)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get Condition for pending BMCVersion update state %v", err)
		}
		if bmcObj.Status.FirmwareVersion != settings.Spec.Version {
			log.V(1).Info("Pending BMC version upgrade", "currentVersion", bmcObj.Status.FirmwareVersion, "requiredVersion", settings.Spec.Version)
			if err := r.Conditions.Update(
				versionCheckCondition,
				conditionutils.UpdateStatus(corev1.ConditionTrue),
				conditionutils.UpdateReason(BMCVersionUpgradePendingReason),
				conditionutils.UpdateMessage(fmt.Sprintf("Waiting to update BMCVersion: %v, current BMCVersion: %v", settings.Spec.Version, bmcObj.Status.FirmwareVersion)),
			); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update Pending BMCVersion update condition: %w", err)
			}
			state = metalv1alpha1.BMCSettingsStatePending
		} else if versionCheckCondition.Status == metav1.ConditionTrue {
			if err := r.Conditions.Update(
				versionCheckCondition,
				conditionutils.UpdateStatus(corev1.ConditionFalse),
				conditionutils.UpdateReason(BMCVersionMatchingReason),
				conditionutils.UpdateMessage(fmt.Sprintf("BMCVersion matches: %v", settings.Spec.Version)),
			); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update Pending BMCVersion update condition: %w", err)
			}
		} else {
			versionCheckCondition = nil
		}
		return ctrl.Result{}, r.updateBMCSettingsStatus(ctx, settings, state, versionCheckCondition)
	case metalv1alpha1.BMCSettingsStateInProgress:
		return r.handleSettingInProgressState(ctx, settings, bmcObj, bmcClient)
	case metalv1alpha1.BMCSettingsStateApplied:
		return ctrl.Result{}, r.handleSettingAppliedState(ctx, settings, bmcObj, bmcClient)
	case metalv1alpha1.BMCSettingsStateFailed:
		return ctrl.Result{}, r.handleFailedState(ctx, settings, bmcObj)
	}
	log.V(1).Info("Unknown State found", "BMCSettings state", settings.Status.State)
	return ctrl.Result{}, nil
}

func (r *BMCSettingsReconciler) handleSettingInProgressState(ctx context.Context, settings *metalv1alpha1.BMCSettings, bmcObj *metalv1alpha1.BMC, bmcClient bmc.BMC) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	settingsDiff, err := r.getBMCSettingsDifference(ctx, settings, bmcObj, bmcClient)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get BMC settings: %w", err)
	}
	if len(settingsDiff) == 0 {
		return ctrl.Result{}, r.updateBMCSettingsStatus(ctx, settings, metalv1alpha1.BMCSettingsStateApplied, nil)
	}

	if req, err := r.requestMaintenanceOnServers(ctx, settings, bmcObj, bmcClient); err != nil || req {
		return ctrl.Result{}, err
	}

	condition, err := GetCondition(r.Conditions, settings.Status.Conditions, ServerMaintenanceConditionWaiting)
	if err != nil {
		return ctrl.Result{}, err
	}

	granted, err := r.checkIfMaintenanceGranted(ctx, settings, bmcObj, bmcClient)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to check if maintenance is granted: %w", err)
	}
	if !granted {
		log.V(1).Info("Waiting for maintenance to be granted before continuing with updating settings")
		if condition.Status != metav1.ConditionTrue {
			if err := r.Conditions.Update(
				condition,
				conditionutils.UpdateStatus(corev1.ConditionTrue),
				conditionutils.UpdateReason(ServerMaintenanceReasonWaiting),
				conditionutils.UpdateMessage(fmt.Sprintf("Waiting for approval of %v", settings.Spec.ServerMaintenanceRefs)),
			); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update creating ServerMaintenance condition: %w", err)
			}
			if err := r.updateBMCSettingsStatus(ctx, settings, settings.Status.State, condition); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to patch BMCSettings ServerMaintenance waiting conditions: %w", err)
			}
		}
		return ctrl.Result{}, nil
	}

	// Once in maintenance, clear the waiting condition if present
	if condition.Reason != ServerMaintenanceReasonApproved {
		if err := r.Conditions.Update(
			condition,
			conditionutils.UpdateStatus(corev1.ConditionFalse),
			conditionutils.UpdateReason(ServerMaintenanceReasonApproved),
			conditionutils.UpdateMessage("Server is now in Maintenance mode"),
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update creating ServerMaintenance condition: %w", err)
		}
		if err := r.updateBMCSettingsStatus(ctx, settings, settings.Status.State, condition); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to patch BMCSettings ServerMaintenance waiting conditions: %w", err)
		}
		return ctrl.Result{}, nil
	}

	// Reset the BMC to ensure it's in a stable state before proceeding
	if ok, err := r.handleBMCReset(ctx, settings, bmcObj, BMCConditionReset); !ok || err != nil {
		return ctrl.Result{}, err
	}
	return r.updateSettingsAndVerify(ctx, settings, bmcObj, settingsDiff, bmcClient)
}

func (r *BMCSettingsReconciler) updateSettingsAndVerify(ctx context.Context, settings *metalv1alpha1.BMCSettings, bmcObj *metalv1alpha1.BMC, settingsDiff schemas.SettingsAttributes, bmcClient bmc.BMC) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	resetBMC, err := GetCondition(r.Conditions, settings.Status.Conditions, BMCResetPostSettingApplyCondition)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get condition for reset of BMC of server %v", err)
	}

	if resetBMC.Reason != BMCReasonReset {
		switch bmcObj.Status.PowerState {
		case metalv1alpha1.OnPowerState:
			fallthrough
		case metalv1alpha1.UnknownPowerState:
			BMCPoweredOffCondition, err := GetCondition(r.Conditions, settings.Status.Conditions, BMCPoweredOffCondition)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to get Condition for powered off BMC state %v", err)
			}
			if BMCPoweredOffCondition.Status == metav1.ConditionTrue {
				if err := r.Conditions.Update(
					BMCPoweredOffCondition,
					conditionutils.UpdateStatus(corev1.ConditionFalse),
					conditionutils.UpdateReason("BMCPoweredOn"),
					conditionutils.UpdateMessage(fmt.Sprintf("BMC in Powered On, Power State: %v", bmcObj.Status.PowerState)),
				); err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to update Pending BMCVersion update condition: %w", err)
				}
				return ctrl.Result{}, r.updateBMCSettingsStatus(ctx, settings, settings.Status.State, BMCPoweredOffCondition)
			}
		default:
			log.V(1).Info("BMC is not powered on, cannot proceed", "PowerState", bmcObj.Status.PowerState)
			BMCPoweredOffCondition, err := GetCondition(r.Conditions, settings.Status.Conditions, BMCPoweredOffCondition)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to get Condition for powered off BMC state %v", err)
			}
			if err := r.Conditions.Update(
				BMCPoweredOffCondition,
				conditionutils.UpdateStatus(corev1.ConditionTrue),
				conditionutils.UpdateReason(BMCPoweredOffReason),
				conditionutils.UpdateMessage(fmt.Sprintf("BMC is not powered on, Power State: %v", bmcObj.Status.PowerState)),
			); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update Pending BMCVersion update condition: %w", err)
			}
			return ctrl.Result{}, r.updateBMCSettingsStatus(ctx, settings, metalv1alpha1.BMCSettingsStateFailed, BMCPoweredOffCondition)
		}

		pendingAttr, err := bmcClient.GetBMCPendingAttributeValues(ctx, bmcObj.Spec.BMCUUID)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to check pending BMC settings: %w", err)
		}

		if len(pendingAttr) == 0 {
			resetBMCReq, err := bmcClient.CheckBMCAttributes(ctx, bmcObj.Spec.BMCUUID, settingsDiff)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to check BMC settings provided: %w", err)
			}

			err = bmcClient.SetBMCAttributesImmediately(ctx, bmcObj.Spec.BMCUUID, settingsDiff)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to set BMC settings: %w", err)
			}
			log.V(1).Info("BMC settings issued successfully", "Settings", settingsDiff)

			BMCSettingsAppliedCondition, err := GetCondition(r.Conditions, settings.Status.Conditions, BMCSettingsChangesIssuedCondition)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to get Condition for Successful issue of BMC Settings %v", err)
			}
			if err := r.Conditions.Update(
				BMCSettingsAppliedCondition,
				conditionutils.UpdateStatus(corev1.ConditionTrue),
				conditionutils.UpdateReason(BMCSettingsChangesIssuedReason),
				conditionutils.UpdateMessage("BMC settings have been issued on the server's BMC"),
			); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update BMCSettings Applied condition: %w", err)
			}
			if err := r.updateBMCSettingsStatus(ctx, settings, settings.Status.State, BMCSettingsAppliedCondition); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update Condition for Successful issue of BMC Settings %v", err)
			}
			if resetBMCReq {
				if ok, err := r.handleBMCReset(ctx, settings, bmcObj, BMCResetPostSettingApplyCondition); !ok || err != nil {
					return ctrl.Result{}, err
				}
			}
		}
	} else {
		log.V(1).Info("Waiting for BMC reset post applying BMC settings")
		if ok, err := r.handleBMCReset(ctx, settings, bmcObj, BMCResetPostSettingApplyCondition); !ok || err != nil {
			return ctrl.Result{}, err
		}
	}

	settingsDiff, err = r.getBMCSettingsDifference(ctx, settings, bmcObj, bmcClient)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get BMC settings: %w", err)
	}
	BMCSettingsVerifiedCondition, err := GetCondition(r.Conditions, settings.Status.Conditions, BMCSettingsChangesVerifiedCondition)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get Condition for verification BMC settings changes %v", err)
	}
	if len(settingsDiff) == 0 {
		if err := r.Conditions.Update(
			BMCSettingsVerifiedCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(BMCSettingsChangesVerifiedReason),
			conditionutils.UpdateMessage("BMC settings changes have been verified on the server's BMC"),
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update BMCSettings verified condition: %w", err)
		}
		return ctrl.Result{}, r.updateBMCSettingsStatus(ctx, settings, metalv1alpha1.BMCSettingsStateApplied, BMCSettingsVerifiedCondition)
	}

	if BMCSettingsVerifiedCondition.Status == metav1.ConditionFalse && BMCSettingsVerifiedCondition.Reason != BMCSettingsChangesNotYetVerifiedReason {
		if err := r.Conditions.Update(
			BMCSettingsVerifiedCondition,
			conditionutils.UpdateStatus(corev1.ConditionFalse),
			conditionutils.UpdateReason(BMCSettingsChangesNotYetVerifiedReason),
			conditionutils.UpdateMessage("BMC Settings changes are not yet verified on the server's BMC"),
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update BMCSettings verified condition: %w", err)
		}
		return ctrl.Result{}, r.updateBMCSettingsStatus(ctx, settings, settings.Status.State, BMCSettingsVerifiedCondition)
	}

	return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
}

func (r *BMCSettingsReconciler) handleSettingAppliedState(ctx context.Context, settings *metalv1alpha1.BMCSettings, bmcObj *metalv1alpha1.BMC, bmcClient bmc.BMC) error {
	log := ctrl.LoggerFrom(ctx)
	// Clean up maintenance CRD and references
	if err := r.cleanupServerMaintenanceReferences(ctx, settings); err != nil {
		return err
	}

	settingsDiff, err := r.getBMCSettingsDifference(ctx, settings, bmcObj, bmcClient)
	if err != nil {
		return fmt.Errorf("failed to fetch and check BMCSettings: %w", err)
	}
	if len(settingsDiff) > 0 {
		return r.updateBMCSettingsStatus(ctx, settings, "", nil)
	}

	log.V(1).Info("Done with BMC setting update", "BMCSetting", settings.Name, "BMC", bmcObj.Name)
	return nil
}

func (r *BMCSettingsReconciler) handleBMCReset(ctx context.Context, settings *metalv1alpha1.BMCSettings, bmcObj *metalv1alpha1.BMC, conditionType string) (bool, error) {
	log := ctrl.LoggerFrom(ctx)
	resetBMC, err := GetCondition(r.Conditions, settings.Status.Conditions, conditionType)
	if err != nil {
		return false, fmt.Errorf("failed to get condition for reset of BMC of server %v", err)
	}

	if resetBMC.Status != metav1.ConditionTrue {
		annotations := bmcObj.GetAnnotations()
		if resetBMC.Reason != BMCReasonReset {
			if annotations != nil {
				if op, ok := annotations[metalv1alpha1.OperationAnnotation]; ok {
					if op == metalv1alpha1.GracefulRestartBMC {
						log.V(1).Info("Waiting for BMC reset as annotation on BMC object is set")
						if err := r.Conditions.Update(
							resetBMC,
							conditionutils.UpdateStatus(corev1.ConditionFalse),
							conditionutils.UpdateReason(BMCReasonReset),
							conditionutils.UpdateMessage("Issued BMC reset to stabilize BMC of the server"),
						); err != nil {
							return false, fmt.Errorf("failed to update reset BMC condition: %w", err)
						}
						return false, r.updateBMCSettingsStatus(ctx, settings, settings.Status.State, resetBMC)
					} else {
						return false, fmt.Errorf("unknown annotation on BMC object for operation annotation %v", op)
					}
				}
			}
			log.V(1).Info("Setting annotation on BMC resource to trigger with BMC reset")

			bmcObjBase := bmcObj.DeepCopy()
			if annotations == nil {
				annotations = map[string]string{}
			}
			annotations[metalv1alpha1.OperationAnnotation] = metalv1alpha1.GracefulRestartBMC
			bmcObj.SetAnnotations(annotations)
			if err := r.Patch(ctx, bmcObj, client.MergeFrom(bmcObjBase)); err != nil {
				return false, err
			}

			if err := r.Conditions.Update(
				resetBMC,
				conditionutils.UpdateStatus(corev1.ConditionFalse),
				conditionutils.UpdateReason(BMCReasonReset),
				conditionutils.UpdateMessage("Issued BMC reset to stabilize BMC of the server"),
			); err != nil {
				return false, fmt.Errorf("failed to update reset BMC condition: %w", err)
			}
			return false, r.updateBMCSettingsStatus(ctx, settings, settings.Status.State, resetBMC)
		}

		// Wait until the BMC resource annotation is removed
		if annotations != nil {
			if op, ok := annotations[metalv1alpha1.OperationAnnotation]; ok {
				if op == metalv1alpha1.GracefulRestartBMC {
					log.V(1).Info("Waiting for BMC reset as annotation on BMC object is set")
					return false, nil
				}
			}
		}

		if err := r.Conditions.Update(
			resetBMC,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(BMCReasonReset),
			conditionutils.UpdateMessage("BMC reset of the server is completed"),
		); err != nil {
			return false, fmt.Errorf("failed to update power on server condition: %w", err)
		}
		return false, r.updateBMCSettingsStatus(ctx, settings, settings.Status.State, resetBMC)
	}
	return true, nil
}

func (r *BMCSettingsReconciler) handleFailedState(ctx context.Context, settings *metalv1alpha1.BMCSettings, bmcObj *metalv1alpha1.BMC) error {
	log := ctrl.LoggerFrom(ctx)
	if shouldRetryReconciliation(settings) {
		log.V(1).Info("Retrying BMCSettings reconciliation")
		settingsBase := settings.DeepCopy()
		settings.Status.State = metalv1alpha1.BMCSettingsStatePending
		if err := r.Status().Patch(ctx, settings, client.MergeFrom(settingsBase)); err != nil {
			return fmt.Errorf("failed to patch BMCSettings status for retrying: %w", err)
		}
		return nil
	}
	// TODO: Revisit this logic to either create maintenance if not present or put server in Error state on failed BMC settings maintenance
	log.V(1).Info("Failed to update BMCSettings", "BMC", bmcObj.Name)
	return nil
}

func (r *BMCSettingsReconciler) getBMCSettingsDifference(ctx context.Context, settings *metalv1alpha1.BMCSettings, bmcObj *metalv1alpha1.BMC, bmcClient bmc.BMC) (diff schemas.SettingsAttributes, err error) {
	log := ctrl.LoggerFrom(ctx)
	currentSettings, err := bmcClient.GetBMCAttributeValues(ctx, bmcObj.Spec.BMCUUID, settings.Spec.SettingsMap)
	if err != nil {
		return diff, fmt.Errorf("failed to get BMC settings: %w", err)
	}

	log.V(1).Info("Current BMC settings fetched", "Settings", currentSettings)

	return computeSettingsDiff(settings.Spec.SettingsMap, currentSettings)
}

func (r *BMCSettingsReconciler) checkIfMaintenanceGranted(ctx context.Context, settings *metalv1alpha1.BMCSettings, bmcObj *metalv1alpha1.BMC, bmcClient bmc.BMC) (bool, error) {
	log := ctrl.LoggerFrom(ctx)
	if settings.Spec.ServerMaintenanceRefs == nil {
		return false, nil
	}

	servers, err := r.getServers(ctx, bmcObj, bmcClient)
	if err != nil {
		return false, fmt.Errorf("failed to get referred servers to determine maintenance state: %w", err)
	}

	if len(settings.Spec.ServerMaintenanceRefs) != len(servers) {
		log.V(1).Info("Not all servers have Maintenance", "ServerMaintenanceRefs", settings.Spec.ServerMaintenanceRefs, "Servers", servers)
		return false, nil
	}

	notInMaintenanceState := make([]string, 0, len(servers))
	for _, server := range servers {
		if server.Status.State == metalv1alpha1.ServerStateMaintenance {
			if server.Spec.ServerMaintenanceRef == nil {
				log.V(1).Info("Server is in maintenance but has no maintenance ref", "Server", server.Name)
				notInMaintenanceState = append(notInMaintenanceState, server.Name)
				continue
			}
			if serverMaintenanceRef := r.getServerMaintenanceRefForServer(settings.Spec.ServerMaintenanceRefs, server.Spec.ServerMaintenanceRef.Name, server.Spec.ServerMaintenanceRef.Namespace); serverMaintenanceRef == nil {
				log.V(1).Info("Server is already in maintenance for other tasks",
					"Server", server.Name,
					"ServerMaintenanceRef", server.Spec.ServerMaintenanceRef,
				)
				notInMaintenanceState = append(notInMaintenanceState, server.Name)
			}
		} else {
			log.V(1).Info("Server not yet in maintenance", "Server", server.Name, "State", server.Status.State, "MaintenanceRef", server.Spec.ServerMaintenanceRef)
			notInMaintenanceState = append(notInMaintenanceState, server.Name)
		}
	}

	if len(notInMaintenanceState) > 0 {
		log.V(1).Info("Some servers not yet in maintenance",
			"Required maintenances on servers", settings.Spec.ServerMaintenanceRefs,
			"Servers not in maintenance", notInMaintenanceState)
		return false, nil
	}

	return true, nil
}

func (r *BMCSettingsReconciler) requestMaintenanceOnServers(ctx context.Context, settings *metalv1alpha1.BMCSettings, bmcObj *metalv1alpha1.BMC, bmcClient bmc.BMC) (bool, error) {
	log := ctrl.LoggerFrom(ctx)

	servers, err := r.getServers(ctx, bmcObj, bmcClient)
	if err != nil {
		return false, fmt.Errorf("failed to get referred servers to request maintenance: %w", err)
	}

	// If ServerMaintenance refs are already given, no further action required
	if settings.Spec.ServerMaintenanceRefs != nil && len(settings.Spec.ServerMaintenanceRefs) == len(servers) {
		if _, errs := r.getReferredServerMaintenances(ctx, settings.Spec.ServerMaintenanceRefs); len(errs) > 0 {
			missingMaintenancesNames := map[string]struct{}{}
			for _, e := range errs {
				if apierrors.IsNotFound(e) {
					missingMaintenancesNames[e.(*MultiErrorTracker).Identifier] = struct{}{}
				}
			}

			if len(missingMaintenancesNames) > 0 {
				ServerMaintenanceRefs := make([]metalv1alpha1.ServerMaintenanceRefItem, 0, len(settings.Spec.ServerMaintenanceRefs))
				for _, maintenance := range settings.Spec.ServerMaintenanceRefs {
					if _, ok := missingMaintenancesNames[maintenance.ServerMaintenanceRef.Name]; ok {
						log.V(1).Info("Referenced ServerMaintenance is missing", "ServerMaintenance", maintenance.ServerMaintenanceRef.Name)
						continue
					}
					ServerMaintenanceRefs = append(
						ServerMaintenanceRefs,
						metalv1alpha1.ServerMaintenanceRefItem{
							ServerMaintenanceRef: &metalv1alpha1.ObjectReference{
								Namespace: maintenance.ServerMaintenanceRef.Namespace,
								Name:      maintenance.ServerMaintenanceRef.Name,
							}})
				}

				if len(ServerMaintenanceRefs) == 0 {
					log.V(1).Info("Referenced ServerMaintenances no longer exists, clearing ref to allow re-creation")
					if err := r.patchMaintenanceRequestRefOnBMCSettings(ctx, settings, nil); err != nil {
						return false, fmt.Errorf("failed to clear stale ServerMaintenance ref: %w", err)
					}
					return true, nil // requeue to re-create
				} else {
					log.V(1).Info("Some referenced ServerMaintenances are still present", "Missing ServerMaintenances", missingMaintenancesNames)
					if err := r.patchMaintenanceRequestRefOnBMCSettings(ctx, settings, ServerMaintenanceRefs); err != nil {
						return false, fmt.Errorf("failed to clear stale ServerMaintenances ref: %w", err)
					}
					return true, nil // requeue to update with remaining refs
				}
			} else {
				return false, fmt.Errorf("failed to verify ServerMaintenance existence: %w", errors.Join(errs...))
			}
		}
		condition, err := GetCondition(r.Conditions, settings.Status.Conditions, ServerMaintenanceConditionCreated)
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
			conditionutils.UpdateMessage(fmt.Sprintf("Created/present %v at %v", settings.Spec.ServerMaintenanceRefs, time.Now())),
		); err != nil {
			return false, fmt.Errorf("failed to update creating ServerMaintenance condition: %w", err)
		}
		if err := r.updateBMCSettingsStatus(ctx, settings, settings.Status.State, condition); err != nil {
			return false, fmt.Errorf("failed to patch BMCSettings conditions: %w", err)
		}
		return true, nil
	}

	// Create ServerMaintenance objects for servers that don't have one yet
	serverWithMaintenances := make(map[string]*metalv1alpha1.ServerMaintenance, len(servers))
	if settings.Spec.ServerMaintenanceRefs != nil {
		serverMaintenances, err := r.getReferredServerMaintenances(ctx, settings.Spec.ServerMaintenanceRefs)
		if err != nil {
			return false, errors.Join(err...)
		}
		for _, serverMaintenance := range serverMaintenances {
			serverWithMaintenances[serverMaintenance.Spec.ServerRef.Name] = serverMaintenance
		}
	}

	// Also fetch references owned by this resource in case we reconcile before refs are patched
	serverMaintenancesList := &metalv1alpha1.ServerMaintenanceList{}
	if err := clientutils.ListAndFilterControlledBy(ctx, r.Client, settings, serverMaintenancesList); err != nil {
		return false, err
	}
	for _, serverMaintenance := range serverMaintenancesList.Items {
		serverWithMaintenances[serverMaintenance.Spec.ServerRef.Name] = &serverMaintenance
	}

	var errs []error
	ServerMaintenanceRefs := make([]metalv1alpha1.ServerMaintenanceRefItem, 0, len(servers))
	for _, server := range servers {
		if maintenance, ok := serverWithMaintenances[server.Name]; ok {
			log.V(1).Info("ServerMaintenance already exists for server, skipping creating new one", "Server", server.Name, "ServerMaintenance", maintenance.Name)
			ServerMaintenanceRefs = append(
				ServerMaintenanceRefs,
				metalv1alpha1.ServerMaintenanceRefItem{
					ServerMaintenanceRef: &metalv1alpha1.ObjectReference{
						Namespace: maintenance.Namespace,
						Name:      maintenance.Name,
					}})
			continue
		}
		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    r.ManagerNamespace,
				GenerateName: "bmc-settings-",
			},
		}
		opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, serverMaintenance, func() error {
			serverMaintenance.Spec.Policy = settings.Spec.ServerMaintenancePolicy
			serverMaintenance.Spec.ServerPower = metalv1alpha1.PowerOn
			serverMaintenance.Spec.ServerRef = &corev1.LocalObjectReference{Name: server.Name}
			if serverMaintenance.Status.State != metalv1alpha1.ServerMaintenanceStateInMaintenance && serverMaintenance.Status.State != "" {
				serverMaintenance.Status.State = ""
			}
			return controllerutil.SetControllerReference(settings, serverMaintenance, r.Client.Scheme())
		})
		if err != nil {
			log.Error(err, "failed to create or patch serverMaintenance", "Server", server.Name)
			errs = append(errs, err)
			continue
		}
		log.V(1).Info("Created serverMaintenance", "ServerMaintenance", serverMaintenance.Name, "ServerMaintenance label", serverMaintenance.Labels, "Operation", opResult)

		ServerMaintenanceRefs = append(
			ServerMaintenanceRefs,
			metalv1alpha1.ServerMaintenanceRefItem{
				ServerMaintenanceRef: &metalv1alpha1.ObjectReference{
					Namespace: serverMaintenance.Namespace,
					Name:      serverMaintenance.Name,
				}})
	}

	if len(errs) > 0 {
		return false, errors.Join(errs...)
	}

	if err := r.patchMaintenanceRequestRefOnBMCSettings(ctx, settings, ServerMaintenanceRefs); err != nil {
		return false, fmt.Errorf("failed to patch serverMaintenance ref in settings status: %w", err)
	}

	log.V(1).Info("Patched serverMaintenanceMap on settings")

	return true, nil
}

func (r *BMCSettingsReconciler) getBMC(ctx context.Context, settings *metalv1alpha1.BMCSettings) (*metalv1alpha1.BMC, error) {
	if settings.Spec.BMCRef == nil {
		return nil, fmt.Errorf("bmc ref not provided")
	}

	key := client.ObjectKey{Name: settings.Spec.BMCRef.Name}
	bmcObj := &metalv1alpha1.BMC{}
	if err := r.Get(ctx, key, bmcObj); err != nil {
		return nil, err
	}

	return bmcObj, nil
}

func (r *BMCSettingsReconciler) getServers(ctx context.Context, bmcObj *metalv1alpha1.BMC, bmcClient bmc.BMC) ([]*metalv1alpha1.Server, error) {
	bmcServers, err := bmcClient.GetSystems(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get servers from BMC %s: %w", bmcObj.Name, err)
	}
	serversRefList := make([]*corev1.LocalObjectReference, len(bmcServers))
	for i := range bmcServers {
		serversRefList[i] = &corev1.LocalObjectReference{Name: bmcutils.GetServerNameFromBMCandIndex(i, bmcObj)}
	}
	servers, err := r.getReferredServers(ctx, serversRefList)
	if err != nil {
		return servers, fmt.Errorf("errors occurred during fetching servers from BMC: %v", err)
	}
	return servers, nil
}

func (r *BMCSettingsReconciler) getReferredServers(ctx context.Context, serverRefList []*corev1.LocalObjectReference) ([]*metalv1alpha1.Server, error) {
	log := ctrl.LoggerFrom(ctx)
	var errs []error
	servers := make([]*metalv1alpha1.Server, 0, len(serverRefList))
	for _, serverRef := range serverRefList {
		key := client.ObjectKey{Name: serverRef.Name}
		server := &metalv1alpha1.Server{}
		if err := r.Get(ctx, key, server); err != nil {
			log.Error(err, "Failed to get referred server", "reference", serverRef.Name)
			errs = append(errs, err)
			continue
		}
		servers = append(servers, server)
	}

	return servers, errors.Join(errs...)
}

func (r *BMCSettingsReconciler) getReferredServerMaintenances(ctx context.Context, ServerMaintenanceRefs []metalv1alpha1.ServerMaintenanceRefItem) ([]*metalv1alpha1.ServerMaintenance, []error) {
	log := ctrl.LoggerFrom(ctx)
	serverMaintenances := make([]*metalv1alpha1.ServerMaintenance, 0, len(ServerMaintenanceRefs))
	var errs []error
	for _, serverMaintenanceRef := range ServerMaintenanceRefs {
		key := client.ObjectKey{Name: serverMaintenanceRef.ServerMaintenanceRef.Name, Namespace: r.ManagerNamespace}
		serverMaintenance := &metalv1alpha1.ServerMaintenance{}
		if err := r.Get(ctx, key, serverMaintenance); err != nil {
			log.Error(err, "Failed to get referred ServerMaintenance", "ServerMaintenance", serverMaintenanceRef.ServerMaintenanceRef.Name)
			errs = append(errs, &MultiErrorTracker{
				Err:        err,
				Identifier: serverMaintenanceRef.ServerMaintenanceRef.Name,
			})
			continue
		}
		serverMaintenances = append(serverMaintenances, serverMaintenance)
	}

	if len(errs) > 0 {
		return serverMaintenances, errs
	}

	return serverMaintenances, nil
}

func (r *BMCSettingsReconciler) getReferredBMCSettings(ctx context.Context, referredBMCSettingsRef *corev1.LocalObjectReference) (*metalv1alpha1.BMCSettings, error) {
	key := client.ObjectKey{Name: referredBMCSettingsRef.Name, Namespace: metav1.NamespaceNone}
	settings := &metalv1alpha1.BMCSettings{}
	if err := r.Get(ctx, key, settings); err != nil {
		return nil, err
	}
	return settings, nil
}

func (r *BMCSettingsReconciler) getServerMaintenanceRefForServer(ServerMaintenanceRefs []metalv1alpha1.ServerMaintenanceRefItem, name, namespace string) *metalv1alpha1.ObjectReference {
	for _, serverMaintenanceRef := range ServerMaintenanceRefs {
		if serverMaintenanceRef.ServerMaintenanceRef.Name == name && serverMaintenanceRef.ServerMaintenanceRef.Namespace == namespace {
			return serverMaintenanceRef.ServerMaintenanceRef
		}
	}
	return nil
}

func (r *BMCSettingsReconciler) patchBMCSettingsRefOnBMC(ctx context.Context, bmcObj *metalv1alpha1.BMC, BMCSettingsReference *corev1.LocalObjectReference) error {
	if (bmcObj.Spec.BMCSettingRef == nil && BMCSettingsReference == nil) ||
		(bmcObj.Spec.BMCSettingRef != nil && BMCSettingsReference != nil &&
			bmcObj.Spec.BMCSettingRef.Name == BMCSettingsReference.Name) {
		return nil
	}

	bmcObjBase := bmcObj.DeepCopy()
	bmcObj.Spec.BMCSettingRef = BMCSettingsReference
	if err := r.Patch(ctx, bmcObj, client.MergeFrom(bmcObjBase)); err != nil {
		return fmt.Errorf("failed to patch BMC settings ref: %w", err)
	}
	return nil
}

func (r *BMCSettingsReconciler) patchMaintenanceRequestRefOnBMCSettings(ctx context.Context, settings *metalv1alpha1.BMCSettings, ServerMaintenanceRefs []metalv1alpha1.ServerMaintenanceRefItem) error {
	settingsBase := settings.DeepCopy()

	settings.Spec.ServerMaintenanceRefs = ServerMaintenanceRefs

	if err := r.Patch(ctx, settings, client.MergeFrom(settingsBase)); err != nil {
		return fmt.Errorf("failed to patch BMCSettings maintenance ref: %w", err)
	}

	return nil
}

func (r *BMCSettingsReconciler) updateBMCSettingsStatus(ctx context.Context, settings *metalv1alpha1.BMCSettings, state metalv1alpha1.BMCSettingsState, condition *metav1.Condition) error {
	log := ctrl.LoggerFrom(ctx)

	if settings.Status.State == state && condition == nil {
		return nil
	}

	BMCSettingsBase := settings.DeepCopy()
	settings.Status.State = state

	if condition != nil {
		if err := r.Conditions.UpdateSlice(
			&settings.Status.Conditions,
			condition.Type,
			conditionutils.UpdateStatus(condition.Status),
			conditionutils.UpdateReason(condition.Reason),
			conditionutils.UpdateMessage(condition.Message),
		); err != nil {
			return fmt.Errorf("failed to patch BMCSettings condition: %w", err)
		}
	} else if state == "" {
		settings.Status.Conditions = []metav1.Condition{}
	}

	if err := r.Status().Patch(ctx, settings, client.MergeFrom(BMCSettingsBase)); err != nil {
		return fmt.Errorf("failed to patch settings status: %w", err)
	}

	log.V(1).Info("Updated settings state", "State", state)

	return nil
}

func (r *BMCSettingsReconciler) enqueueBMCSettingsByServerRefs(ctx context.Context, obj client.Object) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)
	host := obj.(*metalv1alpha1.Server)

	// Return early if server is not in maintenance or has no maintenance ref
	if host.Status.State != metalv1alpha1.ServerStateMaintenance || host.Spec.ServerMaintenanceRef == nil {
		return nil
	}

	settingsList := &metalv1alpha1.BMCSettingsList{}
	if err := r.List(ctx, settingsList); err != nil {
		log.Error(err, "failed to list BMCSettings")
		return nil
	}
	var req []ctrl.Request

	for _, settings := range settingsList.Items {
		// Skip BMCSettings without maintenance requests
		if settings.Spec.ServerMaintenanceRefs == nil {
			continue
		}
		if settings.Status.State == metalv1alpha1.BMCSettingsStateApplied || settings.Status.State == metalv1alpha1.BMCSettingsStateFailed {
			continue
		}
		if host.Spec.ServerMaintenanceRef == nil {
			continue
		}
		serverMaintenanceRef := r.getServerMaintenanceRefForServer(settings.Spec.ServerMaintenanceRefs, host.Spec.ServerMaintenanceRef.Name, host.Spec.ServerMaintenanceRef.Namespace)
		if serverMaintenanceRef != nil {
			req = append(req, ctrl.Request{
				NamespacedName: types.NamespacedName{Namespace: settings.Namespace, Name: settings.Name},
			})
		}
	}
	return req
}

func (r *BMCSettingsReconciler) enqueueBMCSettingsByBMCRefs(ctx context.Context, obj client.Object) []ctrl.Request {

	log := ctrl.LoggerFrom(ctx)
	bmcObj := obj.(*metalv1alpha1.BMC)
	settingsList := &metalv1alpha1.BMCSettingsList{}
	if err := r.List(ctx, settingsList); err != nil {
		log.Error(err, "failed to list BMCSettingsList")
		return nil
	}

	var requests []ctrl.Request
	for _, settings := range settingsList.Items {
		if settings.Spec.BMCRef != nil && settings.Spec.BMCRef.Name == bmcObj.Name {
			if settings.Status.State == metalv1alpha1.BMCSettingsStateApplied || settings.Status.State == metalv1alpha1.BMCSettingsStateFailed {
				continue
			}
			requests = append(requests, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: settings.Namespace, Name: settings.Name}})
		}
	}
	return requests
}
func (r *BMCSettingsReconciler) enqueueBMCSettingsByBMCVersion(ctx context.Context, obj client.Object) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)
	BMCVersion := obj.(*metalv1alpha1.BMCVersion)
	if BMCVersion.Status.State != metalv1alpha1.BMCVersionStateCompleted {
		return nil
	}

	BMCSettingsList := &metalv1alpha1.BMCSettingsList{}
	if err := r.List(ctx, BMCSettingsList); err != nil {
		log.Error(err, "failed to list BMCSettings")
		return nil
	}

	var requests []ctrl.Request
	for _, settings := range BMCSettingsList.Items {
		if settings.Spec.BMCRef != nil && settings.Spec.BMCRef.Name == BMCVersion.Spec.BMCRef.Name {
			if settings.Status.State == metalv1alpha1.BMCSettingsStateApplied || settings.Status.State == metalv1alpha1.BMCSettingsStateFailed {
				continue
			}
			requests = append(requests, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: settings.Namespace, Name: settings.Name}})
		}
	}
	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *BMCSettingsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BMCSettings{}).
		Owns(&metalv1alpha1.ServerMaintenance{}).
		Watches(&metalv1alpha1.Server{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBMCSettingsByServerRefs)).
		Watches(&metalv1alpha1.BMC{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBMCSettingsByBMCRefs)).
		Watches(&metalv1alpha1.BMCVersion{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBMCSettingsByBMCVersion)).
		Complete(r)
}

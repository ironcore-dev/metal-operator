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

const (
	bmcVersionFinalizer                    = "metal.ironcore.dev/bmcversion"
	bmcVersionUpgradeIssued                = "VersionUpgradeIssued"
	bmcVersionUpgradeCompleted             = "VersionUpgradeCompleted"
	bmcVersionUpgradeRebootBMC             = "VersionUpgradeReboot"
	bmcVersionUpgradeVerificationCondition = "VersionUpgradeVerification"
	bmcUpgradeIssuedReason                 = "UpgradeIssued"
	bmcFailedUpgradeIssueReason            = "IssueBMCUpgradeFailed"
	bmcTaskCompletedReason                 = "TaskCompleted"
	bmcUpgradeTaskFailedReason             = "UpgradeTaskFailed"
	bmcVerifiedVersionUpdateReason         = "VerifiedBMCVersionUpdate"
)

// BMCVersionReconciler reconciles a BMCVersion object
type BMCVersionReconciler struct {
	client.Client
	ManagerNamespace string
	Insecure         bool
	Scheme           *runtime.Scheme
	BMCOptions       bmc.Options
	ResyncInterval   time.Duration
	Conditions       *conditionutils.Accessor
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcversions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcversions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcversions/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcs,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="batch",resources=jobs,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *BMCVersionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	bmcVersion := &metalv1alpha1.BMCVersion{}
	if err := r.Get(ctx, req.NamespacedName, bmcVersion); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.V(1).Info("Reconciling BMCVersion")

	return r.reconcileExists(ctx, log, bmcVersion)
}

func (r *BMCVersionReconciler) reconcileExists(ctx context.Context, log logr.Logger, bmcVersion *metalv1alpha1.BMCVersion) (ctrl.Result, error) {
	if r.shouldDelete(log, bmcVersion) {
		return r.delete(ctx, log, bmcVersion)
	}
	return r.reconcile(ctx, log, bmcVersion)
}

func (r *BMCVersionReconciler) shouldDelete(log logr.Logger, bmcVersion *metalv1alpha1.BMCVersion) bool {
	if bmcVersion.DeletionTimestamp.IsZero() {
		return false
	}

	if controllerutil.ContainsFinalizer(bmcVersion, bmcVersionFinalizer) &&
		bmcVersion.Status.State == metalv1alpha1.BMCVersionStateInProgress {
		log.V(1).Info("Postponing deletion as BMC version update is in progress")
		return false
	}
	return true
}

func (r *BMCVersionReconciler) delete(ctx context.Context, log logr.Logger, bmcVersion *metalv1alpha1.BMCVersion) (ctrl.Result, error) {
	log.V(1).Info("Deleting BMCVersion")
	if !controllerutil.ContainsFinalizer(bmcVersion, bmcVersionFinalizer) {
		return ctrl.Result{}, nil
	}

	if bmcVersion.Status.State == metalv1alpha1.BMCVersionStateInProgress {
		log.V(1).Info("Skipping delete as version update is in progress")
		return r.reconcile(ctx, log, bmcVersion)
	}

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, bmcVersion, bmcVersionFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	log.V(1).Info("Deleted BMCVersion")
	return ctrl.Result{}, nil
}

func (r *BMCVersionReconciler) removeServerMaintenances(ctx context.Context, log logr.Logger, bmcVersion *metalv1alpha1.BMCVersion) error {
	if bmcVersion.Spec.ServerMaintenanceRefs == nil {
		return nil
	}

	log.V(1).Info("Removing orphan server Maintenances")
	serverMaintenances, errs := r.getServerMaintenances(ctx, log, bmcVersion)

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

	if len(missingServerMaintenanceRef) != len(bmcVersion.Spec.ServerMaintenanceRefs) {
		// delete the ServerMaintenance if not marked for deletion already
		for _, serverMaintenance := range serverMaintenances {
			if serverMaintenance.DeletionTimestamp.IsZero() && metav1.IsControlledBy(serverMaintenance, bmcVersion) {
				log.V(1).Info("Deleting ServerMaintenance", "ServerMaintenance", client.ObjectKeyFromObject(serverMaintenance))
				if err := r.Delete(ctx, serverMaintenance); err != nil {
					log.V(1).Info("Failed to delete ServerMaintenance", "ServerMaintenance", client.ObjectKeyFromObject(serverMaintenance))
					finalErr = append(finalErr, err)
				}
			} else {
				log.V(1).Info(
					"Skipping deletion of ServerMaintenance as it has a different owner",
					"ServerMaintenance", client.ObjectKeyFromObject(serverMaintenance),
					"State", serverMaintenance.Status.State)
			}
		}
	}

	if len(finalErr) == 0 {
		if err := r.patchMaintenanceRequestRefOnBMCVersion(ctx, bmcVersion, nil); err != nil {
			return fmt.Errorf("failed to clean up serverMaintenance ref in bmcVersion status: %w", err)
		}
		log.V(1).Info("ServerMaintenance ref all cleaned up")
	}
	return errors.Join(finalErr...)
}

func (r *BMCVersionReconciler) reconcile(ctx context.Context, log logr.Logger, bmcVersion *metalv1alpha1.BMCVersion) (ctrl.Result, error) {
	if shouldIgnoreReconciliation(bmcVersion) {
		log.V(1).Info("Skipped BMCVersion reconciliation")
		return ctrl.Result{}, nil
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, bmcVersion, bmcVersionFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	return r.ensureBMCVersionStateTransition(ctx, log, bmcVersion)
}

func (r *BMCVersionReconciler) ensureBMCVersionStateTransition(ctx context.Context, log logr.Logger, bmcVersion *metalv1alpha1.BMCVersion) (ctrl.Result, error) {
	bmcObj, err := r.getBMCFromBMCVersion(ctx, bmcVersion)
	if err != nil {
		log.V(1).Info("Referred server object could not be fetched")
		return ctrl.Result{}, err
	}

	bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, r.Client, bmcObj, r.Insecure, r.BMCOptions)
	if err != nil {
		if errors.As(err, &bmcutils.BMCUnAvailableError{}) {
			log.V(1).Info("BMC is not available, skipping", "BMC", bmcObj.Name, "error", err)
			return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
		}
		log.V(1).Error(err, "failed to create BMC client", "BMC", bmcObj.Name)
		return ctrl.Result{}, err
	}
	defer bmcClient.Logout()

	switch bmcVersion.Status.State {
	case "", metalv1alpha1.BMCVersionStatePending:
		return ctrl.Result{}, r.removeServerMaintenanceRefAndResetConditions(ctx, log, bmcVersion, bmcClient, bmcObj)
	case metalv1alpha1.BMCVersionStateInProgress:
		servers, err := r.getServersForBMCVersion(ctx, log, bmcClient, bmcVersion)
		if err != nil {
			return ctrl.Result{}, err
		}

		if len(bmcVersion.Spec.ServerMaintenanceRefs) != len(servers) {
			requeue, err := r.requestMaintenanceOnServers(ctx, log, bmcClient, bmcVersion)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to request maintenance on servers: %v", err)
			}
			if requeue {
				return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
			}
		}
		condition, err := GetCondition(r.Conditions, bmcVersion.Status.Conditions, ServerMaintenanceConditionWaiting)
		if err != nil {
			return ctrl.Result{}, err
		}

		if ok := r.checkIfMaintenanceGranted(ctx, log, bmcClient, bmcVersion); !ok {
			if condition.Status != metav1.ConditionTrue {
				if err := r.Conditions.Update(
					condition,
					conditionutils.UpdateStatus(corev1.ConditionTrue),
					conditionutils.UpdateReason(ServerMaintenanceReasonWaiting),
					conditionutils.UpdateMessage(fmt.Sprintf("Waiting for approval of %v", bmcVersion.Spec.ServerMaintenanceRefs)),
				); err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to update creating ServerMaintenance condition: %w", err)
				}
				if err := r.patchBMCVersionStatusAndCondition(ctx, log, bmcVersion, bmcVersion.Status.State, bmcVersion.Status.UpgradeTask, condition); err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to patch BMCVersion ServerMaintenance waiting conditions: %w", err)
				}
			}
			return ctrl.Result{}, nil
		}

		// once in maintenance, clear the waiting condition if present
		if condition.Reason != ServerMaintenanceReasonApproved {
			if err := r.Conditions.Update(
				condition,
				conditionutils.UpdateStatus(corev1.ConditionFalse),
				conditionutils.UpdateReason(ServerMaintenanceReasonApproved),
				conditionutils.UpdateMessage("Servers are now in Maintenance mode"),
			); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update ServerMaintenance condition: %w", err)
			}

			if err := r.patchBMCVersionStatusAndCondition(ctx, log, bmcVersion, bmcVersion.Status.State, bmcVersion.Status.UpgradeTask, condition); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to patch BMCVersion ServerMaintenance waiting completed conditions: %w", err)
			}
			return ctrl.Result{}, nil
		}

		if ok, err := r.resetBMC(ctx, log, bmcVersion, bmcObj, BMCConditionReset); !ok || err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to reset bmc %s: %w", client.ObjectKeyFromObject(bmcObj), err)
		}

		return r.handleUpgradeInProgressState(ctx, log, bmcVersion, bmcClient, bmcObj)
	case metalv1alpha1.BMCVersionStateCompleted:
		return ctrl.Result{}, r.removeServerMaintenanceRefAndResetConditions(ctx, log, bmcVersion, bmcClient, bmcObj)
	case metalv1alpha1.BMCVersionStateFailed:
		if shouldRetryReconciliation(bmcVersion) {
			log.V(1).Info("Retrying BMCVersion reconciliation")
			bmcVersionBase := bmcVersion.DeepCopy()
			bmcVersion.Status.State = metalv1alpha1.BMCVersionStatePending
			bmcVersion.Status.Conditions = []metav1.Condition{}
			annotations := bmcVersion.GetAnnotations()
			delete(annotations, metalv1alpha1.OperationAnnotation)
			bmcVersion.SetAnnotations(annotations)
			if err := r.Status().Patch(ctx, bmcVersion, client.MergeFrom(bmcVersionBase)); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to patch BMCVersion status for retrying: %w", err)
			}
			return ctrl.Result{}, nil
		}
		log.V(1).Info("Failed to upgrade BMC via BMCVersion", "BMCVersion", bmcVersion.Name, "BMC", bmcObj.Name)
		return ctrl.Result{}, nil
	}
	log.V(1).Info("Unknown State found", "State", bmcVersion.Status.State)
	return ctrl.Result{}, nil
}

func (r *BMCVersionReconciler) handleUpgradeInProgressState(
	ctx context.Context,
	log logr.Logger,
	bmcVersion *metalv1alpha1.BMCVersion,
	bmcClient bmc.BMC,
	BMC *metalv1alpha1.BMC,
) (ctrl.Result, error) {
	issuedCondition, err := GetCondition(r.Conditions, bmcVersion.Status.Conditions, bmcVersionUpgradeIssued)
	if err != nil {
		return ctrl.Result{}, err
	}

	if issuedCondition.Status != metav1.ConditionTrue {
		log.V(1).Info("Issuing upgrade of BMC version")
		switch BMC.Status.PowerState {
		case metalv1alpha1.OnPowerState:
			// proceed silently
		case metalv1alpha1.UnknownPowerState:
			log.V(1).Info("BMC PowerState unknown, continuing anyway.", "BMC", BMC.Name, "PowerState", BMC.Status.PowerState)
		default:
			log.V(1).Info("BMC is not powered on. Can not proceed", "BMC", BMC.Name, "PowerState", BMC.Status.PowerState)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, r.issueBMCUpgrade(ctx, log, bmcVersion, bmcClient, BMC, issuedCondition)
	}

	completedCondition, err := GetCondition(r.Conditions, bmcVersion.Status.Conditions, bmcVersionUpgradeCompleted)
	if err != nil {
		return ctrl.Result{}, err
	}

	if completedCondition.Status != metav1.ConditionTrue {
		log.V(1).Info("Check upgrade task of BMC")
		ctrlResult, err := r.checkBMCUpgradeStatus(ctx, log, bmcVersion, bmcClient, BMC, bmcVersion.Status.UpgradeTask.URI, completedCondition)
		var TaskFetchFailed *BMCTaskFetchFailedError
		if errors.As(err, &TaskFetchFailed) {
			log.V(1).Info("Failed to fetch BMC upgrade task status from BMC.", "error", err)
			// some vendor detele the task details once upgrade is completed.
			// check the current version and then proceed if version is as per spec
			currentBMCVersion, errVersionFetch := r.getBMCVersionFromBMC(ctx, bmcClient, BMC)
			if errVersionFetch != nil {
				// need to give time if BMC is not responding, hence requeue
				log.V(1).Error(errors.Join(err, errVersionFetch), "Failed to fetch current BMC version from BMC after upgrade task fetch failure.")
				return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
			}
			if currentBMCVersion == bmcVersion.Spec.Version {
				// mark as completed, and procced with the workflow as the task might have been deleted post successful upgrade
				log.V(1).Info("BMC version shows upgraded successfully even though task fetch failure", "Version", currentBMCVersion)
				if err := r.Conditions.Update(
					completedCondition,
					conditionutils.UpdateStatus(corev1.ConditionTrue),
					conditionutils.UpdateReason(bmcTaskCompletedReason),
					conditionutils.UpdateMessage("Upgrade Task is missing. BMC version successfully upgraded to: "+bmcVersion.Spec.Version),
				); err != nil {
					return ctrlResult, fmt.Errorf("failed to update conditions: %w", err)
				}
				return ctrlResult, r.patchBMCVersionStatusAndCondition(ctx, log, bmcVersion, bmcVersion.Status.State, bmcVersion.Status.UpgradeTask, completedCondition)
			}
			log.V(1).Info("BMC version not updated yet, need to wait for task details", "Version", currentBMCVersion, "DesiredVersion", bmcVersion.Spec.Version)
			return ctrlResult, err
		}
		return ctrlResult, err
	}

	if ok, err := r.resetBMC(ctx, log, bmcVersion, BMC, bmcVersionUpgradeRebootBMC); !ok || err != nil {
		return ctrl.Result{}, err
	}

	condition, err := GetCondition(r.Conditions, bmcVersion.Status.Conditions, bmcVersionUpgradeVerificationCondition)
	if err != nil {
		return ctrl.Result{}, err
	}

	if condition.Status != metav1.ConditionTrue {
		log.V(1).Info("Verify BMC Version update")

		currentBMCVersion, err := r.getBMCVersionFromBMC(ctx, bmcClient, BMC)
		if err != nil {
			return ctrl.Result{}, err
		}
		if currentBMCVersion != bmcVersion.Spec.Version {
			// todo: add timeout
			log.V(1).Info("BMC version not updated", "current BMC Version", currentBMCVersion, "Required Version", bmcVersion.Spec.Version)
			if condition.Reason == "" {
				if err := r.Conditions.Update(
					condition,
					conditionutils.UpdateStatus(corev1.ConditionFalse),
					conditionutils.UpdateReason("VerifyBMCVersionUpdate"),
					conditionutils.UpdateMessage("waiting for BMC Version update"),
				); err != nil {
					log.V(1).Error(err, "failed to update the conditions status. retrying...")
					return ctrl.Result{}, err
				}
			}
			log.V(1).Info("Waiting for BMC version to reflect the new version")
			return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
		}

		if err := r.Conditions.Update(
			condition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(bmcVerifiedVersionUpdateReason),
			conditionutils.UpdateMessage("BMC Version updated"),
		); err != nil {
			log.V(1).Error(err, "failed to update the conditions status. retrying...")
			return ctrl.Result{}, err
		}
		err = r.patchBMCVersionStatusAndCondition(
			ctx,
			log,
			bmcVersion,
			metalv1alpha1.BMCVersionStateCompleted,
			bmcVersion.Status.UpgradeTask,
			condition,
		)
		return ctrl.Result{}, err
	}

	log.V(1).Info("Unknown Conditions found", "Condition", condition.Type)
	return ctrl.Result{}, nil
}

func (r *BMCVersionReconciler) getBMCVersionFromBMC(ctx context.Context, bmcClient bmc.BMC, BMC *metalv1alpha1.BMC) (string, error) {
	currentBMCVersion, err := bmcClient.GetBMCVersion(ctx, BMC.Spec.BMCUUID)
	if err != nil {
		return currentBMCVersion, fmt.Errorf("failed to load BMC version: %w for BMC %v", err, BMC.Name)
	}

	return currentBMCVersion, nil
}

func (r *BMCVersionReconciler) checkIfMaintenanceGranted(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, bmcVersion *metalv1alpha1.BMCVersion) bool {
	// todo length
	if bmcVersion.Spec.ServerMaintenanceRefs == nil {
		return true
	}

	servers, err := r.getServersForBMCVersion(ctx, log, bmcClient, bmcVersion)
	if err != nil {
		log.V(1).Error(err, "Failed to get ref. servers to determine maintenance state ")
		return false
	}

	if len(bmcVersion.Spec.ServerMaintenanceRefs) != len(servers) {
		log.V(1).Info("Not all servers have Maintenance", "ServerMaintenanceRefs", bmcVersion.Spec.ServerMaintenanceRefs, "Servers", servers)
		return false
	}

	notInMaintenanceState := make(map[string]bool, len(servers))
	for _, server := range servers {
		if server.Status.State == metalv1alpha1.ServerStateMaintenance {
			serverMaintenanceRef, ok := r.getServerMaintenanceRefForServer(bmcVersion.Spec.ServerMaintenanceRefs, server.Spec.ServerMaintenanceRef.UID)
			if server.Spec.ServerMaintenanceRef == nil || !ok || server.Spec.ServerMaintenanceRef.UID != serverMaintenanceRef.UID {
				// We hit a server in maintenance waiting for other tasks to complete.
				// Alternatively, the server maintenance reference is wrong. Either server or bmcVersion
				// wait for update on the server obj.
				log.V(1).Info("Server is already in maintenance", "Server", server.Name)
				notInMaintenanceState[server.Name] = false
			}
		} else {
			// we still need to wait for server to enter maintenance
			log.V(1).Info("Server not yet in maintenance", "Server", server.Name)
			notInMaintenanceState[server.Name] = false
		}
	}

	if len(notInMaintenanceState) > 0 {
		log.V(1).Info("Found a least one server not in maintenance")
		return false
	}

	return true
}

func (r *BMCVersionReconciler) removeServerMaintenanceRefAndResetConditions(
	ctx context.Context,
	log logr.Logger,
	bmcVersion *metalv1alpha1.BMCVersion,
	bmcClient bmc.BMC,
	BMC *metalv1alpha1.BMC,
) error {
	currentBMCVersion, err := r.getBMCVersionFromBMC(ctx, bmcClient, BMC)
	if err != nil {
		return err
	}
	state := metalv1alpha1.BMCVersionStateInProgress
	if currentBMCVersion == bmcVersion.Spec.Version {
		if err := r.removeServerMaintenances(ctx, log, bmcVersion); err != nil {
			return err
		}
		log.V(1).Info("Upgraded BMC version", "BMCVersion", currentBMCVersion, "BMC", BMC.Name)
		state = metalv1alpha1.BMCVersionStateCompleted
	}
	err = r.patchBMCVersionStatusAndCondition(ctx, log, bmcVersion, state, nil, nil)
	return err
}

func (r *BMCVersionReconciler) getServerMaintenances(ctx context.Context, log logr.Logger, bmcVersion *metalv1alpha1.BMCVersion) ([]*metalv1alpha1.ServerMaintenance, []error) {
	refs := bmcVersion.Spec.ServerMaintenanceRefs
	maintenances := make([]*metalv1alpha1.ServerMaintenance, 0, len(refs))
	var errs []error
	for _, ref := range refs {
		key := client.ObjectKey{Name: ref.Name, Namespace: r.ManagerNamespace}
		serverMaintenance := &metalv1alpha1.ServerMaintenance{}
		if err := r.Get(ctx, key, serverMaintenance); err != nil {
			log.V(1).Error(err, "failed to get referred serverMaintenance obj", "ServerMaintenance", ref.Name)
			errs = append(errs, err)
			continue
		}
		maintenances = append(maintenances, serverMaintenance)
	}

	if len(errs) > 0 {
		return maintenances, errs
	}

	return maintenances, nil
}

func (r *BMCVersionReconciler) resetBMC(ctx context.Context, log logr.Logger, bmcVersion *metalv1alpha1.BMCVersion, bmcObj *metalv1alpha1.BMC, conditionType string) (bool, error) {
	// reset BMC if not already done
	condition, err := GetCondition(r.Conditions, bmcVersion.Status.Conditions, conditionType)
	if err != nil {
		return false, fmt.Errorf("failed to get condition for reset of BMC of server %v", err)
	}

	if condition.Status != metav1.ConditionTrue {
		annotations := bmcObj.GetAnnotations()
		// Once the server is powered on, reset the BMC to make sure it is in stable state.
		// This avoids problems with some BMCs that hang up in subsequent operations.
		if condition.Reason != BMCReasonReset {
			if annotations != nil {
				if op, ok := annotations[metalv1alpha1.OperationAnnotation]; ok {
					if op == metalv1alpha1.GracefulRestartBMC {
						log.V(1).Info("Waiting for BMC reset as annotation on BMC object is set")
						if err := r.Conditions.Update(
							condition,
							conditionutils.UpdateStatus(corev1.ConditionFalse),
							conditionutils.UpdateReason(BMCReasonReset),
							conditionutils.UpdateMessage("Issued BMC reset to stabilize BMC of the server"),
						); err != nil {
							return false, fmt.Errorf("failed to update reset BMC condition: %w", err)
						}
						// patch condition to reset issued
						return false, r.patchBMCVersionStatusAndCondition(ctx, log, bmcVersion, bmcVersion.Status.State, nil, condition)
					} else {
						return false, fmt.Errorf("unknown annotation on BMC object for operation annotation %v", op)
					}
				}
			}
			log.V(1).Info("Setting annotation on BMC resource to trigger with BMC reset")

			bmcBase := bmcObj.DeepCopy()
			if annotations == nil {
				annotations = map[string]string{}
			}
			annotations[metalv1alpha1.OperationAnnotation] = metalv1alpha1.GracefulRestartBMC
			bmcObj.SetAnnotations(annotations)
			if err := r.Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
				return false, err
			}

			if err := r.Conditions.Update(
				condition,
				conditionutils.UpdateStatus(corev1.ConditionFalse),
				conditionutils.UpdateReason(BMCReasonReset),
				conditionutils.UpdateMessage("Issued BMC reset to stabilize BMC of the server"),
			); err != nil {
				return false, fmt.Errorf("failed to update reset BMC condition: %w", err)
			}
			// patch condition to reset issued
			return false, r.patchBMCVersionStatusAndCondition(ctx, log, bmcVersion, bmcVersion.Status.State, nil, condition)
		}

		// we need to wait until the BMC resource annotation is removed
		if annotations != nil {
			if op, ok := annotations[metalv1alpha1.OperationAnnotation]; ok {
				if op == metalv1alpha1.GracefulRestartBMC {
					log.V(1).Info("Waiting for BMC reset as annotation on BMC object is set")
					return false, nil
				}
			}
		}

		if err := r.Conditions.Update(
			condition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(BMCReasonReset),
			conditionutils.UpdateMessage("BMC reset to stabilize BMC of the server is completed"),
		); err != nil {
			return false, fmt.Errorf("failed to update power on server condition: %w", err)
		}
		return false, r.patchBMCVersionStatusAndCondition(ctx, log, bmcVersion, bmcVersion.Status.State, nil, condition)
	}
	return true, nil
}

func (r *BMCVersionReconciler) getServersForBMCVersion(
	ctx context.Context,
	log logr.Logger,
	bmcClient bmc.BMC,
	bmcVersion *metalv1alpha1.BMCVersion,
) ([]*metalv1alpha1.Server, error) {
	if bmcVersion.Spec.BMCRef == nil {
		return nil, fmt.Errorf("BMC reference not found")
	}
	bmcObj, err := r.getBMCFromBMCVersion(ctx, bmcVersion)
	if err != nil {
		log.V(1).Error(err, "failed to get referred BMC")
		return nil, err
	}
	bmcServers, err := bmcClient.GetSystems(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get servers from BMC %s: %w", bmcObj.Name, err)
	}
	serversRefList := make([]*corev1.LocalObjectReference, len(bmcServers))
	for i := range bmcServers {
		serversRefList[i] = &corev1.LocalObjectReference{Name: bmcutils.GetServerNameFromBMCandIndex(i, bmcObj)}
	}
	servers, err := r.getServersForRefs(ctx, log, serversRefList)
	if err != nil {
		return servers, fmt.Errorf("errors occurred during fetching servers from BMC: %v", err)
	}
	return servers, nil
}

func (r *BMCVersionReconciler) getServersForRefs(ctx context.Context, log logr.Logger, serverRefList []*corev1.LocalObjectReference) ([]*metalv1alpha1.Server, error) {
	var errs []error
	servers := make([]*metalv1alpha1.Server, len(serverRefList))
	for idx, serverRef := range serverRefList {
		key := client.ObjectKey{Name: serverRef.Name}
		server := &metalv1alpha1.Server{}
		if err := r.Get(ctx, key, server); err != nil {
			log.V(1).Error(err, "failed to get referred server", "reference", serverRef.Name)
			errs = append(errs, err)
			continue
		}
		servers[idx] = server
	}
	return servers, errors.Join(errs...)
}

func (r *BMCVersionReconciler) getBMCFromBMCVersion(ctx context.Context, bmcVersion *metalv1alpha1.BMCVersion) (*metalv1alpha1.BMC, error) {
	if bmcVersion.Spec.BMCRef == nil {
		return nil, fmt.Errorf("no BMC reference found for BMC version %s", bmcVersion.Name)
	}

	bmcObj := &metalv1alpha1.BMC{}
	if err := r.Get(ctx, client.ObjectKey{Name: bmcVersion.Spec.BMCRef.Name}, bmcObj); err != nil {
		return bmcObj, err
	}

	return bmcObj, nil
}

func (r *BMCVersionReconciler) getServerMaintenanceRefForServer(serverMaintenanceRefs []metalv1alpha1.ObjectReference, serverMaintenanceUID types.UID) (metalv1alpha1.ObjectReference, bool) {
	for _, serverMaintenanceRef := range serverMaintenanceRefs {
		if serverMaintenanceRef.UID == serverMaintenanceUID {
			return serverMaintenanceRef, true
		}
	}
	return metalv1alpha1.ObjectReference{}, false
}

func (r *BMCVersionReconciler) patchBMCVersionStatusAndCondition(
	ctx context.Context,
	log logr.Logger,
	bmcVersion *metalv1alpha1.BMCVersion,
	state metalv1alpha1.BMCVersionState,
	upgradeTask *metalv1alpha1.Task,
	condition *metav1.Condition,
) error {
	log.V(1).Info("Patching BMCVersion status")
	if bmcVersion.Status.State == state && condition == nil && upgradeTask == nil {
		return nil
	}

	bmcVersionBase := bmcVersion.DeepCopy()
	bmcVersion.Status.State = state

	if condition != nil {
		if err := r.Conditions.UpdateSlice(
			&bmcVersion.Status.Conditions,
			condition.Type,
			conditionutils.UpdateStatus(condition.Status),
			conditionutils.UpdateReason(condition.Reason),
			conditionutils.UpdateMessage(condition.Message),
		); err != nil {
			return fmt.Errorf("failed to patch BMCVersion condition: %w", err)
		}
	} else {
		bmcVersion.Status.Conditions = []metav1.Condition{}
	}

	bmcVersion.Status.UpgradeTask = upgradeTask

	if err := r.Status().Patch(ctx, bmcVersion, client.MergeFrom(bmcVersionBase)); err != nil {
		return fmt.Errorf("failed to patch BMCVersion status: %w", err)
	}

	log.V(1).Info("Patched BMCVersion status", "State", state)
	return nil
}

func (r *BMCVersionReconciler) patchMaintenanceRequestRefOnBMCVersion(
	ctx context.Context,
	bmcVersion *metalv1alpha1.BMCVersion,
	serverMaintenanceRefs []metalv1alpha1.ObjectReference,
) error {
	bmcVersionsBase := bmcVersion.DeepCopy()

	if serverMaintenanceRefs == nil {
		bmcVersion.Spec.ServerMaintenanceRefs = nil
	} else {
		bmcVersion.Spec.ServerMaintenanceRefs = serverMaintenanceRefs
	}

	if err := r.Patch(ctx, bmcVersion, client.MergeFrom(bmcVersionsBase)); err != nil {
		return err
	}

	return nil
}

func (r *BMCVersionReconciler) requestMaintenanceOnServers(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, bmcVersion *metalv1alpha1.BMCVersion) (bool, error) {
	servers, err := r.getServersForBMCVersion(ctx, log, bmcClient, bmcVersion)
	if err != nil {
		log.V(1).Error(err, "Failed to get ref. servers to request maintenance on servers")
		return false, err
	}

	// If ServerMaintenance ref is already set, ignoring.
	if bmcVersion.Spec.ServerMaintenanceRefs != nil && len(bmcVersion.Spec.ServerMaintenanceRefs) == len(servers) {
		if _, errs := r.getServerMaintenances(ctx, log, bmcVersion); len(errs) > 0 {
			if apierrors.IsNotFound(errors.Join(errs...)) {
				log.V(1).Info("Referenced ServerMaintenance no longer exists, clearing ref to allow re-creation")
				if err := r.patchMaintenanceRequestRefOnBMCVersion(ctx, bmcVersion, nil); err != nil {
					return false, fmt.Errorf("failed to clear stale ServerMaintenance ref: %w", err)
				}
				return true, nil // requeue to re-create
			} else {
				return false, fmt.Errorf("failed to verify ServerMaintenance existence: %w", errors.Join(errs...))
			}
		}
		condition, err := GetCondition(r.Conditions, bmcVersion.Status.Conditions, ServerMaintenanceConditionCreated)
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
			conditionutils.UpdateMessage(fmt.Sprintf("Created/present %v at %v", bmcVersion.Spec.ServerMaintenanceRefs, time.Now())),
		); err != nil {
			return false, fmt.Errorf("failed to update creating ServerMaintenance condition: %w", err)
		}

		if err := r.patchBMCVersionStatusAndCondition(ctx, log, bmcVersion, bmcVersion.Status.State, bmcVersion.Status.UpgradeTask, condition); err != nil {
			return false, fmt.Errorf("failed to patch BMCVersion conditions: %w", err)
		}
		return false, nil
	}

	// Ensure all affected Servers have a ServerMaintenance requested.
	serverWithMaintenances := make(map[string]*metalv1alpha1.ServerMaintenance, len(servers))
	if bmcVersion.Spec.ServerMaintenanceRefs != nil {
		// We fetch all the references already in the Spec (self created/provided by user)
		serverMaintenances, err := r.getServerMaintenances(ctx, log, bmcVersion)
		if err != nil {
			return false, errors.Join(err...)
		}
		for _, serverMaintenance := range serverMaintenances {
			serverWithMaintenances[serverMaintenance.Spec.ServerRef.Name] = serverMaintenance
		}
	}

	// We also fetch all the references owned by this Resource.
	// This is needed in case we are reconciling before we have patched the references.
	// Possible when we reconcile after CreateOrPatch and before the ref has been written.
	serverMaintenancesList := &metalv1alpha1.ServerMaintenanceList{}
	if err := clientutils.ListAndFilterControlledBy(ctx, r.Client, bmcVersion, serverMaintenancesList); err != nil {
		return false, err
	}
	for _, serverMaintenance := range serverMaintenancesList.Items {
		serverWithMaintenances[serverMaintenance.Spec.ServerRef.Name] = &serverMaintenance
	}

	var errs []error
	serverMaintenanceRefs := make([]metalv1alpha1.ObjectReference, 0, len(servers))
	for _, server := range servers {
		if maintenance, ok := serverWithMaintenances[server.Name]; ok {
			serverMaintenanceRefs = append(
				serverMaintenanceRefs,
				metalv1alpha1.ObjectReference{
					APIVersion: metalv1alpha1.GroupVersion.String(),
					Kind:       "ServerMaintenance",
					Namespace:  maintenance.Namespace,
					Name:       maintenance.Name,
					UID:        maintenance.UID,
				})
			continue
		}
		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    r.ManagerNamespace,
				GenerateName: "bmc-version-",
			},
		}

		opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, serverMaintenance, func() error {
			serverMaintenance.Spec.Policy = bmcVersion.Spec.ServerMaintenancePolicy
			serverMaintenance.Spec.ServerPower = metalv1alpha1.PowerOn
			serverMaintenance.Spec.ServerRef = &corev1.LocalObjectReference{Name: server.Name}
			if serverMaintenance.Status.State != metalv1alpha1.ServerMaintenanceStateInMaintenance && serverMaintenance.Status.State != "" {
				serverMaintenance.Status.State = ""
			}
			return controllerutil.SetControllerReference(bmcVersion, serverMaintenance, r.Client.Scheme())
		})
		if err != nil {
			log.V(1).Error(err, "failed to create or patch ServerMaintenance for Server", "ServerMaintenance", client.ObjectKeyFromObject(serverMaintenance), "Server", server.Name)
			errs = append(errs, err)
			continue
		}
		log.V(1).Info("Created ServerMaintenance", "ServerMaintenance", client.ObjectKeyFromObject(serverMaintenance), "Operation", opResult)

		serverMaintenanceRefs = append(
			serverMaintenanceRefs,
			metalv1alpha1.ObjectReference{
				APIVersion: metalv1alpha1.GroupVersion.String(),
				Kind:       "ServerMaintenance",
				Namespace:  serverMaintenance.Namespace,
				Name:       serverMaintenance.Name,
				UID:        serverMaintenance.UID,
			})
	}

	if len(errs) > 0 {
		return false, errors.Join(errs...)
	}

	if err := r.patchMaintenanceRequestRefOnBMCVersion(ctx, bmcVersion, serverMaintenanceRefs); err != nil {
		return false, fmt.Errorf("failed to patch serverMaintenance ref in BMCVersion status: %w", err)
	}

	log.V(1).Info("Patched ServerMaintenanceMap on BMCVersion")

	return true, nil
}

func (r *BMCVersionReconciler) checkBMCUpgradeStatus(
	ctx context.Context,
	log logr.Logger,
	bmcVersion *metalv1alpha1.BMCVersion,
	bmcClient bmc.BMC,
	bmcObj *metalv1alpha1.BMC,
	bmcUpgradeTaskUri string,
	completedCondition *metav1.Condition,
) (ctrl.Result, error) {
	taskCurrentStatus, err := func() (*redfish.Task, error) {
		if bmcUpgradeTaskUri == "" {
			return nil, fmt.Errorf("invalid task URI. uri provided: '%v'", bmcUpgradeTaskUri)
		}
		return bmcClient.GetBMCUpgradeTask(ctx, bmcObj.Status.Manufacturer, bmcUpgradeTaskUri)
	}()
	if err != nil {
		log.V(1).Error(err, "failed to get the task details of BMC upgrade task", "TaskURI", bmcUpgradeTaskUri)
		return ctrl.Result{}, &BMCTaskFetchFailedError{
			TaskURI:  bmcUpgradeTaskUri,
			Resource: "BMCUpgrade",
			Err:      err,
		}
	}
	log.V(1).Info("BMC upgrade task current status", "TaskStatus", taskCurrentStatus)

	upgradeCurrentTaskStatus := &metalv1alpha1.Task{
		URI:             bmcVersion.Status.UpgradeTask.URI,
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
		return ctrl.Result{}, fmt.Errorf("failed to create checkpoint for Condition. %v", err)
	}

	if taskCurrentStatus.TaskState == redfish.KilledTaskState ||
		taskCurrentStatus.TaskState == redfish.ExceptionTaskState ||
		taskCurrentStatus.TaskState == redfish.CancelledTaskState ||
		(taskCurrentStatus.TaskStatus != common.OKHealth && taskCurrentStatus.TaskStatus != "") {
		message := fmt.Sprintf(
			"Upgrade BMC task has failed. with message %v check '%v' for details",
			taskCurrentStatus.Messages,
			bmcUpgradeTaskUri,
		)
		if err := r.Conditions.Update(
			completedCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(bmcUpgradeTaskFailedReason),
			conditionutils.UpdateMessage(message),
		); err != nil {
			log.V(1).Error(err, "failed to update the conditions status. reconcile again ")
			return ctrl.Result{}, err
		}
		err = r.patchBMCVersionStatusAndCondition(
			ctx,
			log,
			bmcVersion,
			metalv1alpha1.BMCVersionStateFailed,
			upgradeCurrentTaskStatus,
			completedCondition,
		)
		return ctrl.Result{}, err
	}

	if taskCurrentStatus.TaskState == redfish.CompletedTaskState {
		if err := r.Conditions.Update(
			completedCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(bmcTaskCompletedReason),
			conditionutils.UpdateMessage("BMC successfully upgraded to: "+bmcVersion.Spec.Version),
		); err != nil {
			log.V(1).Error(err, "failed to update the conditions status. reconcile again")
			return ctrl.Result{}, err
		}
		err = r.patchBMCVersionStatusAndCondition(
			ctx,
			log,
			bmcVersion,
			bmcVersion.Status.State,
			upgradeCurrentTaskStatus,
			completedCondition,
		)
		return ctrl.Result{}, err
	}

	// in progress task states
	if err := r.Conditions.Update(
		completedCondition,
		conditionutils.UpdateStatus(corev1.ConditionFalse),
		conditionutils.UpdateReason(taskCurrentStatus.TaskState),
		conditionutils.UpdateMessage(
			fmt.Sprintf("BMC upgrade in state: %v: PercentageCompleted %v",
				taskCurrentStatus.TaskState,
				taskCurrentStatus.PercentComplete),
		),
	); err != nil {
		log.V(1).Error(err, "failed to update the conditions status. retrying... ")
		return ctrl.Result{}, err
	}
	ok, err := checkpoint.Transitioned(r.Conditions, *completedCondition)
	if !ok && err == nil {
		log.V(1).Info("BMC upgrade task has not Progressed. retrying....")
		// the job has stalled or slow, we need to requeue with exponential backoff
		return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
	}
	// todo: Fail the state after certain timeout
	err = r.patchBMCVersionStatusAndCondition(
		ctx,
		log,
		bmcVersion,
		bmcVersion.Status.State,
		upgradeCurrentTaskStatus,
		completedCondition,
	)
	return ctrl.Result{}, err
}

func (r *BMCVersionReconciler) issueBMCUpgrade(
	ctx context.Context,
	log logr.Logger,
	bmcVersion *metalv1alpha1.BMCVersion,
	bmcClient bmc.BMC,
	bmcObj *metalv1alpha1.BMC,
	issuedCondition *metav1.Condition,
) error {
	var username, password string
	if bmcVersion.Spec.Image.SecretRef != nil {
		var err error
		password, username, err = GetImageCredentialsForSecretRef(ctx, r.Client, bmcVersion.Spec.Image.SecretRef)
		if err != nil {
			return err
		}
	}

	var forceUpdate bool
	if bmcVersion.Spec.UpdatePolicy != nil && *bmcVersion.Spec.UpdatePolicy == metalv1alpha1.UpdatePolicyForce {
		forceUpdate = true
	}

	parameters := &redfish.SimpleUpdateParameters{
		ForceUpdate:      forceUpdate,
		ImageURI:         bmcVersion.Spec.Image.URI,
		Passord:          password,
		Username:         username,
		TransferProtocol: redfish.TransferProtocolType(bmcVersion.Spec.Image.TransferProtocol),
	}

	taskMonitor, isFatal, err := func() (string, bool, error) {
		return bmcClient.UpgradeBMCVersion(ctx, bmcObj.Status.Manufacturer, parameters)
	}()

	upgradeCurrentTaskStatus := &metalv1alpha1.Task{URI: taskMonitor}

	if isFatal {
		log.V(1).Error(err, "failed to issue bmc upgrade", "requested bmc version", bmcVersion.Spec.Version, "BMC", bmcObj.Name)
		var errCond error
		if errCond = r.Conditions.Update(
			issuedCondition,
			conditionutils.UpdateStatus(corev1.ConditionFalse),
			conditionutils.UpdateReason(bmcFailedUpgradeIssueReason),
			conditionutils.UpdateMessage("Fatal error occurred. Upgrade might still go through on server."),
		); errCond != nil {
			log.V(1).Error(errCond, "failed to update the conditions status")
		}
		err := r.patchBMCVersionStatusAndCondition(
			ctx,
			log,
			bmcVersion,
			metalv1alpha1.BMCVersionStateFailed,
			upgradeCurrentTaskStatus,
			issuedCondition,
		)
		return errors.Join(errCond, err)
	}
	if err != nil {
		log.V(1).Error(err, "failed to issue bmc upgrade", "bmc version", bmcVersion.Spec.Version, "BMC", bmcObj.Name)
		return err
	}
	var errCond error
	state := bmcVersion.Status.State
	if errCond = r.Conditions.Update(
		issuedCondition,
		conditionutils.UpdateStatus(corev1.ConditionTrue),
		conditionutils.UpdateReason(bmcUpgradeIssuedReason),
		conditionutils.UpdateMessage(fmt.Sprintf("Task to upgrade has been created %v", taskMonitor)),
	); errCond != nil {
		log.V(1).Error(errCond, "failed to update the conditions status... retrying")
		if errCond = r.Conditions.Update(
			issuedCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(bmcUpgradeIssuedReason),
			conditionutils.UpdateMessage(fmt.Sprintf("Task to upgrade has been created %v", taskMonitor)),
		); errCond != nil {
			state = metalv1alpha1.BMCVersionStateFailed
			log.V(1).Error(errCond, "failed to update the conditions status, failing the upgrade process! BMC might still be updated to new version")
		}
	}
	err = r.patchBMCVersionStatusAndCondition(
		ctx,
		log,
		bmcVersion,
		state,
		upgradeCurrentTaskStatus,
		issuedCondition,
	)
	return errors.Join(errCond, err)
}

func (r *BMCVersionReconciler) enqueueBMCVersionByServerRefs(ctx context.Context, obj client.Object) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)
	host := obj.(*metalv1alpha1.Server)

	// return early if hosts are not required states
	if host.Status.State != metalv1alpha1.ServerStateMaintenance || host.Spec.ServerMaintenanceRef == nil {
		return nil
	}

	bmcVersionList := &metalv1alpha1.BMCVersionList{}
	if err := r.List(ctx, bmcVersionList); err != nil {
		log.Error(err, "failed to list BMCVersion")
		return nil
	}
	var req []ctrl.Request

	for _, bmcVersion := range bmcVersionList.Items {
		// if we don't have maintenance request on this bmcVersion we do not want to enqueue changes from servers.
		if bmcVersion.Spec.ServerMaintenanceRefs == nil {
			continue
		}
		if bmcVersion.Status.State == metalv1alpha1.BMCVersionStateCompleted || bmcVersion.Status.State == metalv1alpha1.BMCVersionStateFailed {
			continue
		}
		serverMaintenanceRef, ok := r.getServerMaintenanceRefForServer(bmcVersion.Spec.ServerMaintenanceRefs, host.Spec.ServerMaintenanceRef.UID)
		if ok && serverMaintenanceRef.Name != "" {
			req = append(req, ctrl.Request{
				NamespacedName: types.NamespacedName{Namespace: bmcVersion.Namespace, Name: bmcVersion.Name},
			})
		}
	}
	return req
}

func (r *BMCVersionReconciler) enqueueBMCVersionByBMCRefs(ctx context.Context, obj client.Object) []ctrl.Request {

	log := ctrl.LoggerFrom(ctx)
	bmcObj := obj.(*metalv1alpha1.BMC)
	bmcVersionList := &metalv1alpha1.BMCVersionList{}
	if err := r.List(ctx, bmcVersionList); err != nil {
		log.Error(err, "failed to list BMCVersionList")
		return nil
	}

	for _, bmcVersion := range bmcVersionList.Items {
		if bmcVersion.Spec.BMCRef != nil && bmcVersion.Spec.BMCRef.Name == bmcObj.Name {
			if bmcVersion.Status.State == metalv1alpha1.BMCVersionStateCompleted || bmcVersion.Status.State == metalv1alpha1.BMCVersionStateFailed {
				return nil
			}
			return []ctrl.Request{{NamespacedName: types.NamespacedName{Namespace: bmcVersion.Namespace, Name: bmcVersion.Name}}}
		}
		if bmcVersion.Status.State == metalv1alpha1.BMCVersionStateCompleted || bmcVersion.Status.State == metalv1alpha1.BMCVersionStateFailed {
			continue
		}
		if bmcVersion.Spec.BMCRef == nil {
			if referredBMC, err := r.getBMCFromBMCVersion(ctx, &bmcVersion); err != nil && referredBMC != nil && referredBMC.Name == bmcObj.Name {
				return []ctrl.Request{{NamespacedName: types.NamespacedName{Namespace: bmcVersion.Namespace, Name: bmcVersion.Name}}}
			}
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BMCVersionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BMCVersion{}).
		Owns(&metalv1alpha1.ServerMaintenance{}).
		Watches(&metalv1alpha1.Server{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBMCVersionByServerRefs)).
		Watches(&metalv1alpha1.BMC{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBMCVersionByBMCRefs)).
		Complete(r)
}

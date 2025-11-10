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

// BMCVersionReconciler reconciles a BMCVersion object
type BMCVersionReconciler struct {
	client.Client
	ManagerNamespace string
	Insecure         bool
	Scheme           *runtime.Scheme
	BMCOptions       bmc.Options
	ResyncInterval   time.Duration
}

const (
	BMCVersionFinalizer                   = "metal.ironcore.dev/bmcversion"
	bmcVersionUpgradeIssued               = "BMCVersionUpgradeIssued"
	bmcVersionUpgradeCompleted            = "BMCVersionUpgradeCompleted"
	bmcVersionUpgradeRebootBMC            = "BMCVersionUpgradeReboot"
	bmcVersionUpgradeVerficationCondition = "BMCVersionUpgradeVerification"
)

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
		log.V(1).Info("Object is being deleted")
		return r.delete(ctx, log, bmcVersion)
	}
	return r.reconcile(ctx, log, bmcVersion)
}

func (r *BMCVersionReconciler) shouldDelete(log logr.Logger, bmcVersion *metalv1alpha1.BMCVersion) bool {
	if bmcVersion.DeletionTimestamp.IsZero() {
		return false
	}

	if controllerutil.ContainsFinalizer(bmcVersion, BMCVersionFinalizer) &&
		bmcVersion.Status.State == metalv1alpha1.BMCVersionStateInProgress {
		log.V(1).Info("postponing delete as Version update is in progress")
		return false
	}
	return true
}

func (r *BMCVersionReconciler) delete(ctx context.Context, log logr.Logger, bmcVersion *metalv1alpha1.BMCVersion) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(bmcVersion, BMCVersionFinalizer) {
		return ctrl.Result{}, nil
	}

	if bmcVersion.Status.State == metalv1alpha1.BMCVersionStateInProgress {
		log.V(1).Info("Skipping delete as version update is in progress")
		return r.reconcile(ctx, log, bmcVersion)
	}

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, bmcVersion, BMCVersionFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	log.V(1).Info("BMCVersion is deleted")
	return ctrl.Result{}, nil
}

func (r *BMCVersionReconciler) cleanupServerMaintenanceReferences(ctx context.Context, log logr.Logger, bmcVersion *metalv1alpha1.BMCVersion) error {
	if bmcVersion.Spec.ServerMaintenanceRefs == nil {
		return nil
	}
	// try to get the serverMaintenances created
	serverMaintenances, errs := r.getReferredServerMaintenances(ctx, log, bmcVersion.Spec.ServerMaintenanceRefs)

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
		// delete the serverMaintenance if not marked for deletion already
		for _, serverMaintenance := range serverMaintenances {
			if serverMaintenance.DeletionTimestamp.IsZero() && metav1.IsControlledBy(serverMaintenance, bmcVersion) {
				log.V(1).Info("Deleting server maintenance", "ServerMaintenance", serverMaintenance.Name, "State", serverMaintenance.Status.State)
				if err := r.Delete(ctx, serverMaintenance); err != nil {
					log.V(1).Info("Failed to delete server maintenance", "ServerMaintenance", serverMaintenance.Name)
					finalErr = append(finalErr, err)
				}
			} else {
				log.V(1).Info(
					"ServerMaintenance not deleted",
					"ServerMaintenance", serverMaintenance.Name,
					"State", serverMaintenance.Status.State,
					"Owner", serverMaintenance.OwnerReferences,
				)
			}
		}
	}

	if len(finalErr) == 0 {
		// all serverMaintenance are deleted
		err := r.patchMaintenanceRequestRefOnBMCVersion(ctx, log, bmcVersion, nil)
		if err != nil {
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

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, bmcVersion, BMCVersionFinalizer); err != nil || modified {
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
		log.V(1).Error(err, "failed to create BMC client", "BMC", bmcObj.Name)
		return ctrl.Result{}, err
	}
	defer bmcClient.Logout()

	switch bmcVersion.Status.State {
	case "", metalv1alpha1.BMCVersionStatePending:
		return ctrl.Result{}, r.checkVersionAndTransistionState(ctx, log, bmcVersion, bmcClient, bmcObj)
	case metalv1alpha1.BMCVersionStateInProgress:
		servers, err := r.getServers(ctx, log, bmcClient, bmcVersion)
		if err != nil {
			log.V(1).Error(err, "Failed to get ref. servers to determine maintenance state ")
			return ctrl.Result{}, err
		}

		if len(bmcVersion.Spec.ServerMaintenanceRefs) != len(servers) {
			log.V(1).Info("Not all servers have Maintenance", "ServerMaintenanceRefs", bmcVersion.Spec.ServerMaintenanceRefs, "Servers", servers)
			if requeue, err := r.requestMaintenanceOnServers(ctx, log, bmcClient, bmcVersion); err != nil || requeue {
				return ctrl.Result{}, err
			}
		}

		// check if the maintenance is granted
		if ok := r.checkIfMaintenanceGranted(ctx, log, bmcClient, bmcVersion); !ok {
			log.V(1).Info("Waiting for maintenance to be granted before continuing with updating bmc version")
			return ctrl.Result{}, err
		}

		return r.handleUpgradeInProgressState(ctx, log, bmcVersion, bmcClient, bmcObj)
	case metalv1alpha1.BMCVersionStateCompleted:
		// clean up maintenance crd and references and mark completed if version matches.
		return ctrl.Result{}, r.checkVersionAndTransistionState(ctx, log, bmcVersion, bmcClient, bmcObj)
	case metalv1alpha1.BMCVersionStateFailed:
		if shouldRetryReconciliation(bmcVersion) {
			log.V(1).Info("Retrying BMCVersion reconciliation")

			bmcVersionBase := bmcVersion.DeepCopy()
			bmcVersion.Status.State = metalv1alpha1.BMCVersionStatePending
			bmcVersion.Status.Conditions = nil
			annotations := bmcVersion.GetAnnotations()
			delete(annotations, metalv1alpha1.OperationAnnotation)
			bmcVersion.SetAnnotations(annotations)
			if err := r.Status().Patch(ctx, bmcVersion, client.MergeFrom(bmcVersionBase)); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to patch BMCVersion status for retrying: %w", err)
			}
			return ctrl.Result{}, nil
		}
		log.V(1).Info("Failed to upgrade BMCVersion", "ctx", ctx, "BMCVersion", bmcVersion, "BMC", bmcObj.Name)
		return ctrl.Result{}, nil
	}
	log.V(1).Info("Unknown State found", "BMCVersion state", bmcVersion.Status.State)
	return ctrl.Result{}, nil
}

func (r *BMCVersionReconciler) handleUpgradeInProgressState(
	ctx context.Context,
	log logr.Logger,
	bmcVersion *metalv1alpha1.BMCVersion,
	bmcClient bmc.BMC,
	BMC *metalv1alpha1.BMC,
) (ctrl.Result, error) {
	acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
	issuedCondition, err := r.getCondition(acc, bmcVersion.Status.Conditions, bmcVersionUpgradeIssued)
	if err != nil {
		return ctrl.Result{}, err
	}

	if issuedCondition.Status != metav1.ConditionTrue {
		log.V(1).Info("Issuing upgrade of BMC version")
		if BMC.Status.PowerState != metalv1alpha1.OnPowerState {
			log.V(1).Info("BMC is still powered off. waiting", "BMC", BMC.Name, "BMC power state", BMC.Status.PowerState)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, r.issueBMCUpgrade(ctx, log, bmcVersion, bmcClient, BMC, issuedCondition, acc)
	}

	completedCondition, err := r.getCondition(acc, bmcVersion.Status.Conditions, bmcVersionUpgradeCompleted)
	if err != nil {
		return ctrl.Result{}, err
	}

	if completedCondition.Status != metav1.ConditionTrue {
		log.V(1).Info("Check upgrade task of BMC")
		return r.checkBMCUpgradeStatus(ctx, log, bmcVersion, bmcClient, BMC, bmcVersion.Status.UpgradeTask.URI, completedCondition, acc)
	}

	rebootCondition, err := r.getCondition(acc, bmcVersion.Status.Conditions, bmcVersionUpgradeRebootBMC)
	if err != nil {
		return ctrl.Result{}, err
	}

	if rebootCondition.Status != metav1.ConditionTrue {
		log.V(1).Info("Reboot BMC")
		err = bmcClient.ResetManager(ctx, BMC.Spec.BMCUUID, redfish.GracefulRestartResetType)
		if err != nil {
			log.V(1).Error(err, "failed to reset BMC")
			return ctrl.Result{}, err
		}

		if err := acc.Update(
			rebootCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason("RebootOfBMCTriggered"),
			conditionutils.UpdateMessage("BMC reboot has been triggered"),
		); err != nil {
			log.V(1).Error(err, "failed to update the conditions status. retrying...")
			return ctrl.Result{}, err
		}
		err = r.updateBMCVersionStatus(
			ctx,
			log,
			bmcVersion,
			bmcVersion.Status.State,
			bmcVersion.Status.UpgradeTask,
			rebootCondition,
			acc,
		)
		return ctrl.Result{}, err
	}

	VerificationCondition, err := r.getCondition(acc, bmcVersion.Status.Conditions, bmcVersionUpgradeVerficationCondition)
	if err != nil {
		return ctrl.Result{}, err
	}

	if VerificationCondition.Status != metav1.ConditionTrue {
		log.V(1).Info("Verify BMC Version update")

		currentBMCVersion, err := r.getBMCVersionFromBMC(ctx, bmcClient, BMC)
		if err != nil {
			return ctrl.Result{}, err
		}
		if currentBMCVersion != bmcVersion.Spec.Version {
			// todo: add timeout
			log.V(1).Info("BMC version not updated", "current BMC Version", currentBMCVersion, "Required Version", bmcVersion.Spec.Version)
			if VerificationCondition.Reason == "" {
				if err := acc.Update(
					VerificationCondition,
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

		if err := acc.Update(
			VerificationCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason("VerifedBMCVersionUpdate"),
			conditionutils.UpdateMessage("BMC Version updated"),
		); err != nil {
			log.V(1).Error(err, "failed to update the conditions status. retrying...")
			return ctrl.Result{}, err
		}
		err = r.updateBMCVersionStatus(
			ctx,
			log,
			bmcVersion,
			metalv1alpha1.BMCVersionStateCompleted,
			bmcVersion.Status.UpgradeTask,
			VerificationCondition,
			acc,
		)
		return ctrl.Result{}, err
	}

	log.V(1).Info("Unknown Conditions found", "BMCVersion Conditions", bmcVersion.Status.Conditions)
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

	servers, err := r.getServers(ctx, log, bmcClient, bmcVersion)
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
				// server in maintenance for other tasks. or
				// server maintenance ref is wrong in either server or bmcVersion
				// wait for update on the server obj
				log.V(1).Info("Server is already in maintenance for other tasks",
					"Server", server.Name,
					"ServerMaintenanceRef", server.Spec.ServerMaintenanceRef,
					"BMCVersionMaintenaceRef", serverMaintenanceRef,
				)
				notInMaintenanceState[server.Name] = false
			}
		} else {
			// we still need to wait for server to enter maintenance
			// wait for update on the server obj
			log.V(1).Info("Server not yet in maintenance", "Server", server.Name, "State", server.Status.State, "MaintenanceRef", server.Spec.ServerMaintenanceRef)
			notInMaintenanceState[server.Name] = false
		}
	}

	if len(notInMaintenanceState) > 0 {
		log.V(1).Info("Some servers not yet in maintenance", "req maintenances on servers", bmcVersion.Spec.ServerMaintenanceRefs)
		return false
	}

	return true
}

func (r *BMCVersionReconciler) checkVersionAndTransistionState(
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
		if err := r.cleanupServerMaintenanceReferences(ctx, log, bmcVersion); err != nil {
			return err
		}
		log.V(1).Info("Done with BMC version upgrade", "ctx", ctx, "current BMC Version", currentBMCVersion, "BMC", BMC.Name)
		state = metalv1alpha1.BMCVersionStateCompleted
	}
	err = r.updateBMCVersionStatus(ctx, log, bmcVersion, state, nil, nil, nil)
	return err
}

func (r *BMCVersionReconciler) getCondition(acc *conditionutils.Accessor, conditions []metav1.Condition, conditionType string) (*metav1.Condition, error) {
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

func (r *BMCVersionReconciler) getReferredServerMaintenances(
	ctx context.Context,
	log logr.Logger,
	serverMaintenanceRefs []metalv1alpha1.ServerMaintenanceRefItem,
) ([]*metalv1alpha1.ServerMaintenance, []error) {
	serverMaintenances := make([]*metalv1alpha1.ServerMaintenance, 0, len(serverMaintenanceRefs))
	var errs []error
	cnt := 0
	for _, serverMaintenanceRef := range serverMaintenanceRefs {
		key := client.ObjectKey{Name: serverMaintenanceRef.ServerMaintenanceRef.Name, Namespace: r.ManagerNamespace}
		serverMaintenance := &metalv1alpha1.ServerMaintenance{}
		if err := r.Get(ctx, key, serverMaintenance); err != nil {
			log.V(1).Error(err, "failed to get referred serverMaintenance obj", serverMaintenanceRef.ServerMaintenanceRef.Name)
			errs = append(errs, err)
			continue
		}
		serverMaintenances = append(serverMaintenances, serverMaintenance)
		cnt = cnt + 1
	}

	if len(errs) > 0 {
		return serverMaintenances, errs
	}

	return serverMaintenances, nil
}

func (r *BMCVersionReconciler) getReferredSecret(
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

func (r *BMCVersionReconciler) getServers(
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
	servers, err := r.getReferredServers(ctx, log, serversRefList)
	if err != nil {
		return servers, fmt.Errorf("errors occurred during fetching servers from BMC: %v", err)
	}
	return servers, nil
}

func (r *BMCVersionReconciler) getReferredServers(
	ctx context.Context,
	log logr.Logger,
	serverRefList []*corev1.LocalObjectReference,
) ([]*metalv1alpha1.Server, error) {
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
		return bmcObj, fmt.Errorf("failed to get referred server's Manager: %w", err)
	}

	return bmcObj, nil
}

func (r *BMCVersionReconciler) getServerMaintenanceRefForServer(serverMaintenanceRefs []metalv1alpha1.ServerMaintenanceRefItem, serverMaintenanceUID types.UID) (*corev1.ObjectReference, bool) {
	for _, serverMaintenanceRef := range serverMaintenanceRefs {
		if serverMaintenanceRef.ServerMaintenanceRef.UID == serverMaintenanceUID {
			return serverMaintenanceRef.ServerMaintenanceRef, true
		}
	}
	return nil, false
}

func (r *BMCVersionReconciler) updateBMCVersionStatus(
	ctx context.Context,
	log logr.Logger,
	bmcVersion *metalv1alpha1.BMCVersion,
	state metalv1alpha1.BMCVersionState,
	upgradeTask *metalv1alpha1.Task,
	condition *metav1.Condition,
	acc *conditionutils.Accessor,
) error {
	if bmcVersion.Status.State == state && condition == nil && upgradeTask == nil {
		return nil
	}

	bmcVersionBase := bmcVersion.DeepCopy()
	bmcVersion.Status.State = state

	if condition != nil {
		if err := acc.UpdateSlice(
			&bmcVersion.Status.Conditions,
			condition.Type,
			conditionutils.UpdateStatus(condition.Status),
			conditionutils.UpdateReason(condition.Reason),
			conditionutils.UpdateMessage(condition.Message),
		); err != nil {
			return fmt.Errorf("failed to patch BMCVersion condition: %w", err)
		}
	} else {
		bmcVersion.Status.Conditions = nil
	}

	bmcVersion.Status.UpgradeTask = upgradeTask

	if err := r.Status().Patch(ctx, bmcVersion, client.MergeFrom(bmcVersionBase)); err != nil {
		return fmt.Errorf("failed to patch BMCVersion status: %w", err)
	}

	log.V(1).Info("Updated BMCVersion state ",
		"State", state,
		"Conditions", bmcVersion.Status.Conditions,
		"Upgrade Task status", bmcVersion.Status.UpgradeTask,
	)

	return nil
}

func (r *BMCVersionReconciler) patchMaintenanceRequestRefOnBMCVersion(
	ctx context.Context,
	log logr.Logger,
	bmcVersion *metalv1alpha1.BMCVersion,
	serverMaintenanceRefs []metalv1alpha1.ServerMaintenanceRefItem,
) error {
	bmcVersionsBase := bmcVersion.DeepCopy()

	if serverMaintenanceRefs == nil {
		bmcVersion.Spec.ServerMaintenanceRefs = nil
	} else {
		bmcVersion.Spec.ServerMaintenanceRefs = serverMaintenanceRefs
	}

	if err := r.Patch(ctx, bmcVersion, client.MergeFrom(bmcVersionsBase)); err != nil {
		log.V(1).Error(err, "failed to patch BMCVersion ref")
		return err
	}

	return nil
}

func (r *BMCVersionReconciler) requestMaintenanceOnServers(
	ctx context.Context,
	log logr.Logger,
	bmcClient bmc.BMC,
	bmcVersion *metalv1alpha1.BMCVersion,
) (bool, error) {

	servers, err := r.getServers(ctx, log, bmcClient, bmcVersion)
	if err != nil {
		log.V(1).Error(err, "Failed to get ref. servers to request maintenance on servers")
		return false, err
	}

	// if Server maintenance ref is already given. no further action required.
	if bmcVersion.Spec.ServerMaintenanceRefs != nil && len(bmcVersion.Spec.ServerMaintenanceRefs) == len(servers) {
		return false, nil
	}

	// if user gave some server with serverMaintenance but not all
	// we want to request maintenance for the missing servers only.
	// find the servers which has maintenance and do not create maintenance for them.
	serverWithMaintenances := make(map[string]bool, len(servers))
	if bmcVersion.Spec.ServerMaintenanceRefs != nil {
		// we fetch all the references already in the Spec (self created/provided by user)
		serverMaintenances, err := r.getReferredServerMaintenances(ctx, log, bmcVersion.Spec.ServerMaintenanceRefs)
		if err != nil {
			return false, errors.Join(err...)
		}
		for _, serverMaintenance := range serverMaintenances {
			serverWithMaintenances[serverMaintenance.Spec.ServerRef.Name] = true
		}
	}

	// we also fetch all the references owned by this Resource.
	// This is needed in case we are reconciling before we have patched the references.
	// possible when we reconcile after CreateOrPatch, before ref have been written
	serverMaintenancesList := &metalv1alpha1.ServerMaintenanceList{}
	if err := clientutils.ListAndFilterControlledBy(ctx, r.Client, bmcVersion, serverMaintenancesList); err != nil {
		return false, err
	}
	for _, serverMaintenance := range serverMaintenancesList.Items {
		serverWithMaintenances[serverMaintenance.Spec.ServerRef.Name] = true
	}

	var errs []error
	serverMaintenanceRefs := make([]metalv1alpha1.ServerMaintenanceRefItem, 0, len(servers))
	for _, server := range servers {
		if _, ok := serverWithMaintenances[server.Name]; ok {
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
			log.V(1).Error(err, "failed to create or patch serverMaintenance", "Server", server.Name)
			errs = append(errs, err)
			continue
		}
		log.V(1).Info("Created serverMaintenance", "ServerMaintenance", serverMaintenance.Name, "ServerMaintenance label", serverMaintenance.Labels, "Operation", opResult)

		serverMaintenanceRefs = append(
			serverMaintenanceRefs,
			metalv1alpha1.ServerMaintenanceRefItem{
				ServerMaintenanceRef: &corev1.ObjectReference{
					APIVersion: metalv1alpha1.GroupVersion.String(),
					Kind:       "ServerMaintenance",
					Namespace:  serverMaintenance.Namespace,
					Name:       serverMaintenance.Name,
					UID:        serverMaintenance.UID,
				}})
	}

	if len(errs) > 0 {
		return false, errors.Join(errs...)
	}

	err = r.patchMaintenanceRequestRefOnBMCVersion(ctx, log, bmcVersion, serverMaintenanceRefs)
	if err != nil {
		return false, fmt.Errorf("failed to patch serverMaintenance ref in bmcVersion status: %w", err)
	}

	log.V(1).Info("Patched serverMaintenanceMap on BMCVersion")

	return true, nil
}

func (r *BMCVersionReconciler) checkBMCUpgradeStatus(
	ctx context.Context,
	log logr.Logger,
	bmcVersion *metalv1alpha1.BMCVersion,
	bmcClient bmc.BMC,
	bmc *metalv1alpha1.BMC,
	bmcUpgradeTaskUri string,
	completedCondition *metav1.Condition,
	acc *conditionutils.Accessor,
) (ctrl.Result, error) {
	taskCurrentStatus, err := func() (*redfish.Task, error) {
		if bmcUpgradeTaskUri == "" {
			return nil, fmt.Errorf("invalid task URI. uri provided: '%v'", bmcUpgradeTaskUri)
		}
		return bmcClient.GetBMCUpgradeTask(ctx, bmc.Status.Manufacturer, bmcUpgradeTaskUri)
	}()
	if err != nil {
		log.V(1).Error(err, "failed to get the task details of bmc upgrade task", "task uri", bmcUpgradeTaskUri)
		return ctrl.Result{}, err
	}
	log.V(1).Info("bmc upgrade task current status", "Task status", taskCurrentStatus)

	upgradeCurrentTaskStatus := &metalv1alpha1.Task{
		URI:             bmcVersion.Status.UpgradeTask.URI,
		State:           taskCurrentStatus.TaskState,
		Status:          taskCurrentStatus.TaskStatus,
		PercentComplete: int32(taskCurrentStatus.PercentComplete),
	}

	// use checkpoint incase the job has stalled and we need to requeue
	transition := &conditionutils.FieldsTransition{
		IncludeStatus:  true,
		IncludeReason:  true,
		IncludeMessage: true,
	}
	checkpoint, err := transition.Checkpoint(acc, *completedCondition)
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
		if err := acc.Update(
			completedCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason("BMCUpgradeTaskFailed"),
			conditionutils.UpdateMessage(message),
		); err != nil {
			log.V(1).Error(err, "failed to update the conditions status. reconcile again ")
			return ctrl.Result{}, err
		}
		err = r.updateBMCVersionStatus(
			ctx,
			log,
			bmcVersion,
			metalv1alpha1.BMCVersionStateFailed,
			upgradeCurrentTaskStatus,
			completedCondition,
			acc,
		)
		return ctrl.Result{}, err
	}

	if taskCurrentStatus.TaskState == redfish.CompletedTaskState {
		if err := acc.Update(
			completedCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason("taskCompleted"),
			conditionutils.UpdateMessage("BMC successfully upgraded to: "+bmcVersion.Spec.Version),
		); err != nil {
			log.V(1).Error(err, "failed to update the conditions status. reconcile again")
			return ctrl.Result{}, err
		}
		err = r.updateBMCVersionStatus(
			ctx,
			log,
			bmcVersion,
			bmcVersion.Status.State,
			upgradeCurrentTaskStatus,
			completedCondition,
			acc,
		)
		return ctrl.Result{}, err
	}

	// in progress task states
	if err := acc.Update(
		completedCondition,
		conditionutils.UpdateStatus(corev1.ConditionFalse),
		conditionutils.UpdateReason(string(taskCurrentStatus.TaskState)),
		conditionutils.UpdateMessage(
			fmt.Sprintf("BMC upgrade in state: %v: PercentageCompleted %v",
				taskCurrentStatus.TaskState,
				taskCurrentStatus.PercentComplete),
		),
	); err != nil {
		log.V(1).Error(err, "failed to update the conditions status. retrying... ")
		return ctrl.Result{}, err
	}
	ok, err := checkpoint.Transitioned(acc, *completedCondition)
	if !ok && err == nil {
		log.V(1).Info("BMC upgrade task has not Progressed. retrying....")
		// the job has stalled or slow, we need to requeue with exponential backoff
		return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
	}
	// todo: Fail the state after certain timeout
	err = r.updateBMCVersionStatus(
		ctx,
		log,
		bmcVersion,
		bmcVersion.Status.State,
		upgradeCurrentTaskStatus,
		completedCondition,
		acc,
	)
	return ctrl.Result{}, err
}

func (r *BMCVersionReconciler) issueBMCUpgrade(
	ctx context.Context,
	log logr.Logger,
	bmcVersion *metalv1alpha1.BMCVersion,
	bmcClient bmc.BMC,
	bmc *metalv1alpha1.BMC,
	issuedCondition *metav1.Condition,
	acc *conditionutils.Accessor,
) error {
	password, username, err := r.getReferredSecret(ctx, log, bmcVersion.Spec.Image.SecretRef)
	if err != nil {
		log.V(1).Error(err, "failed to get secret ref for", "secretRef", bmcVersion.Spec.Image.SecretRef.Name)
		return err
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
		return bmcClient.UpgradeBMCVersion(ctx, bmc.Status.Manufacturer, parameters)
	}()

	upgradeCurrentTaskStatus := &metalv1alpha1.Task{URI: taskMonitor}

	if isFatal {
		log.V(1).Error(err, "failed to issue bmc upgrade", "requested bmc version", bmcVersion.Spec.Version, "BMC", bmc.Name)
		var errCond error
		if errCond = acc.Update(
			issuedCondition,
			conditionutils.UpdateStatus(corev1.ConditionFalse),
			conditionutils.UpdateReason("IssueBMCUpgradeFailed"),
			conditionutils.UpdateMessage("Fatal error occurred. Upgrade might still go through on server."),
		); errCond != nil {
			log.V(1).Error(errCond, "failed to update the conditions status")
		}
		err := r.updateBMCVersionStatus(
			ctx,
			log,
			bmcVersion,
			metalv1alpha1.BMCVersionStateFailed,
			upgradeCurrentTaskStatus,
			issuedCondition,
			acc,
		)
		return errors.Join(errCond, err)
	}
	if err != nil {
		log.V(1).Error(err, "failed to issue bmc upgrade", "bmc version", bmcVersion.Spec.Version, "BMC", bmc.Name)
		return err
	}
	var errCond error
	state := bmcVersion.Status.State
	if errCond = acc.Update(
		issuedCondition,
		conditionutils.UpdateStatus(corev1.ConditionTrue),
		conditionutils.UpdateReason("UpgradeIssued"),
		conditionutils.UpdateMessage(fmt.Sprintf("Task to upgrade has been created %v", taskMonitor)),
	); errCond != nil {
		log.V(1).Error(errCond, "failed to update the conditions status... retrying")
		if errCond = acc.Update(
			issuedCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason("UpgradeIssued"),
			conditionutils.UpdateMessage(fmt.Sprintf("Task to upgrade has been created %v", taskMonitor)),
		); errCond != nil {
			state = metalv1alpha1.BMCVersionStateFailed
			log.V(1).Error(errCond, "failed to update the conditions status, failing the upgrade process! BMC might still be updated to new version")
		}
	}
	err = r.updateBMCVersionStatus(
		ctx,
		log,
		bmcVersion,
		state,
		upgradeCurrentTaskStatus,
		issuedCondition,
		acc,
	)
	return errors.Join(errCond, err)
}

func (r *BMCVersionReconciler) enqueueBMCVersionByServerRefs(
	ctx context.Context,
	obj client.Object,
) []ctrl.Request {
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
		// if we dont have maintenance request on this bmcVersion we do not want to queue changes from servers.
		if bmcVersion.Spec.ServerMaintenanceRefs == nil {
			continue
		}
		if bmcVersion.Status.State == metalv1alpha1.BMCVersionStateCompleted || bmcVersion.Status.State == metalv1alpha1.BMCVersionStateFailed {
			continue
		}
		serverMaintenanceRef, ok := r.getServerMaintenanceRefForServer(bmcVersion.Spec.ServerMaintenanceRefs, host.Spec.ServerMaintenanceRef.UID)
		if ok && serverMaintenanceRef != nil {
			req = append(req, ctrl.Request{
				NamespacedName: types.NamespacedName{Namespace: bmcVersion.Namespace, Name: bmcVersion.Name},
			})
		}
	}
	return req
}

func (r *BMCVersionReconciler) enqueueBMCVersionByBMCRefs(
	ctx context.Context,
	obj client.Object,
) []ctrl.Request {

	log := ctrl.LoggerFrom(ctx)
	BMC := obj.(*metalv1alpha1.BMC)
	bmcVersionList := &metalv1alpha1.BMCVersionList{}
	if err := r.List(ctx, bmcVersionList); err != nil {
		log.Error(err, "failed to list BMCVersionList")
		return nil
	}

	for _, bmcVersion := range bmcVersionList.Items {
		if bmcVersion.Spec.BMCRef != nil && bmcVersion.Spec.BMCRef.Name == BMC.Name {
			if bmcVersion.Status.State == metalv1alpha1.BMCVersionStateCompleted || bmcVersion.Status.State == metalv1alpha1.BMCVersionStateFailed {
				return nil
			}
			return []ctrl.Request{{NamespacedName: types.NamespacedName{Namespace: bmcVersion.Namespace, Name: bmcVersion.Name}}}
		}
		if bmcVersion.Status.State == metalv1alpha1.BMCVersionStateCompleted || bmcVersion.Status.State == metalv1alpha1.BMCVersionStateFailed {
			continue
		}
		if bmcVersion.Spec.BMCRef == nil {
			if referredBMC, err := r.getBMCFromBMCVersion(ctx, &bmcVersion); err != nil && referredBMC.Name == BMC.Name {
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

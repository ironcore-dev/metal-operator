// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
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

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/controller-utils/clientutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	"github.com/stmcginnis/gofish/redfish"
)

// BMCSettingsReconciler reconciles a BMCSettings object
type BMCSettingsReconciler struct {
	client.Client
	ManagerNamespace string
	ResyncInterval   time.Duration
	Insecure         bool
	Scheme           *runtime.Scheme
	BMCOptions       bmc.Options
}

const BMCSettingFinalizer = "firmware.ironcore.dev/out-of-band-management"

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcsettings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcsettings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcsettings/finalizers,verbs=update
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch;update
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="batch",resources=jobs,verbs=get;list;watch;create;update;patch;delete

func (r *BMCSettingsReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	bmcSetting := &metalv1alpha1.BMCSettings{}
	if err := r.Get(ctx, req.NamespacedName, bmcSetting); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.V(1).Info("Reconciling BMC Settings")

	return r.reconcileExists(ctx, log, bmcSetting)
}

// Determine whether reconciliation is required. It's not required if:
// - object is being deleted;
// - object does not contain reference to server;
// - object contains reference to server, but server references to another object with lower version;
func (r *BMCSettingsReconciler) reconcileExists(
	ctx context.Context,
	log logr.Logger,
	bmcSetting *metalv1alpha1.BMCSettings,
) (ctrl.Result, error) {
	// if object is being deleted - reconcile deletion
	if !bmcSetting.DeletionTimestamp.IsZero() {
		log.V(1).Info("object is being deleted")
		return r.delete(ctx, log, bmcSetting)
	}

	return r.reconcile(ctx, log, bmcSetting)
}

func (r *BMCSettingsReconciler) delete(
	ctx context.Context,
	log logr.Logger,
	bmcSetting *metalv1alpha1.BMCSettings,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(bmcSetting, BMCSettingFinalizer) {
		return ctrl.Result{}, nil
	}
	if err := r.cleanupReferences(ctx, log, bmcSetting); err != nil {
		log.Error(err, "failed to cleanup references")
		return ctrl.Result{}, err
	}
	log.V(1).Info("ensured references were cleaned up")

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, bmcSetting, BMCSettingFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	log.V(1).Info("bmcSetting is deleted")
	return ctrl.Result{}, nil
}

func (r *BMCSettingsReconciler) cleanupServerMaintenanceReferences(
	ctx context.Context,
	log logr.Logger,
	bmcSettings *metalv1alpha1.BMCSettings,
) error {
	if bmcSettings.Spec.ServerMaintenanceRefList == nil {
		return nil
	}
	// try to get the serverMaintenances created
	serverMaintenances, errs := r.getReferredServerMaintenances(ctx, log, bmcSettings.Spec.ServerMaintenanceRefList)

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

	if len(missingServerMaintenanceRef) != len(bmcSettings.Spec.ServerMaintenanceRefList) {
		// delete the serverMaintenance if not marked for deletion already
		for _, serverMaintenance := range serverMaintenances {
			if serverMaintenance.DeletionTimestamp.IsZero() && metav1.IsControlledBy(serverMaintenance, bmcSettings) {
				log.V(1).Info("Deleting server maintenance", "serverMaintenance Name", serverMaintenance.Name, "state", serverMaintenance.Status.State)
				if err := r.Delete(ctx, serverMaintenance); err != nil {
					log.V(1).Info("Failed to delete server maintenance", "serverMaintenance Name", serverMaintenance.Name)
					finalErr = append(finalErr, err)
				}
			} else {
				log.V(1).Info(
					"server maintenance not deleted",
					"serverMaintenance Name", serverMaintenance.Name,
					"state", serverMaintenance.Status.State,
					"owner", serverMaintenance.OwnerReferences,
				)
			}
		}
	}

	if len(finalErr) == 0 {
		// all serverMaintenance are deleted
		err := r.patchMaintenanceRequestRefOnBMCSettings(ctx, log, bmcSettings, nil)
		if err != nil {
			return fmt.Errorf("failed to clean up serverMaintenance ref in bmcSetting status: %w", err)
		}
		log.V(1).Info("server maintenance ref all cleaned up")
	}
	return errors.Join(finalErr...)
}

func (r *BMCSettingsReconciler) cleanupReferences(
	ctx context.Context,
	log logr.Logger,
	bmcSetting *metalv1alpha1.BMCSettings,
) (err error) {
	if bmcSetting.Spec.BMCRef != nil {
		BMC, err := r.getBMC(ctx, log, bmcSetting)
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		// if we can not find the server, nothing else to clean up
		if apierrors.IsNotFound(err) {
			return nil
		}
		// if we have found the server, check if ref is this bmcSetting and remove it
		if err == nil {
			if BMC.Spec.BMCSettingRef != nil {
				if BMC.Spec.BMCSettingRef.Name != bmcSetting.Name {
					return nil
				}
				return r.patchBMCSettingsRefOnBMC(ctx, log, BMC, nil)
			} else {
				// nothing else to clean up
				return nil
			}
		}
	}

	return err
}

func (r *BMCSettingsReconciler) reconcile(
	ctx context.Context,
	log logr.Logger,
	bmcSetting *metalv1alpha1.BMCSettings,
) (ctrl.Result, error) {
	if shouldIgnoreReconciliation(bmcSetting) {
		log.V(1).Info("Skipped BMCSettings reconciliation")
		return ctrl.Result{}, nil
	}

	// if object does not refer to BMC object - stop reconciliation
	// todo length
	if bmcSetting.Spec.BMCRef == nil {
		log.V(1).Info("object does not refer to BMC object")
		return ctrl.Result{}, nil
	}

	// if referred BMC contains reference to different BMCSettings object - stop reconciliation
	BMC, err := r.getBMC(ctx, log, bmcSetting)
	if err != nil {
		log.V(1).Info("referred server object could not be fetched")
		return ctrl.Result{}, err
	}
	// patch BMC with BMCSettings reference
	if BMC.Spec.BMCSettingRef == nil {
		if err := r.patchBMCSettingsRefOnBMC(ctx, log, BMC, &corev1.LocalObjectReference{Name: bmcSetting.Name}); err != nil {
			return ctrl.Result{}, err
		}
	} else if BMC.Spec.BMCSettingRef.Name != bmcSetting.Name {
		referredBMCSettings, err := r.getReferredBMCSettings(ctx, log, BMC.Spec.BMCSettingRef)
		if err != nil {
			log.V(1).Info("referred server contains reference to different BMCSettings object, unable to fetch the referenced BMCSettings")
			return ctrl.Result{}, err
		}
		// check if the current BMCSettings version is newer and update reference if it is newer
		// todo : handle version checks correctly
		if referredBMCSettings.Spec.Version < bmcSetting.Spec.Version {
			log.V(1).Info("Updating BMCSettings reference to the latest BMC version")
			if err := r.patchBMCSettingsRefOnBMC(ctx, log, BMC, &corev1.LocalObjectReference{Name: bmcSetting.Name}); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, bmcSetting, BMCSettingFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	return r.ensureBMCSettingsMaintenanceStateTransition(ctx, log, bmcSetting, BMC)
}

func (r *BMCSettingsReconciler) ensureBMCSettingsMaintenanceStateTransition(
	ctx context.Context,
	log logr.Logger,
	bmcSetting *metalv1alpha1.BMCSettings,
	BMC *metalv1alpha1.BMC,
) (ctrl.Result, error) {
	bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, r.Client, BMC, r.Insecure, r.BMCOptions)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create BMC client: %w", err)
	}
	defer bmcClient.Logout()
	switch bmcSetting.Status.State {
	case "", metalv1alpha1.BMCSettingsStatePending:
		//todo: check that in initial state there is no pending BMCSettings maintenance left behind,

		err := r.updateBMCSettingsStatus(ctx, log, bmcSetting, metalv1alpha1.BMCSettingsStateInProgress)
		return ctrl.Result{}, err
	case metalv1alpha1.BMCSettingsStateInProgress:
		return r.handleSettingInProgressState(ctx, log, bmcSetting, BMC, bmcClient)
	case metalv1alpha1.BMCSettingsStateApplied:
		return ctrl.Result{}, r.handleSettingAppliedState(ctx, log, bmcSetting, BMC, bmcClient)
	case metalv1alpha1.BMCSettingsStateFailed:
		r.handleFailedState(ctx, log, bmcSetting, BMC)
		return ctrl.Result{}, nil
	}
	log.V(1).Info("Unknown State found", "BMCSettings state", bmcSetting.Status.State)
	return ctrl.Result{}, nil
}

func (r *BMCSettingsReconciler) handleSettingInProgressState(
	ctx context.Context,
	log logr.Logger,
	bmcSetting *metalv1alpha1.BMCSettings,
	BMC *metalv1alpha1.BMC,
	bmcClient bmc.BMC,
) (ctrl.Result, error) {
	currentBMCVersion, settingsDiff, err := r.getBMCVersionAndSettingsDifference(ctx, log, bmcSetting, BMC, bmcClient)

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get BMC settings: %w", err)
	}
	// if setting is not different, complete the BMCSettings tasks
	if len(settingsDiff) == 0 {
		// move status to completed
		err := r.updateBMCSettingsStatus(ctx, log, bmcSetting, metalv1alpha1.BMCSettingsStateApplied)
		return ctrl.Result{}, err
	}

	// todo:wait on the result from the resource which does upgrade to requeue.
	if currentBMCVersion != bmcSetting.Spec.Version {
		log.V(1).Info("Pending BMC version upgrade.", "current bmc Version", currentBMCVersion, "required version", bmcSetting.Spec.Version)
		return ctrl.Result{}, nil
	}

	if req, err := r.requestMaintenanceOnServers(ctx, log, bmcSetting, bmcClient); err != nil || req {
		return ctrl.Result{}, err
	}

	// check if the maintenance is granted
	if ok := r.checkIfMaintenanceGranted(ctx, log, bmcSetting, bmcClient); !ok {
		log.V(1).Info("Waiting for maintenance to be granted before continuing with updating settings", "reason", err)
		return ctrl.Result{}, err
	}

	return r.updateSettingsAndVerify(ctx, log, bmcSetting, BMC, settingsDiff, bmcClient)
}

func (r *BMCSettingsReconciler) updateSettingsAndVerify(
	ctx context.Context,
	log logr.Logger,
	bmcSetting *metalv1alpha1.BMCSettings,
	BMC *metalv1alpha1.BMC,
	settingsDiff redfish.SettingsAttributes,
	bmcClient bmc.BMC,
) (ctrl.Result, error) {

	if BMC.Status.PowerState != metalv1alpha1.OnPowerState {
		log.V(1).Info("BMC is not turned On. Can not proceed")
		err := r.updateBMCSettingsStatus(ctx, log, bmcSetting, metalv1alpha1.BMCSettingsStateFailed)
		return ctrl.Result{}, err
	}

	pendingAttr, err := bmcClient.GetBMCPendingAttributeValues(ctx, BMC.Spec.BMCUUID)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to check pending BMC settings: %w", err)
	}

	if len(pendingAttr) == 0 {
		resetBMCReq, err := bmcClient.CheckBMCAttributes(BMC.Spec.BMCUUID, settingsDiff)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to check BMC settings provided: %w", err)
		}

		err = bmcClient.SetBMCAttributesImediately(ctx, BMC.Spec.BMCUUID, settingsDiff)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to set BMC settings: %w", err)
		}

		if resetBMCReq {
			err = bmcClient.ResetManager(ctx, BMC.Spec.BMCUUID, redfish.GracefulRestartResetType)
			if err != nil {
				log.V(1).Error(err, "failed to reset BMC")
				return ctrl.Result{}, err
			}
		}
	}

	// verify setting already applied
	_, settingsDiff, err = r.getBMCVersionAndSettingsDifference(ctx, log, bmcSetting, BMC, bmcClient)

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get BMC settings: %w", err)
	}
	// if setting is not different, complete the BMC settings tasks
	if len(settingsDiff) == 0 {
		// move  bmcSetting state to completed, and revert the settingUpdate state to initial
		err := r.updateBMCSettingsStatus(ctx, log, bmcSetting, metalv1alpha1.BMCSettingsStateApplied)
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil

}

func (r *BMCSettingsReconciler) handleSettingAppliedState(
	ctx context.Context,
	log logr.Logger,
	bmcSetting *metalv1alpha1.BMCSettings,
	BMC *metalv1alpha1.BMC,
	bmcClient bmc.BMC,
) error {
	// clean up maintenance crd and references.
	if err := r.cleanupServerMaintenanceReferences(ctx, log, bmcSetting); err != nil {
		return err
	}

	_, settingsDiff, err := r.getBMCVersionAndSettingsDifference(ctx, log, bmcSetting, BMC, bmcClient)

	if err != nil {
		log.V(1).Error(err, "unable to fetch and check BMCSettings")
		return err
	}
	if len(settingsDiff) > 0 {
		err := r.updateBMCSettingsStatus(ctx, log, bmcSetting, "")
		return err
	}

	log.V(1).Info("Done with BMC setting update", "ctx", ctx, "bmcSetting", bmcSetting, "bmc", BMC)
	return nil
}

func (r *BMCSettingsReconciler) handleFailedState(
	ctx context.Context,
	log logr.Logger,
	bmcSetting *metalv1alpha1.BMCSettings,
	BMC *metalv1alpha1.BMC,
) {
	log.V(1).Info("Handle failed setting update with no maintenance reference")
	// todo: revisit this logic to either create maintenance if not present, put server in Error state on failed bmc settings maintenance
	log.V(1).Info("Failed to update BMC setting", "ctx", ctx, "bmcSetting", bmcSetting, "BMC", BMC)
}

func (r *BMCSettingsReconciler) getBMCVersionAndSettingsDifference(
	ctx context.Context,
	log logr.Logger,
	bmcSetting *metalv1alpha1.BMCSettings,
	BMC *metalv1alpha1.BMC,
	bmcClient bmc.BMC,
) (currentBMCVersion string, diff redfish.SettingsAttributes, err error) {
	keys := slices.Collect(maps.Keys(bmcSetting.Spec.SettingsMap))

	currentSettings, err := bmcClient.GetBMCAttributeValues(ctx, BMC.Spec.BMCUUID, keys)
	if err != nil {
		log.V(1).Info("Failed to get with BMC setting", "error", err)
		return currentBMCVersion, diff, fmt.Errorf("failed to get BMC settings: %w", err)
	}

	diff = redfish.SettingsAttributes{}
	var errs []error
	for key, value := range bmcSetting.Spec.SettingsMap {
		res, ok := currentSettings[key]
		if ok {
			switch data := res.(type) {
			case int:
				intvalue, err := strconv.Atoi(value)
				if err != nil {
					log.V(1).Info("Failed to check type for", "Setting name", key, "setting value", value, "error", err)
					errs = append(errs, fmt.Errorf("failed to check type for name %v; value %v; error: %v", key, value, err))
					continue
				}
				if data != intvalue {
					diff[key] = intvalue
				}
			case string:
				if data != value {
					diff[key] = value
				}
			case float64:
				floatvalue, err := strconv.ParseFloat(value, 64)
				if err != nil {
					log.V(1).Info("Failed to check type for", "Setting name", key, "setting value", value, "error", err)
					errs = append(errs, fmt.Errorf("failed to check type for name %v; value %v; error: %v", key, value, err))
				}
				if data != floatvalue {
					diff[key] = floatvalue
				}
			}
		} else {
			diff[key] = value
		}
	}

	if len(errs) > 0 {
		return currentBMCVersion, diff, fmt.Errorf("failed to find diff for some BMC settings: %v", errs)
	}

	// fetch the current BMC version from the server bmc
	currentBMCVersion, err = bmcClient.GetBMCVersion(ctx, BMC.Spec.BMCUUID)
	if err != nil {
		return currentBMCVersion, diff, fmt.Errorf("failed to load BMC version: %w for BMC %v", err, BMC.Name)
	}

	return currentBMCVersion, diff, nil
}

func (r *BMCSettingsReconciler) checkIfMaintenanceGranted(
	ctx context.Context,
	log logr.Logger,
	bmcSetting *metalv1alpha1.BMCSettings,
	bmcClient bmc.BMC,
) bool {
	if bmcSetting.Spec.ServerMaintenanceRefList == nil {
		return false
	}

	servers, err := r.getServers(ctx, log, bmcSetting, bmcClient)
	if err != nil {
		log.V(1).Error(err, "Failed to get ref. servers to determine maintenance state ")
		return false
	}

	if len(bmcSetting.Spec.ServerMaintenanceRefList) != len(servers) {
		log.V(1).Info("Not all servers have Maintenance", "serverMaintenanceRefList", bmcSetting.Spec.ServerMaintenanceRefList, "servers", servers)
		return false
	}

	notInMaintenanceState := make([]string, 0, len(servers))
	for _, server := range servers {
		if server.Status.State == metalv1alpha1.ServerStateMaintenance {
			serverMaintenanceRef := r.getServerMaintenanceRefForServer(bmcSetting.Spec.ServerMaintenanceRefList, server.Name)
			if server.Spec.ServerMaintenanceRef == nil || server.Spec.ServerMaintenanceRef.UID != serverMaintenanceRef.UID {
				// server in maintenance for other tasks. or
				// server maintenance ref is wrong in either server or bmcSetting
				// wait for update on the server obj
				log.V(1).Info("Server is already in maintenance for other tasks",
					"Server", server.Name,
					"serverMaintenanceRef", server.Spec.ServerMaintenanceRef,
					"bmcSettingMaintenaceRef", serverMaintenanceRef,
				)
				notInMaintenanceState = append(notInMaintenanceState, server.Name)
			}
		} else {
			// we still need to wait for server to enter maintenance
			// wait for update on the server obj
			log.V(1).Info("Server not yet in maintenance", "Server", server.Name, "State", server.Status.State, "MaintenanceRef", server.Spec.ServerMaintenanceRef)
			notInMaintenanceState = append(notInMaintenanceState, server.Name)
		}
	}

	if len(notInMaintenanceState) > 0 {
		log.V(1).Info("some servers not yet in maintenance",
			"req maintenances on servers", bmcSetting.Spec.ServerMaintenanceRefList,
			"servers not in maintence", notInMaintenanceState)
		return false
	}

	return true
}

func (r *BMCSettingsReconciler) requestMaintenanceOnServers(
	ctx context.Context,
	log logr.Logger,
	bmcSetting *metalv1alpha1.BMCSettings,
	bmcClient bmc.BMC,
) (bool, error) {

	servers, err := r.getServers(ctx, log, bmcSetting, bmcClient)
	if err != nil {
		log.V(1).Error(err, "Failed to get ref. servers to request maintenance on servers")
		return false, err
	}

	// if Server maintenance ref is already given. no further action required.
	if bmcSetting.Spec.ServerMaintenanceRefList != nil && len(bmcSetting.Spec.ServerMaintenanceRefList) == len(servers) {
		return false, nil
	}

	var errs []error
	serverMaintenanceRefList := make([]metalv1alpha1.ServerMaintenanceRefItem, 0, len(servers))
	for _, server := range servers {
		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: r.ManagerNamespace,
				Name:      fmt.Sprintf("%s-%s", bmcSetting.Name, server.Name),
			}}

		opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, serverMaintenance, func() error {
			serverMaintenance.Spec.Policy = bmcSetting.Spec.ServerMaintenancePolicy
			serverMaintenance.Spec.ServerPower = metalv1alpha1.PowerOn
			serverMaintenance.Spec.ServerRef = &corev1.LocalObjectReference{Name: server.Name}
			if serverMaintenance.Status.State != metalv1alpha1.ServerMaintenanceStateInMaintenance && serverMaintenance.Status.State != "" {
				serverMaintenance.Status.State = ""
			}
			return controllerutil.SetControllerReference(bmcSetting, serverMaintenance, r.Client.Scheme())
		})
		if err != nil {
			log.V(1).Info("failed to create or patch serverMaintenance for server %v: \nError: %w", server.Name, err)
			errs = append(errs, err)
			continue
		}
		log.V(1).Info("Created serverMaintenance", "serverMaintenance", serverMaintenance.Name, "serverMaintenance label", serverMaintenance.Labels, "Operation", opResult)

		serverMaintenanceRefList = append(
			serverMaintenanceRefList,
			metalv1alpha1.ServerMaintenanceRefItem{
				ServerName: server.Name,
				ServerMaintenanceRef: &corev1.ObjectReference{
					APIVersion: serverMaintenance.GroupVersionKind().GroupVersion().String(),
					Kind:       "ServerMaintenance",
					Namespace:  serverMaintenance.Namespace,
					Name:       serverMaintenance.Name,
					UID:        serverMaintenance.UID,
				}})
	}

	if len(errs) > 0 {
		return false, errors.Join(errs...)
	}

	err = r.patchMaintenanceRequestRefOnBMCSettings(ctx, log, bmcSetting, serverMaintenanceRefList)
	if err != nil {
		return false, fmt.Errorf("failed to patch serverMaintenance ref in bmcSetting status: %w", err)
	}

	log.V(1).Info("Patched serverMaintenanceMap on bmcSetting")

	return true, nil
}

func (r *BMCSettingsReconciler) getBMC(
	ctx context.Context,
	log logr.Logger,
	bmcSetting *metalv1alpha1.BMCSettings,
) (*metalv1alpha1.BMC, error) {

	var refName string
	if bmcSetting.Spec.BMCRef == nil {
		return nil, fmt.Errorf("bmc ref not provided")
	} else {
		refName = bmcSetting.Spec.BMCRef.Name
	}

	key := client.ObjectKey{Name: refName}
	BMC := &metalv1alpha1.BMC{}
	if err := r.Get(ctx, key, BMC); err != nil {
		log.V(1).Error(err, "failed to get referred server's Manager")
		return BMC, err
	}

	return BMC, nil
}

func (r *BMCSettingsReconciler) getServers(
	ctx context.Context,
	log logr.Logger,
	bmcSetting *metalv1alpha1.BMCSettings,
	bmcClient bmc.BMC,
) ([]*metalv1alpha1.Server, error) {
	if bmcSetting.Spec.BMCRef == nil {
		return nil, fmt.Errorf("BMC reference not found")
	}
	BMC, err := r.getBMC(ctx, log, bmcSetting)

	if err != nil {
		log.V(1).Error(err, "failed to get referred BMC")
		return nil, err
	}
	bmcServers, err := bmcClient.GetSystems(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get servers from BMC %s: %w", BMC.Name, err)
	}
	serversRefList := make([]*corev1.LocalObjectReference, len(bmcServers))
	for i := range bmcServers {
		serversRefList[i] = &corev1.LocalObjectReference{Name: bmcutils.GetServerNameFromBMCandIndex(i, BMC)}
	}
	servers, err := r.getReferredServers(ctx, log, serversRefList)
	if err != nil {
		return servers, fmt.Errorf("errors occurred during fetching servers from BMC: %v", err)
	}
	return servers, nil
}

func (r *BMCSettingsReconciler) getReferredServers(
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

func (r *BMCSettingsReconciler) getReferredServerMaintenances(
	ctx context.Context,
	log logr.Logger,
	serverMaintenanceRefList []metalv1alpha1.ServerMaintenanceRefItem,
) ([]*metalv1alpha1.ServerMaintenance, []error) {

	serverMaintenances := make([]*metalv1alpha1.ServerMaintenance, 0, len(serverMaintenanceRefList))
	var errs []error
	cnt := 0
	for _, serverMaintenanceRef := range serverMaintenanceRefList {
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

func (r *BMCSettingsReconciler) getReferredBMCSettings(
	ctx context.Context,
	log logr.Logger,
	referredBMCSettingsRef *corev1.LocalObjectReference,
) (*metalv1alpha1.BMCSettings, error) {
	key := client.ObjectKey{Name: referredBMCSettingsRef.Name, Namespace: metav1.NamespaceNone}
	bmcSetting := &metalv1alpha1.BMCSettings{}
	if err := r.Get(ctx, key, bmcSetting); err != nil {
		log.V(1).Error(err, "failed to get referred bmcSetting")
		return bmcSetting, err
	}
	return bmcSetting, nil
}

func (r *BMCSettingsReconciler) getServerMaintenanceRefForServer(
	serverMaintenanceRefList []metalv1alpha1.ServerMaintenanceRefItem,
	serverName string,
) *corev1.ObjectReference {
	for _, serverMaintenanceRef := range serverMaintenanceRefList {
		if serverMaintenanceRef.ServerName == serverName {
			return serverMaintenanceRef.ServerMaintenanceRef
		}
	}
	return nil
}

func (r *BMCSettingsReconciler) patchBMCSettingsRefOnBMC(
	ctx context.Context,
	log logr.Logger,
	BMC *metalv1alpha1.BMC,
	BMCSettingsReference *corev1.LocalObjectReference,
) error {
	if BMC.Spec.BMCSettingRef == BMCSettingsReference {
		return nil
	}

	var err error
	BMCBase := BMC.DeepCopy()
	BMC.Spec.BMCSettingRef = BMCSettingsReference
	if err = r.Patch(ctx, BMC, client.MergeFrom(BMCBase)); err != nil {
		log.V(1).Error(err, "failed to patch BMC settings ref")
		return err
	}
	return err
}

func (r *BMCSettingsReconciler) patchMaintenanceRequestRefOnBMCSettings(
	ctx context.Context,
	log logr.Logger,
	bmcSetting *metalv1alpha1.BMCSettings,
	serverMaintenanceRefList []metalv1alpha1.ServerMaintenanceRefItem,
) error {
	BMCSettingsBase := bmcSetting.DeepCopy()

	if serverMaintenanceRefList == nil {
		bmcSetting.Spec.ServerMaintenanceRefList = nil
	} else {
		bmcSetting.Spec.ServerMaintenanceRefList = serverMaintenanceRefList
	}

	if err := r.Patch(ctx, bmcSetting, client.MergeFrom(BMCSettingsBase)); err != nil {
		log.V(1).Error(err, "failed to patch BMCSettings ref")
		return err
	}

	return nil
}

func (r *BMCSettingsReconciler) updateBMCSettingsStatus(
	ctx context.Context,
	log logr.Logger,
	bmcSetting *metalv1alpha1.BMCSettings,
	state metalv1alpha1.BMCSettingsState,
) error {

	if bmcSetting.Status.State == state {
		return nil
	}

	BMCSettingsBase := bmcSetting.DeepCopy()
	bmcSetting.Status.State = state

	if err := r.Status().Patch(ctx, bmcSetting, client.MergeFrom(BMCSettingsBase)); err != nil {
		return fmt.Errorf("failed to patch bmcSetting status: %w", err)
	}

	log.V(1).Info("Updated bmcSetting state ", "new state", state)

	return nil
}

func (r *BMCSettingsReconciler) enqueueBMCSettingsByServerRefs(
	ctx context.Context,
	obj client.Object,
) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)
	host := obj.(*metalv1alpha1.Server)

	// return early if hosts are not required states
	if host.Status.State != metalv1alpha1.ServerStateMaintenance {
		return nil
	}

	bmcSettingsList := &metalv1alpha1.BMCSettingsList{}
	if err := r.List(ctx, bmcSettingsList); err != nil {
		log.Error(err, "failed to list BMCSettings")
		return nil
	}
	var req []ctrl.Request

	for _, bmcSetting := range bmcSettingsList.Items {
		// if we dont have maintenance request on this bmcsetting we do not want to queue changes from servers.
		if bmcSetting.Spec.ServerMaintenanceRefList == nil {
			continue
		}
		if bmcSetting.Status.State == metalv1alpha1.BMCSettingsStateApplied || bmcSetting.Status.State == metalv1alpha1.BMCSettingsStateFailed {
			continue
		}
		serverMaintenanceRef := r.getServerMaintenanceRefForServer(bmcSetting.Spec.ServerMaintenanceRefList, host.Name)
		if serverMaintenanceRef != nil {
			req = append(req, ctrl.Request{
				NamespacedName: types.NamespacedName{Namespace: bmcSetting.Namespace, Name: bmcSetting.Name},
			})
		}
	}
	return req
}

func (r *BMCSettingsReconciler) enqueueBMCSettingsByBMCRefs(
	ctx context.Context,
	obj client.Object,
) []ctrl.Request {

	log := ctrl.LoggerFrom(ctx)
	BMC := obj.(*metalv1alpha1.BMC)
	bmcSettingsList := &metalv1alpha1.BMCSettingsList{}
	if err := r.List(ctx, bmcSettingsList); err != nil {
		log.Error(err, "failed to list BMCSettingsList")
		return nil
	}

	for _, bmcSetting := range bmcSettingsList.Items {
		if bmcSetting.Spec.BMCRef != nil && bmcSetting.Spec.BMCRef.Name == BMC.Name {
			if bmcSetting.Status.State == metalv1alpha1.BMCSettingsStateApplied || bmcSetting.Status.State == metalv1alpha1.BMCSettingsStateFailed {
				return nil
			}
			return []ctrl.Request{{NamespacedName: types.NamespacedName{Namespace: bmcSetting.Namespace, Name: bmcSetting.Name}}}
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BMCSettingsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BMCSettings{}).
		Owns(&metalv1alpha1.ServerMaintenance{}).
		Watches(&metalv1alpha1.Server{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBMCSettingsByServerRefs)).
		Watches(&metalv1alpha1.BMC{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBMCSettingsByBMCRefs)).
		Complete(r)
}

// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ironcore-dev/controller-utils/clientutils"
	"github.com/ironcore-dev/controller-utils/conditionutils"
	"github.com/ironcore-dev/controller-utils/metautils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	metalmetrics "github.com/ironcore-dev/metal-operator/internal/metrics"
	"github.com/ironcore-dev/metal-operator/pkg/bmcutils"
	"github.com/stmcginnis/gofish/schemas"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// ServerFinalizer is the finalizer for the server
	ServerFinalizer = "metal.ironcore.dev/server"
)

const (
	// powerOpOn is the power on operation
	powerOpOn = "PowerOn"
	// powerOpOff is the power off operation
	powerOpOff = "PowerOff"
	// powerOpNoOP is the no operation
	powerOpNoOP = "NoOp"
)

// ServerReconciler reconciles a Server object
type ServerReconciler struct {
	client.Client
	APIReader               client.Reader
	Scheme                  *runtime.Scheme
	Recorder                events.EventRecorder
	DefaultProtocol         metalv1alpha1.ProtocolScheme
	SkipCertValidation      bool
	ManagerNamespace        string
	EnforcePowerOff         bool
	ResyncInterval          time.Duration
	BMCOptions              bmc.Options
	MaxConcurrentReconciles int
	Conditions              *conditionutils.Accessor
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcs,verbs=get;list;watch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcsecrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=endpoints,verbs=get;list;watch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="batch",resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, req.NamespacedName, server); err != nil {
		if !apierrors.IsNotFound(err) {
			metalmetrics.ServerReconciliationTotal.WithLabelValues("error_fetch").Inc()
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	result, err := r.reconcileExists(ctx, server)

	// Record reconciliation result
	if err != nil {
		metalmetrics.ServerReconciliationTotal.WithLabelValues("error_reconcile").Inc()
	} else {
		metalmetrics.ServerReconciliationTotal.WithLabelValues("success").Inc()
	}

	return result, err
}

func (r *ServerReconciler) reconcileExists(ctx context.Context, server *metalv1alpha1.Server) (ctrl.Result, error) {
	if !server.DeletionTimestamp.IsZero() {
		return r.delete(ctx, server)
	}
	return r.reconcile(ctx, server)
}

func (r *ServerReconciler) delete(ctx context.Context, server *metalv1alpha1.Server) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	if !controllerutil.ContainsFinalizer(server, ServerFinalizer) {
		return ctrl.Result{}, nil
	}

	log.V(1).Info("Deleting server")

	if server.Spec.BootConfigurationRef != nil {
		if err := r.Delete(ctx, &metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: server.Spec.BootConfigurationRef.Namespace,
				Name:      server.Spec.BootConfigurationRef.Name,
			}}); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("failed to delete server bootconfiguration: %w", err)
		}
		log.V(1).Info("Deleted server boot configuration")
	}

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, server, ServerFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Ensured that the finalizer has been removed")
	log.V(1).Info("Deleted server")
	return ctrl.Result{}, nil
}

func (r *ServerReconciler) reconcile(ctx context.Context, server *metalv1alpha1.Server) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Reconciling Server")
	if shouldIgnoreReconciliation(server) {
		log.V(1).Info("Skipped Server reconciliation")
		return ctrl.Result{}, nil
	}

	// do late state initialization
	if server.Status.State == "" {
		state := metalv1alpha1.ServerStateAvailable
		if server.Spec.ServerClaimRef != nil {
			state = metalv1alpha1.ServerStateReserved
		}
		if modified, err := r.patchServerState(ctx, server, state); err != nil || modified {
			return ctrl.Result{}, err
		}
	}

	bmcClient, err := bmcutils.GetBMCClientForServer(ctx, r.Client, server, r.DefaultProtocol, r.SkipCertValidation, r.BMCOptions)
	if err != nil {
		if errors.As(err, &bmcutils.BMCUnAvailableError{}) {
			log.V(1).Info("BMC is not available, skipping", "BMC", server.Spec.BMCRef.Name, "Server", server.Name, "error", err)
			return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get BMC client for server: %w", err)
	}
	defer bmcClient.Logout()

	if modified, err := r.patchServerURI(ctx, bmcClient, server); err != nil || modified {
		return ctrl.Result{}, err
	}

	if result, modified, err := r.handleAnnotationOperations(ctx, bmcClient, server); err != nil || modified {
		return result, err
	}
	log.V(1).Info("Handled annotation operations")

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, server, ServerFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Ensured finalizer has been added")

	if err := r.updateServerStatus(ctx, bmcClient, server); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update server status: %w", err)
	}

	if err := r.applyBootOrder(ctx, bmcClient, server); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to apply server BIOS boot order: %w", err)
	}

	_, err = r.ensureServerStateTransition(ctx, bmcClient, server)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure server state transition: %w", err)
	}
	log.V(1).Info("Updating Server status after state transition")
	// we need to update the ServerStatus after state transition to make sure it reflects the changes done
	if err := r.updateServerStatus(ctx, bmcClient, server); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update server status: %w", err)
	}

	log.V(1).Info("Reconciled Server")
	return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
}

// Server state-machine:
//
// A Server starts in the Available state (or Reserved when created with a claim).
//
// Available:
// In the available state, a Server can be claimed by a ServerClaim. Here the claim reconciler takes over to
// generate the necessary boot configuration. In the available state the power state and indicator LEDs are being controlled.
//
// Reserved:
// A Server in a reserved state can not be claimed by another claim and is powered on with the claim's boot configuration.
//
// Released:
// A released Server is powered off and held until its claim reference is removed, then it transitions back to Available.
//
// Parked:
// An overlay state for external day-2 operations; normal state-machine progression, boot, and power healing are suspended.
func (r *ServerReconciler) ensureServerStateTransition(ctx context.Context, bmcClient bmc.BMC, server *metalv1alpha1.Server) (bool, error) {
	switch server.Status.State {
	case metalv1alpha1.ServerStateAvailable:
		return r.handleAvailableState(ctx, bmcClient, server)
	case metalv1alpha1.ServerStateReserved:
		return r.handleReservedState(ctx, bmcClient, server)
	case metalv1alpha1.ServerStateReleased:
		return r.handleReleasedState(ctx, bmcClient, server)
	case metalv1alpha1.ServerStateParked:
		return r.resumeParkedServer(ctx, bmcClient, server)
	default:
		return false, nil
	}
}

func (r *ServerReconciler) handleAvailableState(ctx context.Context, bmcClient bmc.BMC, server *metalv1alpha1.Server) (bool, error) {
	log := ctrl.LoggerFrom(ctx)
	if server.Status.Manufacturer == "" {
		if err := r.updateServerStatusFromSystemInfo(ctx, bmcClient, server); err != nil {
			return false, fmt.Errorf("failed to update server status system info: %w", err)
		}
		log.V(1).Info("Updated Server status system info")
	}
	if server.Status.PowerState != metalv1alpha1.ServerOffPowerState {
		if err := r.ensureServerPowerState(ctx, bmcClient, server, metalv1alpha1.PowerOff); err != nil {
			return false, fmt.Errorf("failed to ensure server power state: %w", err)
		}
		log.V(1).Info("Server state set to power off")
	}

	if err := r.ensureIndicatorLED(ctx, bmcClient, server); err != nil {
		return false, fmt.Errorf("failed to ensure server indicator led: %w", err)
	}

	// Re-fetch directly from the API server before checking ServerClaimRef.
	// The object passed into this handler may be from the informer cache and
	// could be stale if a ServerClaim controller just wrote the ref. Without
	// this, a BMC-triggered reconcile that arrives with a stale cache snapshot
	// would skip the Reserved transition even though the claim already landed.
	fresh := &metalv1alpha1.Server{}
	if err := r.APIReader.Get(ctx, client.ObjectKeyFromObject(server), fresh); err != nil {
		return false, client.IgnoreNotFound(err)
	}
	*server = *fresh

	if server.Spec.ServerClaimRef != nil {
		if modified, err := r.patchServerState(ctx, server, metalv1alpha1.ServerStateReserved); err != nil || modified {
			return true, err
		}
	}
	log.V(1).Info("Reconciled available state")
	return true, nil
}

func (r *ServerReconciler) handleReservedState(ctx context.Context, bmcClient bmc.BMC, server *metalv1alpha1.Server) (bool, error) {
	log := ctrl.LoggerFrom(ctx)
	serverClaimRef := server.Spec.ServerClaimRef
	if serverClaimRef == nil {
		if modified, err := r.patchServerState(ctx, server, metalv1alpha1.ServerStateAvailable); err != nil || modified {
			return true, err
		}
		return true, nil
	}

	log = log.WithValues("ServerClaimRef", serverClaimRef)

	claim := &metalv1alpha1.ServerClaim{}
	claimKey := client.ObjectKey{
		Name:      server.Spec.ServerClaimRef.Name,
		Namespace: server.Spec.ServerClaimRef.Namespace,
	}
	if err := r.Get(ctx, claimKey, claim); err != nil {
		if !apierrors.IsNotFound(err) {
			return false, fmt.Errorf("getting server claim %s: %w", claimKey, err)
		}

		// Don't release the server until the BMC has confirmed it's off.
		off, err := r.ensureServerPoweredOff(ctx, bmcClient, server)
		if err != nil {
			return false, err
		}
		if !off {
			return true, nil
		}

		switch server.Spec.ReclaimPolicy {
		case metalv1alpha1.ServerReclaimPolicyRetain:
			log.V(1).Info("Transitioning server to released state")
			return r.patchServerState(ctx, server, metalv1alpha1.ServerStateReleased)
		case metalv1alpha1.ServerReclaimPolicyRecycle:
			log.V(1).Info("Server claim not found, releasing server")
			serverBase := server.DeepCopy()
			server.Spec.ServerClaimRef = nil
			if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
				return false, fmt.Errorf("failed to remove ServerClaimRef: %w", err)
			}
			return false, nil
		default:
			return false, reconcile.TerminalError(fmt.Errorf("unknown reclaim policy %q", server.Spec.ReclaimPolicy))
		}
	}

	if ready, err := r.serverBootConfigurationIsReady(ctx, server); err != nil || !ready {
		log.V(1).Info("Server boot configuration is not ready, retrying")
		return true, err
	}
	log.V(1).Info("Server boot configuration is ready")

	// TODO: handle working Reserved Server that was suddenly powered off but needs to boot from disk
	if server.Status.PowerState == metalv1alpha1.ServerOffPowerState {
		if err := r.pxeBootServer(ctx, bmcClient, server); err != nil {
			return false, fmt.Errorf("failed to boot server: %w", err)
		}
		log.V(1).Info("Server is powered off, booting Server in PXE")
	}
	if err := r.ensureServerPowerState(ctx, bmcClient, server, claim.Spec.Power); err != nil {
		return false, fmt.Errorf("failed to ensure server power state: %w", err)
	}

	if err := r.ensureIndicatorLED(ctx, bmcClient, server); err != nil {
		return false, fmt.Errorf("failed to ensure server indicator led: %w", err)
	}
	log.V(1).Info("Reconciled reserved state")
	return true, nil
}

func (r *ServerReconciler) ensureServerPoweredOff(ctx context.Context, bmcClient bmc.BMC, server *metalv1alpha1.Server) (off bool, err error) {
	log := ctrl.LoggerFrom(ctx)

	if server.Spec.BootConfigurationRef != nil {
		base := server.DeepCopy()
		server.Spec.BootConfigurationRef = nil
		if err := r.Patch(ctx, server, client.MergeFrom(base)); err != nil {
			return false, fmt.Errorf("failed to clear spec.bootConfigurationRef: %w", err)
		}
		log.V(1).Info("Cleared boot configuration")
	}

	if server.Status.PowerState == metalv1alpha1.ServerOffPowerState {
		if err := r.clearWaitingForPowerOffCondition(ctx, server); err != nil {
			log.V(1).Info("Could not clear WaitingForPowerOff condition", "error", err)
		}
		return true, nil
	}

	// Don't block here: updateServerStatus refreshes the power state on every
	// reconcile, so we'll pick up the transition on the next resync.
	if err := bmcClient.PowerOff(ctx, server.Spec.SystemURI); err != nil {
		return false, fmt.Errorf("failed to power off server: %w", err)
	}

	if err := r.setWaitingForPowerOffCondition(ctx, server); err != nil {
		log.V(1).Info("Could not set WaitingForPowerOff condition", "error", err)
	}
	return false, nil
}

func (r *ServerReconciler) setWaitingForPowerOffCondition(ctx context.Context, server *metalv1alpha1.Server) error {
	original := server.DeepCopy()
	msg := fmt.Sprintf("Waiting for BMC to confirm power off (current: %q)", server.Status.PowerState)
	if err := r.Conditions.UpdateSlice(
		&server.Status.Conditions,
		ConditionWaitingForPowerOff,
		conditionutils.UpdateStatus(metav1.ConditionTrue),
		conditionutils.UpdateReason(ReasonWaitingForPowerOff),
		conditionutils.UpdateMessage(msg),
		conditionutils.UpdateObserved(server),
	); err != nil {
		return fmt.Errorf("failed to update WaitingForPowerOff condition: %w", err)
	}
	return r.Status().Patch(ctx, server, client.MergeFrom(original))
}

func (r *ServerReconciler) clearWaitingForPowerOffCondition(ctx context.Context, server *metalv1alpha1.Server) error {
	if meta.FindStatusCondition(server.Status.Conditions, ConditionWaitingForPowerOff) == nil {
		return nil
	}
	original := server.DeepCopy()
	if err := r.Conditions.UpdateSlice(
		&server.Status.Conditions,
		ConditionWaitingForPowerOff,
		conditionutils.UpdateStatus(metav1.ConditionFalse),
		conditionutils.UpdateReason(ReasonPowerOffConfirmed),
		conditionutils.UpdateMessage("BMC confirmed PowerState=Off"),
		conditionutils.UpdateObserved(server),
	); err != nil {
		return fmt.Errorf("failed to update WaitingForPowerOff condition: %w", err)
	}
	return r.Status().Patch(ctx, server, client.MergeFrom(original))
}

func (r *ServerReconciler) handleReleasedState(ctx context.Context, bmcClient bmc.BMC, server *metalv1alpha1.Server) (bool, error) {
	log := ctrl.LoggerFrom(ctx)

	off, err := r.ensureServerPoweredOff(ctx, bmcClient, server)
	if err != nil {
		return false, err
	}
	if !off {
		return true, nil
	}

	if serverClaimRef := server.Spec.ServerClaimRef; serverClaimRef != nil {
		log.V(1).Info("ServerClaimRef still present, nothing to do", "ServerClaimRef", serverClaimRef)
		return false, nil
	}

	log.V(1).Info("ServerClaimRef is gone, transitioning server to available state")
	return r.patchServerState(ctx, server, metalv1alpha1.ServerStateAvailable)
}

func (r *ServerReconciler) handleAnnotationOperations(ctx context.Context, bmcClient bmc.BMC, server *metalv1alpha1.Server) (ctrl.Result, bool, error) {
	log := ctrl.LoggerFrom(ctx)
	operation := server.GetAnnotations()[metalv1alpha1.OperationAnnotation]

	var (
		result ctrl.Result
		done   bool
		err    error
	)

	switch {
	case operation == metalv1alpha1.OperationAnnotationUnpark:
		done, err = true, r.unparkServer(ctx, server)
	case isServerParked(server) && (operation == metalv1alpha1.OperationAnnotationPark || isResetAnnotation(operation) || operation == ""):
		done, err = true, r.standDownParked(ctx, server, operation)
	case operation == metalv1alpha1.OperationAnnotationPark:
		result, done, err = r.parkServer(ctx, bmcClient, server)
	case isResetAnnotation(operation):
		done, err = true, r.resetServer(ctx, bmcClient, server, operation)
	case operation != "":
		r.rejectUnsupportedOperation(ctx, server, operation)
		done = true
	default:
		return ctrl.Result{}, false, nil
	}

	// not done yet
	if err != nil || !done || !result.IsZero() {
		return result, done, err
	}

	if operation != "" {
		if err := r.removeOperationAnnotation(ctx, server); err != nil {
			return ctrl.Result{}, false, err
		}
		log.V(1).Info("Removed operation annotation", "Operation", operation)
	}
	return ctrl.Result{}, true, nil
}

func (r *ServerReconciler) standDownParked(ctx context.Context, server *metalv1alpha1.Server, operation string) error {
	log := ctrl.LoggerFrom(ctx)
	if operation != "" {
		log.V(1).Info("Dropping Server operation annotation because Server is parked", "Operation", operation)
		if operation != metalv1alpha1.OperationAnnotationPark {
			r.Recorder.Eventf(server, nil, v1.EventTypeWarning, "ParkedOperationDropped", "ClearOperationAnnotation", "Operation %q dropped because server is parked", operation)
		}
	}
	return r.stayParked(ctx, server)
}

func (r *ServerReconciler) removeOperationAnnotation(ctx context.Context, server *metalv1alpha1.Server) error {
	serverBase := server.DeepCopy()
	metautils.DeleteAnnotation(server, metalv1alpha1.OperationAnnotation)
	if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return fmt.Errorf("failed to consume operation annotation: %w", err)
	}
	return nil
}

func (r *ServerReconciler) rejectUnsupportedOperation(ctx context.Context, server *metalv1alpha1.Server, operation string) {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Unsupported Server operation annotation", "Operation", operation, "SupportedOperations", metalv1alpha1.AnnotationToRedfishMapping)
	r.Recorder.Eventf(server, nil, v1.EventTypeWarning, "UnsupportedOperation", "ClearOperationAnnotation", "Unsupported operation annotation %q", operation)
}

func (r *ServerReconciler) unparkServer(ctx context.Context, server *metalv1alpha1.Server) error {
	log := ctrl.LoggerFrom(ctx)

	if !isServerParked(server) {
		return nil
	}

	log.V(1).Info("Unpark requested, releasing server")
	serverBase := server.DeepCopy()
	metautils.DeleteAnnotation(server, metalv1alpha1.ParkedAnnotation)
	if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return fmt.Errorf("failed to release parked annotation: %w", err)
	}
	r.Recorder.Eventf(server, nil, v1.EventTypeNormal, "Unparking", "Unpark", "Unpark requested, releasing server")
	return nil
}

func (r *ServerReconciler) resumeParkedServer(ctx context.Context, bmcClient bmc.BMC, server *metalv1alpha1.Server) (bool, error) {
	log := ctrl.LoggerFrom(ctx)
	prePark := r.resolvePreParkState(server)

	log.Info("Resuming Server from Parked state", "preParkState", prePark)
	if err := r.updateServerStatusFromSystemInfo(ctx, bmcClient, server); err != nil {
		return false, fmt.Errorf("failed to refresh server system info on resume: %w", err)
	}

	modified, err := r.patchServerState(ctx, server, prePark)
	if err == nil && modified {
		r.Recorder.Eventf(server, nil, v1.EventTypeNormal, "Resumed", "Resume", "Resumed Server from Parked state to %s", prePark)
	}
	return modified, err
}

func (r *ServerReconciler) stayParked(ctx context.Context, server *metalv1alpha1.Server) error {
	if server.Status.State == metalv1alpha1.ServerStateParked {
		return nil
	}
	_, err := r.patchServerState(ctx, server, metalv1alpha1.ServerStateParked)
	return err
}

func (r *ServerReconciler) resetServer(ctx context.Context, bmcClient bmc.BMC, server *metalv1alpha1.Server, operation string) error {
	log := ctrl.LoggerFrom(ctx)
	resetType := metalv1alpha1.AnnotationToRedfishMapping[operation]

	log.V(1).Info("Handling operation", "Operation", operation, "RedfishResetType", resetType)
	if err := bmcClient.Reset(ctx, server.Spec.SystemURI, resetType); err != nil {
		return fmt.Errorf("failed to reset server: %w", err)
	}
	log.V(1).Info("Operation completed", "Operation", operation, "RedfishResetType", resetType)
	return nil
}

func (r *ServerReconciler) parkServer(ctx context.Context, bmcClient bmc.BMC, server *metalv1alpha1.Server) (ctrl.Result, bool, error) {
	log := ctrl.LoggerFrom(ctx)

	if !isParkableState(server.Status.State) {
		log.V(1).Info("Server is not in a parkable state, deferring park request", "currentState", server.Status.State)
		r.Recorder.Eventf(server, nil, v1.EventTypeWarning, "ParkDeferred", "DeferPark", "Park request deferred because server is in state %q", server.Status.State)
		return ctrl.Result{}, false, nil
	}

	if err := r.updateServerStatus(ctx, bmcClient, server); err != nil {
		return ctrl.Result{}, false, fmt.Errorf("failed to update server status: %w", err)
	}

	if server.Status.PowerState != metalv1alpha1.ServerOffPowerState &&
		server.Status.PowerState != metalv1alpha1.ServerPoweringOffPowerState {
		if err := bmcClient.PowerOff(ctx, server.Spec.SystemURI); err != nil {
			return ctrl.Result{}, false, fmt.Errorf("failed to power off server for parking: %w", err)
		}
		log.V(1).Info("Requested power off for parking")
	}

	if server.Status.PowerState != metalv1alpha1.ServerOffPowerState {
		// Wait for the power off to take effect; requeue explicitly since a BMC-side power
		// transition does not produce a watch event.
		return ctrl.Result{RequeueAfter: r.ResyncInterval}, true, nil
	}

	serverBase := server.DeepCopy()
	metav1.SetMetaDataAnnotation(&server.ObjectMeta, metalv1alpha1.ParkedAnnotation, "true")
	if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return ctrl.Result{}, false, fmt.Errorf("failed to patch parked annotations: %w", err)
	}

	log.Info("Parked Server")
	r.Recorder.Eventf(server, nil, v1.EventTypeNormal, "Parked", "PowerOff", "Parked Server, powered off")
	return ctrl.Result{}, true, nil
}

func (r *ServerReconciler) resolvePreParkState(server *metalv1alpha1.Server) metalv1alpha1.ServerState {
	if server.Spec.ServerClaimRef != nil {
		return metalv1alpha1.ServerStateReserved
	}
	return metalv1alpha1.ServerStateAvailable
}

// updates the Server status which can be changed via Spec
func (r *ServerReconciler) updateServerStatus(ctx context.Context, bmcClient bmc.BMC, server *metalv1alpha1.Server) error {
	log := ctrl.LoggerFrom(ctx)
	if server.Spec.BMCRef == nil && server.Spec.BMC == nil {
		log.V(1).Info("Server has no BMC connection configured")
		return nil
	}
	systemInfo, err := bmcClient.GetSystemInfo(ctx, server.Spec.SystemURI)
	if err != nil {
		return fmt.Errorf("failed to get system info for Server: %w", err)
	}

	updatedPowerState := metalv1alpha1.ServerPowerState(systemInfo.PowerState)
	updatedIndicatorLED := metalv1alpha1.IndicatorLED(systemInfo.IndicatorLED)

	if updatedPowerState == server.Status.PowerState && updatedIndicatorLED == server.Status.IndicatorLED {
		return nil
	}

	serverBase := server.DeepCopy()
	server.Status.PowerState = updatedPowerState
	server.Status.IndicatorLED = updatedIndicatorLED
	if err = r.Status().Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return fmt.Errorf("failed to patch Server status: %w", err)
	}
	log.V(1).Info("Updated Server status", "Status", server.Status.State, "powerState", server.Status.PowerState)
	return nil
}

func (r *ServerReconciler) updateServerStatusFromSystemInfo(ctx context.Context, bmcClient bmc.BMC, server *metalv1alpha1.Server) error {
	log := ctrl.LoggerFrom(ctx)
	serverBase := server.DeepCopy()
	systemInfo, err := bmcClient.GetSystemInfo(ctx, server.Spec.SystemURI)
	if err != nil {
		return fmt.Errorf("failed to get system info for Server: %w", err)
	}
	biosVersion, err := bmcClient.GetBiosVersion(ctx, server.Spec.SystemURI)
	if err != nil {
		return fmt.Errorf("failed to get BIOS version for Server: %w", err)
	}
	server.Status.BIOSVersion = biosVersion
	server.Status.PowerState = metalv1alpha1.ServerPowerState(systemInfo.PowerState)
	server.Status.SerialNumber = systemInfo.SerialNumber
	server.Status.SKU = systemInfo.SKU
	server.Status.Manufacturer = systemInfo.Manufacturer
	server.Status.Model = systemInfo.Model
	server.Status.TotalSystemMemory = &systemInfo.TotalSystemMemory

	processors, err := bmcClient.GetProcessors(ctx, server.Spec.SystemURI)
	if err != nil {
		return fmt.Errorf("failed to get processors for Server: %w", err)
	}
	server.Status.Processors = make([]metalv1alpha1.Processor, 0, len(processors))
	for _, processor := range processors {
		server.Status.Processors = append(server.Status.Processors, metalv1alpha1.Processor{
			ID:             processor.ID,
			Type:           processor.Type,
			Architecture:   processor.Architecture,
			InstructionSet: processor.InstructionSet,
			Manufacturer:   processor.Manufacturer,
			Model:          processor.Model,
			MaxSpeedMHz:    processor.MaxSpeedMHz,
			TotalCores:     processor.TotalCores,
			TotalThreads:   processor.TotalThreads,
		})
	}
	storages, err := bmcClient.GetStorages(ctx, server.Spec.SystemURI)
	if err != nil {
		return fmt.Errorf("failed to get storages for Server: %w", err)
	}
	server.Status.Storages = nil
	for _, storage := range storages {
		metalStorage := metalv1alpha1.Storage{
			Name:  storage.Name,
			State: metalv1alpha1.StorageState(storage.State),
		}
		for _, drive := range storage.Drives {
			metalStorage.Drives = append(metalStorage.Drives, metalv1alpha1.StorageDrive{
				Name:      drive.Name,
				Model:     drive.Model,
				Vendor:    drive.Vendor,
				Capacity:  resource.NewQuantity(drive.SizeBytes, resource.BinarySI),
				Type:      string(drive.Type),
				State:     metalv1alpha1.StorageState(drive.State),
				MediaType: drive.MediaType,
			})
		}
		metalStorage.Volumes = make([]metalv1alpha1.StorageVolume, 0, len(storage.Volumes))
		for _, volume := range storage.Volumes {
			metalStorage.Volumes = append(metalStorage.Volumes, metalv1alpha1.StorageVolume{
				Name:        volume.Name,
				Capacity:    resource.NewQuantity(volume.SizeBytes, resource.BinarySI),
				State:       metalv1alpha1.StorageState(volume.State),
				RAIDType:    string(volume.RAIDType),
				VolumeUsage: volume.VolumeUsage,
			})
		}
		server.Status.Storages = append(server.Status.Storages, metalStorage)
	}

	if err := r.Status().Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return fmt.Errorf("failed to patch Server status: %w", err)
	}
	log.V(1).Info("Updated Server status", "Status", server.Status.State, "powerState", server.Status.PowerState)
	return nil
}

func (r *ServerReconciler) serverBootConfigurationIsReady(ctx context.Context, server *metalv1alpha1.Server) (bool, error) {
	if server.Spec.BootConfigurationRef == nil {
		return false, nil
	}
	config := &metalv1alpha1.ServerBootConfiguration{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: server.Spec.BootConfigurationRef.Namespace, Name: server.Spec.BootConfigurationRef.Name}, config); err != nil {
		return false, err
	}
	return config.Status.State == metalv1alpha1.ServerBootConfigurationStateReady, nil
}

func (r *ServerReconciler) pxeBootServer(ctx context.Context, bmcClient bmc.BMC, server *metalv1alpha1.Server) error {
	log := ctrl.LoggerFrom(ctx)
	if server == nil || server.Spec.BootConfigurationRef == nil {
		log.V(1).Info("Server not ready for netboot")
		return nil
	}

	if server.Spec.BMCRef == nil && server.Spec.BMC == nil {
		return fmt.Errorf("can only PXE boot server with valid BMC ref or inline BMC configuration")
	}

	if err := bmcClient.SetBootOverride(ctx, server.Spec.SystemURI); err != nil {
		return fmt.Errorf("failed to set PXE boot one for server: %w", err)
	}
	return nil
}

func (r *ServerReconciler) patchServerState(ctx context.Context, server *metalv1alpha1.Server, state metalv1alpha1.ServerState) (bool, error) {
	if server.Status.State == state {
		return false, nil
	}
	serverBase := server.DeepCopy()
	server.Status.State = state
	if err := r.Status().Patch(ctx, server, client.MergeFromWithOptions(serverBase, client.MergeFromWithOptimisticLock{})); err != nil {
		return false, fmt.Errorf("failed to patch server state: %w", err)
	}
	return true, nil
}

func (r *ServerReconciler) patchServerURI(ctx context.Context, bmcClient bmc.BMC, server *metalv1alpha1.Server) (bool, error) {
	log := ctrl.LoggerFrom(ctx)
	if len(server.Spec.SystemURI) != 0 {
		return false, nil
	}
	log.V(1).Info("Patching systemURI to the server resource")

	systems, err := bmcClient.GetSystems(ctx)
	if err != nil {
		return false, err
	}

	for _, system := range systems {
		if strings.EqualFold(system.UUID, server.Spec.SystemUUID) {
			serverBase := server.DeepCopy()
			server.Spec.SystemURI = system.URI
			if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
				return false, fmt.Errorf("failed to patch server URI: %w", err)
			}
		}
	}
	if len(server.Spec.SystemURI) == 0 {
		log.V(1).Info("Patching systemURI failed", "no system found for UUID", server.Spec.SystemUUID)
		return false, fmt.Errorf("unable to find system URI for UUID: %v", server.Spec.SystemUUID)
	}

	return true, nil
}

func (r *ServerReconciler) ensureServerPowerState(ctx context.Context, bmcClient bmc.BMC, server *metalv1alpha1.Server, power metalv1alpha1.Power) error {
	log := ctrl.LoggerFrom(ctx)
	if power == "" {
		// no desired power state set
		return nil
	}

	powerOp := powerOpNoOP
	if server.Status.PowerState != metalv1alpha1.ServerOnPowerState &&
		server.Status.PowerState != metalv1alpha1.ServerPoweringOnPowerState &&
		power == metalv1alpha1.PowerOn {
		powerOp = powerOpOn
	}

	if server.Status.PowerState != metalv1alpha1.ServerOffPowerState &&
		server.Status.PowerState != metalv1alpha1.ServerPoweringOffPowerState &&
		power == metalv1alpha1.PowerOff {
		powerOp = powerOpOff
	}

	if powerOp == powerOpNoOP {
		log.V(1).Info("Server already in target power state", "powerState", server.Status.PowerState)
		return nil
	}

	switch powerOp {
	case powerOpOn:
		log.V(1).Info("Server Power On")
		if err := bmcClient.PowerOn(ctx, server.Spec.SystemURI); err != nil {
			return fmt.Errorf("failed to power on server: %w", err)
		}
		if err := bmcClient.WaitForServerPowerState(ctx, server.Spec.SystemURI, schemas.OnPowerState); err != nil {
			return fmt.Errorf("failed to wait for server power on server: %w", err)
		}
		if err := r.updatePowerOnCondition(ctx, server); err != nil {
			return fmt.Errorf("failed to update power on condition: %w", err)
		}
	case powerOpOff:
		log.V(1).Info("Server Power Off")
		powerOffType := bmcClient.PowerOff

		if err := powerOffType(ctx, server.Spec.SystemURI); err != nil {
			return fmt.Errorf("failed to power off server: %w", err)
		}
		if err := bmcClient.WaitForServerPowerState(ctx, server.Spec.SystemURI, schemas.OffPowerState); err != nil {
			if r.EnforcePowerOff {
				log.V(1).Info("Failed to wait for server graceful shutdown, retrying with force power off")
				powerOffType = bmcClient.ForcePowerOff
				if err := powerOffType(ctx, server.Spec.SystemURI); err != nil {
					return fmt.Errorf("failed to power off server: %w", err)
				}
				if err := bmcClient.WaitForServerPowerState(ctx, server.Spec.SystemURI, schemas.OffPowerState); err != nil {
					return fmt.Errorf("failed to wait for server force power off: %w", err)
				}
			} else {
				return fmt.Errorf("failed to wait for server power off: %w", err)
			}
		}
	}
	log.V(1).Info("Ensured server power state", "targetPower", power)

	return nil
}

func (r *ServerReconciler) updatePowerOnCondition(ctx context.Context, server *metalv1alpha1.Server) error {
	original := server.DeepCopy()
	err := r.Conditions.UpdateSlice(
		&server.Status.Conditions,
		ConditionPoweringOn,
		conditionutils.UpdateStatus(metav1.ConditionTrue),
		conditionutils.UpdateReason("ServerPowerOn"),
		conditionutils.UpdateMessage("Server is powering on"),
		conditionutils.UpdateObserved(server),
	)
	if err != nil {
		return fmt.Errorf("failed to update powering on condition: %w", err)
	}
	return r.Status().Patch(ctx, server, client.MergeFrom(original))
}

func (r *ServerReconciler) ensureIndicatorLED(ctx context.Context, bmcClient bmc.BMC, server *metalv1alpha1.Server) error {
	if server.Spec.IndicatorLED == "" {
		return nil
	}
	desired := schemas.IndicatorLED(server.Spec.IndicatorLED)   //nolint:staticcheck
	current := schemas.IndicatorLED(server.Status.IndicatorLED) //nolint:staticcheck
	if desired == current {
		return nil
	}
	return bmcClient.SetIndicatorLED(ctx, server.Spec.SystemURI, desired)
}

func (r *ServerReconciler) applyBootOrder(ctx context.Context, bmcClient bmc.BMC, server *metalv1alpha1.Server) error {
	log := ctrl.LoggerFrom(ctx)
	if server.Spec.BMCRef == nil && server.Spec.BMC == nil {
		log.V(1).Info("Server has no BMC connection configured")
		return nil
	}

	if len(server.Spec.BootOrder) == 0 {
		return nil
	}

	order, err := bmcClient.GetBootOrder(ctx, server.Spec.SystemURI)
	if err != nil {
		return fmt.Errorf("failed to get boot order: %w", err)
	}

	slices.SortFunc(server.Spec.BootOrder, func(a, b metalv1alpha1.BootOrder) int {
		return cmp.Compare(a.Priority, b.Priority)
	})
	newOrder := []string{}
	change := len(order) != len(server.Spec.BootOrder)
	for i, boot := range server.Spec.BootOrder {
		newOrder = append(newOrder, boot.Device)
		if i >= len(order) || order[i] != boot.Device {
			change = true
		}
	}
	if change {
		log.V(1).Info("Applying boot order update", "newOrder", newOrder)
		return bmcClient.SetBootOrder(ctx, server.Spec.SystemURI, newOrder)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.MaxConcurrentReconciles,
		}).
		For(&metalv1alpha1.Server{}).
		Watches(
			&metalv1alpha1.ServerBootConfiguration{},
			r.enqueueServerByServerBootConfiguration(),
		).
		Watches(
			&metalv1alpha1.ServerClaim{},
			r.enqueueServerByClaim(),
		).
		Complete(r)
}

func (r *ServerReconciler) enqueueServerByServerBootConfiguration() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []ctrl.Request {
		config := obj.(*metalv1alpha1.ServerBootConfiguration)
		return []ctrl.Request{
			{
				NamespacedName: types.NamespacedName{Name: config.Spec.ServerRef.Name},
			},
		}
	})
}

func (r *ServerReconciler) enqueueServerByClaim() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []ctrl.Request {
		claim := obj.(*metalv1alpha1.ServerClaim)
		if claim.Spec.ServerRef == nil {
			return nil
		}
		return []ctrl.Request{
			{
				NamespacedName: types.NamespacedName{Name: claim.Spec.ServerRef.Name},
			},
		}
	})
}

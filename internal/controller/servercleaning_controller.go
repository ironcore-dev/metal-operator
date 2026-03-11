// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/ironcore-dev/controller-utils/clientutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// ServerCleaningFinalizer is the finalizer for the ServerCleaning resource.
	ServerCleaningFinalizer = "metal.ironcore.dev/servercleaning"

	// ServerCleaningConditionTypeCleaning indicates that cleaning is in progress
	ServerCleaningConditionTypeCleaning = "Cleaning"

	// ServerCleaningConditionReasonInProgress indicates cleaning is in progress
	ServerCleaningConditionReasonInProgress = "CleaningInProgress"

	// ServerCleaningConditionReasonCompleted indicates cleaning is completed
	ServerCleaningConditionReasonCompleted = "CleaningCompleted"

	// ServerCleaningConditionReasonFailed indicates cleaning failed
	ServerCleaningConditionReasonFailed = "CleaningFailed"

	// Task state constants
	taskStateCompleted = "Completed"
	taskStateException = "Exception"
	taskStateCancelled = "Cancelled"
	taskStateKilled    = "Killed"
	taskStateNew       = "New"
)

// ServerCleaningReconciler reconciles a ServerCleaning object
type ServerCleaningReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servercleanings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servercleanings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servercleanings/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ServerCleaningReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	cleaning := &metalv1alpha1.ServerCleaning{}
	if err := r.Get(ctx, req.NamespacedName, cleaning); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	return r.reconcileExists(ctx, cleaning)
}

func (r *ServerCleaningReconciler) reconcileExists(ctx context.Context, cleaning *metalv1alpha1.ServerCleaning) (ctrl.Result, error) {
	if !cleaning.DeletionTimestamp.IsZero() {
		return r.delete(ctx, cleaning)
	}
	return r.reconcile(ctx, cleaning)
}

func (r *ServerCleaningReconciler) reconcile(ctx context.Context, cleaning *metalv1alpha1.ServerCleaning) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Reconciling ServerCleaning")

	// Ensure finalizer
	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, cleaning, ServerCleaningFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	// Set initial state if not set
	if cleaning.Status.State == "" {
		if modified, err := r.patchCleaningState(ctx, cleaning, metalv1alpha1.ServerCleaningStatePending); err != nil || modified {
			return ctrl.Result{}, err
		}
	}

	return r.ensureServerCleaningStateTransition(ctx, cleaning)
}

func (r *ServerCleaningReconciler) ensureServerCleaningStateTransition(ctx context.Context, cleaning *metalv1alpha1.ServerCleaning) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	switch cleaning.Status.State {
	case metalv1alpha1.ServerCleaningStatePending:
		return r.handlePendingState(ctx, cleaning)
	case metalv1alpha1.ServerCleaningStateInProgress:
		return r.handleInProgressState(ctx, cleaning)
	case metalv1alpha1.ServerCleaningStateCompleted:
		return r.handleCompletedState(ctx, cleaning)
	case metalv1alpha1.ServerCleaningStateFailed:
		return r.handleFailedState(ctx, cleaning)
	default:
		log.V(1).Info("Unknown ServerCleaning state, skipping reconciliation", "State", cleaning.Status.State)
		return ctrl.Result{}, nil
	}
}

func (r *ServerCleaningReconciler) handlePendingState(ctx context.Context, cleaning *metalv1alpha1.ServerCleaning) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Get list of servers to clean
	servers, err := r.getServersForCleaning(ctx, cleaning)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get servers for cleaning: %w", err)
	}

	if len(servers) == 0 {
		log.V(1).Info("No servers found for cleaning")
		return ctrl.Result{}, nil
	}

	// Update selected servers count
	if err := r.updateSelectedServersCount(ctx, cleaning, int32(len(servers))); err != nil {
		return ctrl.Result{}, err
	}

	// Initialize server status entries
	if err := r.initializeServerStatuses(ctx, cleaning, servers); err != nil {
		return ctrl.Result{}, err
	}

	// Initiate BMC cleaning operations for each server
	pendingCount := int32(0)
	inProgressCount := int32(0)
	failedCount := int32(0)

	for _, server := range servers {
		if server.Status.State != metalv1alpha1.ServerStateTainted {
			log.V(1).Info("Server is not in Tainted state, skipping", "Server", server.Name, "State", server.Status.State)
			continue
		}

		// Initiate cleaning operations via BMC
		if err := r.initiateBMCCleaning(ctx, cleaning, &server); err != nil {
			log.Error(err, "Failed to initiate BMC cleaning for server", "Server", server.Name)
			if err := r.updateServerStatus(ctx, cleaning, server.Name, metalv1alpha1.ServerCleaningStateFailed, fmt.Sprintf("Failed to initiate cleaning: %v", err)); err != nil {
				return ctrl.Result{}, err
			}
			failedCount++
			continue
		}

		inProgressCount++
		if err := r.updateServerStatus(ctx, cleaning, server.Name, metalv1alpha1.ServerCleaningStateInProgress, "Cleaning initiated"); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Update status counts
	if err := r.updateCleaningCounts(ctx, cleaning, pendingCount, inProgressCount, 0, failedCount); err != nil {
		return ctrl.Result{}, err
	}

	// Update status condition
	if err := r.setCondition(ctx, cleaning, metav1.Condition{
		Type:               ServerCleaningConditionTypeCleaning,
		Status:             metav1.ConditionTrue,
		Reason:             ServerCleaningConditionReasonInProgress,
		Message:            fmt.Sprintf("Cleaning operations initiated for %d servers", inProgressCount),
		ObservedGeneration: cleaning.Generation,
	}); err != nil {
		return ctrl.Result{}, err
	}

	// Transition to InProgress
	if modified, err := r.patchCleaningState(ctx, cleaning, metalv1alpha1.ServerCleaningStateInProgress); err != nil || modified {
		return ctrl.Result{}, err
	}

	// Requeue to monitor task progress
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *ServerCleaningReconciler) handleInProgressState(ctx context.Context, cleaning *metalv1alpha1.ServerCleaning) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Get servers for cleaning
	servers, err := r.getServersForCleaning(ctx, cleaning)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get servers for cleaning: %w", err)
	}

	if len(servers) == 0 {
		log.V(1).Info("No servers found for monitoring")
		return ctrl.Result{}, nil
	}

	// Track counts
	var inProgressCount, completedCount, failedCount int32
	allComplete := true

	// Monitor each server's cleaning tasks
	for _, server := range servers {
		// Find the server status entry
		var serverStatus *metalv1alpha1.ServerCleaningStatusEntry
		for i := range cleaning.Status.ServerCleaningStatuses {
			if cleaning.Status.ServerCleaningStatuses[i].ServerName == server.Name {
				serverStatus = &cleaning.Status.ServerCleaningStatuses[i]
				break
			}
		}

		if serverStatus == nil {
			log.V(1).Info("No status entry found for server", "server", server.Name)
			continue
		}

		// Skip servers that are already in terminal states
		if serverStatus.State == metalv1alpha1.ServerCleaningStateCompleted {
			completedCount++
			continue
		}
		if serverStatus.State == metalv1alpha1.ServerCleaningStateFailed {
			failedCount++
			continue
		}

		// Monitor BMC tasks for this server
		isComplete, err := r.monitorBMCTasks(ctx, cleaning, &server, serverStatus)
		if err != nil {
			log.Error(err, "Failed to monitor BMC tasks", "server", server.Name)
			// Don't fail the entire reconciliation, just mark this server as having issues
			allComplete = false
			inProgressCount++
			continue
		}

		// Update counts based on final server state
		// Re-fetch the status since monitorBMCTasks updated it
		for i := range cleaning.Status.ServerCleaningStatuses {
			if cleaning.Status.ServerCleaningStatuses[i].ServerName == server.Name {
				serverStatus = &cleaning.Status.ServerCleaningStatuses[i]
				break
			}
		}

		switch serverStatus.State {
		case metalv1alpha1.ServerCleaningStateCompleted:
			completedCount++
		case metalv1alpha1.ServerCleaningStateFailed:
			failedCount++
		default:
			inProgressCount++
			allComplete = false
		}

		if !isComplete {
			allComplete = false
		}
	}

	// Update counts
	if err := r.updateCleaningCounts(ctx, cleaning, 0, inProgressCount, completedCount, failedCount); err != nil {
		return ctrl.Result{}, err
	}

	// Check if all cleanings are complete
	totalServers := cleaning.Status.SelectedServers
	processedServers := completedCount + failedCount

	if allComplete && processedServers >= totalServers {
		// All servers processed
		if failedCount > 0 {
			log.V(1).Info("Cleaning completed with failures", "completed", completedCount, "failed", failedCount)
			if err := r.setCondition(ctx, cleaning, metav1.Condition{
				Type:               ServerCleaningConditionTypeCleaning,
				Status:             metav1.ConditionFalse,
				Reason:             ServerCleaningConditionReasonFailed,
				Message:            fmt.Sprintf("Cleaning completed: %d succeeded, %d failed", completedCount, failedCount),
				ObservedGeneration: cleaning.Generation,
			}); err != nil {
				return ctrl.Result{}, err
			}
			if modified, err := r.patchCleaningState(ctx, cleaning, metalv1alpha1.ServerCleaningStateFailed); err != nil || modified {
				return ctrl.Result{}, err
			}
		} else {
			log.V(1).Info("Cleaning completed successfully", "completed", completedCount)
			if err := r.setCondition(ctx, cleaning, metav1.Condition{
				Type:               ServerCleaningConditionTypeCleaning,
				Status:             metav1.ConditionTrue,
				Reason:             ServerCleaningConditionReasonCompleted,
				Message:            fmt.Sprintf("Cleaning completed successfully for %d servers", completedCount),
				ObservedGeneration: cleaning.Generation,
			}); err != nil {
				return ctrl.Result{}, err
			}
			if modified, err := r.patchCleaningState(ctx, cleaning, metalv1alpha1.ServerCleaningStateCompleted); err != nil || modified {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Still in progress, requeue to check again
	log.V(1).Info("Cleaning still in progress", "inProgress", inProgressCount, "completed", completedCount, "failed", failedCount)
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *ServerCleaningReconciler) handleCompletedState(ctx context.Context, _ *metalv1alpha1.ServerCleaning) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("ServerCleaning completed, nothing to do")
	return ctrl.Result{}, nil
}

func (r *ServerCleaningReconciler) handleFailedState(ctx context.Context, _ *metalv1alpha1.ServerCleaning) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("ServerCleaning failed, manual intervention required")
	return ctrl.Result{}, nil
}

func (r *ServerCleaningReconciler) delete(ctx context.Context, cleaning *metalv1alpha1.ServerCleaning) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Deleting ServerCleaning")

	// Remove finalizer
	if err := clientutils.PatchRemoveFinalizer(ctx, r.Client, cleaning, ServerCleaningFinalizer); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *ServerCleaningReconciler) patchCleaningState(ctx context.Context, cleaning *metalv1alpha1.ServerCleaning, state metalv1alpha1.ServerCleaningState) (bool, error) {
	if cleaning.Status.State == state {
		return false, nil
	}

	cleaningBase := cleaning.DeepCopy()
	cleaning.Status.State = state
	if err := r.Status().Patch(ctx, cleaning, client.MergeFrom(cleaningBase)); err != nil {
		return false, fmt.Errorf("failed to patch ServerCleaning state: %w", err)
	}

	return true, nil
}

func (r *ServerCleaningReconciler) setCondition(ctx context.Context, cleaning *metalv1alpha1.ServerCleaning, condition metav1.Condition) error {
	cleaningBase := cleaning.DeepCopy()
	condition.LastTransitionTime = metav1.Now()
	meta.SetStatusCondition(&cleaning.Status.Conditions, condition)
	if err := r.Status().Patch(ctx, cleaning, client.MergeFrom(cleaningBase)); err != nil {
		return fmt.Errorf("failed to update conditions: %w", err)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerCleaningReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.ServerCleaning{}).
		Owns(&metalv1alpha1.ServerMaintenance{}).
		Watches(
			&metalv1alpha1.Server{},
			handler.EnqueueRequestsFromMapFunc(r.mapServerToServerCleaning),
		).
		Complete(r)
}

func (r *ServerCleaningReconciler) mapServerToServerCleaning(ctx context.Context, obj client.Object) []reconcile.Request {
	server := obj.(*metalv1alpha1.Server)

	cleaningList := &metalv1alpha1.ServerCleaningList{}
	if err := r.List(ctx, cleaningList); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, cleaning := range cleaningList.Items {
		if cleaning.Spec.ServerRef.Name == server.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&cleaning),
			})
		}
	}

	return requests
}

func (r *ServerCleaningReconciler) getServersForCleaning(ctx context.Context, cleaning *metalv1alpha1.ServerCleaning) ([]metalv1alpha1.Server, error) {
	// If ServerRef is specified, return that single server
	if cleaning.Spec.ServerRef != nil {
		server, err := GetServerByName(ctx, r.Client, cleaning.Spec.ServerRef.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get server %s: %w", cleaning.Spec.ServerRef.Name, err)
		}
		return []metalv1alpha1.Server{*server}, nil
	}

	// If ServerSelector is specified, list matching servers
	if cleaning.Spec.ServerSelector != nil {
		serverList := &metalv1alpha1.ServerList{}
		selector, err := metav1.LabelSelectorAsSelector(cleaning.Spec.ServerSelector)
		if err != nil {
			return nil, fmt.Errorf("failed to convert label selector: %w", err)
		}

		if err := r.List(ctx, serverList, client.MatchingLabelsSelector{Selector: selector}); err != nil {
			return nil, fmt.Errorf("failed to list servers: %w", err)
		}

		return serverList.Items, nil
	}

	return nil, fmt.Errorf("neither serverRef nor serverSelector is specified")
}

func (r *ServerCleaningReconciler) updateSelectedServersCount(ctx context.Context, cleaning *metalv1alpha1.ServerCleaning, count int32) error {
	cleaningBase := cleaning.DeepCopy()
	cleaning.Status.SelectedServers = count
	if err := r.Status().Patch(ctx, cleaning, client.MergeFrom(cleaningBase)); err != nil {
		return fmt.Errorf("failed to update selected servers count: %w", err)
	}
	return nil
}

func (r *ServerCleaningReconciler) initializeServerStatuses(ctx context.Context, cleaning *metalv1alpha1.ServerCleaning, servers []metalv1alpha1.Server) error {
	cleaningBase := cleaning.DeepCopy()
	cleaning.Status.ServerCleaningStatuses = make([]metalv1alpha1.ServerCleaningStatusEntry, 0, len(servers))

	for _, server := range servers {
		cleaning.Status.ServerCleaningStatuses = append(cleaning.Status.ServerCleaningStatuses, metalv1alpha1.ServerCleaningStatusEntry{
			ServerName:     server.Name,
			State:          metalv1alpha1.ServerCleaningStatePending,
			Message:        "Waiting to start cleaning",
			LastUpdateTime: metav1.Now(),
		})
	}

	if err := r.Status().Patch(ctx, cleaning, client.MergeFrom(cleaningBase)); err != nil {
		return fmt.Errorf("failed to initialize server statuses: %w", err)
	}
	return nil
}

func (r *ServerCleaningReconciler) updateServerStatus(ctx context.Context, cleaning *metalv1alpha1.ServerCleaning, serverName string, state metalv1alpha1.ServerCleaningState, message string) error {
	cleaningBase := cleaning.DeepCopy()

	// Find and update the server status entry
	found := false
	for i := range cleaning.Status.ServerCleaningStatuses {
		if cleaning.Status.ServerCleaningStatuses[i].ServerName == serverName {
			cleaning.Status.ServerCleaningStatuses[i].State = state
			cleaning.Status.ServerCleaningStatuses[i].Message = message
			cleaning.Status.ServerCleaningStatuses[i].LastUpdateTime = metav1.Now()
			found = true
			break
		}
	}

	// If not found, add new entry
	if !found {
		cleaning.Status.ServerCleaningStatuses = append(cleaning.Status.ServerCleaningStatuses, metalv1alpha1.ServerCleaningStatusEntry{
			ServerName:     serverName,
			State:          state,
			Message:        message,
			LastUpdateTime: metav1.Now(),
		})
	}

	if err := r.Status().Patch(ctx, cleaning, client.MergeFrom(cleaningBase)); err != nil {
		return fmt.Errorf("failed to update server status: %w", err)
	}
	return nil
}

func (r *ServerCleaningReconciler) updateCleaningCounts(ctx context.Context, cleaning *metalv1alpha1.ServerCleaning, pending, inProgress, completed, failed int32) error {
	cleaningBase := cleaning.DeepCopy()
	cleaning.Status.PendingCleanings = pending
	cleaning.Status.InProgressCleanings = inProgress
	cleaning.Status.CompletedCleanings = completed
	cleaning.Status.FailedCleanings = failed

	if err := r.Status().Patch(ctx, cleaning, client.MergeFrom(cleaningBase)); err != nil {
		return fmt.Errorf("failed to update cleaning counts: %w", err)
	}
	return nil
}

// initiateBMCCleaning initiates cleaning operations directly via BMC and stores task information
func (r *ServerCleaningReconciler) initiateBMCCleaning(ctx context.Context, cleaning *metalv1alpha1.ServerCleaning, server *metalv1alpha1.Server) error {
	log := ctrl.LoggerFrom(ctx)

	// Get BMC client for this server
	bmcClient, err := bmcutils.GetBMCClientForServer(ctx, r.Client, server, false, bmc.Options{})
	if err != nil {
		return fmt.Errorf("failed to get BMC client: %w", err)
	}
	defer bmcClient.Logout()

	systemURI := server.Spec.SystemURI
	if systemURI == "" {
		return fmt.Errorf("server %s has no system URI", server.Name)
	}

	var allTasks []metalv1alpha1.CleaningTaskStatus

	// Initiate disk wipe if requested
	if cleaning.Spec.DiskWipe != nil {
		log.V(1).Info("Initiating disk erase", "server", server.Name, "method", cleaning.Spec.DiskWipe.Method)
		tasks, err := bmcClient.EraseDisk(ctx, systemURI, bmc.DiskWipeMethod(cleaning.Spec.DiskWipe.Method))
		if err != nil {
			return fmt.Errorf("failed to initiate disk wipe: %w", err)
		}
		for _, task := range tasks {
			allTasks = append(allTasks, metalv1alpha1.CleaningTaskStatus{
				TaskURI:        task.TaskURI,
				TaskType:       string(task.TaskType),
				TargetID:       task.TargetID,
				State:          taskStateNew,
				LastUpdateTime: metav1.Now(),
			})
		}
		log.V(1).Info("Disk wipe tasks created", "server", server.Name, "count", len(tasks))
	}

	// Initiate BIOS reset if requested
	if cleaning.Spec.BIOSReset {
		log.V(1).Info("Initiating BIOS reset", "server", server.Name)
		task, err := bmcClient.ResetBIOSToDefaults(ctx, systemURI)
		if err != nil {
			return fmt.Errorf("failed to initiate BIOS reset: %w", err)
		}
		if task != nil {
			allTasks = append(allTasks, metalv1alpha1.CleaningTaskStatus{
				TaskURI:        task.TaskURI,
				TaskType:       string(task.TaskType),
				TargetID:       task.TargetID,
				State:          "New",
				LastUpdateTime: metav1.Now(),
			})
			log.V(1).Info("BIOS reset task created", "server", server.Name, "taskURI", task.TaskURI)
		}
	}

	// Initiate BMC reset if requested
	// TODO: BMC reset requires manager UUID which is not readily available from server spec.
	// For now, BMC reset will be handled via ServerMaintenance or manual intervention.
	if cleaning.Spec.BMCReset {
		log.V(1).Info("BMC reset requested but not yet implemented via direct BMC access", "server", server.Name)
		// Note: BMC reset is a critical operation that may disconnect the BMC client,
		// so it should be done last or via ServerMaintenance with proper handling.
	}

	// Initiate network config clear if requested
	if cleaning.Spec.NetworkCleanup {
		log.V(1).Info("Initiating network configuration clear", "server", server.Name)
		task, err := bmcClient.ClearNetworkConfiguration(ctx, systemURI)
		if err != nil {
			// Network cleanup is non-critical, log and continue
			log.Error(err, "Failed to initiate network config clear (non-critical)", "server", server.Name)
		} else if task != nil {
			allTasks = append(allTasks, metalv1alpha1.CleaningTaskStatus{
				TaskURI:        task.TaskURI,
				TaskType:       string(task.TaskType),
				TargetID:       task.TargetID,
				State:          "New",
				LastUpdateTime: metav1.Now(),
			})
			log.V(1).Info("Network config clear task created", "server", server.Name, "taskURI", task.TaskURI)
		}
	}

	// Store task information in server status
	if len(allTasks) > 0 {
		if err := r.updateServerTasks(ctx, cleaning, server.Name, allTasks); err != nil {
			return fmt.Errorf("failed to store task information: %w", err)
		}
		log.Info("Cleaning tasks initiated", "server", server.Name, "taskCount", len(allTasks))
	} else {
		log.Info("No cleaning tasks created (all operations completed synchronously)", "server", server.Name)
	}

	return nil
}

// monitorBMCTasks checks the status of BMC tasks and updates progress
func (r *ServerCleaningReconciler) monitorBMCTasks(ctx context.Context, cleaning *metalv1alpha1.ServerCleaning, server *metalv1alpha1.Server, serverStatus *metalv1alpha1.ServerCleaningStatusEntry) (bool, error) {
	log := ctrl.LoggerFrom(ctx)

	if len(serverStatus.CleaningTasks) == 0 {
		log.V(1).Info("No tasks to monitor", "server", server.Name)
		return true, nil // No tasks means cleaning is complete
	}

	// Get BMC client for this server
	bmcClient, err := bmcutils.GetBMCClientForServer(ctx, r.Client, server, false, bmc.Options{})
	if err != nil {
		return false, fmt.Errorf("failed to get BMC client: %w", err)
	}
	defer bmcClient.Logout()

	allComplete := true
	anyFailed := false
	updatedTasks := make([]metalv1alpha1.CleaningTaskStatus, 0, len(serverStatus.CleaningTasks))

	// Check status of each task
	for _, task := range serverStatus.CleaningTasks {
		// Skip already completed/failed tasks
		if task.State == taskStateCompleted || task.State == taskStateException || task.State == taskStateCancelled || task.State == taskStateKilled {
			updatedTasks = append(updatedTasks, task)
			if task.State != taskStateCompleted {
				anyFailed = true
			}
			continue
		}

		// Query task status from BMC
		status, err := bmcClient.GetTaskStatus(ctx, task.TaskURI)
		if err != nil {
			log.Error(err, "Failed to get task status", "server", server.Name, "taskURI", task.TaskURI)
			// Keep existing task info, mark as still in progress
			allComplete = false
			updatedTasks = append(updatedTasks, task)
			continue
		}

		// Update task information
		updatedTask := metalv1alpha1.CleaningTaskStatus{
			TaskURI:         task.TaskURI,
			TaskType:        task.TaskType,
			TargetID:        task.TargetID,
			State:           status.State,
			PercentComplete: status.PercentComplete,
			Message:         status.Message,
			LastUpdateTime:  metav1.Now(),
		}
		updatedTasks = append(updatedTasks, updatedTask)

		log.V(1).Info("Task status updated",
			"server", server.Name,
			"taskType", task.TaskType,
			"state", status.State,
			"percentComplete", status.PercentComplete)

		// Check if task is still running
		if status.State != taskStateCompleted && status.State != taskStateException && status.State != taskStateCancelled && status.State != taskStateKilled {
			allComplete = false
		}

		if status.State == taskStateException || status.State == taskStateCancelled || status.State == taskStateKilled {
			anyFailed = true
		}
	}

	// Update task information in status
	if err := r.updateServerTasks(ctx, cleaning, server.Name, updatedTasks); err != nil {
		return false, fmt.Errorf("failed to update task information: %w", err)
	}

	// Calculate overall progress message
	completedCount := 0
	totalPercent := 0
	for _, task := range updatedTasks {
		if task.State == "Completed" {
			completedCount++
		}
		totalPercent += task.PercentComplete
	}
	avgPercent := 0
	if len(updatedTasks) > 0 {
		avgPercent = totalPercent / len(updatedTasks)
	}

	progressMsg := fmt.Sprintf("Cleaning progress: %d%% (%d/%d tasks completed)", avgPercent, completedCount, len(updatedTasks))

	// Update server status based on task completion
	if allComplete {
		if anyFailed {
			log.Info("Cleaning completed with failures", "server", server.Name)
			if err := r.updateServerStatus(ctx, cleaning, server.Name, metalv1alpha1.ServerCleaningStateFailed, progressMsg); err != nil {
				return false, err
			}
		} else {
			log.Info("Cleaning completed successfully", "server", server.Name)
			if err := r.updateServerStatus(ctx, cleaning, server.Name, metalv1alpha1.ServerCleaningStateCompleted, progressMsg); err != nil {
				return false, err
			}
		}
	} else {
		// Update progress message
		if err := r.updateServerStatus(ctx, cleaning, server.Name, metalv1alpha1.ServerCleaningStateInProgress, progressMsg); err != nil {
			return false, err
		}
	}

	return allComplete, nil
}

// updateServerTasks updates the cleaning tasks for a specific server
func (r *ServerCleaningReconciler) updateServerTasks(ctx context.Context, cleaning *metalv1alpha1.ServerCleaning, serverName string, tasks []metalv1alpha1.CleaningTaskStatus) error {
	cleaningBase := cleaning.DeepCopy()

	// Find and update the server status entry
	found := false
	for i := range cleaning.Status.ServerCleaningStatuses {
		if cleaning.Status.ServerCleaningStatuses[i].ServerName == serverName {
			cleaning.Status.ServerCleaningStatuses[i].CleaningTasks = tasks
			cleaning.Status.ServerCleaningStatuses[i].LastUpdateTime = metav1.Now()
			found = true
			break
		}
	}

	// If not found, create new entry with tasks
	if !found {
		cleaning.Status.ServerCleaningStatuses = append(cleaning.Status.ServerCleaningStatuses, metalv1alpha1.ServerCleaningStatusEntry{
			ServerName:     serverName,
			State:          metalv1alpha1.ServerCleaningStateInProgress,
			CleaningTasks:  tasks,
			LastUpdateTime: metav1.Now(),
		})
	}

	if err := r.Status().Patch(ctx, cleaning, client.MergeFrom(cleaningBase)); err != nil {
		return fmt.Errorf("failed to update server tasks: %w", err)
	}
	return nil
}

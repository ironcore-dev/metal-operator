// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"time"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	"github.com/stmcginnis/gofish/schemas"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// BMCTaskReconciler reconciles BMC tasks by polling task status from the BMC.
// This controller is responsible for monitoring all in-progress BMC operations
// and updating task status in BMC.Status.Tasks.
type BMCTaskReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// Insecure allows insecure connections to the BMC.
	Insecure bool
	// BMCOptions contains additional options for BMC clients.
	BMCOptions bmc.Options
	// PollInterval defines how often to poll task status from the BMC.
	PollInterval time.Duration
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcs,verbs=get;list;watch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcsecrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=endpoints,verbs=get;list;watch

// Reconcile monitors BMC tasks and updates their status by polling the BMC.
func (r *BMCTaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Reconciling BMC tasks")

	// Fetch the BMC object
	bmcObj := &metalv1alpha1.BMC{}
	if err := r.Get(ctx, req.NamespacedName, bmcObj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Skip reconciliation if the BMC is being deleted
	if !bmcObj.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	// Skip if there are no tasks to monitor
	if len(bmcObj.Status.Tasks) == 0 {
		log.V(1).Info("No tasks to monitor")
		return ctrl.Result{}, nil
	}

	// Check if there are any non-terminal tasks
	hasActiveTasks := false
	for i := range bmcObj.Status.Tasks {
		task := &bmcObj.Status.Tasks[i]
		if !isTerminalState(task.State) {
			hasActiveTasks = true
			break
		}
	}

	if !hasActiveTasks {
		log.V(1).Info("All tasks are in terminal state")
		return ctrl.Result{}, nil
	}

	// Get BMC client
	bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, r.Client, bmcObj, r.Insecure, r.BMCOptions)
	if err != nil {
		log.V(1).Info("Failed to get BMC client, will retry", "error", err)
		// Don't fail the reconciliation, just requeue
		return ctrl.Result{RequeueAfter: r.PollInterval}, nil
	}
	defer bmcClient.Logout()

	// Poll and update task statuses
	needsUpdate := false
	for i := range bmcObj.Status.Tasks {
		task := &bmcObj.Status.Tasks[i]

		// Skip tasks in terminal states
		if isTerminalState(task.State) {
			continue
		}

		// Poll task status from BMC
		taskStatus, err := bmcClient.GetTaskStatus(ctx, task.TaskURI)
		if err != nil {
			log.V(1).Info("Failed to get task status", "taskURI", task.TaskURI, "error", err)
			continue
		}

		// Update task if status changed
		if taskStatus != nil {
			oldState := task.State
			oldPercent := task.PercentComplete

			task.State = taskStatus.State
			task.PercentComplete = int32(taskStatus.PercentComplete)
			task.Message = taskStatus.Message
			task.LastUpdateTime = metav1.Now()

			// Log if status changed
			if oldState != task.State || oldPercent != task.PercentComplete {
				log.V(1).Info("Updated task status",
					"taskURI", task.TaskURI,
					"taskType", task.TaskType,
					"state", task.State,
					"percentComplete", task.PercentComplete)
				needsUpdate = true
			}
		}
	}

	// Persist changes if any tasks were updated
	if needsUpdate {
		bmcBase := bmcObj.DeepCopy()
		if err := r.Status().Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
			log.Error(err, "Failed to update BMC task status")
			return ctrl.Result{}, err
		}
		log.V(1).Info("Successfully updated BMC task status")
	}

	// Requeue to continue monitoring active tasks
	return ctrl.Result{RequeueAfter: r.PollInterval}, nil
}

// isTerminalState checks if a task state is terminal (no further updates expected).
func isTerminalState(state string) bool {
	return state == "Completed" ||
		state == "Failed" ||
		state == string(schemas.CompletedTaskState) ||
		state == string(schemas.KilledTaskState) ||
		state == string(schemas.ExceptionTaskState) ||
		state == string(schemas.CancelledTaskState)
}

// SetupWithManager sets up the controller with the Manager.
func (r *BMCTaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BMC{}).
		WithEventFilter(hasTasksPredicate()).
		Complete(r)
}

// hasTasksPredicate filters BMC events to only reconcile BMCs that have tasks.
func hasTasksPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			bmc, ok := e.Object.(*metalv1alpha1.BMC)
			if !ok {
				return false
			}
			return len(bmc.Status.Tasks) > 0
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			bmcNew, ok := e.ObjectNew.(*metalv1alpha1.BMC)
			if !ok {
				return false
			}
			return len(bmcNew.Status.Tasks) > 0
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Don't reconcile on delete
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			bmc, ok := e.Object.(*metalv1alpha1.BMC)
			if !ok {
				return false
			}
			return len(bmc.Status.Tasks) > 0
		},
	}
}

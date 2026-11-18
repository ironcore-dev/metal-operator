package task

import (
	"time"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Result struct {
	// Pending indicates the task is not yet completed
	// and an external trigger will eventually show when it is.
	Pending bool
	// RetryAfter indicates a task can be checked again after
	// a certain time.
	RetryAfter time.Duration
}

func (r *Result) Done() bool {
	return !r.Pending && r.RetryAfter <= 0
}

func (r *Result) AsReconcileResult() reconcile.Result {
	return reconcile.Result{
		RequeueAfter: r.RetryAfter,
	}
}

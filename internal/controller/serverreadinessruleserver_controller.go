// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"slices"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	serverReadinessRuleBootstrapCompletedAnnotationPrefix = "readiness.metal.ironcore.dev/bootstrap-completed-"
)

type ServerReadinessRuleServerReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverreadinessrules,verbs=get;list;watch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch;patch;update

func (r *ServerReadinessRuleServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, req.NamespacedName, server); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	ruleList := &metalv1alpha1.ServerReadinessRuleList{}
	if err := r.List(ctx, ruleList); err != nil {
		return ctrl.Result{}, fmt.Errorf("listing server readiness rules: %w", err)
	}

	var (
		modified bool
		base     = server.DeepCopy()
	)
	for _, rule := range ruleList.Items {
		log := log.WithValues("ServerReadinessRule", klog.KObj(&rule))

		if !rule.DeletionTimestamp.IsZero() {
			log.V(5).Info("Skipping deleting rule")
			continue
		}

		if rule.Spec.EnforcementMode == metalv1alpha1.EnforcementModeBootstrapOnly &&
			r.isServerReadinessRuleBootstrapCompleted(server, rule.Name) {
			log.V(5).Info("Skipping bootstrap completed rule")
			continue
		}

		log.V(5).Info("Evaluating rule")
		matches, err := r.ruleMatchesServer(server, &rule)
		if err != nil {
			log.Error(err, "Error evaluating whether rule matches server")
			continue
		}
		if !matches {
			log.V(5).Info("Rule does not match server")
			continue
		}

		if ok := r.evalRule(server, &rule); ok {
			log.V(5).Info("Server passes rule")
			modified = removeTaint(server, rule.Spec.Taint) || modified
			if rule.Spec.EnforcementMode == metalv1alpha1.EnforcementModeBootstrapOnly {
				modified = setBootstrapCompleted(server, rule.Name) || modified
			}
			continue
		}

		log.V(5).Info("Server fails rule")
		modified = addTaint(server, rule.Spec.Taint) || modified
	}
	if !modified {
		log.V(1).Info("Server was not modified")
		return ctrl.Result{}, nil
	}

	log.V(1).Info("Updating server readiness")
	if err := r.Patch(ctx, server, client.MergeFrom(base)); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating server readiness: %w", err)
	}

	return ctrl.Result{}, nil
}

func equalTaints(a, b metalv1alpha1.Taint) bool {
	return a.Key == b.Key && a.Effect == b.Effect && a.Value == b.Value
}

func removeTaint(server *metalv1alpha1.Server, toRemove metalv1alpha1.Taint) (modified bool) {
	idx := slices.IndexFunc(server.Spec.Taints, func(t metalv1alpha1.Taint) bool { return equalTaints(t, toRemove) })
	if idx < 0 {
		return false
	}
	server.Spec.Taints = slices.Delete(server.Spec.Taints, idx, idx+1)
	return true
}

func addTaint(server *metalv1alpha1.Server, toAdd metalv1alpha1.Taint) (modified bool) {
	idx := slices.IndexFunc(server.Spec.Taints, func(t metalv1alpha1.Taint) bool { return equalTaints(t, toAdd) })
	if idx >= 0 {
		return false
	}
	server.Spec.Taints = append(server.Spec.Taints, toAdd)
	return true
}

func setBootstrapCompleted(server *metalv1alpha1.Server, ruleName string) (modified bool) {
	if _, ok := server.Annotations[serverReadinessRuleBootstrapCompletedAnnotationPrefix+ruleName]; ok {
		return false
	}
	metav1.SetMetaDataAnnotation(&server.ObjectMeta, serverReadinessRuleBootstrapCompletedAnnotationPrefix+ruleName, "true")
	return true
}

func (r *ServerReadinessRuleServerReconciler) ruleMatchesServer(
	server *metalv1alpha1.Server,
	rule *metalv1alpha1.ServerReadinessRule,
) (bool, error) {
	sel, err := metav1.LabelSelectorAsSelector(&rule.Spec.ServerSelector)
	if err != nil {
		return false, fmt.Errorf("parsing rule %s server selector: %w", rule.Name, err)
	}

	return sel.Matches(labels.Set(server.GetLabels())), nil
}

func (r *ServerReadinessRuleServerReconciler) evalRule(
	server *metalv1alpha1.Server,
	rule *metalv1alpha1.ServerReadinessRule,
) bool {
	reqs := r.compileServerReadinessRuleRequirements(rule)
	return r.serverMatchesServerReadinessRuleRequirements(server, reqs)
}

func (r *ServerReadinessRuleServerReconciler) compileServerReadinessRuleRequirements(rule *metalv1alpha1.ServerReadinessRule) map[string]metav1.ConditionStatus {
	res := make(map[string]metav1.ConditionStatus, len(rule.Spec.Conditions))
	for _, condition := range rule.Spec.Conditions {
		res[condition.Type] = condition.RequiredStatus
	}
	return res
}

func (r *ServerReadinessRuleServerReconciler) isServerReadinessRuleBootstrapCompleted(server *metalv1alpha1.Server, ruleName string) bool {
	_, ok := server.Annotations[serverReadinessRuleBootstrapCompletedAnnotationPrefix+ruleName]
	return ok
}

func (r *ServerReadinessRuleServerReconciler) serverMatchesServerReadinessRuleRequirements(
	server *metalv1alpha1.Server,
	reqs map[string]metav1.ConditionStatus,
) bool {
	seenTypes := sets.New[string]()
	for _, cond := range server.Status.Conditions {
		if expected, ok := reqs[cond.Type]; ok {
			if expected != cond.Status {
				return false
			}

			seenTypes.Insert(cond.Type)
		}
	}
	return seenTypes.Len() == len(reqs)
}

func (r *ServerReadinessRuleServerReconciler) enqueueByServerReadinessRule() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		log := ctrl.LoggerFrom(ctx)

		rule := obj.(*metalv1alpha1.ServerReadinessRule)
		sel, err := metav1.LabelSelectorAsSelector(&rule.Spec.ServerSelector)
		if err != nil {
			log.Error(err, "Parsing label selector")
			return nil
		}

		serverList := &metalv1alpha1.ServerList{}
		if err := r.List(ctx, serverList, client.MatchingLabelsSelector{Selector: sel}); err != nil {
			log.Error(err, "Listing servers")
			return nil
		}

		reqs := make([]reconcile.Request, 0, len(serverList.Items))
		for _, server := range serverList.Items {
			reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&server)})
		}
		return reqs
	})
}

func (r *ServerReadinessRuleServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.Server{}).
		Named("serverreadinessruleserver").
		Watches(
			&metalv1alpha1.ServerReadinessRule{},
			r.enqueueByServerReadinessRule(),
			builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
				return obj.GetDeletionTimestamp().IsZero()
			})),
		).
		Complete(r)
}

// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
    "context"
    "errors"
    "fmt"
    "time"

    "github.com/ironcore-dev/controller-utils/clientutils"
    metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/labels"
    "k8s.io/apimachinery/pkg/runtime"
    utilvalidation "k8s.io/apimachinery/pkg/util/validation"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/builder"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
    "sigs.k8s.io/controller-runtime/pkg/handler"
    "sigs.k8s.io/controller-runtime/pkg/predicate"
)

const NICVersionSetFinalizer = "metal.ironcore.dev/nicversionset"

// NICVersionSetReconciler reconciles a NICVersionSet object
type NICVersionSetReconciler struct {
    client.Client
    Scheme         *runtime.Scheme
    ResyncInterval time.Duration
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=nicversionsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=nicversionsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=nicversionsets/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=nicversions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch

func (r *NICVersionSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    versionSet := &metalv1alpha1.NICVersionSet{}
    if err := r.Get(ctx, req.NamespacedName, versionSet); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    return r.reconcileExists(ctx, versionSet)
}

func (r *NICVersionSetReconciler) reconcileExists(ctx context.Context, versionSet *metalv1alpha1.NICVersionSet) (ctrl.Result, error) {
    if !versionSet.DeletionTimestamp.IsZero() {
        return r.delete(ctx, versionSet)
    }
    return r.reconcile(ctx, versionSet)
}

func (r *NICVersionSetReconciler) delete(ctx context.Context, versionSet *metalv1alpha1.NICVersionSet) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)
    log.V(1).Info("Deleting NICVersionSet")

    if !controllerutil.ContainsFinalizer(versionSet, NICVersionSetFinalizer) {
        return ctrl.Result{}, nil
    }

    if err := r.handleIgnoreAnnotationPropagation(ctx, versionSet); err != nil {
        return ctrl.Result{}, err
    }

    versions, err := r.getOwnedNICVersions(ctx, versionSet)
    if err != nil {
        return ctrl.Result{}, fmt.Errorf("failed to get owned NICVersions: %w", err)
    }

    status := r.getOwnedNICVersionSetStatus(versions)
    if status.AvailableNICVersion != (status.CompletedNICVersion+status.FailedNICVersion) ||
        versionSet.Status.AvailableNICVersion != status.AvailableNICVersion {
        if err = r.patchStatus(ctx, status, versionSet); err != nil {
            return ctrl.Result{}, fmt.Errorf("failed to patch NICVersionSet status: %w", err)
        }
        log.V(1).Info("NICVersionSet status patched", "Status", status)

        if err := r.handleRetryAnnotationPropagation(ctx, versionSet); err != nil {
            return ctrl.Result{}, err
        }
        log.Info("Waiting on the created NICVersion to reach terminal status")
        return ctrl.Result{}, nil
    }

    log.V(1).Info("Ensuring that the finalizer is removed")
    if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, versionSet, NICVersionSetFinalizer); err != nil || modified {
        return ctrl.Result{}, err
    }

    log.V(1).Info("Deleted NICVersionSet")
    return ctrl.Result{}, nil
}

func (r *NICVersionSetReconciler) handleIgnoreAnnotationPropagation(ctx context.Context, versionSet *metalv1alpha1.NICVersionSet) error {
    log := ctrl.LoggerFrom(ctx)
    versions, err := r.getOwnedNICVersions(ctx, versionSet)
    if err != nil {
        return err
    }
    if len(versions.Items) == 0 {
        log.V(1).Info("No NICVersions found, skipping ignore annotation propagation")
        return nil
    }
    return handleIgnoreAnnotationPropagation(ctx, r.Client, versionSet, versions)
}

func (r *NICVersionSetReconciler) handleRetryAnnotationPropagation(ctx context.Context, versionSet *metalv1alpha1.NICVersionSet) error {
    log := ctrl.LoggerFrom(ctx)
    versions, err := r.getOwnedNICVersions(ctx, versionSet)
    if err != nil {
        return err
    }
    if len(versions.Items) == 0 {
        log.V(1).Info("No NICVersion found, skipping retry annotation propagation")
        return nil
    }
    return handleRetryAnnotationPropagation(ctx, r.Client, versionSet, versions)
}

func (r *NICVersionSetReconciler) reconcile(ctx context.Context, versionSet *metalv1alpha1.NICVersionSet) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)
    log.V(1).Info("Reconciling NICVersionSet")

    if err := r.handleIgnoreAnnotationPropagation(ctx, versionSet); err != nil {
        return ctrl.Result{}, err
    }

    if shouldIgnoreReconciliation(versionSet) {
        log.V(1).Info("Skipped NICVersionSet reconciliation")
        return ctrl.Result{}, nil
    }

    if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, versionSet, NICVersionSetFinalizer); err != nil || modified {
        return ctrl.Result{}, err
    }

    servers, err := r.getServersBySelector(ctx, versionSet)
    if err != nil {
        return ctrl.Result{}, fmt.Errorf("failed to get servers by selector: %w", err)
    }

    versions, err := r.getOwnedNICVersions(ctx, versionSet)
    if err != nil {
        return ctrl.Result{}, fmt.Errorf("failed to get owned NICVersions: %w", err)
    }

    log.V(1).Info("Summary of Servers and NICVersions", "ServerCount", len(servers.Items), "NICVersionCount", len(versions.Items))

    if err := r.ensureNICVersionsForServers(ctx, servers, versions, versionSet); err != nil {
        return ctrl.Result{}, fmt.Errorf("failed to create NICVersions: %w", err)
    }

    if err := r.deleteOrphanNICVersions(ctx, servers, versions); err != nil {
        return ctrl.Result{}, fmt.Errorf("failed to delete orphaned NICVersions: %w", err)
    }

    var pendingPatchingVersion bool
    if pendingPatchingVersion, err = r.patchNICVersionFromTemplate(ctx, &versionSet.Spec.NICVersionTemplate, versions); err != nil {
        return ctrl.Result{}, fmt.Errorf("failed to patch NICVersion spec from template: %w", err)
    }

    log.V(1).Info("Updating the status of NICVersionSet")
    status := r.getOwnedNICVersionSetStatus(versions)
    status.FullyLabeledServers = int32(len(servers.Items))

    if err := r.patchStatus(ctx, status, versionSet); err != nil {
        return ctrl.Result{}, fmt.Errorf("failed to update NICVersionSet status: %w", err)
    }
    log.V(1).Info("Patched NICVersionSet status", "Status", status)

    if err := r.handleRetryAnnotationPropagation(ctx, versionSet); err != nil {
        return ctrl.Result{}, err
    }

    if status.FullyLabeledServers != status.AvailableNICVersion || pendingPatchingVersion {
        log.V(1).Info("Waiting for all NICVersion to be created/Patched for the labeled Servers", "Status", status)
        return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
    }

    log.V(1).Info("Reconciled NICVersionSet")
    return ctrl.Result{}, nil
}

func (r *NICVersionSetReconciler) ensureNICVersionsForServers(ctx context.Context, servers *metalv1alpha1.ServerList, versions *metalv1alpha1.NICVersionList, versionSet *metalv1alpha1.NICVersionSet) error {
    log := ctrl.LoggerFrom(ctx)
    withNICVersion := make(map[string]bool)
    for _, version := range versions.Items {
        withNICVersion[version.Spec.ServerRef.Name] = true
    }

    var errs []error
    for _, server := range servers.Items {
        if !withNICVersion[server.Name] {
            var newNICVersion *metalv1alpha1.NICVersion
            newNICVersionName := fmt.Sprintf("%s-%s", versionSet.Name, server.Name)
            if len(newNICVersionName) > utilvalidation.DNS1123SubdomainMaxLength {
                log.V(1).Info("NICVersion name is too long, it will be shortened using random string", "NICVersionName", newNICVersionName)
                newNICVersion = &metalv1alpha1.NICVersion{
                    ObjectMeta: metav1.ObjectMeta{
                        GenerateName: newNICVersionName[:utilvalidation.DNS1123SubdomainMaxLength-10] + "-",
                    },
                }
            } else {
                newNICVersion = &metalv1alpha1.NICVersion{
                    ObjectMeta: metav1.ObjectMeta{
                        Name: newNICVersionName,
                    },
                }
            }

            opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, newNICVersion, func() error {
                newNICVersion.Spec.NICVersionTemplate = *versionSet.Spec.NICVersionTemplate.DeepCopy()
                newNICVersion.Spec.ServerRef = &corev1.LocalObjectReference{Name: server.Name}
                return controllerutil.SetControllerReference(versionSet, newNICVersion, r.Client.Scheme())
            })
            if err != nil {
                errs = append(errs, err)
            }
            log.V(1).Info("Created NICVersion", "NICVersion", newNICVersion.Name, "Server", server.Name, "Operation", opResult)
        }
    }
    return errors.Join(errs...)
}

func (r *NICVersionSetReconciler) deleteOrphanNICVersions(ctx context.Context, servers *metalv1alpha1.ServerList, versions *metalv1alpha1.NICVersionList) error {
    log := ctrl.LoggerFrom(ctx)
    serverMap := make(map[string]bool)
    for _, server := range servers.Items {
        serverMap[server.Name] = true
    }

    var errs []error
    for _, nicVersion := range versions.Items {
        if !serverMap[nicVersion.Spec.ServerRef.Name] {
            if nicVersion.Status.State == metalv1alpha1.NICVersionStateInProgress {
                log.V(1).Info("Waiting for NICVersion to move out of InProgress state", "NICVersion", nicVersion.Name, "Status", nicVersion.Status)
                continue
            }
            if err := r.Delete(ctx, &nicVersion); err != nil {
                errs = append(errs, err)
            }
        }
    }
    return errors.Join(errs...)
}

func (r *NICVersionSetReconciler) patchNICVersionFromTemplate(ctx context.Context, template *metalv1alpha1.NICVersionTemplate, versions *metalv1alpha1.NICVersionList) (bool, error) {
    log := ctrl.LoggerFrom(ctx)
    if len(versions.Items) == 0 {
        log.V(1).Info("No NICVersion found, skipping spec template update")
        return false, nil
    }

    var pendingPatchingVersion bool
    var errs []error
    for _, version := range versions.Items {
        if version.Status.State == metalv1alpha1.NICVersionStateInProgress {
            pendingPatchingVersion = true
            continue
        }
        opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, &version, func() error {
            version.Spec.NICVersionTemplate = *template.DeepCopy()
            return nil
        })
        if err != nil {
            errs = append(errs, err)
        }
        if opResult != controllerutil.OperationResultNone {
            log.V(1).Info("Patched NICVersion with updated spec", "NICVersion", version.Name, "Operation", opResult)
        }
    }
    return pendingPatchingVersion, errors.Join(errs...)
}

func (r *NICVersionSetReconciler) getOwnedNICVersionSetStatus(versionList *metalv1alpha1.NICVersionList) *metalv1alpha1.NICVersionSetStatus {
    status := &metalv1alpha1.NICVersionSetStatus{}
    status.AvailableNICVersion = int32(len(versionList.Items))
    for _, nicVersion := range versionList.Items {
        switch nicVersion.Status.State {
        case metalv1alpha1.NICVersionStateCompleted:
            status.CompletedNICVersion += 1
        case metalv1alpha1.NICVersionStateFailed:
            status.FailedNICVersion += 1
        case metalv1alpha1.NICVersionStateInProgress:
            status.InProgressNICVersion += 1
        case metalv1alpha1.NICVersionStatePending, "":
            status.PendingNICVersion += 1
        }
    }
    return status
}

func (r *NICVersionSetReconciler) getOwnedNICVersions(ctx context.Context, versionSet *metalv1alpha1.NICVersionSet) (*metalv1alpha1.NICVersionList, error) {
    nicVersionList := &metalv1alpha1.NICVersionList{}
    if err := clientutils.ListAndFilterControlledBy(ctx, r.Client, versionSet, nicVersionList); err != nil {
        return nil, err
    }
    return nicVersionList, nil
}

func (r *NICVersionSetReconciler) getServersBySelector(ctx context.Context, set *metalv1alpha1.NICVersionSet) (*metalv1alpha1.ServerList, error) {
    selector, err := metav1.LabelSelectorAsSelector(&set.Spec.ServerSelector)
    if err != nil {
        return nil, err
    }
    servers := &metalv1alpha1.ServerList{}
    if err := r.List(ctx, servers, client.MatchingLabelsSelector{Selector: selector}); err != nil {
        return nil, err
    }
    return servers, nil
}

func (r *NICVersionSetReconciler) patchStatus(ctx context.Context, status *metalv1alpha1.NICVersionSetStatus, versionSet *metalv1alpha1.NICVersionSet) error {
    versionSetBase := versionSet.DeepCopy()
    versionSet.Status = *status
    if err := r.Status().Patch(ctx, versionSet, client.MergeFrom(versionSetBase)); err != nil {
        return err
    }
    return nil
}

func (r *NICVersionSetReconciler) enqueueByServer(ctx context.Context, obj client.Object) []ctrl.Request {
    log := ctrl.LoggerFrom(ctx)
    server := obj.(*metalv1alpha1.Server)

    setList := &metalv1alpha1.NICVersionSetList{}
    if err := r.List(ctx, setList); err != nil {
        log.Error(err, "failed to list NICVersionSet")
        return nil
    }

    reqs := make([]ctrl.Request, 0)
    for _, set := range setList.Items {
        selector, err := metav1.LabelSelectorAsSelector(&set.Spec.ServerSelector)
        if err != nil {
            log.Error(err, "failed to convert label selector")
            return nil
        }
        if selector.Matches(labels.Set(server.GetLabels())) {
            reqs = append(reqs, ctrl.Request{NamespacedName: client.ObjectKey{Name: set.Name}})
        } else {
            versions, err := r.getOwnedNICVersions(ctx, &set)
            if err != nil {
                log.Error(err, "failed to get owned NICVersions")
                return nil
            }
            for _, version := range versions.Items {
                if version.Spec.ServerRef.Name == server.Name {
                    reqs = append(reqs, ctrl.Request{NamespacedName: client.ObjectKey{Name: set.Name}})
                }
            }
        }
    }
    return reqs
}

// SetupWithManager sets up the controller with the Manager.
func (r *NICVersionSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&metalv1alpha1.NICVersionSet{}).
        Owns(&metalv1alpha1.NICVersion{}).
        Watches(&metalv1alpha1.Server{},
            handler.EnqueueRequestsFromMapFunc(r.enqueueByServer),
            builder.WithPredicates(predicate.LabelChangedPredicate{})).
        Complete(r)
}
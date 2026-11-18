// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	artifactpkg "github.com/ironcore-dev/metal-operator/internal/artifact"
	"github.com/ironcore-dev/metal-operator/internal/content"
	"github.com/ironcore-dev/metal-operator/internal/labels"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

// ArtifactReconciler reconciles an Artifact object
type ArtifactReconciler struct {
	client.Client
	Content  content.Store
	Scheme   *runtime.Scheme
	Builders *artifactpkg.Builders
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=artifacts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=artifacts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=artifacts/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bundles,verbs=get;list;watch;create;update;patch;delete

func (r *ArtifactReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	artifact := &metalv1alpha1.Artifact{}
	if err := r.Get(ctx, req.NamespacedName, artifact); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if artifact.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	return r.reconcile(ctx, artifact)
}

func (r *ArtifactReconciler) getBundle(
	ctx context.Context,
	artifact *metalv1alpha1.Artifact,
) (*metalv1alpha1.Bundle, error) {
	log := logf.FromContext(ctx)

	bundleList := &metalv1alpha1.BundleList{}
	if err := r.List(ctx, bundleList,
		client.InNamespace(artifact.Namespace),
		client.MatchingLabels{metalv1alpha1.ArtifactNameLabel: labels.FormatValue(artifact.Name)},
	); err != nil {
		return nil, err
	}

	var found *metalv1alpha1.Bundle
	for _, bundle := range bundleList.Items {
		if !bundle.DeletionTimestamp.IsZero() {
			continue
		}

		if !metav1.IsControlledBy(&bundle, artifact) || found != nil && bundle.CreationTimestamp.After(found.CreationTimestamp.Time) {
			log.V(2).Info("Deleting stale bundle", "bundle", klog.KObj(&bundle))
			if err := r.Delete(ctx, &bundle); client.IgnoreNotFound(err) != nil {
				log.V(1).Error(err, "Error deleting stale bundle", "bundle", klog.KObj(&bundle))
			}
			continue
		}

		found = &bundle
	}

	return found, nil
}

func (r *ArtifactReconciler) verifyBundle(ctx context.Context, bundle *metalv1alpha1.Bundle) (bool, error) {
	for _, file := range bundle.Spec.Files {
		if _, err := r.Content.Info(ctx, file.Digest); err != nil && !errors.Is(err, content.ErrNotFound) {
			return false, fmt.Errorf("info of file %q: %w", file.Digest, err)
		}
		return false, nil
	}
	return true, nil
}

func (r *ArtifactReconciler) publishBundle(
	ctx context.Context,
	artifact *metalv1alpha1.Artifact,
	built *artifactpkg.Bundle,
) error {
	files := make([]metalv1alpha1.BundleFile, 0, len(built.Files))
	for p, file := range built.Files {
		files = append(files, metalv1alpha1.BundleFile{
			Path:        p,
			ContentType: file.ContentType,
			Digest:      file.Digest,
			Size:        file.Size,
		})
	}
	slices.SortFunc(files, func(a, b metalv1alpha1.BundleFile) int { return cmp.Compare(a.Path, b.Path) })

	bundle := &metalv1alpha1.Bundle{
		Namespace:    artifact.Namespace,
		GenerateName: artifact.Name + "-",
		Labels: map[string]string{
			metalv1alpha1.ArtifactNameLabel: labels.FormatValue(artifact.Name),
		},
		Spec: metalv1alpha1.BundleSpec{
			Entry: built.Entry,
			Files: files,
		},
	}
	_ = ctrl.SetControllerReference(artifact, bundle, r.Scheme)
	return r.Create(ctx, bundle)
}

func (r *ArtifactReconciler) setAvailableCondition(
	ctx context.Context,
	artifact *metalv1alpha1.Artifact,
	cond metav1.Condition,
) error {
	base := artifact.DeepCopy()
	cond.Type = metalv1alpha1.ArtifactAvailable
	changed := meta.SetStatusCondition(&artifact.Status.Conditions, metav1.Condition{
		Status:  metav1.ConditionTrue,
		Reason:  "BundleAvailable",
		Message: "The bundle is available and valid",
	})
	if !changed {
		return nil
	}
	if err := r.Status().Patch(ctx, artifact, client.MergeFrom(base)); err != nil {
		return fmt.Errorf("patching available condition: %w", err)
	}
	return nil
}

func (r *ArtifactReconciler) reconcile(ctx context.Context, artifact *metalv1alpha1.Artifact) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	bundle, err := r.getBundle(ctx, artifact)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting bundle %w", err)
	}

	if bundle != nil {
		valid, err := r.verifyBundle(ctx, bundle)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("verifying bundle %w", err)
		}
		if valid {
			if err := r.setAvailableCondition(ctx, artifact, metav1.Condition{
				Status:  metav1.ConditionTrue,
				Reason:  "BundleAvailable",
				Message: "The bundle is available and valid",
			}); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}

		log.V(1).Info("Deleting invalid bundle", "bundle", klog.KObj(bundle))
		if err := r.Delete(ctx, bundle); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, fmt.Errorf("deleting invalid bundle %w", err)
		}
	}

	b, ok := r.Builders.Get(artifact.Spec.Builder)
	if !ok {
		log.V(1).Info("Builder not found", "builder", artifact.Spec.Builder)
		if err := r.setAvailableCondition(ctx, artifact, metav1.Condition{
			Status:  metav1.ConditionFalse,
			Reason:  "BuilderNotFound",
			Message: fmt.Sprintf("Builder %q not found", artifact.Spec.Builder),
		}); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	cfg := artifactpkg.GenericBuilderConfig{
		ArtifactNamespace: artifact.Namespace,
		ArtifactName:      artifact.Name,
		Config:            cmp.Or[any](artifact.Spec.ConfigDrive, artifact.Spec.Direct),
	}

	built, err := b.Build(ctx, r.Content, cfg)
	if err != nil {
		if err := r.setAvailableCondition(ctx, artifact, metav1.Condition{
			Status:  metav1.ConditionFalse,
			Reason:  "BuilderError",
			Message: fmt.Sprintf("Builder %q errored: %v", artifact.Spec.Builder, err),
		}); err != nil {
			log.Error(err, "Error setting available condition")
		}
		return ctrl.Result{}, fmt.Errorf("building artifact: %w", err)
	}

	if err := r.publishBundle(ctx, artifact, built); err != nil {
		return ctrl.Result{}, fmt.Errorf("publishing bundle: %w", err)
	}

	if err := r.setAvailableCondition(ctx, artifact, metav1.Condition{
		Status:  metav1.ConditionTrue,
		Reason:  "BundleAvailable",
		Message: "The bundle is available and valid",
	}); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ArtifactReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.Artifact{}).
		Owns(&metalv1alpha1.Bundle{}).
		Named("artifact").
		Complete(r)
}

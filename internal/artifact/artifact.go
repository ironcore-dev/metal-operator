package artifact

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"net/url"
	"strings"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/labels"
	"github.com/ironcore-dev/metal-operator/internal/task"
	"github.com/ironcore-dev/metal-operator/internal/xclient"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ServerClaimNameLabel = "metal.ironcore.io/serverclaim-name"
	TypeLabel            = "metal.ironcore.io/artifact-type"
	SecretTypeLabel      = "metal.ironcore.io/secret-type"
)

type ExposeOptions struct {
	Global bool
	Labels map[string]string
}

func Expose(
	ctx context.Context,
	c client.Client,
	namespace string,
	name func() string,
	generateName string,
	artifactName string,
	opts ExposeOptions,
) (*url.URL, task.Result, error) {
	var (
		obj     client.Object
		setSpec func(artifactName string)
		getURL  func() string
	)
	if opts.Global {
		exposure := &metalv1alpha1.GlobalExposure{}
		obj = exposure
		setSpec = func(artifactName string) {
			exposure.Spec = metalv1alpha1.GlobalExposureSpec{
				ArtifactRef: metalv1alpha1.LocalObjectReference{Name: artifactName},
			}
		}
		getURL = func() string {
			return exposure.Status.URL
		}
	} else {
		exposure := &metalv1alpha1.Exposure{}
		obj = exposure
		setSpec = func(artifactName string) {
			exposure.Spec = metalv1alpha1.ExposureSpec{
				ArtifactRef: metalv1alpha1.LocalObjectReference{Name: artifactName},
			}
		}
		getURL = func() string {
			return exposure.Status.URL
		}
	}

	lbls := map[string]string{
		serverClaimNameLabel: labels.FormatValue(claimMeta.Name),
		artifactTypeLabel:    artifactType,
	}
	maps.Copy(lbls, extraLabels)
	if err := xclient.SelectOrCreate(ctx, c, obj,
		func() error {
			if obj.GetName() == "" && name != nil {
				obj.SetName(name())
			}
			obj.SetGenerateName(generateName)
			setSpec(artifactName)
			return nil
		},
		xclient.InNamespace(claimMeta.Namespace),
		xclient.WithLabels(lbls),
		xclient.WithOwner{Object: serverClaim},
	); err != nil {
		return nil, task.Result{}, fmt.Errorf("applying exposure: %w", err)
	}
	urlString := getURL()
	if urlString == "" {
		return nil, task.Result{}, nil
	}
	u, err := url.Parse(urlString)
	if err != nil {
		return nil, task.Result{}, fmt.Errorf("parsing exposure url: %w", err)
	}
	return u, task.Result{}, nil
}

func SelectOrCreate(
	ctx context.Context,
	c client.Client,
	namespace string,
	spec metalv1alpha1.ArtifactSpec,
	owner client.Object,
	extraLabels map[string]string,
) (*metalv1alpha1.Artifact, error) {
	lbls := map[string]string{TypeLabel: strings.ToLower(spec.Builder)}
	maps.Copy(lbls, extraLabels)
	artifact := &metalv1alpha1.Artifact{}
	if err := xclient.SelectOrCreate(ctx, c, artifact,
		func() error {
			artifact.GenerateName = strings.ToLower(spec.Builder) + "-"
			artifact.Spec = spec
			return nil
		},
		xclient.InNamespace(namespace),
		xclient.WithLabels(lbls),
		xclient.WithOwner{Object: owner},
	); err != nil {
		return nil, err
	}
	return artifact, nil
}

func SelectBundle(
	ctx context.Context,
	c client.Client,
	artifact *metalv1alpha1.Artifact,
) (*metalv1alpha1.Bundle, error) {
	bundleList := &metalv1alpha1.BundleList{}
	if err := c.List(ctx, bundleList,
		client.InNamespace(artifact.GetNamespace()),
		client.MatchingLabels{metalv1alpha1.ArtifactNameLabel: labels.FormatValue(artifact.Name)},
	); err != nil {
		return nil, fmt.Errorf("listing bundles for artifact %s: %w", client.ObjectKeyFromObject(artifact), err)
	}

	var found *metalv1alpha1.Bundle
	for _, bundle := range bundleList.Items {
		if !bundle.DeletionTimestamp.IsZero() ||
			!metav1.IsControlledBy(&bundle, artifact) ||
			found != nil && found.CreationTimestamp.Before(&bundle.CreationTimestamp) {
			continue
		}
		found = &bundle
	}
	return found, nil
}

func BundleFile(path string, bundle *metalv1alpha1.Bundle) *metalv1alpha1.BundleFile {
	path = cmp.Or(path, bundle.Spec.Entry)
	for _, file := range bundle.Spec.Files {
		if file.Path == path {
			return &file
		}
	}
	return nil
}

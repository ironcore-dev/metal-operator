package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"strings"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	artifactpkg "github.com/ironcore-dev/metal-operator/internal/artifact"
	"github.com/ironcore-dev/metal-operator/internal/boot"
	"github.com/ironcore-dev/metal-operator/internal/labels"
	"github.com/ironcore-dev/metal-operator/internal/task"
	"github.com/ironcore-dev/metal-operator/internal/xclient"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	serverClaimNameLabel = "metal.ironcore.io/serverclaim-name"
	artifactTypeLabel    = "metal.ironcore.io/artifact-type"
	secretTypeLabel      = "metal.ironcore.io/secret-type"
)

func expose(
	ctx context.Context,
	c client.Client,
	claimMeta boot.ServerClaimMetadata,
	artifactType string,
	name func() string,
	generateName string,
	extraLabels map[string]string,
	artifactName string,
	global bool,
) (*url.URL, task.Result, error) {
	var (
		obj     client.Object
		setSpec func(artifactName string)
		getURL  func() string
	)
	if global {
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

	serverClaim := &metalv1alpha1.ServerClaim{
		Namespace: claimMeta.Namespace,
		Name:      claimMeta.Name,
		UID:       claimMeta.UID,
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

func applyArtifact(
	ctx context.Context,
	c client.Client,
	claimMeta boot.ServerClaimMetadata,
	spec metalv1alpha1.ArtifactSpec,
	extraLabels map[string]string,
) (*metalv1alpha1.Artifact, error) {
	serverClaim := &metalv1alpha1.ServerClaim{
		Namespace: claimMeta.Namespace,
		Name:      claimMeta.Name,
		UID:       claimMeta.UID,
	}
	lbls := map[string]string{
		serverClaimNameLabel: labels.FormatValue(claimMeta.Name),
		artifactTypeLabel:    strings.ToLower(spec.Builder),
	}
	maps.Copy(lbls, extraLabels)
	artifact := &metalv1alpha1.Artifact{}
	if err := xclient.SelectOrCreate(ctx, c, artifact,
		func() error {
			artifact.GenerateName = strings.ToLower(spec.Builder) + "-"
			artifact.Spec = spec
			return nil
		},
		xclient.InNamespace(claimMeta.Namespace),
		xclient.WithLabels(lbls),
		xclient.WithOwner{Object: serverClaim},
	); err != nil {
		return nil, err
	}
	return artifact, nil
}

func applySecret(
	ctx context.Context,
	c client.Client,
	claimMeta boot.ServerClaimMetadata,
	artifactType string,
	secretType string,
	data map[string][]byte,
	extraLabels map[string]string,
) (*corev1.Secret, error) {
	serverClaim := &metalv1alpha1.ServerClaim{
		Namespace: claimMeta.Namespace,
		Name:      claimMeta.Name,
		UID:       claimMeta.UID,
	}
	secret := &corev1.Secret{}
	lbls := map[string]string{
		serverClaimNameLabel: labels.FormatValue(claimMeta.Name),
		artifactTypeLabel:    artifactType,
		secretTypeLabel:      secretType,
	}
	maps.Copy(lbls, extraLabels)
	if err := xclient.SelectOrCreate(ctx, c, secret,
		func() error {
			secret.GenerateName = secretType + "-"
			secret.Data = data
			return nil
		},
		xclient.InNamespace(claimMeta.Namespace),
		xclient.WithLabels(lbls),
		xclient.WithOwner{Object: serverClaim},
	); err != nil {
		return nil, err
	}
	return secret, nil
}

func newArtifactTargetProvider(
	c client.Client,
	builder string,
	newTarget func(url *url.URL) boot.Target,
) *artifactTargetProvider {
	return &artifactTargetProvider{
		client:    c,
		builder:   builder,
		newTarget: newTarget,
	}
}

func NewArtifactISOTargetProvider(c client.Client) boot.TargetProvider {
	return newArtifactTargetProvider(c, artifactpkg.ISOBuilderName, func(url *url.URL) boot.Target {
		return &boot.ISOImage{URI: url}
	})
}

func NewArtifactIPXETargetProvider(c client.Client) boot.TargetProvider {
	return newArtifactTargetProvider(c, artifactpkg.IPXEName, func(url *url.URL) boot.Target {
		return &boot.PXE{}
	})
}

func NewArtifactUKITargetProvider(c client.Client) boot.TargetProvider {
	return newArtifactTargetProvider(c, artifactpkg.UKIBuilderName, func(url *url.URL) boot.Target {
		return &boot.EFIImage{URI: url}
	})
}

type artifactTargetProvider struct {
	client  client.Client
	builder string

	exposureName         func() string
	exposureGenerateName string
	exposeGlobal         bool

	extraLabels map[string]string
	newTarget   func(url *url.URL) boot.Target
}

func (p *artifactTargetProvider) ProvisionTarget(
	ctx context.Context,
	server *metalv1alpha1.Server,
	config boot.TargetConfig,
) (boot.Target, task.Result, error) {
	directCfg := &metalv1alpha1.DirectArtifactConfig{
		ImageRef:  config.ImageRef,
		Cmdline:   config.Cmdline,
		Args:      config.Args,
		OSRelease: config.OSRelease,
	}
	if embedConfigDrive := config.EmbedConfigDrive; embedConfigDrive != nil {
		directCfg.EmbedConfigDrive = &metalv1alpha1.ConfigDriveArtifactConfig{
			ServerID:       embedConfigDrive.Metadata.Name,
			Hostname:       embedConfigDrive.Hostname,
			NetworkDataRef: nil,
			UserDataRef:    nil,
		}
	}

	spec := metalv1alpha1.ArtifactSpec{
		Builder: p.builder,
		Config: metalv1alpha1.ArtifactConfig{
			Direct: directCfg,
		},
	}
	artifact, err := applyArtifact(ctx, p.client, config.Metadata, spec, p.extraLabels)
	if err != nil {
		return nil, task.Result{}, fmt.Errorf("applying artifact: %w", err)
	}
	if !meta.IsStatusConditionTrue(artifact.Status.Conditions, metalv1alpha1.ArtifactAvailable) {
		return nil, task.Result{}, nil
	}

	var exposureName string
	if p.exposureName != nil {
		exposureName = p.exposureName()
	}
	u, res, err := expose(ctx, p.client, config.Metadata, exposureName, p.exposureGenerateName, p.extraLabels, artifact.Name, p.exposeGlobal)
	if err != nil {
		return nil, task.Result{}, fmt.Errorf("exposing artifact: %w", err)
	}
	if !res.Done() {
		return nil, res, nil
	}

	return p.newTarget(u), task.Result{}, nil
}

func (p *artifactTargetProvider) CleanupTarget(ctx context.Context, server *metalv1alpha1.Server, namespacedName types.NamespacedName) (task.Result, error) {
	return task.Result{}, p.client.DeleteAllOf(ctx, &metalv1alpha1.Artifact{},
		client.InNamespace(namespacedName.Namespace),
		client.MatchingLabels{
			serverClaimNameLabel: labels.FormatValue(namespacedName.Name),
			artifactTypeLabel:    strings.ToLower(p.builder),
		},
	)
}

func NewArtifactConfigDriveProvider(c client.Client) boot.ConfigDriveProvider {
	return &artifactConfigDriveProvider{
		client: c,
	}
}

type artifactConfigDriveProvider struct {
	client      client.Client
	extraLabels map[string]string
}

func (p *artifactConfigDriveProvider) ProvisionConfigDrive(
	ctx context.Context,
	server *metalv1alpha1.Server,
	config boot.ConfigDriveConfig,
) (boot.ConfigDrive, task.Result, error) {
	artifactType := strings.ToLower(artifactpkg.ConfigDriveBuilderName)

	var networkDataRef *metalv1alpha1.LocalObjectKeyReference
	if config.Network != nil {
		networkData, err := json.Marshal(config.Network)
		if err != nil {
			return nil, task.Result{}, fmt.Errorf("marshalling network: %w", err)
		}

		networkDataSecret, err := applySecret(ctx,
			p.client,
			config.Metadata,
			"networkdata",
			map[string][]byte{
				metalv1alpha1.DefaultNetworkDataKey: networkData,
			},
			p.extraLabels,
		)
		if err != nil {
			return nil, task.Result{}, fmt.Errorf("applying network data secret: %w", err)
		}

		networkDataRef = &metalv1alpha1.LocalObjectKeyReference{
			Name: networkDataSecret.Name,
			Key:  metalv1alpha1.DefaultNetworkDataKey,
		}
	}

	var userDataRef *metalv1alpha1.LocalObjectKeyReference
	if config.UserData != nil {
		userDataSecret, err := applySecret(ctx,
			p.client,
			config.Metadata,
			"userdata",
			map[string][]byte{
				metalv1alpha1.DefaultUserDataKey: config.UserData,
			},
			p.extraLabels,
		)
		if err != nil {
			return nil, task.Result{}, fmt.Errorf("applying userdata secret: %w", err)
		}

		userDataRef = &metalv1alpha1.LocalObjectKeyReference{
			Name: userDataSecret.Name,
			Key:  metalv1alpha1.DefaultUserDataKey,
		}
	}

	spec := metalv1alpha1.ArtifactSpec{
		Builder: artifactpkg.ConfigDriveBuilderName,
		Config: metalv1alpha1.ArtifactConfig{
			ConfigDrive: &metalv1alpha1.ConfigDriveArtifactConfig{
				ServerID:       config.Metadata.Name,
				Hostname:       config.Hostname,
				NetworkDataRef: networkDataRef,
				UserDataRef:    userDataRef,
			},
		},
	}
	artifact, err := applyArtifact(ctx, p.client, config.Metadata, spec, p.extraLabels)
	if err != nil {
		return nil, task.Result{}, fmt.Errorf("applying artifact: %w", err)
	}
	if !meta.IsStatusConditionTrue(artifact.Status.Conditions, metalv1alpha1.ArtifactAvailable) {
		return nil, task.Result{}, nil
	}

	u, res, err := expose(ctx, p.client, config.Metadata, "", "configdrive-", p.extraLabels, artifact.Name, false)
	if err != nil {
		return nil, task.Result{}, fmt.Errorf("exposing artifact: %w", err)
	}
	if !res.Done() {
		return nil, res, nil
	}

	return &boot.ConfigDriveImage{
		URI: u,
	}, task.Result{}, nil
}

func (p *artifactConfigDriveProvider) CleanupConfigDrive(ctx context.Context, server *metalv1alpha1.Server, namespacedName types.NamespacedName) (task.Result, error) {
	if err := p.client.DeleteAllOf(ctx, &metalv1alpha1.Artifact{},
		client.InNamespace(namespacedName.Namespace),
		client.MatchingLabels{
			serverClaimNameLabel: labels.FormatValue(namespacedName.Name),
			artifactTypeLabel:    strings.ToLower(artifactpkg.ConfigDriveBuilderName),
		},
	); err != nil {
		return task.Result{}, err
	}

	if err := p.client.DeleteAllOf(ctx, &corev1.Secret{},
		client.InNamespace(namespacedName.Namespace),
		client.MatchingLabels{
			serverClaimNameLabel: labels.FormatValue(namespacedName.Name),
			secretTypeLabel:      "userdata",
		},
	); err != nil {
		return task.Result{}, err
	}

	if err := p.client.DeleteAllOf(ctx, &corev1.Secret{},
		client.InNamespace(namespacedName.Namespace),
		client.MatchingLabels{
			serverClaimNameLabel: labels.FormatValue(namespacedName.Name),
			secretTypeLabel:      "network",
		},
	); err != nil {
		return task.Result{}, err
	}

	return task.Result{}, nil
}

package artifact

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/configdrive"
	"github.com/ironcore-dev/metal-operator/internal/content"
	"github.com/opencontainers/go-digest"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ConfigDriveBuilderName = "ConfigDrive"
)

type ConfigDriveBuilder struct {
	client.Client
	ConfigDriveBuilder *configdrive.Builder
}

func (b *ConfigDriveBuilder) Name() string { return ConfigDriveBuilderName }

func (b *ConfigDriveBuilder) buildAndPushConfigDrive(
	ctx context.Context,
	store content.Ingester,
	resolved *resolvedConfigDriveArtifactConfig,
) (dgst digest.Digest, size int64, err error) {
	return content.IngestBuild(ctx, store, func(w io.Writer) error {
		var userData io.Reader
		if resolved.UserData != nil {
			userData = bytes.NewReader(resolved.UserData)
		}

		var networkData io.Reader
		if resolved.NetworkData != nil {
			networkData = bytes.NewReader(resolved.NetworkData)
		}

		return b.ConfigDriveBuilder.Build(w, configdrive.Args{
			Metadata: configdrive.Metadata{
				ServerID: resolved.ServerID,
				Hostname: resolved.Hostname,
			},
			UserData:    userData,
			NetworkData: networkData,
		})
	})
}

type resolvedConfigDriveArtifactConfig struct {
	ServerID    string
	Hostname    string
	NetworkData []byte
	UserData    []byte
}

func (b *ConfigDriveBuilder) getSecretKey(
	ctx context.Context,
	namespace string,
	ref metalv1alpha1.LocalObjectKeyReference,
	defaultKey string,
) ([]byte, error) {
	key := cmp.Or(ref.Key, defaultKey)
	if key == "" {
		return nil, fmt.Errorf("missing key")
	}

	secret := &corev1.Secret{}
	secretKey := client.ObjectKey{
		Namespace: namespace,
		Name:      ref.Name,
	}
	if err := b.Client.Get(ctx, secretKey, secret); err != nil {
		return nil, fmt.Errorf("getting secret %s: %w", secretKey.Name, err)
	}

	data, ok := secret.Data[key]
	if !ok {
		return nil, fmt.Errorf("secret %s does not contain key %s", secretKey.Name, key)
	}
	return data, nil
}

func (b *ConfigDriveBuilder) resolveConfigDriveArtifactConfig(
	ctx context.Context,
	namespace string,
	cfg metalv1alpha1.ConfigDriveArtifactConfig,
) (*resolvedConfigDriveArtifactConfig, error) {
	var networkData []byte
	if ref := cfg.NetworkDataRef; ref != nil {
		data, err := b.getSecretKey(ctx, namespace, *ref, "")
		if err != nil {
			return nil, err
		}

		networkData = data
	}

	var userData []byte
	if ref := cfg.UserDataRef; ref != nil {
		data, err := b.getSecretKey(ctx, namespace, *ref, metalv1alpha1.DefaultUserDataKey)
		if err != nil {
			return nil, err
		}

		userData = data
	}

	return &resolvedConfigDriveArtifactConfig{
		NetworkData: networkData,
		UserData:    userData,
	}, nil
}

func (b *ConfigDriveBuilder) Build(
	ctx context.Context,
	store content.Ingester,
	cfg BuilderConfig[metalv1alpha1.ConfigDriveArtifactConfig],
) (*Bundle, error) {
	resolved, err := b.resolveConfigDriveArtifactConfig(ctx, cfg.ArtifactNamespace, cfg.Config)
	if err != nil {
		return nil, err
	}

	dgst, size, err := b.buildAndPushConfigDrive(ctx, store, resolved)
	if err != nil {
		return nil, err
	}

	return &Bundle{
		Entry: "configdrive",
		Files: map[string]*File{
			"configdrive": {
				Digest:      dgst,
				Size:        size,
				ContentType: "application/octet-stream",
			},
		},
	}, nil
}

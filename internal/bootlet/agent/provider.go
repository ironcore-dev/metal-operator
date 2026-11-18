package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	ironcoreimage "github.com/ironcore-dev/ironcore-image/v2/image"
	"github.com/ironcore-dev/ironcore-image/v2/image/direct"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/agent"
	"github.com/ironcore-dev/metal-operator/internal/boot"
	bootletconfig "github.com/ironcore-dev/metal-operator/internal/bootlet/config"
	"github.com/ironcore-dev/metal-operator/internal/deploy"
	"github.com/ironcore-dev/metal-operator/internal/image"
	"github.com/ironcore-dev/metal-operator/internal/task"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"k8s.io/apimachinery/pkg/types"
	"oras.land/oras-go/v2/errdef"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type Provider struct {
	image         string
	serverBaseURL string

	tokenIssuer *EdDSATokenIssuer
	tokenStore  deploy.AgentTokenStore
	states      StateReader

	repositoryProvider image.RepositoryProvider

	configDriveProvider boot.ConfigDriveProvider

	pollInterval time.Duration
}

type ProviderOptions struct {
	Image         string
	ServerBaseURL string

	TokenIssuer *EdDSATokenIssuer
	TokenStore  deploy.AgentTokenStore

	RepositoryProvider  image.RepositoryProvider
	ConfigDriveProvider boot.ConfigDriveProvider
}

const (
	defaultConfigDriveTag = "configdrive"
)

func NewProvider(opts ProviderOptions) (*Provider, error) {
	if opts.Image == "" {
		return nil, fmt.Errorf("must specify Image")
	}
	if opts.ServerBaseURL == "" {
		return nil, fmt.Errorf("must specify ServerBaseURL")
	}
	if opts.RepositoryProvider == nil {
		opts.RepositoryProvider = image.DefaultRepositoryProvider
	}
	if opts.ConfigDriveProvider == nil {
		return nil, fmt.Errorf("must specify ConfigDriveBuilder")
	}
	return &Provider{
		image:               opts.Image,
		serverBaseURL:       opts.ServerBaseURL,
		repositoryProvider:  opts.RepositoryProvider,
		configDriveProvider: opts.ConfigDriveProvider,
	}, nil
}

func (p *Provider) getToken(ctx context.Context, claimMeta boot.ServerClaimMetadata) (string, error) {
	log := logf.FromContext(ctx)

	tok, err := p.tokenStore.AgentToken(ctx)
	if err != nil {
		return "", fmt.Errorf("getting current token: %w", err)
	}
	if tok != "" {
		ok, err := p.tokenIssuer.Validate(claimMeta.Name, tok)
		if err != nil {
			return "", fmt.Errorf("validating token: %w", err)
		}
		if ok {
			return tok, nil
		}
	}

	log.V(1).Info("Issuing new token")
	tok, err = p.tokenIssuer.Issue(claimMeta.Name)
	if err != nil {
		return "", fmt.Errorf("issuing new token: %w", err)
	}
	if err := p.tokenStore.SetAgentToken(ctx, tok); err != nil {
		return "", fmt.Errorf("setting new token: %w", err)
	}
	return tok, nil
}

func (p *Provider) Provision(ctx context.Context, server *metalv1alpha1.Server, spec agent.Config) (*agent.Spec, task.Result, error) {
	tok, err := p.getToken(ctx, spec.Metadata)
	if err != nil {
		return nil, task.Result{}, fmt.Errorf("getting token: %w", err)
	}

	// TODO: Indicate to the provisioner here that we're creating a config drive for an agent.
	cfgDrive, res, err := p.configDriveProvider.ProvisionConfigDrive(ctx, server, boot.ConfigDriveConfig{
		Metadata: spec.Metadata,
		Hostname: spec.ConfigDrive.Hostname,
		UserData: spec.ConfigDrive.UserData,
		Network:  spec.ConfigDrive.Network,
	})
	if err != nil {
		return nil, task.Result{}, fmt.Errorf("provisioning config drive: %w", err)
	}
	if !res.Done() {
		return nil, res, nil
	}

	cfgDriveImage, ok := cfgDrive.(*boot.ConfigDriveImage)
	if !ok {
		return nil, task.Result{}, fmt.Errorf("unsupported config drive: expected %T, got %T", cfgDriveImage, cfgDrive)
	}

	bootletCfg := bootletconfig.Config{
		BaseURL:        p.serverBaseURL,
		Token:          tok,
		ConfigDriveURL: cfgDriveImage.URI.String(),
		ImageRef:       spec.ImageRef,
	}
	bootletCfgData, err := json.Marshal(bootletCfg)
	if err != nil {
		return nil, task.Result{}, fmt.Errorf("marshalling bootlet config: %w", err)
	}

	src, err := p.repositoryProvider.RepositoryFor(p.image)
	if err != nil {
		return nil, task.Result{}, fmt.Errorf("getting repository for %s: %w", p.image, err)
	}

	desc, err := ironcoreimage.Resolve(ctx, src, p.image, ironcoreimage.ResolveOptions{
		MatchArtifactType: ironcoreimage.MatchArtifactType(direct.ArtifactType),
		MatchPlatform:     ironcoreimage.MatchPlatform(&ocispec.Platform{Architecture: server.Spec.Architecture}),
	})
	if err != nil {
		return nil, task.Result{}, fmt.Errorf("resolving agent image %s: %w", p.image, err)
	}

	imageRef, err := image.ToRef(p.image, desc.Digest)
	if err != nil {
		return nil, task.Result{}, fmt.Errorf("converting image to ref: %w", err)
	}

	return &agent.Spec{
		Handle:   server.UID,
		ImageRef: imageRef,
		UserData: bootletCfgData,
	}, task.Result{}, nil
}

func (p *Provider) CheckCompleted(ctx context.Context, handle any) (task.Result, error) {
	serverID, ok := handle.(string)
	if !ok {
		return task.Result{}, fmt.Errorf("expected handle to be of type string, got %T", handle)
	}

	state, err := p.states.Get(ctx, serverID)
	if err != nil {
		if !errors.Is(err, errdef.ErrNotFound) {
			return task.Result{}, fmt.Errorf("getting state for %s: %w", serverID, err)
		}

		return task.Result{RetryAfter: p.pollInterval}, nil
	}

	if state.Phase != PhaseFinal {
		return task.Result{RetryAfter: p.pollInterval}, nil
	}

	if state.Status != StatusSuccess {
		return task.Result{}, fmt.Errorf("failure in phase %s: %s", state.Phase, state.Message)
	}
	return task.Result{}, nil
}

func (p *Provider) Cleanup(
	ctx context.Context,
	server *metalv1alpha1.Server,
	namespacedName types.NamespacedName,
) (task.Result, error) {
	return p.configDriveProvider.CleanupConfigDrive(ctx, server, namespacedName)
}

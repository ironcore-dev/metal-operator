package daemon

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/ironcore-dev/ironcore-image/v2/image"
	"github.com/ironcore-dev/ironcore-image/v2/image/direct"
	"github.com/ironcore-dev/ironcore-image/v2/xio"
	"github.com/ironcore-dev/metal-operator/sbt/inventory"
	"github.com/ironcore-dev/metal-operator/sbt/server"
	"github.com/ironcore-dev/metal-operator/sbt/store"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
)

var (
	ErrAlreadyExists = errors.New("already exists")
	ErrConflict      = errors.New("conflict")
	ErrNotFound      = errors.New("not found")
)

type ServerConfig struct {
	Name    string
	HostID  string
	Image   string
	Cmdline string
}

type Daemon struct {
	mu      sync.Mutex
	address string
	baseURL string
	started atomic.Pointer[chan struct{}]

	store store.Store

	log       logr.Logger
	stubs     map[string]xio.Source
	inventory inventory.Inventory
	repoFunc  func(repo string) (*remote.Repository, error)
}

type Options struct {
	Log logr.Logger
}

func New(
	address string,
	store store.Store,
	inventory inventory.Inventory,
	repoFunc func(repository string) (*remote.Repository, error),
	stubs map[string]xio.Source,
	opts Options,
) *Daemon {
	if opts.Log.GetSink() == nil {
		opts.Log = logr.Discard()
	}

	return &Daemon{
		address:   address,
		store:     store,
		log:       opts.Log,
		stubs:     maps.Clone(stubs),
		inventory: inventory,
		repoFunc:  repoFunc,
	}
}

func interpretStoreError(resource string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, store.ErrNotFound):
		return fmt.Errorf("%s %w", resource, err)
	case errors.Is(err, store.ErrAlreadyExists):
		return fmt.Errorf("%s %w", resource, err)
	case errors.Is(err, store.ErrConflict):
		return fmt.Errorf("%s %w", resource, err)
	default:
		return err
	}
}

func (d *Daemon) BaseURL() string {
	return d.baseURL
}

func (d *Daemon) getStarted() chan struct{} {
	started := d.started.Load()
	if started != nil {
		return *started
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	started = d.started.Load()
	if started != nil {
		return *started
	}

	startedChan := make(chan struct{})
	d.started.Store(&startedChan)
	return startedChan
}

func (d *Daemon) Started() <-chan struct{} {
	return d.getStarted()
}

func (d *Daemon) Start(ctx context.Context) error {
	var (
		srvErr chan error
		srv    *http.Server
	)
	if err := func() error {
		d.mu.Lock()
		defer d.mu.Unlock()

		ln, err := net.Listen("tcp", d.address)
		if err != nil {
			return err
		}

		srvErr = make(chan error, 1)
		mux := http.NewServeMux()
		mux.HandleFunc("/httpboot/{id}", d.handleHTTPBoot)
		h := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			d.log.Info("Request", "Method", req.Method, "URL", req.URL)
			mux.ServeHTTP(w, req)
		})
		srv = &http.Server{
			Handler: h,
		}

		go func() {
			defer close(srvErr)
			defer func() { _ = ln.Close() }()
			srvErr <- srv.Serve(ln)
		}()

		if d.baseURL == "" {
			d.baseURL = "http://" + ln.Addr().String()
		}
		close(d.getStarted())
		return nil
	}(); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		<-srvErr
		return nil
	case err := <-srvErr:
		return fmt.Errorf("server error: %w", err)
	}
}

func (d *Daemon) resolveImage(ctx context.Context, img string) (imageRef string, desc ocispec.Descriptor, err error) {
	ref, err := registry.ParseReference(img)
	if err != nil {
		return "", ocispec.Descriptor{}, fmt.Errorf("not a registry image reference: %w", err)
	}

	repo, err := d.repoFunc(ref.Host() + "/" + ref.Repository)
	if err != nil {
		return "", ocispec.Descriptor{}, fmt.Errorf("getting repository %q: %w", ref.Repository, err)
	}

	desc, err = image.Resolve(ctx, repo, img, image.ResolveOptions{
		MatchArtifactType: image.MatchArtifactType(direct.ArtifactType),
		OnIterateError:    image.IgnoreImageOrIndexNotFound,
	})
	if err != nil {
		return "", ocispec.Descriptor{}, fmt.Errorf("resolving %q: %w", img, err)
	}

	withDigest := ref
	withDigest.Reference = desc.Digest.String()
	return withDigest.String(), desc, nil
}

func (d *Daemon) ServerCreate(ctx context.Context, cfg *ServerConfig) (string, error) {
	_, err := d.inventory.GetHost(ctx, cfg.HostID)
	if err != nil {
		return "", fmt.Errorf("getting host %q: %w", cfg.HostID, err)
	}

	servers, err := d.store.ListServers(ctx, &store.ServerFilter{HostID: cfg.HostID})
	if err != nil {
		return "", fmt.Errorf("listing servers: %w", err)
	}

	if len(servers) > 0 {
		return "", fmt.Errorf("server for host %q %w", cfg.HostID, ErrAlreadyExists)
	}

	imgRef, _, err := d.resolveImage(ctx, cfg.Image)
	if err != nil {
		return "", fmt.Errorf("resolving image %q: %w", cfg.Image, err)
	}

	srv := &server.Server{
		ID:       uuid.NewString(),
		HostID:   cfg.HostID,
		Image:    cfg.Image,
		ImageRef: imgRef,
		Cmdline:  cfg.Cmdline,
		Status: server.Status{
			State: server.StateCreated,
		},
	}

	if err := d.store.CreateServer(ctx, srv); err != nil {
		return "", fmt.Errorf("creating server: %w", err)
	}

	return srv.ID, nil
}

func (d *Daemon) connectToHost(ctx context.Context, host *inventory.Host) (*gofish.APIClient, error) {
	c, err := gofish.ConnectContext(ctx, gofish.ClientConfig{
		Endpoint:  host.Address,
		Username:  host.User,
		Password:  host.Password,
		BasicAuth: true,
	})
	if err != nil {
		return nil, fmt.Errorf("connecting to host: %w", err)
	}
	return c, nil
}

func (d *Daemon) ServerBoot(ctx context.Context, id string) error {
	srv, err := d.store.GetServer(ctx, id)
	if err != nil {
		return fmt.Errorf("getting server %q: %w", id, err)
	}

	switch srv.Status.State {
	case server.StateBooted:
		return fmt.Errorf("%w: server %q is already booted", ErrConflict, id)
	}

	host, err := d.inventory.GetHost(ctx, srv.HostID)
	if err != nil {
		return fmt.Errorf("getting host %q: %w", srv.HostID, err)
	}

	bmcClient, err := d.connectToHost(ctx, host)
	if err != nil {
		return err
	}
	defer bmcClient.Logout()

	systems, err := bmcClient.Service.Systems()
	if err != nil {
		return fmt.Errorf("getting systems: %w", err)
	}

	var system *schemas.ComputerSystem
	for _, s := range systems {
		if s.ID == host.SystemID {
			system = s
			break
		}
	}
	if system == nil {
		return fmt.Errorf("host %q system %q not found", srv.HostID, host.SystemID)
	}

	httpBootURI := fmt.Sprintf("%s/httpboot/%s", d.baseURL, srv.ID)
	if err := system.SetBoot(&schemas.Boot{
		BootSourceOverrideTarget:  schemas.UefiHTTPBootSource,
		BootSourceOverrideEnabled: schemas.ContinuousBootSourceOverrideEnabled,
		BootSourceOverrideMode:    schemas.UEFIBootSourceOverrideMode,
		HTTPBootURI:               httpBootURI,
	}); err != nil {
		return fmt.Errorf("setting boot: %w", err)
	}

	mon, err := system.Reset(schemas.ForceRestartResetType)
	if err != nil {
		return fmt.Errorf("resetting system: %w", err)
	}

	if mon != nil {
		resp, err := schemas.WaitForTaskMonitor(ctx, bmcClient, 100*time.Millisecond, mon, nil)
		if err != nil {
			return fmt.Errorf("waiting for monitor: %w", err)
		}

		_ = resp.Body.Close()
	}

	srv.Status.State = server.StateStarting
	return nil
}

func (d *Daemon) ServerList(ctx context.Context) ([]server.Server, error) {
	return d.store.ListServers(ctx, nil)
}

func (d *Daemon) ServerGet(ctx context.Context, id string) (*server.Server, error) {
	return d.store.GetServer(ctx, id)
}

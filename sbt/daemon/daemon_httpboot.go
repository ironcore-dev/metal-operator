package daemon

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/ironcore-dev/ironcore-image/v2/image/direct"
	"github.com/ironcore-dev/ironcore-image/v2/ukify"
	"github.com/ironcore-dev/ironcore-image/v2/xcontent"
	"github.com/ironcore-dev/ironcore-image/v2/xio"
	"github.com/ironcore-dev/metal-operator/sbt/server"
	"oras.land/oras-go/v2/registry"
)

type Status struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

func reasonOf(err error) string {
	switch {
	case errors.Is(err, ErrAlreadyExists):
		return "AlreadyExists"
	case errors.Is(err, ErrNotFound):
		return "NotFound"
	case errors.Is(err, ErrConflict):
		return "Conflict"
	default:
		return ""
	}
}

func toStatus(err error) *Status {
	reason := reasonOf(err)
	if reason == "" {
		return nil
	}
	return &Status{
		Reason:  reason,
		Message: err.Error(),
	}
}

func (d *Daemon) handleError(w http.ResponseWriter, r *http.Request, err error) {
	status := toStatus(err)
	if status == nil {
		d.log.Error(err, "Unhandled error")
		status = &Status{
			Message: "Internal Server Error",
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

func (d *Daemon) buildUKIFor(ctx context.Context, srv *server.Server, w io.Writer) error {
	ref, err := registry.ParseReference(srv.ImageRef)
	if err != nil {
		return err
	}

	repo, err := d.repoFunc(ref.Host() + "/" + ref.Repository)
	if err != nil {
		return err
	}

	desc, err := repo.Resolve(ctx, ref.Reference)
	if err != nil {
		return err
	}

	anyImg, err := direct.Decode(ctx, repo, desc)
	if err != nil {
		return fmt.Errorf("inspecting image: %w", err)
	}

	img, ok := anyImg.(*direct.Image)
	if !ok {
		return fmt.Errorf("expected direct.Image, got %T", anyImg)
	}

	cmdline := cmp.Or(srv.Cmdline, img.Config.Cmdline)

	initrds := make([]xio.Source, 0, len(img.Initrds))
	for _, desc := range img.Initrds {
		initrds = append(initrds, xcontent.FetchSource(ctx, repo, desc))
	}

	stub, ok := d.stubs[img.Config.Platform.Architecture]
	if !ok {
		return fmt.Errorf("no stub for architecture %q", img.Config.Platform.Architecture)
	}

	if err := ukify.Build(w, ukify.BuildOptions{
		Stub:      stub,
		Kernel:    xcontent.FetchSource(ctx, repo, img.Kernel),
		Initrds:   initrds,
		Cmdline:   cmdline,
		OSRelease: img.Config.OSRelease,
	}); err != nil {
		return fmt.Errorf("ukifying: %w", err)
	}
	return nil
}

func (d *Daemon) handleHTTPBoot(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	ctx := req.Context()

	srv, err := d.store.GetServer(ctx, id)
	if err != nil {
		d.handleError(w, req, interpretStoreError(fmt.Sprintf("server %s", id), err))
		return
	}

	w.Header().Set("Content-Type", "application/efi")

	if err := d.buildUKIFor(ctx, srv, w); err != nil {
		d.handleError(w, req, err)
		return
	}

	srv.Status.State = server.StateBooted
	if err := d.store.UpdateServer(ctx, srv); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

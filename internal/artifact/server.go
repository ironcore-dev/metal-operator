package artifact

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/content"
	"github.com/ironcore-dev/metal-operator/internal/httpserver"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Server struct {
	listener net.Listener
	addr     string
	client   client.Client
	store    content.Store
}

type ServerOptions struct {
	Addr     string
	Listener net.Listener
	Client   client.Client
	Store    content.Store
}

func NewServer(opts ServerOptions) (*Server, error) {
	if opts.Client == nil {
		return nil, fmt.Errorf("must specify Client")
	}
	if opts.Store == nil {
		return nil, fmt.Errorf("must specify Store")
	}
	return &Server{
		listener: opts.Listener,
		addr:     opts.Addr,
		client:   opts.Client,
		store:    opts.Store,
	}, nil
}

func (s *Server) buildHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /l/{namespace}/{name}/{path...}", s.handleExposure)
	mux.HandleFunc("GET /gl/{name}/{path...}", s.handleGlobalExposure)
	return mux
}

func (s *Server) handleError(w http.ResponseWriter, req *http.Request, err error) {
}

func (s *Server) serveArtifact(
	ctx context.Context,
	w http.ResponseWriter,
	req *http.Request,
	artifact *metalv1alpha1.Artifact,
	path string,
) {
	bundle, err := SelectBundle(ctx, s.client, artifact)
	if err != nil {
		s.handleError(w, req, err)
		return
	}
	if bundle == nil {
		s.handleError(w, req, fmt.Errorf("bundle not found"))
		return
	}

	file := BundleFile(path, bundle)
	if file == nil {
		s.handleError(w, req, fmt.Errorf("file %q not found", path))
		return
	}

	ra, err := s.store.ReaderAt(ctx, file.Digest)
	if err != nil {
		s.handleError(w, req, err)
		return
	}

	http.ServeContent(w, req, file.Path, time.Time{}, io.NewSectionReader(ra, 0, ra.Size()))
}

func (s *Server) handleExposure(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	namespace := req.PathValue("namespace")
	name := req.PathValue("name")
	path := req.PathValue("path")

	exposure := &metalv1alpha1.Exposure{}
	if err := s.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, exposure); err != nil {
		s.handleError(w, req, err)
		return
	}

	artifact := &metalv1alpha1.Artifact{}
	if err := s.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: exposure.Spec.ArtifactRef.Name}, artifact); err != nil {
		s.handleError(w, req, err)
		return
	}

	s.serveArtifact(ctx, w, req, artifact, path)
}

func (s *Server) handleGlobalExposure(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	name := req.PathValue("name")
	path := req.PathValue("path")

	exposureList := &metalv1alpha1.GlobalExposureList{}
	if err := s.client.List(ctx, exposureList); err != nil {
		s.handleError(w, req, err)
		return
	}

	// TODO: By contract, we should only have a single global exposure with a given name.
	// This is just a stand-in until we enforce it.
	var exposure *metalv1alpha1.GlobalExposure
	for _, item := range exposureList.Items {
		if item.Name == name {
			exposure = &item
			break
		}
	}
	if exposure == nil {
		s.handleError(w, req, fmt.Errorf("exposure %q not found", name))
		return
	}

	artifact := &metalv1alpha1.Artifact{}
	artifactKey := client.ObjectKey{Namespace: exposure.Namespace, Name: exposure.Spec.ArtifactRef.Name}
	if err := s.client.Get(ctx, artifactKey, artifact); err != nil {
		s.handleError(w, req, err)
		return
	}

	s.serveArtifact(ctx, w, req, artifact, path)
}

func (s *Server) Start(ctx context.Context) error {
	srv := &http.Server{
		Addr:    s.addr,
		Handler: s.buildHandler(),
	}
	return httpserver.Serve(ctx, srv, httpserver.WithListener(s.listener))
}

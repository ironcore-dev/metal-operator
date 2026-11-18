package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/metal-operator/internal/bootlet/agent"
	"github.com/ironcore-dev/metal-operator/internal/httpserver"
	"golang.org/x/sync/errgroup"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type Server struct {
	listener net.Listener
	addr     string

	authn Authenticator
	store agent.Store

	errorHandler httpserver.ErrorHandler
	logger       logr.Logger
}

type Options struct {
	Listener      net.Listener
	Addr          string
	Authenticator Authenticator
	ErrorHandler  httpserver.ErrorHandler
	Logger        logr.Logger
	Store         agent.Store
}

func NewServer(opts Options) (*Server, error) {
	if opts.Authenticator == nil {
		return nil, fmt.Errorf("must specify Authenticator")
	}
	if opts.Logger.IsZero() {
		opts.Logger = logf.Log.WithName("agent.server")
	}
	if opts.ErrorHandler == nil {
		opts.ErrorHandler = httpserver.DefaultErrorHandler
	}
	if opts.Store == nil {
		return nil, fmt.Errorf("must specify Store")
	}
	if opts.Addr == "" && opts.Listener == nil {
		return nil, fmt.Errorf("must specify Addr or Listener")
	}
	if opts.Addr != "" && opts.Listener != nil {
		return nil, fmt.Errorf("must specify only one of Addr or Listener")
	}

	return &Server{
		addr:         opts.Addr,
		listener:     opts.Listener,
		authn:        opts.Authenticator,
		store:        opts.Store,
		errorHandler: opts.ErrorHandler,
		logger:       opts.Logger,
	}, nil
}

func (s *Server) buildHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/agents/report", s.handleReport)

	handler := http.Handler(mux)
	handler = WithAuthentication(handler, s.authn, s.errorHandler)
	handler = httpserver.WithLogRequests(handler)
	handler = httpserver.WithLogger(handler, s.logger)
	return handler
}

func (s *Server) handleReport(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	u, ok := UserFrom(ctx)
	if !ok {
		s.errorHandler.Handle(w, req, fmt.Errorf("no user present in request"))
		return
	}

	agentUser, ok := u.(*AgentUser)
	if !ok {
		s.errorHandler.Handle(w, req, fmt.Errorf("non-agent user %T used for reporting", u))
		return
	}

	report := &agent.Report{}
	if err := json.NewDecoder(req.Body).Decode(report); err != nil {
		s.errorHandler.Handle(w, req, fmt.Errorf("invalid request body: %v: %w", err, httpserver.ErrBadRequest))
		return
	}

	if err := s.store.Update(ctx, agentUser.Claims.Subject, report); err != nil {
		s.errorHandler.Handle(w, req, err)
		return
	}

	httpserver.WriteJSON(w, &httpserver.Status{
		Message: "Updated status",
	}, http.StatusOK)
}

func (s *Server) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		defer cancel()
		srv := &http.Server{Addr: s.addr}
		return httpserver.Serve(ctx, srv, httpserver.WithListener(s.listener))
	})
	g.Go(func() error {
		s.runGC(ctx)
		return nil
	})
	return g.Wait()
}

func (s *Server) runGC(ctx context.Context) {
	log := logf.FromContext(ctx)

	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			log.V(5).Info("Running GC")
			s.store.GC()
		}
	}
}

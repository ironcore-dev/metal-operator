package fmi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DefaultFMIServer struct {
	address         string
	mux             *http.ServeMux
	shutdownTimeout time.Duration
	taskRunner      TaskRunner
}

// NewDefaultFMIServer creates a new DefaultFMIServer.
func NewDefaultFMIServer(
	address string,
	kubeClient client.Client,
	shutdownTimeout time.Duration,
	insecure bool,
) *DefaultFMIServer {
	mux := http.NewServeMux()
	taskRunner := NewDefaultTaskRunner(kubeClient, insecure)
	server := &DefaultFMIServer{
		address:         address,
		mux:             mux,
		shutdownTimeout: shutdownTimeout,
		taskRunner:      taskRunner,
	}
	server.registerRoutes()
	return server
}

// registerRoutes registers the server's routes.
func (s *DefaultFMIServer) registerRoutes() {
	s.mux.HandleFunc("/scan", s.scanHandler)
	s.mux.HandleFunc("/settings-apply", s.settingsApplyHandler)
	s.mux.HandleFunc("/version-update", s.versionUpdateHandler)
}

// scanHandler handles the /scan endpoint.
func (s *DefaultFMIServer) scanHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload TaskPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	result, err := s.taskRunner.ExecuteScan(r.Context(), payload.ServerBIOSRef)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	_ = json.NewEncoder(w).Encode(result)
}

// settingsApplyHandler handles the /settings-apply endpoint.
func (s *DefaultFMIServer) settingsApplyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload TaskPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := s.taskRunner.ExecuteSettingsApply(r.Context(), payload.ServerBIOSRef); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// versionUpdateHandler handles the /version-update endpoint.
func (s *DefaultFMIServer) versionUpdateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload TaskPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := s.taskRunner.ExecuteVersionUpdate(r.Context(), payload.ServerBIOSRef); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// Start starts the FMI server.
func (s *DefaultFMIServer) Start(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	server := &http.Server{
		Addr:    s.address,
		Handler: s.mux,
	}

	g.Go(func() error {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	})

	g.Go(func() error {
		return server.ListenAndServe()
	})

	return g.Wait()
}

// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package fmi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"
)

type ServerHTTP struct {
	TaskRunner
	address         string
	mux             *http.ServeMux
	shutdownTimeout time.Duration
}

// NewServerHTTP creates a new ServerHTTP.
func NewServerHTTP(config ServerConfig, insecureBMC bool) (*ServerHTTP, error) {
	mux := http.NewServeMux()
	taskRunner, err := NewTaskRunner(config.TaskRunnerType, config.KubeClient, insecureBMC)
	if err != nil {
		return nil, err
	}
	server := &ServerHTTP{
		TaskRunner:      taskRunner,
		address:         fmt.Sprintf("%s:%d", config.Hostname, config.Port),
		mux:             mux,
		shutdownTimeout: config.ShutdownTimeout,
	}
	server.registerRoutes()
	return server, nil
}

// Start starts the server.
func (s *ServerHTTP) Start(ctx context.Context) error {
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

// registerRoutes registers the server's routes.
func (s *ServerHTTP) registerRoutes() {
	s.mux.HandleFunc("/scan", s.scanHandler)
	s.mux.HandleFunc("/settings-apply", s.settingsApplyHandler)
	s.mux.HandleFunc("/version-update", s.versionUpdateHandler)
}

// scanHandler handles the /scan endpoint.
func (s *ServerHTTP) scanHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload TaskPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	result, err := s.ExecuteScan(r.Context(), payload.ServerBIOSRef)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	_ = json.NewEncoder(w).Encode(result)
}

// settingsApplyHandler handles the /settings-apply endpoint.
func (s *ServerHTTP) settingsApplyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload TaskPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	result, err := s.ExecuteSettingsApply(r.Context(), payload.ServerBIOSRef)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
}

// versionUpdateHandler handles the /version-update endpoint.
func (s *ServerHTTP) versionUpdateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload TaskPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := s.ExecuteVersionUpdate(r.Context(), payload.ServerBIOSRef); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

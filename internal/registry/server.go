// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/go-logr/logr"

	"github.com/ironcore-dev/metal-operator/internal/api/registry"
)

// Server holds the HTTP server's state, including the systems store.
type Server struct {
	addr         string
	mux          *http.ServeMux
	systemsStore *sync.Map
	log          logr.Logger
}

// NewServer initializes and returns a new Server instance.
func NewServer(log logr.Logger, addr string) *Server {
	mux := http.NewServeMux()
	server := &Server{
		addr:         addr,
		mux:          mux,
		systemsStore: &sync.Map{},
		log:          log,
	}
	server.routes()
	return server
}

// routes registers the server's routes.
func (s *Server) routes() {
	s.mux.HandleFunc("/register", s.registerHandler)
	s.mux.HandleFunc("/delete/", s.deleteHandler)
	s.mux.HandleFunc("/systems/", s.systemsHandler)
}

// registerHandler handles the /register endpoint.
func (s *Server) registerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	var reg registry.RegistrationPayload
	if err := json.NewDecoder(r.Body).Decode(&reg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Store the registration information.
	s.systemsStore.Store(reg.SystemUUID, reg.Data)
	s.log.Info("Registered system UUID", "uuid", reg.SystemUUID)
	w.WriteHeader(http.StatusCreated)
}

// systemsHandler handles the /systems/{uuid} endpoint.
func (s *Server) systemsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET method is allowed", http.StatusMethodNotAllowed)
		return
	}

	uuid := r.URL.Path[len("/systems/"):]

	if value, ok := s.systemsStore.Load(uuid); ok {
		server, ok := value.(registry.Server)
		if !ok {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			s.log.Info("Error asserting type of endpoints", "uuid", uuid)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(server); err != nil {
			http.Error(w, "Failed to encode result", http.StatusInternalServerError)
			s.log.Error(err, "Error encoding server")
		}
	} else {
		s.log.Info("System not found", "uuid", uuid)
		http.NotFound(w, r)
	}
}

// deleteHandler handles the DELETE requests to remove a system by UUID.
func (s *Server) deleteHandler(w http.ResponseWriter, r *http.Request) {
	s.log.Info("Received delete request", "method", r.Method, "uri", r.RequestURI)

	if r.Method != http.MethodDelete {
		http.Error(w, "Only DELETE method is allowed", http.StatusMethodNotAllowed)
		return
	}

	uuid := r.URL.Path[len("/delete/"):] // Assuming the URL is like /delete/{uuid}

	// Attempt to delete the entry from the store
	if _, ok := s.systemsStore.Load(uuid); !ok {
		http.NotFound(w, r)
		return
	}

	s.systemsStore.Delete(uuid) // Perform the deletion

	// Respond with success message
	w.WriteHeader(http.StatusOK)
	s.log.Info("Deleted system UUID", "uuid", uuid)
}

// Start starts the server on the specified address and adds logging for key events.
func (s *Server) Start(ctx context.Context) error {
	s.log.Info("Starting registry server", "address", s.addr)
	server := &http.Server{Addr: s.addr, Handler: s.mux}

	// Start the server in a new goroutine.
	errChan := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errChan <- fmt.Errorf("HTTP registry server ListenAndServe: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		s.log.Info("Shutting down registry server...")
		if err := server.Shutdown(ctx); err != nil {
			return fmt.Errorf("HTTP server Shutdown: %w", err)
		}
		s.log.Info("Registry server graciously stopped")
		return nil
	case err := <-errChan:
		// In case of server startup error, attempt to shut down gracefully before returning the error.
		if shutdownErr := server.Shutdown(ctx); shutdownErr != nil {
			s.log.Error(shutdownErr, "Error shutting down registry server")
		}
		return err
	}
}

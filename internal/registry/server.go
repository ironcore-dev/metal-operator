/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/ironcore-dev/metal-operator/internal/api/registry"
)

// Ensure the log output goes to standard out (this is useful if you're running in a containerized environment).
func init() {
	log.SetOutput(os.Stdout)
}

// Server holds the HTTP server's state, including the systems store.
type Server struct {
	addr         string
	mux          *http.ServeMux
	systemsStore *sync.Map
}

// NewServer initializes and returns a new Server instance.
func NewServer(addr string) *Server {
	mux := http.NewServeMux()
	server := &Server{
		addr:         addr,
		mux:          mux,
		systemsStore: &sync.Map{},
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
	log.Printf("Registered system UUID: %s\n", reg.SystemUUID)
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
			log.Println("Error asserting type of endpoints")
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(server); err != nil {
			log.Printf("Failed to encode result: %v\n", err)
			http.Error(w, "Failed to encode result", http.StatusInternalServerError)
		}
	} else {
		log.Printf("System UUID not found: %s\n", uuid)
		http.NotFound(w, r)
	}
}

// deleteHandler handles the DELETE requests to remove a system by UUID.
func (s *Server) deleteHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received method: %s", r.Method)   // This will log the method of the request
	log.Printf("Requested URI: %s", r.RequestURI) // This logs the full request URI

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
	log.Printf("System with UUID %s deleted successfully", uuid)
}

// Start starts the server on the specified address and adds logging for key events.
func (s *Server) Start(ctx context.Context) error {
	log.Printf("Starting registry server on port %s\n", s.addr)
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
		log.Println("Shutting down registry server...")
		if err := server.Shutdown(context.Background()); err != nil {
			return fmt.Errorf("HTTP server Shutdown: %w", err)
		}
		log.Println("Registry server gracefully stopped.")
		return nil
	case err := <-errChan:
		// In case of server startup error, attempt to shutdown gracefully before returning the error.
		if shutdownErr := server.Shutdown(context.Background()); shutdownErr != nil {
			log.Printf("HTTP registry server Shutdown: %v", shutdownErr)
		}
		return err
	}
}

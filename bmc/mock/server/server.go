// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
)

var (
	//go:embed data/**
	dataFS embed.FS
)

type Collection struct {
	Members []Member `json:"Members"`
}

type Member struct {
	OdataID string `json:"@odata.id"`
}

type MockServer struct {
	log       logr.Logger
	addr      string
	handler   http.Handler
	mu        sync.RWMutex
	overrides map[string]any
}

func NewMockServer(log logr.Logger, addr string) *MockServer {
	mux := http.NewServeMux()
	server := &MockServer{
		addr:      addr,
		log:       log,
		overrides: make(map[string]any),
	}

	mux.HandleFunc("/redfish/v1/", server.redfishHandler)
	server.handler = mux

	return server
}

func (s *MockServer) redfishHandler(w http.ResponseWriter, r *http.Request) {
	s.log.Info("Received request", "method", r.Method, "path", r.URL.Path)

	switch r.Method {
	case http.MethodGet:
		s.handleRedfishGET(w, r)
	case http.MethodPost:
		s.handleRedfishPOST(w, r)
	case http.MethodPatch:
		s.handleRedfishPATCH(w, r)
	case http.MethodDelete:
		s.handleRedfishDelete(w, r)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *MockServer) handleRedfishPATCH(w http.ResponseWriter, r *http.Request) {
	s.log.Info("Received request", "method", r.Method, "path", r.URL.Path)

	urlPath := resolvePath(r.URL.Path)
	body, err := io.ReadAll(r.Body)
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			s.log.Error(err, "Failed to close request body")
		}
	}(r.Body)

	if err != nil || len(body) == 0 {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	var update map[string]any
	if err := json.Unmarshal(body, &update); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Load existing resource: from override if exists, else embedded
	var base map[string]any
	if cached, ok := s.overrides[urlPath]; ok {
		base = deepCopy(cached.(map[string]any))
	} else {
		data, err := dataFS.ReadFile(urlPath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if err := json.Unmarshal(data, &base); err != nil {
			http.Error(w, "Corrupt embedded JSON", http.StatusInternalServerError)
			return
		}
	}

	// If it's a Collection (has "Members"), reject
	if _, isCollection := base["Members"]; isCollection {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Merge update into the copy
	mergeJSON(base, update)

	// Store the newly modified version
	s.overrides[urlPath] = base

	w.WriteHeader(http.StatusNoContent)
}

func deepCopy(m map[string]any) map[string]any {
	c := make(map[string]any)
	for k, v := range m {
		if vMap, ok := v.(map[string]any); ok {
			c[k] = deepCopy(vMap)
		} else {
			c[k] = v
		}
	}
	return c
}

func resolvePath(urlPath string) string {
	trimmed := strings.TrimPrefix(urlPath, "/redfish/v1")
	trimmed = strings.Trim(trimmed, "/")

	if trimmed == "" {
		return "data/index.json"
	}
	return path.Join("data", trimmed, "index.json")
}

func (s *MockServer) handleRedfishDelete(w http.ResponseWriter, r *http.Request) {
	s.log.Info("Received request", "method", r.Method, "path", r.URL.Path)

	urlPath := resolvePath(r.URL.Path)
	s.mu.RLock()
	defer s.mu.Unlock()
	_, hasOverride := s.overrides[urlPath]
	if hasOverride {
		// remove the resource
		delete(s.overrides, urlPath)
	}
	// get collection of the resource
	collectionPath := path.Dir(urlPath)
	cached, hasOverride := s.overrides[collectionPath]
	var collection Collection
	if hasOverride {
		collection = cached.(Collection)
	} else {
		data, err := dataFS.ReadFile(collectionPath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if err := json.Unmarshal(data, &collection); err != nil {
			http.Error(w, "Corrupt embedded JSON", http.StatusInternalServerError)
			return
		}
	}
	// remove member from collection
	newMembers := make([]Member, 0)
	for _, member := range collection.Members {
		if member.OdataID != r.URL.Path {
			newMembers = append(newMembers, member)
		}
	}
	s.log.Info("Removing member from collection", "members", newMembers, "collection", collectionPath)
	collection.Members = newMembers
	s.overrides[collectionPath] = collection
	w.WriteHeader(http.StatusNoContent)
}

func (s *MockServer) handleRedfishGET(w http.ResponseWriter, r *http.Request) {
	urlPath := resolvePath(r.URL.Path)

	s.mu.RLock()
	cached, hasOverride := s.overrides[urlPath]
	s.mu.RUnlock()

	if hasOverride {
		resp, _ := json.MarshalIndent(cached, "", "  ")
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write(resp)
		if err != nil {
			s.log.Error(err, "Failed to write response")
		}
		return
	}

	content, err := dataFS.ReadFile(urlPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(content)
	if err != nil {
		s.log.Error(err, "Failed to write response")
	}
}

func mergeJSON(base, update map[string]interface{}) {
	for k, v := range update {
		if bv, ok := base[k]; ok {
			if bvMap, ok1 := bv.(map[string]interface{}); ok1 {
				if vMap, ok2 := v.(map[string]interface{}); ok2 {
					mergeJSON(bvMap, vMap)
					continue
				}
			}
		}
		base[k] = v
	}
}

func (s *MockServer) handleRedfishPOST(w http.ResponseWriter, r *http.Request) {
	s.log.Info("Received request", "method", r.Method, "path", r.URL.Path)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			s.log.Error(err, "Failed to close request body")
		}
	}(r.Body)

	var update map[string]any
	if err := json.Unmarshal(body, &update); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	s.log.Info("POST body received", "body", string(body))
	urlPath := resolvePath(r.URL.Path)
	s.mu.Lock()
	defer s.mu.Unlock()
	cached, hasOverride := s.overrides[urlPath]
	var base Collection
	if hasOverride {
		s.log.Info("Using overridden data for POST", "path", urlPath)
		base = cached.(Collection)
	} else {
		s.log.Info("Using embedded data for POST", "path", urlPath)
		data, err := dataFS.ReadFile(urlPath)
		if err != nil {
			s.log.Error(err, "Failed to read embedded data for POST", "path", urlPath)
			http.NotFound(w, r)
			return
		}
		if err := json.Unmarshal(data, &base); err != nil {
			http.Error(w, "Corrupt embedded JSON", http.StatusInternalServerError)
			return
		}
	}
	// If resource collection (has "Members"), add a new member
	if len(base.Members) > 0 {
		newID := fmt.Sprintf("%d", len(base.Members)+1)
		location := path.Join(r.URL.Path, newID)
		newMemberPath := resolvePath(location)
		base.Members = append(base.Members, Member{
			OdataID: location,
		})
		s.log.Info("Adding new member", "id", newID, "location", location, "memberPath", newMemberPath)
		if strings.HasSuffix(r.URL.Path, "/Subscriptions") {
			w.Header().Set("Location", location)
		}
		s.overrides[urlPath] = base
		s.overrides[newMemberPath] = update
	} else {
		base.Members = make([]Member, 0)
		location := r.URL.JoinPath("1").String()
		base.Members = []Member{
			{
				OdataID: r.URL.JoinPath("1").String(),
			},
		}
		s.overrides[urlPath] = base
		if strings.HasSuffix(r.URL.Path, "/Subscriptions") {
			w.Header().Set("Location", location)
		}
	}
	s.log.Info("Storing updated data for POST", "path", urlPath, "data", update)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, err = w.Write([]byte(`{"status": "created"}`))
	if err != nil {
		s.log.Error(err, "Failed to write response")
		return
	}
}

// Start starts the mock server and stops on ctx cancellation.
func (s *MockServer) Start(ctx context.Context) error {
	if s.handler == nil {
		return fmt.Errorf("mock redfish handler is nil")
	}

	srv := &http.Server{
		Addr:    s.addr,
		Handler: s.handler,
	}

	done := make(chan struct{})

	go func() {
		s.log.Info("Started mock server", "address", s.addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.log.Error(err, "Server failed")
		}
		close(done)
	}()

	<-ctx.Done()
	s.log.Info("Shutting down mock server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		s.log.Error(err, "Mock server shutdown failed")
	}

	return nil
}

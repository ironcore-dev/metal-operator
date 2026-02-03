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
	"maps"
	"net/http"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
)

var (
	//go:embed data/**
	dataFS embed.FS
)

const (
	PowerOffState = "Off"
	PowerOnState  = "On"

	biosSettingsPathSuffix = "Bios/Settings"
	attributesKey          = "Attributes"
)

// BIOS settings that can be applied without reboot.
var noRebootSettings = []string{"AdminPhone"}

// Power state categories for system reset actions.
var (
	powerOffStates   = []string{"ForceOff", "GracefulShutdown", "PushPowerButton"}
	powerOnStates    = []string{"On", "ForceOn"}
	powerResetStates = []string{"GracefulRestart", "ForceRestart", "Nmi", "PowerCycle"}
)

// Power state categories for BMC reset actions.
var powerResetBMCStates = []string{"GracefulRestart", "ForceRestart"}

// Sentinel errors for HTTP response mapping.
var (
	errNotFound    = errors.New("resource not found")
	errCorruptJSON = errors.New("corrupt embedded JSON")
	errLocked      = errors.New("resource locked")
	errBadReset    = errors.New("unknown reset type")
)

type MockServer struct {
	log       logr.Logger
	addr      string
	handler   http.Handler
	mu        sync.RWMutex
	overrides map[string]any
}

func NewMockServer(log logr.Logger, addr string) *MockServer {
	s := &MockServer{
		addr:      addr,
		log:       log,
		overrides: make(map[string]any),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/redfish/v1/", s.redfishHandler)
	s.handler = mux

	return s
}

func (s *MockServer) redfishHandler(w http.ResponseWriter, r *http.Request) {
	s.log.Info("Received request", "method", r.Method, "path", r.URL.Path)

	switch r.Method {
	case http.MethodGet:
		s.handleGet(w, r)
	case http.MethodPost:
		s.handlePost(w, r)
	case http.MethodPatch:
		s.handlePatch(w, r)
	case http.MethodDelete:
		s.handleDelete(w, r)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *MockServer) handleGet(w http.ResponseWriter, r *http.Request) {
	filePath := resolvePath(r.URL.Path)

	s.mu.RLock()
	cached, hasOverride := s.overrides[filePath]
	s.mu.RUnlock()

	if hasOverride {
		s.writeJSON(w, http.StatusOK, cached)
		return
	}

	content, err := dataFS.ReadFile(filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(content); err != nil {
		s.log.Error(err, "Failed to write response")
	}
}

func (s *MockServer) handlePost(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	if err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	urlPath := r.URL.Path
	switch {
	case strings.HasSuffix(urlPath, "/Actions/ComputerSystem.Reset"):
		s.handleSystemReset(w, r, body)
	case strings.HasSuffix(urlPath, "/Actions/Manager.Reset"):
		s.handleBMCReset(w, r, body)
	case strings.Contains(urlPath, "UpdateService/Actions/UpdateService.SimpleUpdate"):
		s.writeJSON(w, http.StatusAccepted, map[string]string{"status": "Accepted"})
	default:
		s.writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
	}
}

func (s *MockServer) handlePatch(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	if err != nil || len(body) == 0 {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	var update map[string]any
	if err := json.Unmarshal(body, &update); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	filePath := resolvePath(r.URL.Path)
	base, err := s.loadResource(filePath)
	if err != nil {
		s.handleError(w, r, err)
		return
	}

	if _, isCollection := base["Members"]; isCollection {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := s.applyBiosSettings(r.URL.Path, update); err != nil {
		s.handleError(w, r, err)
		return
	}

	mergeJSON(base, update)
	s.saveResource(filePath, base)

	w.WriteHeader(http.StatusNoContent)
}

func (s *MockServer) handleDelete(w http.ResponseWriter, r *http.Request) {
	filePath := resolvePath(r.URL.Path)

	base, err := s.loadResource(filePath)
	if err != nil {
		s.handleError(w, r, err)
		return
	}

	if _, isCollection := base["Members"]; isCollection {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	delete(s.overrides, filePath)
	s.mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

func (s *MockServer) handleSystemReset(w http.ResponseWriter, r *http.Request, body []byte) {
	basePath := strings.TrimSuffix(r.URL.Path, "/Actions/ComputerSystem.Reset")
	systemPath := resolvePath(basePath)

	resetType, err := s.parseResetType(body, powerOffStates, powerOnStates, powerResetStates)
	if err != nil {
		s.handleError(w, r, err)
		return
	}

	s.mu.Lock()
	base, err := s.loadResourceLocked(systemPath)
	if err != nil {
		s.mu.Unlock()
		s.handleError(w, r, err)
		return
	}

	if s.isLocked(base) {
		s.mu.Unlock()
		s.handleError(w, r, errLocked)
		return
	}

	s.setLocked(base, true)
	s.overrides[systemPath] = base
	s.mu.Unlock()

	switch resetType {
	case "off":
		go s.doPowerOff(systemPath)
	case "on":
		go s.doPowerOn(systemPath, basePath)
	case "reset":
		go s.doPowerReset(systemPath, basePath)
	}

	s.writeJSON(w, http.StatusAccepted, map[string]string{"status": "Accepted"})
}

func (s *MockServer) handleBMCReset(w http.ResponseWriter, r *http.Request, body []byte) {
	basePath := strings.TrimSuffix(r.URL.Path, "/Actions/Manager.Reset")
	bmcPath := resolvePath(basePath)

	if !containsAny(string(body), powerResetBMCStates) {
		s.handleError(w, r, fmt.Errorf("%w: %s", errBadReset, string(body)))
		return
	}

	s.mu.Lock()
	base, err := s.loadResourceLocked(bmcPath)
	if err != nil {
		s.mu.Unlock()
		s.handleError(w, r, err)
		return
	}

	if s.isLocked(base) {
		s.mu.Unlock()
		s.handleError(w, r, errLocked)
		return
	}

	s.setLocked(base, true)
	s.overrides[bmcPath] = base
	s.mu.Unlock()

	go s.doBMCReset(bmcPath)

	s.writeJSON(w, http.StatusAccepted, map[string]string{"status": "Accepted"})
}

func (s *MockServer) parseResetType(body []byte, offStates, onStates, resetStates []string) (string, error) {
	bodyStr := string(body)
	switch {
	case containsAny(bodyStr, offStates):
		return "off", nil
	case containsAny(bodyStr, onStates):
		return "on", nil
	case containsAny(bodyStr, resetStates):
		return "reset", nil
	default:
		return "", fmt.Errorf("%w: %s", errBadReset, bodyStr)
	}
}

func (s *MockServer) doPowerOff(systemPath string) {
	time.Sleep(150 * time.Millisecond)
	s.mu.Lock()
	defer s.mu.Unlock()

	if base, ok := s.overrides[systemPath].(map[string]any); ok {
		base["PowerState"] = PowerOffState
		s.setLocked(base, false)
		s.log.Info("Powered off the system")
	}
}

func (s *MockServer) doPowerOn(systemPath, basePath string) {
	time.Sleep(150 * time.Millisecond)
	s.mu.Lock()
	if base, ok := s.overrides[systemPath].(map[string]any); ok {
		base["PowerState"] = PowerOnState
		s.setLocked(base, false)
		s.log.Info("Powered on the system")
	}
	s.mu.Unlock()

	if err := s.applyPendingBiosSettings(basePath); err != nil {
		s.log.Error(err, "Failed to apply pending BIOS settings")
	}
}

func (s *MockServer) doPowerReset(systemPath, basePath string) {
	time.Sleep(150 * time.Millisecond)
	s.mu.Lock()
	if base, ok := s.overrides[systemPath].(map[string]any); ok {
		base["PowerState"] = PowerOffState
		s.log.Info("Powered off the system")
	}
	s.mu.Unlock()

	time.Sleep(50 * time.Millisecond)

	s.mu.Lock()
	if base, ok := s.overrides[systemPath].(map[string]any); ok {
		base["PowerState"] = PowerOnState
		s.setLocked(base, false)
		s.log.Info("Powered on the system")
	}
	s.mu.Unlock()

	if err := s.applyPendingBiosSettings(basePath); err != nil {
		s.log.Error(err, "Failed to apply pending BIOS settings")
	}
}

func (s *MockServer) doBMCReset(bmcPath string) {
	time.Sleep(150 * time.Millisecond)
	s.mu.Lock()
	if base, ok := s.overrides[bmcPath].(map[string]any); ok {
		base["PowerState"] = PowerOffState
		s.log.Info("Powered off the BMC")
	}
	s.mu.Unlock()

	time.Sleep(150 * time.Millisecond)

	s.mu.Lock()
	if base, ok := s.overrides[bmcPath].(map[string]any); ok {
		base["PowerState"] = PowerOnState
		s.setLocked(base, false)
		s.log.Info("Powered on the BMC")
	}
	s.mu.Unlock()
}

func (s *MockServer) applyBiosSettings(urlPath string, update map[string]any) error {
	if !strings.Contains(urlPath, biosSettingsPathSuffix) {
		return nil
	}

	attrs, ok := update[attributesKey].(map[string]any)
	if !ok || len(attrs) == 0 {
		return nil
	}

	// Find settings that can be applied immediately without reboot.
	immediate := make(map[string]any)
	for key, val := range attrs {
		if slices.Contains(noRebootSettings, key) {
			immediate[key] = val
		}
	}

	if len(immediate) == 0 {
		return nil
	}

	s.log.Info("Applying BIOS settings without reboot", "settings", immediate)

	// Remove immediate settings from pending update.
	for key := range immediate {
		delete(attrs, key)
	}

	// Apply to current BIOS settings.
	biosURL := strings.TrimSuffix(urlPath, "/Settings")
	biosPath := resolvePath(biosURL)

	s.mu.Lock()
	defer s.mu.Unlock()

	biosBase, err := s.loadResourceLocked(biosPath)
	if err != nil {
		return err
	}

	if biosAttrs, ok := biosBase[attributesKey].(map[string]any); ok {
		maps.Copy(biosAttrs, immediate)
	}
	s.overrides[biosPath] = biosBase

	return nil
}

func (s *MockServer) applyPendingBiosSettings(basePath string) error {
	pendingPath := resolvePath(path.Join(basePath, biosSettingsPathSuffix))
	currentPath := resolvePath(path.Join(basePath, "Bios"))

	s.mu.Lock()
	defer s.mu.Unlock()

	pending, err := s.loadResourceLocked(pendingPath)
	if err != nil {
		return err
	}

	pendingAttrs, ok := pending[attributesKey].(map[string]any)
	if !ok || len(pendingAttrs) == 0 {
		return nil
	}

	current, err := s.loadResourceLocked(currentPath)
	if err != nil {
		return err
	}

	currentAttrs, ok := current[attributesKey].(map[string]any)
	if !ok {
		return nil
	}

	// Apply pending settings to current.
	maps.Copy(currentAttrs, pendingAttrs)

	// Clear pending settings.
	pending[attributesKey] = map[string]any{}

	s.overrides[currentPath] = current
	s.overrides[pendingPath] = pending
	s.log.Info("Applied pending BIOS settings")

	return nil
}

// loadResource loads a resource from override or embedded data.
// Returns a copy that can be safely modified.
func (s *MockServer) loadResource(filePath string) (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadResourceLocked(filePath)
}

// loadResourceLocked loads a resource while the caller already holds a lock.
func (s *MockServer) loadResourceLocked(filePath string) (map[string]any, error) {
	if cached, ok := s.overrides[filePath].(map[string]any); ok {
		return deepCopy(cached), nil
	}

	data, err := dataFS.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errNotFound, err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("%w: %v", errCorruptJSON, err)
	}

	return result, nil
}

func (s *MockServer) saveResource(filePath string, data map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.overrides[filePath] = data
}

func (s *MockServer) isLocked(resource map[string]any) bool {
	locked, _ := resource["resourceLock"].(string)
	return locked == "Locked"
}

func (s *MockServer) setLocked(resource map[string]any, locked bool) {
	if locked {
		resource["resourceLock"] = "Locked"
	} else {
		delete(resource, "resourceLock")
	}
}

func (s *MockServer) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, errNotFound):
		http.NotFound(w, r)
	case errors.Is(err, errCorruptJSON):
		http.Error(w, err.Error(), http.StatusInternalServerError)
	case errors.Is(err, errLocked):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, errBadReset):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *MockServer) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp, _ := json.MarshalIndent(data, "", "  ")
	if _, err := w.Write(resp); err != nil {
		s.log.Error(err, "Failed to write response")
	}
}

// Start starts the mock server and stops on ctx cancellation.
func (s *MockServer) Start(ctx context.Context) error {
	if s.handler == nil {
		return errors.New("mock redfish handler is nil")
	}

	srv := &http.Server{
		Addr:    s.addr,
		Handler: s.handler,
	}

	go func() {
		s.log.Info("Started mock server", "address", s.addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.log.Error(err, "Server failed")
		}
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

func resolvePath(urlPath string) string {
	trimmed := strings.TrimPrefix(urlPath, "/redfish/v1")
	trimmed = strings.Trim(trimmed, "/")

	if trimmed == "" {
		return "data/index.json"
	}
	if strings.HasSuffix(trimmed, ".json") {
		return path.Join("data", trimmed)
	}
	return path.Join("data", trimmed, "index.json")
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

func mergeJSON(base, update map[string]any) {
	for k, v := range update {
		if bv, ok := base[k]; ok {
			if bvMap, ok := bv.(map[string]any); ok {
				if vMap, ok := v.(map[string]any); ok {
					mergeJSON(bvMap, vMap)
					continue
				}
			}
		}
		base[k] = v
	}
}

func containsAny(s string, substrs []string) bool {
	return slices.ContainsFunc(substrs, func(sub string) bool {
		return strings.Contains(s, sub)
	})
}

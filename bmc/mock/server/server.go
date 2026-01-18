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
	LockedResourceState   = "Locked"
	UnlockedResourceState = "Unlocked"
	ResourceLockKey       = "resourceLock"
	PowerOffState         = "Off"
	PowerOnState          = "On"

	BIOSUpdateTask = "/redfish/v1/TaskService/Tasks/dummyBIOSTask"
	BMCUpdateTask  = "/redfish/v1/TaskService/Tasks/dummyBMCTask"
)

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

// currently hardcoded until specific logic is required
var currentSettingsNeedsNoReboot = []string{"AdminPhone"}

// power states from data/Systems/437XR1138R2/index.json
var powerOffStates = []string{"ForceOff", "GracefulShutdown", "PushPowerButton"}
var powerOnStates = []string{"On", "ForceOn"}
var powerResetStates = []string{"GracefulRestart", "ForceRestart", "Nmi", "PowerCycle"}

// power states from data/Managers/BMC/index.json
var powerResetBMCStates = []string{"GracefulRestart", "ForceRestart"}

const (
	BiosSettingsPathSufficx = "Bios/Settings"
	AttributesKey           = "Attributes"
)

func (s *MockServer) redfishHandler(w http.ResponseWriter, r *http.Request) {
	s.log.Info("Received request", "method", r.Method, "path", r.URL.Path, "address", s.addr)

	switch r.Method {
	case http.MethodGet:
		s.handleRedfishGET(w, r)
	case http.MethodPost:
		s.handleRedfishPOST(w, r)
	case http.MethodPatch:
		s.handleRedfishPATCH(w, r)
	case http.MethodDelete:
		s.handleRedfishDELETE(w, r)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *MockServer) handleRedfishPATCH(w http.ResponseWriter, r *http.Request) {
	s.log.Info("Received request", "method", r.Method, "path", r.URL.Path, "address", s.addr)

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

	// Load existing resource: from override if exists, else embedded
	base, err := fetchCurrentDataForPath(s, urlPath)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "resource not found"):
			http.NotFound(w, r)
			return
		case strings.Contains(err.Error(), "corrupt embedded JSON"):
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	// If it's a Collection (has "Members"), reject
	if _, isCollection := base["Members"]; isCollection {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	err = handlebiosSettingsApply(s, r.URL.Path, update)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "resource not found"):
			http.NotFound(w, r)
			return
		case strings.Contains(err.Error(), "corrupt embedded JSON"):
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Merge update into the copy
	mergeJSON(base, update)
	s.mu.Lock()
	defer s.mu.Unlock()
	// Store the newly modified version
	s.overrides[urlPath] = base

	w.WriteHeader(http.StatusNoContent)
}

func (s *MockServer) handleRedfishDELETE(w http.ResponseWriter, r *http.Request) {
	s.log.Info("Received request", "method", r.Method, "path", r.URL.Path, "address", s.addr)

	urlPath := resolvePath(r.URL.Path)

	// Load existing resource: from override if exists, else embedded
	base, err := fetchCurrentDataForPath(s, urlPath)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "resource not found"):
			http.NotFound(w, r)
			return
		case strings.Contains(err.Error(), "corrupt embedded JSON"):
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// If it's a Collection (has "Members"), reject
	if _, isCollection := base["Members"]; isCollection {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove the override to revert to embedded JSON
	delete(s.overrides, urlPath)

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
	if strings.HasSuffix(trimmed, ".json") {
		return path.Join("data", trimmed)
	}
	return path.Join("data", trimmed, "index.json")
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

	if r.URL.Path == BIOSUpdateTask || r.URL.Path == BMCUpdateTask {
		// special handling for firmware update tasks
		s.log.Info("Task details requested not found", "Task", r.URL.Path)
		http.NotFound(w, r)
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
	s.log.Info("Received request", "method", r.Method, "path", r.URL.Path, "address", s.addr)

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

	s.log.Info("POST body received", "body", string(body))

	urlPath := resolvePath(r.URL.Path)
	switch {
	case strings.Contains(urlPath, "Actions/ComputerSystem.Reset"):
		// Simulate a system reset action
		err := handleSystemReset(s, r, urlPath, body)
		if err != nil {
			switch {
			case strings.Contains(err.Error(), "resource not found"):
				http.NotFound(w, r)
				return
			case strings.Contains(err.Error(), "corrupt embedded JSON"):
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			case strings.Contains(err.Error(), "resource locked"):
				http.Error(w, err.Error(), http.StatusConflict)
				return
			case strings.Contains(err.Error(), "unknown reset type"):
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			default:
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, err = w.Write([]byte(`{"status": "Accepted"}`))
		if err != nil {
			s.log.Error(err, "Failed to write response")
			return
		}
	case strings.Contains(urlPath, "Managers/BMC/Actions/Manager.Reset"):
		// Simulate a BMC reset action
		err := handleBMCReset(s, r, urlPath, body)
		if err != nil {
			switch {
			case strings.Contains(err.Error(), "resource not found"):
				http.NotFound(w, r)
				return
			case strings.Contains(err.Error(), "corrupt embedded JSON"):
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			case strings.Contains(err.Error(), "resource locked"):
				http.Error(w, err.Error(), http.StatusConflict)
				return
			case strings.Contains(err.Error(), "unknown reset type"):
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			default:
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, err = w.Write([]byte(`{"status": "Accepted"}`))
		if err != nil {
			s.log.Error(err, "Failed to write response")
			return
		}
	case strings.Contains(urlPath, "UpdateService/Actions/SimpleUpdate"):
		// Simulate a firmware update action
		status, err := handleSimpleUpdate(s, body)
		if err != nil {
			http.Error(w, err.Error(), status)
			return
		}
		if status != http.StatusAccepted {
			w.Header().Set("Location", BIOSUpdateTask)
		} else {
			w.Header().Set("Location", BMCUpdateTask)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, err = w.Write([]byte(`{"status": "Accepted"}`))
		if err != nil {
			s.log.Error(err, "Failed to write response")
			return
		}
	default:
		s.log.Info("Unhandled POST request", "path", urlPath)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, err = w.Write([]byte(`{"status": "created"}`))
		if err != nil {
			s.log.Error(err, "Failed to write response")
			return
		}
	}
}

func handleSystemReset(s *MockServer, r *http.Request, urlPath string, body []byte) error {
	basePath := strings.TrimSuffix(r.URL.Path, "/Actions/ComputerSystem.Reset")
	systemPath := resolvePath(basePath)
	base, err := fetchCurrentDataForPath(s, systemPath)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if val, ok := base[ResourceLockKey]; ok && val == LockedResourceState {
		s.mu.Unlock()
		s.log.Info("System resource is locked, cannot perform reset", base)
		go func() {
			// unlock after waiting period incase of stuck lock
			time.Sleep(300 * time.Millisecond)
			delete(base, ResourceLockKey)
		}()
		return errors.New("system resource locked, cannot perform reset")
	}
	base[ResourceLockKey] = LockedResourceState
	s.overrides[urlPath] = base
	s.mu.Unlock()
	s.log.Info("System resource locked for reset operation")
	if slices.ContainsFunc(powerOffStates, func(s string) bool {
		return strings.Contains(string(body), s)
	}) {
		go func() {
			time.Sleep(150 * time.Millisecond)
			s.mu.Lock()
			defer s.mu.Unlock()
			base["PowerState"] = PowerOffState
			delete(base, ResourceLockKey)
			s.overrides[urlPath] = base
			s.log.Info("Powered Off the system")
		}()
	} else if slices.ContainsFunc(powerOnStates, func(s string) bool {
		return strings.Contains(string(body), s)
	}) {
		go func() {
			time.Sleep(150 * time.Millisecond)
			s.mu.Lock()
			base["PowerState"] = PowerOnState
			delete(base, ResourceLockKey)
			s.overrides[urlPath] = base
			s.mu.Unlock()
			s.log.Info("Powered On the system")
			err := handlePostPowerOnActions(s, basePath)
			if err != nil {
				s.log.Error(err, "Failed to handle post power-on actions")
			}
			s.log.Info("Handled system power-on actions")
		}()
	} else if slices.ContainsFunc(powerResetStates, func(s string) bool {
		return strings.Contains(string(body), s)
	}) {
		go func() {
			time.Sleep(150 * time.Millisecond)
			s.mu.Lock()
			base["PowerState"] = PowerOffState
			// Store the newly modified version
			s.overrides[urlPath] = base
			s.mu.Unlock()
			s.log.Info("Powered Off the system")
			time.Sleep(50 * time.Millisecond)
			s.mu.Lock()
			base["PowerState"] = PowerOnState
			delete(base, ResourceLockKey)
			// Store the newly modified version
			s.overrides[urlPath] = base
			s.mu.Unlock()
			s.log.Info("Powered On the system")
			err := handlePostPowerOnActions(s, basePath)
			if err != nil {
				s.log.Error(err, "Failed to handle post power-on actions")
			}
			s.log.Info("Handled system power-on actions")
		}()
	} else {
		return fmt.Errorf("unknown reset type in request body: %s", string(body))
	}
	return nil
}
func handlePostPowerOnActions(s *MockServer, basePath string) error {
	// Handle Bios settings applicationn
	biosPendingSettingsURL := path.Join(basePath, strings.Trim(BiosSettingsPathSufficx, "/"))
	pendingSettingsFilePath := resolvePath(biosPendingSettingsURL)
	// get the Pending Bios settings
	pendingBiosSettingsBase, err := fetchCurrentDataForPath(s, pendingSettingsFilePath)
	if err != nil {
		return err
	}
	if data, ok := pendingBiosSettingsBase[AttributesKey]; ok {
		// if pending settings exist, apply them to current settings
		if pendingAttributes, ok := data.(map[string]any); ok && len(pendingAttributes) > 0 {
			// save the pending Attributes
			pendingAttributesCopy := deepCopy(pendingAttributes)
			// get the current Bios settings
			biosCurrentSettingsURL := path.Join(basePath, "Bios")
			currentSettingsFilePath := resolvePath(biosCurrentSettingsURL)
			currentBiosSettingsBase, err := fetchCurrentDataForPath(s, currentSettingsFilePath)
			if err != nil {
				return err
			}
			s.mu.Lock()
			currentAttributesCopy := deepCopy(currentBiosSettingsBase[AttributesKey].(map[string]any))
			mergeJSON(currentAttributesCopy, pendingAttributesCopy)
			currentBiosSettingsBase[AttributesKey] = currentAttributesCopy
			pendingBiosSettingsBase[AttributesKey] = map[string]any{}
			s.overrides[currentSettingsFilePath] = currentBiosSettingsBase
			s.overrides[pendingSettingsFilePath] = pendingBiosSettingsBase
			s.mu.Unlock()
			s.log.Info("Post power-on actions completed for system")
		}
	}
	return nil
}

func handleBMCReset(s *MockServer, r *http.Request, urlPath string, body []byte) error {
	// Placeholder for handling system reset logic
	basePath := strings.TrimSuffix(r.URL.Path, "Actions/Manager.Reset")
	systemPath := resolvePath(basePath)
	base, err := fetchCurrentDataForPath(s, systemPath)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if val, ok := base[ResourceLockKey]; ok && val == LockedResourceState {
		s.mu.Unlock()
		go func() {
			// unlock after waiting period incase of stuck lock
			time.Sleep(300 * time.Millisecond)
			base["PowerState"] = PowerOnState
			base["Status"].(map[string]any)["State"] = "Enabled"
			delete(base, ResourceLockKey)
			s.overrides[urlPath] = base
		}()
		return errors.New("BMC resource locked, cannot perform reset")
	}
	base[ResourceLockKey] = LockedResourceState
	s.overrides[urlPath] = base
	s.mu.Unlock()
	if slices.ContainsFunc(powerResetBMCStates, func(s string) bool {
		return strings.Contains(string(body), s)
	}) {
		go func() {
			time.Sleep(50 * time.Millisecond)
			s.mu.Lock()
			base["Status"].(map[string]any)["State"] = "Rebooting"
			base["PowerState"] = PowerOffState
			// Store the newly modified version
			s.overrides[urlPath] = base
			s.log.Info("Powered Off the BMC")
			s.mu.Unlock()
			time.Sleep(50 * time.Millisecond)
			s.mu.Lock()
			base["PowerState"] = PowerOnState
			base["Status"].(map[string]any)["State"] = "Enabled"
			delete(base, ResourceLockKey)
			// Store the newly modified version
			s.overrides[urlPath] = base
			s.mu.Unlock()
			s.log.Info("Powered On the BMC")
		}()
	} else {
		return fmt.Errorf("unknown reset type in request body: %s", string(body))
	}
	return nil
}

func handlebiosSettingsApply(s *MockServer, settingsPath string, bodyUpdate map[string]any) error {
	replace := map[string]any{}
	switch {
	case strings.Contains(settingsPath, BiosSettingsPathSufficx):
		// Handle Bios Settings PATCH if needed
		s.log.Info("Check if BIOS settings that do not require reboot are present", "settings", bodyUpdate)
		if len(bodyUpdate[AttributesKey].(map[string]any)) > 0 {
			// currently, hardcoded until specific logic is required
			if updatedAttributes, ok := bodyUpdate[AttributesKey]; ok {
				if updatedAttributesMap, ok := updatedAttributes.(map[string]any); ok {
					for key, newData := range updatedAttributesMap {
						if slices.Contains(currentSettingsNeedsNoReboot, key) {
							// apply immediately
							replace[key] = newData
						}
					}
				}
			}
		}
	default:
		return nil
	}

	if len(replace) > 0 {
		s.log.Info("Applying BIOS settings that do not require reboot", "settings", replace)
		for key := range replace {
			delete(bodyUpdate[AttributesKey].(map[string]any), key)
		}
		BiosURL := strings.TrimSuffix(settingsPath, "Settings")
		biosPath := resolvePath(BiosURL)
		// Load existing resource: from override if exists, else embedded
		biosBase, err := fetchCurrentDataForPath(s, biosPath)
		if err != nil {
			return err
		}
		if biosAttributes, ok := biosBase[AttributesKey].(map[string]any); ok {
			for key, value := range replace {
				biosAttributes[key] = value
			}
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		s.overrides[biosPath] = biosBase
	}
	return nil
}

func fetchCurrentDataForPath(s *MockServer, path string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	base := make(map[string]any)
	if cached, ok := s.overrides[path]; ok {
		base = deepCopy(cached.(map[string]any))
	} else {
		data, err := dataFS.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("resource not found: %v", err)
		}
		if err := json.Unmarshal(data, &base); err != nil {
			return nil, fmt.Errorf("corrupt embedded JSON: %v", err)
		}
	}
	s.overrides[path] = base
	return base, nil
}

func handleSimpleUpdate(s *MockServer, body []byte) (int, error) {
	if len(body) == 0 {
		return http.StatusBadRequest, errors.New("empty body")
	}
	var updatePayload map[string]any
	if err := json.Unmarshal(body, &updatePayload); err != nil {
		return http.StatusBadRequest, fmt.Errorf("invalid JSON for the update BIOS payload: %w", err)
	}
	// to mock the server to update, we follow following format for the body
	// https://github.com/ironcore-dev/metal-operator/blob/493d58792737e64c2df32f5e95ff1181ad4359b3/bmc/oem/types.go#SimpleUpdateRequestBody
	// SimpleUpdateRequestBody{
	//    ImageURI : <json string>"
	//	{
	//	 "updatedVersion": "2.3.4",
	//	 "ResourceURI": "/redfish/v1/Systems/437XR1138R2" | "redfish/v1/Managers/BMC",
	//	 "Module": "Bios" | "BMC" | <any other string>
	//	}
	//	"
	var details map[string]string
	if err := json.Unmarshal([]byte(updatePayload["ImageURI"].(string)), &details); err != nil {
		return http.StatusBadRequest, fmt.Errorf("invalid JSON string for ImageURI: %w", err)
	}

	systemPath := resolvePath(details["ResourceURI"])
	base, err := fetchCurrentDataForPath(s, systemPath)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "resource not found"):
			s.log.Error(err, "Cannot update version as resource not found", details["ResourceURI"])
			return http.StatusNotFound, err
		case strings.Contains(err.Error(), "corrupt embedded JSON"):
			s.log.Error(err, "Cannot update version as resource JSON is corrupt")
			return http.StatusInternalServerError, err
		default:
			s.log.Error(err, "Cannot update version with error")
			return http.StatusInternalServerError, err
		}
	}
	switch details["Module"] {
	case "BIOS":
		biosTasksPath := resolvePath(BIOSUpdateTask)
		if taskDetails, ok := s.overrides[biosTasksPath]; ok && taskDetails.(map[string]any)["TaskState"] != "Completed" {
			return http.StatusConflict, errors.New("upgrade BIOS tasks already running at /redfish/v1/TaskService/Tasks/dummyBIOSTask")
		}
		if base["BiosVersion"] == nil || base["BiosVersion"] == "" {
			return http.StatusInternalServerError, errors.New("BiosVersion field not found in system resource")
		}
		// Simulate a BMC firmware update action
		go func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			mockedFirmwareUpgradeTasksProgress(base["BiosVersion"].(string), biosTasksPath, s, details)
			base["BiosVersion"] = details["updatedVersion"]
		}()
		return http.StatusAccepted, nil
	case "BMC":
		bmcTasksPath := resolvePath(BMCUpdateTask)
		if taskDetails, ok := s.overrides[bmcTasksPath]; ok && taskDetails.(map[string]any)["TaskState"] != "Completed" {
			return http.StatusConflict, errors.New("upgrade BMC tasks already running at /redfish/v1/TaskService/Tasks/dummyBMCTask")
		}
		if base["FirmwareVersion"] == nil || base["FirmwareVersion"] == "" {
			return http.StatusInternalServerError, errors.New("FirmwareVersion field not found in system resource")
		}
		// Simulate a BMC firmware update action
		go func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			mockedFirmwareUpgradeTasksProgress(base["FirmwareVersion"].(string), bmcTasksPath, s, details)
			base["FirmwareVersion"] = details["updatedVersion"]
		}()
		return http.StatusCreated, nil
	default:
		s.log.Info("Unhandled SimpleUpdate request", "body", string(body))
		return http.StatusBadRequest, errors.New("module not supported for SimpleUpdate")
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

func mockedFirmwareUpgradeTasksProgress(currentVersion, resourcePath string, s *MockServer, details map[string]string) {
	if currentVersion == details["updatedVersion"] {
		s.log.Info("version is already at the requested version, no upgrade needed", "version", details["updatedVersion"], "Resource", details["Module"])
		// note: this is a mocked tasks
		// will be deleted once its completed and has been read by the client to unblock the next upgrades
		s.overrides[resourcePath] = map[string]any{
			"TaskState":       "Completed",
			"PercentComplete": 100,
			"Description":     "BMC upgrade not needed, already at requested version",
		}
	}

	type mockedtasks struct {
		TaskState       string
		PercentComplete int
	}
	// mock the bios upgrade progress at incre
	TaskInProgress := []mockedtasks{
		{"New", 0},
		{"Pending", 0},
		{"Starting", 0},
		{"Running", 10},
		{"Running", 20},
		{"Running", 100},
		{"Completed", 100}, // final state, will be deleted after read by client to unblock next upgrades
	}
	for _, task := range TaskInProgress {
		// mock delays for each tasks state
		time.Sleep(200 * time.Millisecond)
		s.overrides[resourcePath] = map[string]any{
			"TaskState":       task.TaskState,
			"PercentComplete": task.PercentComplete,
		}
	}
}

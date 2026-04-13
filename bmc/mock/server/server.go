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
	"net/url"
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

type Collection struct {
	Members []Member `json:"Members"`
}

type Member struct {
	OdataID string `json:"@odata.id"`
}

// upgradeTarget describes a resource path and the field to overwrite with the new version on completion.
type upgradeTarget struct {
	path  string
	field string
}

const (
	PowerOffState = "Off"
	PowerOnState  = "On"

	biosSettingsPathSuffix = "Bios/Settings"
	attributesKey          = "Attributes"

	upgradeTaskPath      = "data/TaskService/Tasks/upgrade/index.json"
	upgradeTaskStepsPath = "data/TaskService/Tasks/upgrade/steps.json"
)

// upgradeTaskURI is the @odata.id of the upgrade task, read from the embedded index.json.
var upgradeTaskURI = embeddedStringField(upgradeTaskPath, "@odata.id")

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
	var copied any
	if hasOverride {
		copied = deepCopyAny(cached)
	}
	s.mu.RUnlock()

	if hasOverride {
		s.writeJSON(w, http.StatusOK, copied)
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
	case strings.HasSuffix(urlPath, "/Actions/UpdateService.SimpleUpdate") ||
		strings.HasSuffix(urlPath, "/Actions/SimpleUpdate"):
		var params struct {
			ImageURI string `json:"ImageURI"`
		}
		_ = json.Unmarshal(body, &params)

		// The ImageURI scheme encodes the target type and (for BIOS) the system URI:
		//   bios:///redfish/v1/Systems/<id>/<version>
		//   bmc://<version>
		// This avoids relying on the optional Targets field.
		isBIOS := strings.HasPrefix(params.ImageURI, "bios://")
		rest := strings.TrimPrefix(strings.TrimPrefix(params.ImageURI, "bios://"), "bmc://")

		// For BIOS, rest is "<systemURI>/<version>" where systemURI starts with /redfish/v1/Systems/...
		// For BMC,  rest is just the plain version string.
		var imageVersion string
		var targets []upgradeTarget
		if isBIOS {
			// rest = "/redfish/v1/Systems/<id>/<encoded-version>" — split off the last path segment as version.
			// The version is URL-encoded (by the client) to avoid ambiguity with slashes in version strings.
			// If no system URI is present, fall back to the first system in the collection.
			lastSlash := strings.LastIndex(rest, "/")
			var sysPath string
			if lastSlash > 0 {
				systemURI := rest[:lastSlash]
				encodedVersion := rest[lastSlash+1:]
				imageVersion, _ = url.PathUnescape(encodedVersion)
				sysPath = resolvePath(systemURI)
			} else {
				imageVersion, _ = url.PathUnescape(rest)
				sysPath = embeddedMemberDataPath("data/Systems/index.json", "")
			}
			if sysPath != "" {
				targets = []upgradeTarget{{path: sysPath, field: "BiosVersion"}}
			}
		} else {
			imageVersion = rest
			if bmcPath := embeddedMemberDataPath("data/Managers/index.json", ""); bmcPath != "" {
				targets = append(targets, upgradeTarget{path: bmcPath, field: "FirmwareVersion"})
			}
			if fwPath := embeddedMemberDataPath("data/UpdateService/FirmwareInventory/index.json", "/BMC"); fwPath != "" {
				targets = append(targets, upgradeTarget{path: fwPath, field: "Version"})
			}
		}

		steps := loadUpgradeSteps()
		s.mu.Lock()
		if len(steps) > 0 {
			s.overrides[upgradeTaskPath] = buildUpgradeTask(steps[0])
		}
		s.mu.Unlock()

		go s.doUpgradeTask(imageVersion, targets)

		w.Header().Set("Location", upgradeTaskURI)
		s.writeJSON(w, http.StatusAccepted, map[string]string{"@odata.id": upgradeTaskURI})
	default:
		//
		urlPath := resolvePath(r.URL.Path)
		var update map[string]any
		if err := json.Unmarshal(body, &update); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		// Handle resource creation in collections
		s.mu.Lock()
		defer s.mu.Unlock()
		cached, hasOverride := s.overrides[urlPath]
		var base Collection
		if hasOverride {
			s.log.Info("Using overridden data for POST", "path", urlPath)
			var ok bool
			base, ok = cached.(Collection)
			if !ok {
				http.Error(w, "Corrupt overridden JSON", http.StatusInternalServerError)
				return
			}
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
	// Hold a single lock for the entire delete + collection update to avoid
	// unsynchronised reads of s.overrides between the two operations.
	s.mu.Lock()
	delete(s.overrides, filePath)

	// get collection of the resource
	collectionPath := path.Dir(filePath)
	cached, hasOverride := s.overrides[collectionPath]
	var collection Collection
	if hasOverride {
		var ok bool
		collection, ok = cached.(Collection)
		if !ok {
			s.mu.Unlock()
			http.Error(w, "Corrupt embedded JSON", http.StatusInternalServerError)
			return
		}
	} else {
		data, err := dataFS.ReadFile(collectionPath + "/index.json")
		if err != nil {
			s.mu.Unlock()
			http.NotFound(w, r)
			return
		}
		if err := json.Unmarshal(data, &collection); err != nil {
			s.mu.Unlock()
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

func (s *MockServer) doUpgradeTask(imageVersion string, targets []upgradeTarget) {
	steps := loadUpgradeSteps()

	// Step 0 ("New") was already set by handlePost; advance through the remaining steps.
	for i := 1; i < len(steps); i++ {
		time.Sleep(50 * time.Millisecond)
		s.mu.Lock()
		s.overrides[upgradeTaskPath] = buildUpgradeTask(steps[i])
		if taskState, _ := steps[i]["TaskState"].(string); taskState == "Completed" {
			for _, t := range targets {
				if data, err := s.loadResourceLocked(t.path); err == nil {
					data[t.field] = imageVersion
					s.overrides[t.path] = data
				}
			}
		}
		s.mu.Unlock()
	}
}

// loadUpgradeSteps reads the upgrade task step definitions from the embedded JSON file.
func loadUpgradeSteps() []map[string]any {
	data, err := dataFS.ReadFile(upgradeTaskStepsPath)
	if err != nil {
		return nil
	}
	var steps []map[string]any
	if err := json.Unmarshal(data, &steps); err != nil {
		return nil
	}
	return steps
}

// buildUpgradeTask merges a step's fields on top of the base task defined in upgradeTaskPath.
func buildUpgradeTask(step map[string]any) map[string]any {
	data, _ := dataFS.ReadFile(upgradeTaskPath)
	var base map[string]any
	if err := json.Unmarshal(data, &base); err != nil {
		base = make(map[string]any)
	}
	for k, v := range step {
		base[k] = v
	}
	return base
}

// embeddedMemberDataPath returns the data file path for a collection member whose @odata.id ends with idSuffix.
// If idSuffix is empty, the first member's path is returned.
func embeddedMemberDataPath(collectionDataPath, idSuffix string) string {
	data, err := dataFS.ReadFile(collectionDataPath)
	if err != nil {
		return ""
	}
	var col Collection
	if err := json.Unmarshal(data, &col); err != nil || len(col.Members) == 0 {
		return ""
	}
	if idSuffix == "" {
		return resolvePath(col.Members[0].OdataID)
	}
	for _, m := range col.Members {
		if strings.HasSuffix(m.OdataID, idSuffix) {
			return resolvePath(m.OdataID)
		}
	}
	return ""
}

// embeddedStringField reads a single top-level string field from an embedded JSON data file.
func embeddedStringField(filePath, field string) string {
	data, err := dataFS.ReadFile(filePath)
	if err != nil {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return ""
	}
	v, _ := m[field].(string)
	return v
}

// ResetBIOSVersion resets the BIOS version and clears any in-progress upgrade task.
func (s *MockServer) ResetBIOSVersion() {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.overrides, upgradeTaskPath)
	sysPath := embeddedMemberDataPath("data/Systems/index.json", "")
	if sysPath == "" {
		return
	}
	if sysData, err := s.loadResourceLocked(sysPath); err == nil {
		sysData["BiosVersion"] = embeddedStringField(sysPath, "BiosVersion")
		s.overrides[sysPath] = sysData
	}
}

// ResetBMCVersion resets the BMC firmware version and clears any in-progress upgrade task.
func (s *MockServer) ResetBMCVersion() {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.overrides, upgradeTaskPath)
	bmcPath := embeddedMemberDataPath("data/Managers/index.json", "")
	if bmcPath != "" {
		if bmcData, err := s.loadResourceLocked(bmcPath); err == nil {
			bmcData["FirmwareVersion"] = embeddedStringField(bmcPath, "FirmwareVersion")
			s.overrides[bmcPath] = bmcData
		}
	}
	bmcFirmPath := embeddedMemberDataPath("data/UpdateService/FirmwareInventory/index.json", "/BMC")
	if bmcFirmPath != "" {
		if bmcFirmData, err := s.loadResourceLocked(bmcFirmPath); err == nil {
			bmcFirmData["Version"] = embeddedStringField(bmcFirmPath, "Version")
			s.overrides[bmcFirmPath] = bmcFirmData
		}
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
		return nil, fmt.Errorf("%w: %w", errNotFound, err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("%w: %w", errCorruptJSON, err)
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
		c[k] = deepCopyAny(v)
	}
	return c
}

func deepCopyAny(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return deepCopy(val)
	case []any:
		c := make([]any, len(val))
		for i, elem := range val {
			c[i] = deepCopyAny(elem)
		}
		return c
	case Collection:
		members := make([]Member, len(val.Members))
		copy(members, val.Members)
		return Collection{Members: members}
	default:
		return v
	}
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

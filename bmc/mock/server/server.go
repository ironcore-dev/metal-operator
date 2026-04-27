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

type Collection struct {
	Members []Member `json:"Members"`
}

type Member struct {
	OdataID string `json:"@odata.id"`
}

const (
	PowerOffState = "Off"
	PowerOnState  = "On"

	biosSettingsPathSuffix = "Bios/Settings"
	attributesKey          = "Attributes"

	// Fixed file paths for BMC manager resources.
	bmcFilePath         = "data/Managers/BMC/index.json"
	bmcSettingsFilePath = "data/Managers/BMC/Settings/index.json"

	// Fixed file paths for firmware upgrade simulation.
	upgradeTaskFilePath      = "data/TaskService/Tasks/upgrade/index.json"
	upgradeStepsFilePath     = "data/TaskService/Tasks/upgrade/steps.json"
	upgradeStepsFailFilePath = "data/TaskService/Tasks/upgrade/steps-fail.json"
	upgradeTaskURI           = "/redfish/v1/TaskService/Tasks/upgrade"

	// Firmware version JSON field names updated on upgrade task completion.
	// The target resource path is resolved dynamically via FirmwareInventory RelatedItem links.
	biosVersionField = "BiosVersion"
	bmcVersionField  = "FirmwareVersion"
)

// noRebootSettings and noRebootBMCSettings are populated at init time by
// scanning the embedded attribute registries for entries where ResetRequired == false.
var (
	noRebootSettings    []string
	noRebootBMCSettings []string
)

func init() {
	noRebootSettings = mustLoadNoRebootAttrs("data/Registries/BiosAttributeRegistry.v1_0_0.json")
	noRebootBMCSettings = mustLoadNoRebootAttrs("data/Registries/BMCAttributeRegistry/index.json")
}

// mustLoadNoRebootAttrs reads an attribute registry JSON from the embedded FS and
// returns the names of all attributes whose ResetRequired field is false.
// It panics if the file cannot be read or parsed so that a renamed or malformed
// embedded registry is caught immediately at startup rather than silently
// flipping all settings onto the slow (reboot-required) path.
func mustLoadNoRebootAttrs(registryPath string) []string {
	data, err := dataFS.ReadFile(registryPath)
	if err != nil {
		panic(fmt.Sprintf("read %s: %v", registryPath, err))
	}
	var registry struct {
		RegistryEntries struct {
			Attributes []struct {
				AttributeName string `json:"AttributeName"`
				ResetRequired bool   `json:"ResetRequired"`
			} `json:"Attributes"`
		} `json:"RegistryEntries"`
	}
	if err := json.Unmarshal(data, &registry); err != nil {
		panic(fmt.Sprintf("parse %s: %v", registryPath, err))
	}
	var result []string
	for _, attr := range registry.RegistryEntries.Attributes {
		if !attr.ResetRequired {
			result = append(result, attr.AttributeName)
		}
	}
	return result
}

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

// memberHook is invoked when a collection member is created (onCreate) or
// deleted (onDelete). data holds the member resource's JSON fields.
// Hooks are keyed by collection URL suffix (e.g. "/AccountService/Accounts");
// register a new entry in NewMockServer to handle additional collection types.
type memberHook func(s *MockServer, data map[string]any)

// actionHandler pairs a URL path predicate with a POST action handler.
// Handlers are tested in order; the first match wins and the default
// collection-creation logic is skipped. Add entries to MockServer.actionHandlers
// in NewMockServer to support new Redfish action endpoints without touching
// handlePost.
type actionHandler struct {
	matches func(path string) bool
	handle  func(w http.ResponseWriter, r *http.Request, body []byte)
}

// Option is a functional option for configuring a MockServer.
type Option func(*MockServer)

// WithAuth enables HTTP Basic Auth enforcement on all non-service-root endpoints.
// By default the mock server accepts all requests without credentials, which is
// convenient for local development. Pass WithAuth() in unit-test suites to
// exercise the authentication path.
func WithAuth() Option {
	return func(s *MockServer) { s.authEnabled = true }
}

type MockServer struct {
	log               logr.Logger
	addr              string
	handler           http.Handler
	mu                sync.RWMutex
	overrides         map[string]any
	upgradeGen        int64                 // incremented on each SimpleUpdate to cancel stale goroutines
	upgradedResources map[string]string     // odata.id URI → file path for resources updated by the last upgrade
	accounts          map[string]string     // username → password (authentication store)
	authEnabled       bool                  // when true, all non-service-root requests require Basic Auth
	unavailable       bool                  // when true, all requests return 503 Service Unavailable
	actionHandlers    []actionHandler       // ordered POST action dispatch table (first match wins)
	onCreate          map[string]memberHook // collection URL suffix → hook called after a member is added
	onDelete          map[string]memberHook // collection URL suffix → hook called before a member is removed
}

// loadAccountsFromEmbedded seeds the authentication store by reading the
// embedded AccountService/Accounts collection and extracting UserName/Password
// pairs. This keeps the initial credentials in sync with the data files rather
// than duplicating them in Go code.
func loadAccountsFromEmbedded() map[string]string {
	result := make(map[string]string)
	data, err := dataFS.ReadFile("data/AccountService/Accounts/index.json")
	if err != nil {
		return result
	}
	var collection Collection
	if err := json.Unmarshal(data, &collection); err != nil {
		return result
	}
	for _, member := range collection.Members {
		memberData, err := dataFS.ReadFile(resolvePath(member.OdataID))
		if err != nil {
			continue
		}
		var account struct {
			UserName string `json:"UserName"`
			Password string `json:"Password"`
		}
		if err := json.Unmarshal(memberData, &account); err != nil {
			continue
		}
		if account.UserName != "" && account.Password != "" {
			result[account.UserName] = account.Password
		}
	}
	return result
}

func NewMockServer(log logr.Logger, addr string, opts ...Option) *MockServer {
	s := &MockServer{
		addr:              addr,
		log:               log,
		overrides:         make(map[string]any),
		upgradedResources: make(map[string]string),
		accounts:          loadAccountsFromEmbedded(),
		// onCreate hooks run after a new collection member is stored.
		// Add an entry here to handle side-effects for additional collection types.
		onCreate: map[string]memberHook{
			"/AccountService/Accounts": func(s *MockServer, data map[string]any) {
				// Seed missing fields from the embedded account template so that the
				// new account has the full Redfish structure (Actions, PasswordExpiration,
				// AccountTypes, Links, etc.) without enumerating individual fields here.
				if raw, err := dataFS.ReadFile("data/AccountService/Accounts/2/index.json"); err == nil {
					var tmpl map[string]any
					if json.Unmarshal(raw, &tmpl) == nil {
						for k, v := range tmpl {
							if _, exists := data[k]; !exists {
								data[k] = deepCopyAny(v)
							}
						}
					}
				}
				// Always overwrite Actions with the correct target for this account's
				// @odata.id (the template value would point to account 2).
				if odataID, ok := data["@odata.id"].(string); ok && odataID != "" {
					data["Actions"] = map[string]any{
						"#ManagerAccount.ChangePassword": map[string]any{
							"target": odataID + "/Actions/ManagerAccount.ChangePassword",
						},
					}
					// PasswordExpiration: use template value if present, else one year from now.
					if exp, ok := data["PasswordExpiration"].(string); !ok || exp == "" {
						data["PasswordExpiration"] = time.Now().AddDate(1, 0, 0).UTC().Format(time.RFC3339)
					}
				}
				// Update the authentication store with the new account's credentials.
				if username, ok := data["UserName"].(string); ok && username != "" {
					password, _ := data["Password"].(string)
					s.accounts[username] = password
				}
			},
		},
		// onDelete hooks run before a collection member override is erased.
		// data is the full member resource loaded prior to deletion.
		onDelete: map[string]memberHook{
			"/AccountService/Accounts": func(s *MockServer, data map[string]any) {
				if username, ok := data["UserName"].(string); ok && username != "" {
					delete(s.accounts, username)
				}
			},
		},
	}

	// actionHandlers is the ordered POST dispatch table.
	// Add a new actionHandler entry here to support additional Redfish action
	// endpoints — no changes to handlePost are needed.
	s.actionHandlers = []actionHandler{
		{
			matches: hasSuffix("/Actions/ComputerSystem.Reset"),
			handle:  s.handleSystemReset,
		},
		{
			matches: hasSuffix("/Actions/Manager.Reset"),
			handle:  s.handleBMCReset,
		},
		{
			matches: func(path string) bool {
				return strings.Contains(path, "UpdateService/Actions/SimpleUpdate") ||
					strings.Contains(path, "UpdateService/Actions/UpdateService.SimpleUpdate")
			},
			handle: s.handleSimpleUpdate,
		},
		{
			matches: hasSuffix("/Actions/ManagerAccount.ChangePassword"),
			handle:  s.handleChangePassword,
		},
	}

	for _, opt := range opts {
		opt(s)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/redfish/v1/", s.redfishHandler)
	s.handler = mux

	return s
}

// hasSuffix returns a path predicate that checks for a fixed URL suffix.
// Use this as a shorthand when registering actionHandlers.
func hasSuffix(suffix string) func(string) bool {
	return func(path string) bool { return strings.HasSuffix(path, suffix) }
}

func (s *MockServer) redfishHandler(w http.ResponseWriter, r *http.Request) {
	s.log.Info("Received request", "method", r.Method, "path", r.URL.Path)

	s.mu.RLock()
	unavailable := s.unavailable
	s.mu.RUnlock()
	if unavailable {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	// When auth is enabled, the Redfish service root is the only publicly
	// accessible endpoint (gofish fetches it without credentials during
	// ConnectContext). All other requests require valid Basic Auth credentials.
	if s.authEnabled && r.URL.Path != "/redfish/v1/" && r.URL.Path != "/redfish/v1" {
		username, password, ok := r.BasicAuth()
		if !ok || !s.Authenticate(username, password) {
			w.Header().Set("WWW-Authenticate", `Basic realm="redfish"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

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
	for _, a := range s.actionHandlers {
		if a.matches(urlPath) {
			a.handle(w, r, body)
			return
		}
	}
	s.handleCollectionPost(w, r, body)
}

func (s *MockServer) handleCollectionPost(w http.ResponseWriter, r *http.Request, body []byte) {
	var update map[string]any
	if err := json.Unmarshal(body, &update); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	urlPath := resolvePath(r.URL.Path)
	s.mu.Lock()
	defer s.mu.Unlock()

	cached, hasOverride := s.overrides[urlPath]
	var base Collection
	if hasOverride {
		s.log.Info("Using overridden data for POST", "path", urlPath)
		switch v := cached.(type) {
		case Collection:
			base = v
		case map[string]any:
			// Normalise an override stored as map[string]any (e.g. after ResetAccounts).
			b, err := json.Marshal(v)
			if err != nil {
				http.Error(w, "Corrupt overridden JSON", http.StatusInternalServerError)
				return
			}
			if err := json.Unmarshal(b, &base); err != nil {
				http.Error(w, "Corrupt overridden JSON", http.StatusInternalServerError)
				return
			}
		default:
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

	// Derive the next ID from the maximum numeric ID already in the collection
	// rather than from len(Members). Using len() causes collisions after a
	// non-trailing delete (e.g. deleting member 2 from [1,2,3] leaves len=2,
	// so the next POST computes newID=3 which already exists).
	maxID := 0
	for _, m := range base.Members {
		// member OdataID is e.g. "/redfish/v1/AccountService/Accounts/3"
		segment := path.Base(m.OdataID)
		var n int
		if _, err := fmt.Sscanf(segment, "%d", &n); err == nil && n > maxID {
			maxID = n
		}
	}
	newID := fmt.Sprintf("%d", maxID+1)
	location := path.Join(r.URL.Path, newID)
	newMemberPath := resolvePath(location)
	base.Members = append(base.Members, Member{OdataID: location})
	s.log.Info("Adding new member", "id", newID, "location", location, "memberPath", newMemberPath)
	if strings.HasSuffix(r.URL.Path, "/Subscriptions") {
		w.Header().Set("Location", location)
	}
	// Inject standard Redfish identity fields so gofish can parse Id and ODataID.
	update["Id"] = newID
	update["@odata.id"] = location
	s.overrides[urlPath] = base
	s.overrides[newMemberPath] = update
	// Dispatch create hooks for this collection (e.g. credential tracking).
	for suffix, hook := range s.onCreate {
		if strings.HasSuffix(r.URL.Path, suffix) {
			hook(s, update)
			break
		}
	}
	s.writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
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

	if err := s.applyBMCSettings(r.URL.Path, update); err != nil {
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
	// Dispatch delete hooks for this resource (e.g. credential cleanup).
	// base was loaded before the lock and covers both overridden and embedded resources.
	for suffix, hook := range s.onDelete {
		if strings.Contains(r.URL.Path, suffix+"/") {
			hook(s, base)
			break
		}
	}
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

func (s *MockServer) handleSimpleUpdate(w http.ResponseWriter, _ *http.Request, body []byte) {
	var req struct {
		ImageURI string   `json:"ImageURI"`
		Targets  []string `json:"Targets"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "malformed SimpleUpdate JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.ImageURI == "" {
		http.Error(w, "ImageURI is required", http.StatusBadRequest)
		return
	}

	stepsPath := upgradeStepsFilePath
	if strings.Contains(req.ImageURI, "fail") {
		stepsPath = upgradeStepsFailFilePath
	}

	stepsData, err := dataFS.ReadFile(stepsPath)
	if err != nil {
		http.Error(w, "steps not found", http.StatusInternalServerError)
		return
	}
	var steps []map[string]any
	if err := json.Unmarshal(stepsData, &steps); err != nil || len(steps) == 0 {
		http.Error(w, "corrupt steps JSON", http.StatusInternalServerError)
		return
	}

	// Load the base task template and seed it with the first step.
	taskBase, err := s.loadResource(upgradeTaskFilePath)
	if err != nil {
		http.Error(w, "task not found", http.StatusInternalServerError)
		return
	}
	mergeJSON(taskBase, steps[0])

	s.mu.Lock()
	s.upgradeGen++
	gen := s.upgradeGen
	s.overrides[upgradeTaskFilePath] = taskBase
	s.mu.Unlock()

	go s.doUpgradeSteps(gen, steps, req.ImageURI, req.Targets)

	w.Header().Set("Location", upgradeTaskURI)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"status":"Accepted"}`))
}

// doUpgradeSteps advances the task through its steps, updating the override on
// each tick. When the terminal step is reached and it indicates Completed, the
// firmware version fields in the targeted resources are updated so that
// GetBiosVersion / GetBMCVersion return the new version. Target resources are
// resolved dynamically by following the FirmwareInventory items' RelatedItem
// links — no system UUID is hardcoded here. A stale goroutine (superseded by a
// newer SimpleUpdate call) exits without side-effects once it detects a
// generation mismatch.
func (s *MockServer) doUpgradeSteps(gen int64, steps []map[string]any, imageURI string, targets []string) {
	time.Sleep(20 * time.Millisecond)
	for i := 1; i < len(steps); i++ {
		time.Sleep(5 * time.Millisecond)
		s.mu.Lock()
		if s.upgradeGen != gen {
			s.mu.Unlock()
			return
		}
		taskBase, err := s.loadResourceLocked(upgradeTaskFilePath)
		if err == nil {
			mergeJSON(taskBase, steps[i])
			s.overrides[upgradeTaskFilePath] = taskBase
		}
		s.mu.Unlock()
	}

	// Only update version fields when the task reaches Completed.
	lastState, _ := steps[len(steps)-1]["TaskState"].(string)
	if lastState != "Completed" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.upgradeGen != gen {
		return
	}

	s.applyFirmwareVersionsLocked(targets, imageURI)
}

// applyFirmwareVersionsLocked follows each FirmwareInventory target's
// RelatedItem links to discover which System or Manager resource to update,
// then writes the new imageURI into the appropriate version field.
// The caller must hold s.mu.
func (s *MockServer) applyFirmwareVersionsLocked(targets []string, imageURI string) {
	for _, target := range targets {
		invPath := resolvePath(target)
		inv, err := s.loadResourceLocked(invPath)
		if err != nil {
			s.log.Error(err, "Failed to load firmware inventory item", "target", target)
			continue
		}
		relatedItems, _ := inv["RelatedItem"].([]any)
		for _, ri := range relatedItems {
			riMap, _ := ri.(map[string]any)
			odataID, _ := riMap["@odata.id"].(string)
			if odataID == "" {
				continue
			}
			resPath := resolvePath(odataID)
			res, err := s.loadResourceLocked(resPath)
			if err != nil {
				s.log.Error(err, "Failed to load related resource", "path", odataID)
				continue
			}
			switch {
			case strings.Contains(odataID, "/Systems/"):
				res[biosVersionField] = imageURI
			case strings.Contains(odataID, "/Managers/"):
				res[bmcVersionField] = imageURI
			default:
				continue
			}
			s.overrides[resPath] = res
			s.upgradedResources[odataID] = resPath
			s.log.Info("Updated firmware version", "resource", odataID, "version", imageURI)
		}
	}
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

func (s *MockServer) handleChangePassword(w http.ResponseWriter, r *http.Request, body []byte) {
	var req struct {
		NewPassword string `json:"NewPassword"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if req.NewPassword == "" {
		http.Error(w, "NewPassword must not be empty", http.StatusBadRequest)
		return
	}
	accountPath := strings.TrimSuffix(r.URL.Path, "/Actions/ManagerAccount.ChangePassword")
	filePath := resolvePath(accountPath)
	s.mu.Lock()
	defer s.mu.Unlock()
	account, err := s.loadResourceLocked(filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	username, ok := account["UserName"].(string)
	if !ok || username == "" {
		http.Error(w, "account has no UserName", http.StatusBadRequest)
		return
	}
	// Update both the auth store and the persisted resource so that a
	// subsequent GET returns the current password.
	s.accounts[username] = req.NewPassword
	account["Password"] = req.NewPassword
	s.overrides[filePath] = account
	w.WriteHeader(http.StatusNoContent)
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
	// Simulate the BMC being offline during reset.
	// The lock set by handleBMCReset prevents new POST operations during this window.
	time.Sleep(150 * time.Millisecond)

	s.mu.Lock()
	if base, ok := s.overrides[bmcPath].(map[string]any); ok {
		s.setLocked(base, false)
		s.log.Info("BMC reset complete")
	}
	s.mu.Unlock()

	if err := s.applyPendingBMCSettings(); err != nil {
		s.log.Error(err, "Failed to apply pending BMC settings")
	}
}

func (s *MockServer) applyBMCSettings(urlPath string, update map[string]any) error {
	if !strings.Contains(urlPath, "Managers/BMC/Settings") {
		return nil
	}

	attrs, ok := update[attributesKey].(map[string]any)
	if !ok || len(attrs) == 0 {
		return nil
	}

	// Attrs that can be applied immediately without a BMC reset.
	immediate := make(map[string]any)
	for key, val := range attrs {
		if slices.Contains(noRebootBMCSettings, key) {
			immediate[key] = val
		}
	}

	if len(immediate) == 0 {
		return nil
	}

	// Apply to the current BMC manager resource.
	s.mu.Lock()
	defer s.mu.Unlock()

	bmcBase, err := s.loadResourceLocked(bmcFilePath)
	if err != nil {
		return err
	}

	// If the BMC is mid-reset (locked), leave the immediate settings in attrs
	// so they are written to the pending Settings resource and picked up by
	// applyPendingBMCSettings once the reset completes.
	if s.isLocked(bmcBase) {
		return nil
	}

	s.log.Info("Applying BMC settings without reset", "settings", immediate)

	// Remove immediate settings from the pending update so they are not
	// written to the Settings (pending) resource.
	for key := range immediate {
		delete(attrs, key)
	}

	if bmcAttrs, ok := bmcBase[attributesKey].(map[string]any); ok {
		maps.Copy(bmcAttrs, immediate)
	}
	s.overrides[bmcFilePath] = bmcBase

	return nil
}

func (s *MockServer) applyPendingBMCSettings() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	pending, err := s.loadResourceLocked(bmcSettingsFilePath)
	if err != nil {
		return err
	}

	pendingAttrs, ok := pending[attributesKey].(map[string]any)
	if !ok || len(pendingAttrs) == 0 {
		return nil
	}

	current, err := s.loadResourceLocked(bmcFilePath)
	if err != nil {
		return err
	}

	currentAttrs, ok := current[attributesKey].(map[string]any)
	if !ok {
		return nil
	}

	maps.Copy(currentAttrs, pendingAttrs)
	pending[attributesKey] = map[string]any{}

	s.overrides[bmcFilePath] = current
	s.overrides[bmcSettingsFilePath] = pending
	s.log.Info("Applied pending BMC settings")

	return nil
}

// GetBMCSettingAttr returns the current BMC Attributes map for the given managerID
// (e.g. "BMC"). Returns nil if the resource cannot be loaded.
func (s *MockServer) GetBMCSettingAttr(managerID string) map[string]any {
	filePath := fmt.Sprintf("data/Managers/%s/index.json", managerID)
	resource, err := s.loadResource(filePath)
	if err != nil {
		return nil
	}
	attrs, _ := resource[attributesKey].(map[string]any)
	return attrs
}

// ResetBMCSettings resets the BMC attribute state on the server to defaults,
// clearing both current and pending attributes. managerID is the folder name under data/Managers/ (e.g. "BMC").
func (s *MockServer) ResetBMCSettings(managerID string) {
	filePath := fmt.Sprintf("data/Managers/%s/index.json", managerID)
	settingsFilePath := fmt.Sprintf("data/Managers/%s/Settings/index.json", managerID)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resetResourceFromEmbeddedLocked(filePath)
	s.resetResourceFromEmbeddedLocked(settingsFilePath)
}

// ResetBIOSSettings resets the BIOS attribute state on the server to defaults,
// clearing both current and pending attributes. systemID is the folder name under data/Systems/ (e.g. "437XR1138R2").
func (s *MockServer) ResetBIOSSettings(systemID string) {
	filePath := fmt.Sprintf("data/Systems/%s/Bios/index.json", systemID)
	settingsFilePath := fmt.Sprintf("data/Systems/%s/Bios/Settings/index.json", systemID)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resetResourceFromEmbeddedLocked(filePath)
	s.resetResourceFromEmbeddedLocked(settingsFilePath)
}

// ResetUpgradeTask resets upgrade state on the mock server.
//
// With no arguments it resets everything: the task JSON, any in-flight
// goroutine, and all System/Manager resources whose firmware version was
// written by a completed upgrade.
//
// With one or more Redfish resource URIs (e.g. server.Spec.SystemURI or
// "/redfish/v1/Managers/BMC") it resets only the version field for those
// specific resources, leaving others untouched. The task JSON is also reset
// once no upgraded resources remain.
//
// Call this in AfterEach to ensure a clean slate between upgrade-related tests.
func (s *MockServer) ResetUpgradeTask(resourceURIs ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upgradeGen++ // invalidate any running doUpgradeSteps goroutine
	if len(resourceURIs) == 0 {
		s.resetResourceFromEmbeddedLocked(upgradeTaskFilePath)
		for _, p := range s.upgradedResources {
			s.resetResourceFromEmbeddedLocked(p)
		}
		s.upgradedResources = make(map[string]string)
		return
	}
	for _, uri := range resourceURIs {
		if resPath, ok := s.upgradedResources[uri]; ok {
			s.resetResourceFromEmbeddedLocked(resPath)
			delete(s.upgradedResources, uri)
		}
	}
	if len(s.upgradedResources) == 0 {
		s.resetResourceFromEmbeddedLocked(upgradeTaskFilePath)
	}
}

// resetResourceFromEmbeddedLocked replaces the override for filePath with the
// full contents of the embedded file, clearing all mutated fields including
// resourceLock, Attributes, and any other state accumulated during a test.
// The caller must hold s.mu.
func (s *MockServer) resetResourceFromEmbeddedLocked(filePath string) {
	raw, err := dataFS.ReadFile(filePath)
	if err != nil {
		s.log.Error(err, "Failed to read embedded default", "path", filePath)
		return
	}
	var defaults map[string]any
	if err := json.Unmarshal(raw, &defaults); err != nil {
		s.log.Error(err, "Failed to parse embedded default", "path", filePath)
		return
	}
	s.overrides[filePath] = defaults
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
	// Collection overrides are stored as Collection structs by handleCollectionPost/handleDelete.
	// Marshal to map[string]any so this path is uniform with the embedded-data path.
	if col, ok := s.overrides[filePath].(Collection); ok {
		b, err := json.Marshal(col)
		if err != nil {
			return nil, fmt.Errorf("%w: marshal collection: %w", errCorruptJSON, err)
		}
		var result map[string]any
		if err := json.Unmarshal(b, &result); err != nil {
			return nil, fmt.Errorf("%w: unmarshal collection: %w", errCorruptJSON, err)
		}
		return result, nil
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

// SetUnavailable toggles the simulated-unavailable mode. When true every request
// returns 503 Service Unavailable, allowing tests to verify BMC connection-error
// handling without touching UnitTestMockUps.
func (s *MockServer) SetUnavailable(unavailable bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unavailable = unavailable
}

// Authenticate returns true if username/password match the stored account credentials.
func (s *MockServer) Authenticate(username, password string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stored, ok := s.accounts[username]
	return ok && stored == password
}

// GetAccountNames returns the set of UserName values currently in the account
// collection (including any created by tests). Suitable for gomega HaveKey assertions.
func (s *MockServer) GetAccountNames() map[string]struct{} {
	collection, err := s.loadResource("data/AccountService/Accounts/index.json")
	if err != nil {
		return map[string]struct{}{}
	}
	members, _ := collection["Members"].([]any)
	result := make(map[string]struct{}, len(members))
	for _, m := range members {
		mMap, ok := m.(map[string]any)
		if !ok {
			continue
		}
		odataID, _ := mMap["@odata.id"].(string)
		if odataID == "" {
			continue
		}
		member, err := s.loadResource(resolvePath(odataID))
		if err != nil {
			continue
		}
		if uname, ok := member["UserName"].(string); ok && uname != "" {
			result[uname] = struct{}{}
		}
	}
	return result
}

// ResetAccounts clears all account-collection overrides (restoring the embedded
// default collection) and resets the password store to defaults. Call this in
// AfterEach to ensure a clean slate between account-related tests.
func (s *MockServer) ResetAccounts() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resetResourceFromEmbeddedLocked("data/AccountService/Accounts/index.json")
	for key := range s.overrides {
		if strings.HasPrefix(key, "data/AccountService/Accounts/") &&
			key != "data/AccountService/Accounts/index.json" {
			delete(s.overrides, key)
		}
	}
	s.accounts = loadAccountsFromEmbedded()
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

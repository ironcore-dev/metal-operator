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

	"github.com/ironcore-dev/controller-utils/conditionutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/api/registry"
	metaltoken "github.com/ironcore-dev/metal-operator/internal/token"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Server holds the HTTP server's state, including the systems store and signing secret.
type Server struct {
	addr              string
	mux               *http.ServeMux
	systemsStore      *sync.Map
	signingSecret     []byte       // Shared secret for HMAC token verification
	signingSecretName string       // Name of the signing secret
	signingSecretNs   string       // Namespace of the signing secret
	signingSecretMu   sync.RWMutex // Read-write mutex for signing secret access
	log               logr.Logger
	k8sClient         client.Client
}

// NewServer initializes and returns a new Server instance.
// It loads the signing secret from Kubernetes for token verification.
func NewServer(logger logr.Logger, addr string, k8sClient client.Client, signingSecretName, signingSecretNamespace string) *Server {
	mux := http.NewServeMux()

	// Warn if running without K8s client (test mode)
	if k8sClient == nil {
		logger.Info("WARNING: Running without K8s client - token validation DISABLED (test mode only)")
	}

	// Load signing secret from Kubernetes
	var signingSecret []byte
	if k8sClient != nil && signingSecretName != "" && signingSecretNamespace != "" {
		ctx := context.Background()
		secret := &corev1.Secret{}
		err := k8sClient.Get(ctx, client.ObjectKey{
			Name:      signingSecretName,
			Namespace: signingSecretNamespace,
		}, secret)

		if err != nil {
			logger.Error(err, "Failed to load signing secret",
				"name", signingSecretName, "namespace", signingSecretNamespace)
		} else {
			if key, ok := secret.Data[metaltoken.DiscoveryTokenSigningSecretKey]; ok && len(key) == 32 {
				signingSecret = key
				logger.Info("Loaded discovery token signing secret",
					"name", signingSecretName, "namespace", signingSecretNamespace)
			} else {
				logger.Error(nil, "Signing secret found but invalid",
					"name", signingSecretName, "namespace", signingSecretNamespace)
			}
		}

		if len(signingSecret) == 0 {
			logger.Info("Signing secret not loaded, token validation will fail until secret is available",
				"name", signingSecretName, "namespace", signingSecretNamespace)
		}
	}

	server := &Server{
		addr:              addr,
		mux:               mux,
		systemsStore:      &sync.Map{},
		signingSecret:     signingSecret,
		signingSecretName: signingSecretName,
		signingSecretNs:   signingSecretNamespace,
		log:               logger,
		k8sClient:         k8sClient,
	}
	server.routes()
	return server
}

// getSigningSecret returns the signing secret, loading it from Kubernetes if not cached.
// This method is thread-safe and only memoizes successful loads, allowing recovery from transient failures.
// It uses a fast-path read lock check and upgrades to write lock only when loading is needed.
// It respects the provided context for cancellation and timeouts.
func (s *Server) getSigningSecret(ctx context.Context) ([]byte, error) {
	// Fast path: check if we already have a valid cached secret (read lock)
	s.signingSecretMu.RLock()
	if len(s.signingSecret) == 32 {
		secret := s.signingSecret
		s.signingSecretMu.RUnlock()
		return secret, nil
	}
	s.signingSecretMu.RUnlock()

	// Slow path: need to load the secret (write lock)
	if s.signingSecretName == "" || s.signingSecretNs == "" {
		return nil, fmt.Errorf("signing secret name or namespace not configured")
	}

	// Attempt to load the secret from Kubernetes
	secret := &corev1.Secret{}
	err := s.k8sClient.Get(ctx, client.ObjectKey{
		Name:      s.signingSecretName,
		Namespace: s.signingSecretNs,
	}, secret)

	if err != nil {
		return nil, fmt.Errorf("failed to load signing secret: %w", err)
	}

	key, ok := secret.Data[metaltoken.DiscoveryTokenSigningSecretKey]
	if !ok || len(key) != 32 {
		return nil, fmt.Errorf("signing secret invalid or missing")
	}

	// Cache the loaded secret (write lock)
	s.signingSecretMu.Lock()
	s.signingSecret = key
	s.signingSecretMu.Unlock()

	s.log.Info("Loaded discovery token signing secret", "name", s.signingSecretName)
	return key, nil
}

// validateDiscoveryToken verifies a JWT-signed discovery token.
// Returns (systemUUID, valid) where systemUUID is extracted from the token.
// If k8sClient is nil (unit test mode), validation is skipped.
// Implements single-retry refresh on validation failure to handle secret rotation.
func (s *Server) validateDiscoveryToken(ctx context.Context, receivedToken string) (string, bool) {
	// Skip validation in test mode (no k8s client)
	if s.k8sClient == nil {
		s.log.V(1).Info("Running in TEST MODE without K8s client - skipping secret loading")
		// In test mode, extract systemUUID from token if possible, otherwise return empty
		// For now, just allow all requests in test mode
		return "", true
	}

	// Reject if token is missing
	if receivedToken == "" {
		s.log.Info("Rejected request with missing discovery token")
		return "", false
	}

	// Get signing secret (thread-safe, loads on first call)
	secret, err := s.getSigningSecret(ctx)
	if err != nil {
		s.log.Error(err, "Signing secret not available")
		return "", false
	}

	// Verify the signed token
	systemUUID, timestamp, valid, err := metaltoken.VerifySignedDiscoveryToken(secret, receivedToken)
	if err != nil || !valid {
		// Token validation failed - could be due to secret rotation
		// Clear cache and retry once with fresh secret
		s.log.V(1).Info("Token validation failed, clearing cache and retrying", "error", err)

		s.signingSecretMu.Lock()
		s.signingSecret = nil
		s.signingSecretMu.Unlock()

		// Retry with fresh secret
		secret, err = s.getSigningSecret(ctx)
		if err != nil {
			s.log.Error(err, "Failed to reload signing secret for retry")
			return "", false
		}

		systemUUID, timestamp, valid, err = metaltoken.VerifySignedDiscoveryToken(secret, receivedToken)
		if err != nil {
			s.log.Error(err, "Error verifying discovery token after retry")
			return "", false
		}

		if !valid {
			s.log.Info("Rejected request with invalid discovery token after retry", "tokenLength", len(receivedToken))
			return "", false
		}
	}

	// Token is valid
	s.log.V(1).Info("Validated discovery token", "systemUUID", systemUUID, "timestamp", timestamp)
	return systemUUID, true
}

// routes registers the server's routes.
func (s *Server) routes() {
	s.mux.HandleFunc("/register", s.registerHandler)
	s.mux.HandleFunc("/delete/", s.deleteHandler)
	s.mux.HandleFunc("/systems/", s.systemsHandler)
	s.mux.HandleFunc("/bootstate", s.bootstateHandler)
}

// registerHandler handles the /register endpoint.
// Requires a valid discovery token for authentication.
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

	// Validate discovery token and extract systemUUID
	systemUUID, valid := s.validateDiscoveryToken(r.Context(), reg.DiscoveryToken)
	if !valid {
		http.Error(w, "Unauthorized: invalid or missing discovery token", http.StatusUnauthorized)
		s.log.Info("Rejected registration attempt with invalid token")
		return
	}

	// Verify the systemUUID from the token matches the payload (skip in unit test mode)
	if s.k8sClient != nil && systemUUID != "" && systemUUID != reg.SystemUUID {
		http.Error(w, "Unauthorized: systemUUID mismatch", http.StatusUnauthorized)
		s.log.Info("Rejected registration attempt with mismatched systemUUID",
			"claimed", reg.SystemUUID, "actual", systemUUID)
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

func (s *Server) bootstateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		s.log.Info("Received unsupported HTTP method", "method", r.Method)
		return
	}
	var payload registry.BootstatePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		s.log.Error(err, "Failed to decode bootstate payload")
		return
	}

	// Validate discovery token and extract systemUUID
	systemUUID, valid := s.validateDiscoveryToken(r.Context(), payload.DiscoveryToken)
	if !valid {
		http.Error(w, "Unauthorized: invalid or missing token", http.StatusUnauthorized)
		s.log.Info("Rejected bootstate attempt with invalid token")
		return
	}

	// Verify the systemUUID from the token matches the payload (skip in unit test mode)
	if s.k8sClient != nil && systemUUID != "" && systemUUID != payload.SystemUUID {
		http.Error(w, "Unauthorized: systemUUID mismatch", http.StatusUnauthorized)
		s.log.Info("Rejected bootstate attempt with mismatched systemUUID",
			"claimed", payload.SystemUUID, "actual", systemUUID)
		return
	}

	s.log.Info("Received boot state for system", "SystemUUID", payload.SystemUUID, "BootState", payload.Booted)
	if !payload.Booted {
		w.WriteHeader(http.StatusOK)
		return
	}
	var servers metalv1alpha1.ServerList
	if err := s.k8sClient.List(r.Context(), &servers, client.MatchingFields{"spec.systemUUID": payload.SystemUUID}); err != nil {
		http.Error(w, fmt.Sprintf("Failed to list servers for system UUID %s: %v", payload.SystemUUID, err), http.StatusInternalServerError)
		s.log.Error(err, "Failed to list servers for system", "systemUUID", payload.SystemUUID)
		return
	}
	if len(servers.Items) != 1 {
		http.Error(w, fmt.Sprintf("No servers found for system UUID %s", payload.SystemUUID), http.StatusNotFound)
		s.log.Info("Found unexpected number of server of system", "systemUUID", payload.SystemUUID, "count", len(servers.Items))
		return
	}
	server := servers.Items[0]
	bootConfigRef := server.Spec.BootConfigurationRef
	if bootConfigRef == nil {
		http.Error(w, fmt.Sprintf("Servers for system UUID %s does not reference a ServerBootConfiguration", payload.SystemUUID), http.StatusNotFound)
		s.log.Info("Server does not reference a ServerBootConfiguration", "server", server.Name)
		return
	}
	bootConfigKey := client.ObjectKey{Namespace: bootConfigRef.Namespace, Name: bootConfigRef.Name}
	var bootConfig metalv1alpha1.ServerBootConfiguration
	if err := s.k8sClient.Get(r.Context(), bootConfigKey, &bootConfig); err != nil {
		http.Error(w, fmt.Sprintf("No ServerBootConfig found for system UUID %s", payload.SystemUUID), http.StatusNotFound)
		s.log.Error(err, "Failed to retrieve ServerBootConfiguration", "name", bootConfigKey.Name, "namespace", bootConfig.Namespace)
		return
	}
	acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
	original := bootConfig.DeepCopy()
	err := acc.UpdateSlice(
		&bootConfig.Status.Conditions,
		registry.BootStateReceivedCondition,
		conditionutils.UpdateStatus(metav1.ConditionTrue),
		conditionutils.UpdateReason("BootStateReceived"),
		conditionutils.UpdateMessage("Server successfully posted boot state"),
		conditionutils.UpdateObserved(&bootConfig),
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to update booted condition for ServerBootConfig %s: %v", bootConfig.Name, err), http.StatusInternalServerError)
		s.log.Error(err, "Failed to update booted condition for ServerBootConfig", "name", bootConfigKey.Name, "namespace", bootConfig.Namespace)
		return
	}
	if err := s.k8sClient.Status().Patch(r.Context(), &bootConfig, client.MergeFrom(original)); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update boot state for ServerBootConfig %s: %v", bootConfig.Name, err), http.StatusInternalServerError)
		s.log.Error(err, "Failed to update boot state for ServerBootConfig", "name", bootConfigKey.Name, "namespace", bootConfig.Namespace)
		return
	}
	s.log.Info("Updated boot state for ServerBootConfig", "name", bootConfigKey.Name, "namespace", bootConfig.Namespace)
	w.WriteHeader(http.StatusOK)
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
		s.log.Info("Shutting down registry server")
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

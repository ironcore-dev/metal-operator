// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package metaldata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

const (
	shutdownTimeout = 5 * time.Second
)

type Server struct {
	srv      *http.Server
	listener net.Listener
	log      logr.Logger
	ready    chan struct{}
}

// Ready returns true as soon as the listener has been opened. It does not
// guarantee that the HTTP server is already running. Once it returns true it is
// safe to call [Server.Addr].
func (s *Server) Ready() bool {
	select {
	case <-s.ready:
		return true
	default:
		return false
	}
}

func (s *Server) Addr() net.Addr {
	return s.listener.Addr()
}

func (s *Server) Start(ctx context.Context) (err error) {
	s.log.Info("Starting HTTP server", "addr", s.srv.Addr)

	s.listener, err = net.Listen("tcp", s.srv.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.srv.Addr, err)
	}

	close(s.ready)

	errChan := make(chan error, 1)
	go func() {
		if err := s.srv.Serve(s.listener); !errors.Is(err, http.ErrServerClosed) {
			errChan <- fmt.Errorf("HTTP metaldata server Serve: %w", err)
		}
	}()
	select {
	case <-ctx.Done():
		s.log.Info("Shutting down HTTP server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := s.srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("HTTP server Shutdown: %w", err)
		}
		s.log.Info("HTTP server gracefully stopped")
		return nil
	case err := <-errChan:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if shutdownErr := s.srv.Shutdown(shutdownCtx); shutdownErr != nil {
			s.log.Error(shutdownErr, "Error shutting down HTTP server")
		}
		return err
	}
}

func NewServer(log logr.Logger, idx *Index, reader client.Reader, addr string) *Server {
	v1 := &v1Handler{
		log:    log.WithName("v1"),
		index:  idx,
		reader: reader,
	}
	v1Mux := http.NewServeMux()
	v1Mux.HandleFunc("GET /v1/{$}", v1.serveFullMetadata)
	v1Mux.HandleFunc("GET /v1/user-data", v1.serveUserData)
	v1Mux.HandleFunc("GET /v1/user-data/{key}", v1.serveUserDataKey)
	v1Mux.HandleFunc("GET /v1/{field}", v1.serveField)

	mux := http.NewServeMux()
	mux.Handle("/v1/", metadataFlavorMiddleware(v1Mux))

	return &Server{
		srv: &http.Server{
			Addr:    addr,
			Handler: accessLogger(log.WithName("access"), mux),
		},
		log:   log,
		ready: make(chan struct{}),
	}
}

func accessLogger(log logr.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wrapping the response writer is the only way to capture the status
		// code from net/http; doing so breaks any handler that relies on
		// casting the writer to other interfaces. Unwrap mitigates this for
		// callers that use http.ResponseController, at the cost of losing the
		// status code recording for that path.
		rec := &statusRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		start := time.Now()

		next.ServeHTTP(rec, r)

		log.Info("Handled request",
			"client", r.RemoteAddr,
			"duration", time.Since(start),
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.statusCode,
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func metadataFlavorMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(metalv1alpha1.MetadataFlavorHeader) != metalv1alpha1.MetadataFlavorValue {
			http.Error(w, "missing required header: Metadata-Flavor: IronCore Metal", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type v1Handler struct {
	log    logr.Logger
	index  *Index
	reader client.Reader
}

// lookupEntry resolves the client's IP to a ServerEntry. On failure it writes
// an error response (400 or 404) to w and returns false.
func (h *v1Handler) lookupEntry(w http.ResponseWriter, r *http.Request) (*ServerEntry, bool) {
	addrPort, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return nil, false
	}
	entry, ok := h.index.Lookup(addrPort.Addr())
	if !ok {
		http.NotFound(w, r)
		return nil, false
	}
	return entry, true
}

// buildMetadata returns a mutable copy of the entry's metadata as
// map[string]any. A copy is needed because callers add the user-data key (a
// nested map) and because the original map is shared with the index.
func (h *v1Handler) buildMetadata(entry *ServerEntry) map[string]any {
	md := make(map[string]any, len(entry.Metadata))
	for k, v := range entry.Metadata {
		md[k] = v
	}
	return md
}

// fetchUserData resolves a Server's claimed user-data Secret. It looks up the
// ServerClaim referenced by the entry, follows its UserDataRef, fetches the
// Secret, and returns its data. Both reads are direct API calls.
func (h *v1Handler) fetchUserData(ctx context.Context, claimRef *metalv1alpha1.ImmutableObjectReference) (map[string][]byte, error) {
	if claimRef == nil {
		return nil, nil
	}
	claim := &metalv1alpha1.ServerClaim{}
	claimKey := client.ObjectKey{Namespace: claimRef.Namespace, Name: claimRef.Name}
	if err := h.reader.Get(ctx, claimKey, claim); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		h.log.Error(err, "Failed to fetch ServerClaim", "namespace", claimKey.Namespace, "name", claimKey.Name)
		return nil, err
	}
	if claim.Spec.UserDataRef == nil {
		return nil, nil
	}

	secret := &corev1.Secret{}
	secretKey := client.ObjectKey{Namespace: claim.Namespace, Name: claim.Spec.UserDataRef.Name}
	if err := h.reader.Get(ctx, secretKey, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		h.log.Error(err, "Failed to fetch user-data Secret", "namespace", secretKey.Namespace, "name", secretKey.Name)
		return nil, err
	}
	if secret.Type != metalv1alpha1.SecretTypeUserData {
		return nil, fmt.Errorf("wrong secret type for user-data '%s'", secret.Type)
	}
	return secret.Data, nil
}

func userDataToStrings(data map[string][]byte) map[string]string {
	m := make(map[string]string, len(data))
	for k, v := range data {
		m[k] = string(v)
	}
	return m
}

func (h *v1Handler) serveFullMetadata(w http.ResponseWriter, r *http.Request) {
	entry, ok := h.lookupEntry(w, r)
	if !ok {
		return
	}

	md := h.buildMetadata(entry)
	userData, err := h.fetchUserData(r.Context(), entry.ClaimRef)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if userData != nil {
		md["user-data"] = userDataToStrings(userData)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(md)
	if err != nil {
		h.log.V(1).Info("Failed to encode metadata response", "error", err)
	}
}

func (h *v1Handler) serveField(w http.ResponseWriter, r *http.Request) {
	entry, ok := h.lookupEntry(w, r)
	if !ok {
		return
	}

	field := r.PathValue("field")
	val, ok := entry.Metadata[field]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprint(w, val)
}

func (h *v1Handler) serveUserData(w http.ResponseWriter, r *http.Request) {
	entry, ok := h.lookupEntry(w, r)
	if !ok {
		return
	}

	userData, err := h.fetchUserData(r.Context(), entry.ClaimRef)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if userData == nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(userDataToStrings(userData))
	if err != nil {
		h.log.V(1).Info("Failed to encode user-data response", "error", err)
	}
}

func (h *v1Handler) serveUserDataKey(w http.ResponseWriter, r *http.Request) {
	entry, ok := h.lookupEntry(w, r)
	if !ok {
		return
	}

	userData, err := h.fetchUserData(r.Context(), entry.ClaimRef)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if userData == nil {
		http.NotFound(w, r)
		return
	}

	key := r.PathValue("key")
	val, ok := userData[key]
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(val)
	if err != nil {
		h.log.V(1).Info("Failed to send user-data key response", "error", err)
	}
}

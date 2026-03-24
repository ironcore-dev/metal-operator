// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package serverevents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Server struct {
	addr      string
	mux       *http.ServeMux
	log       logr.Logger
	collector *RedfishEventCollector
}

type MetricsReport struct {
	MetricsValues []MetricsValue `json:"MetricsValues"`
}

type MetricsValue struct {
	MetricID        string `json:"MetricId"`
	MetricProperty  string `json:"MetricProperty"`
	MetricValue     string `json:"MetricValue"`
	Units           string `json:"Units"`
	MetricValueKind string `json:"MetricValueKind"`
	Timestamp       string `json:"Timestamp"`
	Oem             any    `json:"Oem"`
}

type EventData struct {
	Events []Event `json:"Alerts"`
	Name   string  `json:"Name"`
}

type Event struct {
	EventID           string `json:"EventId"`
	Message           string `json:"Message"`
	Severity          string `json:"Severity"`
	EventTimestamp    string `json:"EventTimestamp"`
	OriginOfCondition string `json:"OriginOfCondition"`
}

func NewServer(log logr.Logger, addr string) *Server {
	mux := http.NewServeMux()
	collector := NewRedfishEventCollector()
	collector.SetLogger(log)
	server := &Server{
		addr:      addr,
		mux:       mux,
		log:       log,
		collector: collector,
	}
	server.routes()
	return server
}

// SetClient sets the Kubernetes client on the collector for server tainting
func (s *Server) SetClient(k8sClient client.Client) {
	s.collector.SetClient(k8sClient)
}

// SetCriticalEventHandler sets the handler for critical events
func (s *Server) SetCriticalEventHandler(handler CriticalEventHandler) {
	s.collector.SetCriticalEventHandler(handler)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/serverevents/alerts/", s.alertHandler)
	s.mux.HandleFunc("/serverevents/metricsreport/", s.metricsreportHandler)
}

func hostnameFromPath(requestPath, prefix string) (string, bool) {
	if !strings.HasPrefix(requestPath, prefix) {
		return "", false
	}

	hostname := strings.TrimPrefix(requestPath, prefix)
	if hostname == "" || strings.Contains(hostname, "/") {
		return "", false
	}
	return hostname, true
}

func (s *Server) alertHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}
	s.log.Info("Received alert data")
	// expected path: /serverevents/alerts/{hostname}
	hostname, ok := hostnameFromPath(r.URL.Path, "/serverevents/alerts/")
	if !ok {
		s.log.Error(nil, "Invalid hostname in event URL", "path", r.URL.Path, "extracted", hostname)
		http.Error(w, "Hostname missing in URL path", http.StatusBadRequest)
		return
	}
	eventData := EventData{}
	if err := json.NewDecoder(r.Body).Decode(&eventData); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.collector.UpdateFromEvent(hostname, eventData)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) metricsreportHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}
	// expected path: /serverevents/metricsreport/{hostname}
	hostname, ok := hostnameFromPath(r.URL.Path, "/serverevents/metricsreport/")
	if !ok {
		s.log.Error(nil, "Invalid hostname in event URL", "path", r.URL.Path, "extracted", hostname)
		http.Error(w, "Hostname missing in URL path", http.StatusBadRequest)
		return
	}
	s.log.Info("received metrics report", "hostname", hostname)
	metricsReport := MetricsReport{}
	if err := json.NewDecoder(r.Body).Decode(&metricsReport); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.collector.UpdateFromMetricsReport(hostname, metricsReport)
	w.WriteHeader(http.StatusOK)
}

// Start starts the server on the specified address and adds logging for key events.
func (s *Server) Start(ctx context.Context) error {
	s.log.Info("Starting event server", "address", s.addr)
	server := &http.Server{Addr: s.addr, Handler: s.mux}

	errChan := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errChan <- fmt.Errorf("HTTP event server ListenAndServe: %w", err)
		}
	}()
	select {
	case <-ctx.Done():
		s.log.Info("Shutting down event server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("HTTP server Shutdown: %w", err)
		}
		s.log.Info("Event server graciously stopped")
		return nil
	case err := <-errChan:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if shutdownErr := server.Shutdown(shutdownCtx); shutdownErr != nil {
			s.log.Error(shutdownErr, "Error shutting down event server")
		}
		return err
	}
}

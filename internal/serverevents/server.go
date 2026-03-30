// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package serverevents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"time"

	"github.com/go-logr/logr"
)

type Server struct {
	addr      string
	mux       *http.ServeMux
	log       logr.Logger
	collector *RedfishEventCollector
}

const (
	// maxEventBodyBytes is the maximum allowed size for event payloads (1MB)
	// This prevents DoS attacks via large request bodies
	maxEventBodyBytes = 1 << 20 // 1 MB
)

type MetricsReport struct {
	// Standard Redfish fields
	ODataID   string `json:"@odata.id,omitempty"`
	ODataType string `json:"@odata.type,omitempty"`
	ID        string `json:"Id,omitempty"`
	Name      string `json:"Name,omitempty"`
	// Metric values array - correct Redfish field name
	MetricValues []MetricValue `json:"MetricValues,omitempty"`
}

type MetricValue struct {
	MetricID        string `json:"MetricId"`
	MetricProperty  string `json:"MetricProperty"`
	MetricValue     string `json:"MetricValue"`
	Units           string `json:"Units"`
	MetricValueKind string `json:"MetricValueKind"`
	Timestamp       string `json:"Timestamp"`
	Oem             any    `json:"Oem"`
}

type EventData struct {
	Events []Event `json:"Events,omitempty"` // Standard Redfish field
	Alerts []Event `json:"Alerts,omitempty"` // Alternative vendor field
	Name   string  `json:"Name"`
}

// GetEvents returns events from whichever field is populated
func (e *EventData) GetEvents() []Event {
	if len(e.Events) > 0 {
		return e.Events
	}
	return e.Alerts
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
	server := &Server{
		addr:      addr,
		mux:       mux,
		log:       log,
		collector: NewRedfishEventCollector(),
	}
	server.routes()
	return server
}

func (s *Server) routes() {
	s.mux.HandleFunc("/serverevents/alerts/", s.alertHandler)
	s.mux.HandleFunc("/serverevents/metricsreport/", s.metricsreportHandler)
}

func (s *Server) alertHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}
	hostname := path.Base(r.URL.Path)

	// Limit request body size to prevent DoS attacks
	r.Body = http.MaxBytesReader(w, r.Body, maxEventBodyBytes)

	// Read body into buffer so we can log it
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		// Check if error is due to exceeding size limit
		if err.Error() == "http: request body too large" {
			s.log.Info("Request body too large", "hostname", hostname, "maxBytes", maxEventBodyBytes)
			http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		s.log.Error(err, "Failed to read request body", "hostname", hostname)
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	// Log raw payload at V(1) for debugging
	s.log.V(1).Info("Received alert payload", "hostname", hostname, "payload", string(bodyBytes))

	eventData := EventData{}
	if err := json.Unmarshal(bodyBytes, &eventData); err != nil {
		s.log.Error(err, "Failed to decode alert data", "hostname", hostname, "payload", string(bodyBytes))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	events := eventData.GetEvents()
	if len(events) == 0 {
		s.log.Info("Received empty event data - check payload format", "hostname", hostname, "payload", string(bodyBytes))
	} else {
		s.log.Info("Processed events successfully", "hostname", hostname, "count", len(events))
	}

	s.collector.UpdateFromEvent(hostname, eventData)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) metricsreportHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}
	hostname := path.Base(r.URL.Path)

	// Limit request body size to prevent DoS attacks
	r.Body = http.MaxBytesReader(w, r.Body, maxEventBodyBytes)

	// Read body into buffer
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		// Check if error is due to exceeding size limit
		if err.Error() == "http: request body too large" {
			s.log.Info("Request body too large", "hostname", hostname, "maxBytes", maxEventBodyBytes)
			http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		s.log.Error(err, "Failed to read request body", "hostname", hostname)
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	// Log raw payload at V(1) for debugging
	s.log.V(1).Info("Received metrics payload", "hostname", hostname, "payload", string(bodyBytes))

	metricsReport := MetricsReport{}
	if err := json.Unmarshal(bodyBytes, &metricsReport); err != nil {
		s.log.Error(err, "Failed to decode metrics report", "hostname", hostname, "payload", string(bodyBytes))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if len(metricsReport.MetricValues) == 0 {
		s.log.Info("Received empty metrics report - check payload format", "hostname", hostname, "payload", string(bodyBytes))
	} else {
		s.log.Info("Processed metrics successfully", "hostname", hostname, "count", len(metricsReport.MetricValues))
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

// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package serverevents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

type Server struct {
	addr      string
	mux       *http.ServeMux
	log       logr.Logger
	collector *RedfishEventCollector
}

var (
	alertsGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "redfish_alerts_total",
			Help: "Number of redfish alerts",
		},
		[]string{"hostname", "severity"},
	)
	metricsReportCollectors map[string]*prometheus.GaugeVec
)

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

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(alertsGauge)
	metricsReportCollectors = make(map[string]*prometheus.GaugeVec)
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
	s.mux.HandleFunc("/serverevents/alerts", s.alertHandler)
	s.mux.HandleFunc("/serverevents/metricsreport", s.metricsreportHandler)
}

func (s *Server) alertHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}
	s.log.Info("Received alert data")
	// expected path: /serverevents/alerts/{hostname}
	hostname := path.Base(r.URL.Path)
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
	hostname := path.Base(r.URL.Path)
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
	s.log.Info("Starting registry server", "address", s.addr)
	server := &http.Server{Addr: s.addr, Handler: s.mux}

	errChan := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errChan <- fmt.Errorf("HTTP registry server ListenAndServe: %w", err)
		}
	}()
	select {
	case <-ctx.Done():
		s.log.Info("Shutting down registry server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("HTTP server Shutdown: %w", err)
		}
		s.log.Info("Registry server graciously stopped")
		return nil
	case err := <-errChan:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if shutdownErr := server.Shutdown(shutdownCtx); shutdownErr != nil {
			s.log.Error(shutdownErr, "Error shutting down registry server")
		}
		return err
	}
}

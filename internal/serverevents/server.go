package serverevents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

type Server struct {
	addr string
	mux  *http.ServeMux
	log  logr.Logger
}

var (
	alertsGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "redfish_alerts_total",
			Help: "Number of redfish alerts",
		},
		[]string{"hostname", "vendor", "severity"},
	)
	metricsReportCollectors map[string]*prometheus.GaugeVec
)

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(alertsGauge)
}

func NewServer(log logr.Logger, addr string) *Server {
	mux := http.NewServeMux()
	server := &Server{
		addr: addr,
		mux:  mux,
		log:  log,
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
	// expected path: /serverevents/alerts/{vendor}/{hostname}
	hostname := path.Base(r.URL.Path)
	vendor := path.Base(path.Dir(r.URL.Path))
	eventData := EventData{}
	if err := json.NewDecoder(r.Body).Decode(&eventData); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	totalWarnings := 0
	totalCriticals := 0
	for _, event := range eventData.Events {
		if event.Severity == "Warning" {
			totalWarnings++
		}
		if event.Severity == "Critical" {
			totalCriticals++
		}
	}
	alertsGauge.WithLabelValues(hostname, vendor, "Warning").Set(float64(totalWarnings))
	alertsGauge.WithLabelValues(hostname, vendor, "Critical").Set(float64(totalCriticals))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Alert data received"))
}

func (s *Server) metricsreportHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}
	// expected path: /serverevents/metricsreport/{vendor}/{hostname}
	hostname := path.Base(r.URL.Path)
	vendor := path.Base(path.Dir(r.URL.Path))
	s.log.Info("receieved metrics report", "uuid", hostname)
	metricsReport := MetricsReport{}
	if err := json.NewDecoder(r.Body).Decode(&metricsReport); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
	for _, mv := range metricsReport.Data.MetricsValues {
		if _, ok := metricsReportCollectors[mv.MetricId]; !ok {
			gauge := prometheus.NewGaugeVec(
				prometheus.GaugeOpts{
					Name: mv.MetricProperty,
					Help: "Metric with ID " + mv.MetricId,
				},
				[]string{"hostname", "vendor"},
			)
			floatVal, err := strconv.ParseFloat(mv.MetricValue, 64)
			if err != nil {
				if mv.MetricValue == "Up" || mv.MetricValue == "Operational" {
					floatVal = 1
				}
			}
			gauge.WithLabelValues(hostname, vendor).Set(floatVal)
			metricsReportCollectors[mv.MetricId] = gauge
			metrics.Registry.MustRegister(gauge)
		}
		s.log.Info("Metric", "id", mv.MetricId, "property", mv.MetricProperty, "value", mv.MetricValue, "timestamp", mv.Timestamp)
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Metrics report data received"))
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
		if err := server.Shutdown(ctx); err != nil {
			return fmt.Errorf("HTTP server Shutdown: %w", err)
		}
		s.log.Info("Registry server graciously stopped")
		return nil
	case err := <-errChan:
		if shutdownErr := server.Shutdown(ctx); shutdownErr != nil {
			s.log.Error(shutdownErr, "Error shutting down registry server")
		}
		return err
	}
}

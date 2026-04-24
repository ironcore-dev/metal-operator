// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package serverevents

import (
	"context"
	"sync"
	"time"

	"github.com/go-logr/logr"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Server polls BMCs for metrics and events and exposes them via Prometheus.
type Server struct {
	client           client.Client
	interval         time.Duration
	log              logr.Logger
	collector        *RedfishEventCollector
	defaultProtocol  metalv1alpha1.ProtocolScheme
	skipCertValidate bool
	bmcOptions       bmc.Options
}

// ServerConfig contains configuration for the metrics polling server.
type ServerConfig struct {
	Client           client.Client
	Interval         time.Duration
	DefaultProtocol  metalv1alpha1.ProtocolScheme
	SkipCertValidate bool
	BMCOptions       bmc.Options
}

type MetricsReport struct {
	ODataID      string        `json:"@odata.id,omitempty"`
	ODataType    string        `json:"@odata.type,omitempty"`
	ID           string        `json:"Id,omitempty"`
	Name         string        `json:"Name,omitempty"`
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
	Events []Event `json:"Events,omitempty"`
	Alerts []Event `json:"Alerts,omitempty"`
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

// NewServer creates a new metrics polling server.
func NewServer(log logr.Logger, config ServerConfig) *Server {
	return &Server{
		client:           config.Client,
		interval:         config.Interval,
		log:              log.WithName("metrics-server"),
		collector:        GetCollector(),
		defaultProtocol:  config.DefaultProtocol,
		skipCertValidate: config.SkipCertValidate,
		bmcOptions:       config.BMCOptions,
	}
}

// Start starts the metrics polling server. It polls all BMCs at the configured interval.
// This method blocks until the context is cancelled.
func (s *Server) Start(ctx context.Context) error {
	s.log.Info("Starting BMC metrics polling server", "interval", s.interval)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.pollAllBMCs(ctx)

	for {
		select {
		case <-ticker.C:
			s.pollAllBMCs(ctx)
		case <-ctx.Done():
			s.log.Info("Stopping metrics polling server")
			return nil
		}
	}
}

// pollAllBMCs lists all BMCs and polls each one for metrics and events.
func (s *Server) pollAllBMCs(ctx context.Context) {
	bmcList := &metalv1alpha1.BMCList{}
	if err := s.client.List(ctx, bmcList); err != nil {
		s.log.Error(err, "Failed to list BMCs for polling")
		return
	}

	if len(bmcList.Items) == 0 {
		s.log.V(2).Info("No BMCs to poll")
		return
	}

	s.log.V(1).Info("Polling BMCs for metrics", "count", len(bmcList.Items))

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 10)

	for i := range bmcList.Items {
		bmcObj := &bmcList.Items[i]

		if !bmcObj.DeletionTimestamp.IsZero() {
			continue
		}

		wg.Add(1)
		go func(bmc *metalv1alpha1.BMC) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			s.pollBMC(ctx, bmc)
		}(bmcObj)
	}

	wg.Wait()
	s.log.V(1).Info("Finished polling cycle")
}

// pollBMC polls a single BMC for metrics and events.
func (s *Server) pollBMC(ctx context.Context, bmcObj *metalv1alpha1.BMC) {
	log := s.log.WithValues("bmc", bmcObj.Name)

	bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, s.client, bmcObj, s.defaultProtocol, s.skipCertValidate, s.bmcOptions)
	if err != nil {
		log.V(2).Info("Failed to get BMC client for polling", "error", err.Error())
		return
	}
	defer bmcClient.Logout()

	if report, err := bmcClient.GetMetricReport(ctx); err != nil {
		log.V(2).Info("Failed to poll metrics from BMC", "error", err.Error())
	} else if len(report.MetricValues) > 0 {
		seReport := convertToMetricsReport(report)
		s.collector.UpdateFromMetricsReport(bmcObj.Name, seReport)
		log.V(2).Info("Updated metrics from poll", "metricCount", len(report.MetricValues))
	}

	if events, err := bmcClient.GetEventLog(ctx); err != nil {
		log.V(2).Info("Failed to poll events from BMC", "error", err.Error())
	} else if len(events) > 0 {
		seEvents := convertToEvents(events)
		eventData := EventData{Events: seEvents}
		s.collector.UpdateFromEvent(bmcObj.Name, eventData)
		log.V(2).Info("Updated events from poll", "eventCount", len(events))
	}
}

// convertToMetricsReport converts bmc.MetricsReport to serverevents.MetricsReport
func convertToMetricsReport(report bmc.MetricsReport) MetricsReport {
	metricValues := make([]MetricValue, len(report.MetricValues))
	for i, mv := range report.MetricValues {
		metricValues[i] = MetricValue{
			MetricID:        mv.MetricID,
			MetricProperty:  mv.MetricProperty,
			MetricValue:     mv.MetricValue,
			Units:           mv.Units,
			MetricValueKind: mv.MetricValueKind,
			Timestamp:       mv.Timestamp,
			Oem:             mv.Oem,
		}
	}
	return MetricsReport{
		ODataID:      report.ODataID,
		ODataType:    report.ODataType,
		ID:           report.ID,
		Name:         report.Name,
		MetricValues: metricValues,
	}
}

// convertToEvents converts []bmc.Event to []serverevents.Event
func convertToEvents(events []bmc.Event) []Event {
	seEvents := make([]Event, len(events))
	for i, e := range events {
		seEvents[i] = Event{
			EventID:           e.EventID,
			Message:           e.Message,
			Severity:          e.Severity,
			EventTimestamp:    e.EventTimestamp,
			OriginOfCondition: e.OriginOfCondition,
		}
	}
	return seEvents
}

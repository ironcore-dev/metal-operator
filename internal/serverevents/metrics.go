// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package serverevents

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

type MetricEntry struct {
	MetricID      string
	Value         float64
	Type          string
	Unit          string
	OriginContext string
	Source        string
	Timestamp     time.Time
}

// CriticalEventHandler is a callback function that handles critical events
type CriticalEventHandler func(ctx context.Context, bmcName string, event Event)

type RedfishEventCollector struct {
	lastReadings         map[string]MetricEntry
	alertCounts          map[EventKey]uint64
	mux                  sync.RWMutex
	sensorDesc           *prometheus.Desc
	alertDesc            *prometheus.Desc
	client               client.Client
	log                  logr.Logger
	criticalEventHandler CriticalEventHandler
}

type EventKey struct {
	Source    string
	Severity  string
	EventID   string
	Component string
}

// NewRedfishEventCollector initializes a new RedfishEventCollector and registers it with Prometheus.
func NewRedfishEventCollector() *RedfishEventCollector {
	c := &RedfishEventCollector{
		lastReadings: make(map[string]MetricEntry),
		alertCounts:  make(map[EventKey]uint64),
		sensorDesc: prometheus.NewDesc(
			"redfish_monitor_reading",
			"Latest value pushed via Redfish MetricReport event",
			[]string{"hostname", "metric_id", "type", "unit", "origin_context"},
			nil,
		),
		alertDesc: prometheus.NewDesc(
			"redfish_event_alert_total",
			"Total count of Redfish alerts/events received",
			[]string{"hostname", "severity", "message_id", "component"},
			nil,
		),
		log: logr.Discard(),
	}
	metrics.Registry.MustRegister(c)
	return c
}

// SetClient sets the Kubernetes client for the collector
func (c *RedfishEventCollector) SetClient(client client.Client) {
	c.mux.Lock()
	defer c.mux.Unlock()
	c.client = client
}

// SetLogger sets the logger for the collector
func (c *RedfishEventCollector) SetLogger(log logr.Logger) {
	c.mux.Lock()
	defer c.mux.Unlock()
	c.log = log
}

// SetCriticalEventHandler sets the handler for critical events
func (c *RedfishEventCollector) SetCriticalEventHandler(handler CriticalEventHandler) {
	c.mux.Lock()
	defer c.mux.Unlock()
	c.criticalEventHandler = handler
}

// UpdateFromMetricsReport processes incoming MetricReport events and updates the internal state.
func (c *RedfishEventCollector) UpdateFromMetricsReport(hostname string, report MetricsReport) {
	c.mux.Lock()
	defer c.mux.Unlock()

	for _, entry := range report.MetricsValues {
		unit := entry.Units
		if unit == "" {
			unit = "seconds"
		}
		mType := entry.MetricValueKind
		if mType == "" {
			// Fallback: Try to guess from the ID
			if strings.Contains(strings.ToLower(entry.MetricID), "temp") {
				mType = "Temperature"
			} else {
				mType = "Gauge"
			}
		}
		val, err := strconv.ParseFloat(entry.MetricValue, 64)
		if err != nil {
			continue
		}
		key := entry.MetricID + entry.MetricProperty
		c.lastReadings[key] = MetricEntry{
			Value:         val,
			Type:          mType,
			Unit:          unit,
			MetricID:      entry.MetricID,
			OriginContext: entry.MetricProperty,
			Source:        hostname,
			Timestamp:     time.Now(),
		}
	}
}

// UpdateFromEvent processes incoming Redfish events and updates the alert counts.
func (c *RedfishEventCollector) UpdateFromEvent(hostname string, data EventData) {
	c.mux.Lock()
	defer c.mux.Unlock()

	for _, event := range data.Events {
		// Determine the component from the URI (e.g., .../Sensors/Fan1 -> Fan1)
		component := "system"
		if event.OriginOfCondition != "" {
			parts := strings.Split(strings.TrimRight(event.OriginOfCondition, "/"), "/")
			component = parts[len(parts)-1]
		}
		event.OriginOfCondition = component
		key := EventKey{
			Source:    hostname,
			Severity:  event.Severity,
			EventID:   event.EventID,
			Component: component,
		}
		c.alertCounts[key]++

		// Handle critical events
		if strings.EqualFold(event.Severity, "Critical") {
			c.log.Info("Critical event received", "bmcName", hostname, "eventID", event.EventID, "component", component, "message", event.Message)
			if c.criticalEventHandler != nil {
				// Call the handler asynchronously to avoid blocking the HTTP handler
				go c.criticalEventHandler(context.Background(), hostname, event)
			}
		}
	}

}

// Describe and Collect implement the prometheus.Collector interface to expose metrics.
func (c *RedfishEventCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.sensorDesc
}

// Collect gathers the latest metrics and sends them to Prometheus.
func (c *RedfishEventCollector) Collect(ch chan<- prometheus.Metric) {
	c.mux.RLock()
	defer c.mux.RUnlock()

	for _, data := range c.lastReadings {
		if time.Since(data.Timestamp) > 10*time.Minute {
			continue
		}
		ch <- prometheus.MustNewConstMetric(
			c.sensorDesc,
			prometheus.GaugeValue,
			data.Value,
			data.Source,
			data.MetricID,
			data.Type,
			data.Unit,
			data.OriginContext,
		)
	}
	for key, count := range c.alertCounts {
		ch <- prometheus.MustNewConstMetric(
			c.alertDesc,
			prometheus.CounterValue,
			float64(count),
			key.Source,
			key.Severity,
			key.EventID,
			key.Component,
		)
	}
}

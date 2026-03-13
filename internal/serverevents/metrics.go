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

type AlertEntry struct {
	Count    uint64
	LastSeen time.Time
}

// CriticalEventHandler is a callback function that handles critical events
type CriticalEventHandler func(ctx context.Context, bmcName string, event Event)

type RedfishEventCollector struct {
	lastReadings         map[string]MetricEntry
	alertCounts          map[EventKey]AlertEntry
	mux                  sync.RWMutex
	sensorDesc           *prometheus.Desc
	alertDesc            *prometheus.Desc
	client               client.Client
	log                  logr.Logger
	criticalEventHandler CriticalEventHandler
	eventSem             chan struct{}
}

const (
	staleMetricTTL = 10 * time.Minute
	staleAlertTTL  = 24 * time.Hour
)

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
		alertCounts:  make(map[EventKey]AlertEntry),
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
		log:      logr.Discard(),
		eventSem: make(chan struct{}, 10),
	}
	metrics.Registry.MustRegister(c)
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			c.cleanupStaleData()
		}
	}()
	return c
}

// SetClient sets the Kubernetes client for the collector
func (c *RedfishEventCollector) SetClient(k8sClient client.Client) {
	c.mux.Lock()
	defer c.mux.Unlock()
	c.client = k8sClient
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
		key := hostname + ":" + entry.MetricID + ":" + entry.MetricProperty
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
		entry := c.alertCounts[key]
		entry.Count++
		entry.LastSeen = time.Now()
		c.alertCounts[key] = entry

		// Handle critical events
		if strings.EqualFold(event.Severity, "Critical") {
			c.log.Info("Critical event received", "bmcName", hostname, "eventID", event.EventID, "component", component, "message", event.Message)
			if c.criticalEventHandler != nil {
				// Call the handler asynchronously to avoid blocking the HTTP handler
				// Acquire semaphore before spawning goroutine to apply backpressure immediately
				go func(h string, e Event) {
					// Block until capacity is available - critical events must not be dropped
					c.eventSem <- struct{}{}
					defer func() { <-c.eventSem }()
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					c.criticalEventHandler(ctx, h, e)
				}(hostname, event)
			}
		}
	}

}

// Describe and Collect implement the prometheus.Collector interface to expose metrics.
func (c *RedfishEventCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.sensorDesc
	ch <- c.alertDesc
}

// Collect gathers the latest metrics and sends them to Prometheus.
func (c *RedfishEventCollector) Collect(ch chan<- prometheus.Metric) {
	c.mux.RLock()
	defer c.mux.RUnlock()

	for _, data := range c.lastReadings {
		if time.Since(data.Timestamp) > staleMetricTTL {
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
	for key, alert := range c.alertCounts {
		if time.Since(alert.LastSeen) > staleAlertTTL {
			continue
		}
		ch <- prometheus.MustNewConstMetric(
			c.alertDesc,
			prometheus.CounterValue,
			float64(alert.Count),
			key.Source,
			key.Severity,
			key.EventID,
			key.Component,
		)
	}
}

// cleanupStaleData removes stale entries from maps to prevent memory leaks
func (c *RedfishEventCollector) cleanupStaleData() {
	c.mux.Lock()
	defer c.mux.Unlock()

	now := time.Now()
	for key, entry := range c.lastReadings {
		if now.Sub(entry.Timestamp) > staleMetricTTL {
			delete(c.lastReadings, key)
		}
	}
	for key, entry := range c.alertCounts {
		if now.Sub(entry.LastSeen) > staleAlertTTL {
			delete(c.alertCounts, key)
		}
	}
}

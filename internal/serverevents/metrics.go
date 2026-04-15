// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package serverevents

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/prometheus/client_golang/prometheus"
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

type RedfishEventCollector struct {
	lastReadings       map[string]MetricEntry
	alertCounts        map[EventKey]uint64
	metricsSourceType  map[string]float64 // hostname -> source type (0=events, 1=polling)
	sensorPollCount    map[string]uint64  // hostname -> successful poll count
	sensorPollErrors   map[string]uint64  // hostname -> failed poll count
	mux                sync.RWMutex
	sensorDesc         *prometheus.Desc
	alertDesc          *prometheus.Desc
	metricsSourceDesc  *prometheus.Desc
	pollCountDesc      *prometheus.Desc
	pollErrorCountDesc *prometheus.Desc
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
		lastReadings:      make(map[string]MetricEntry),
		alertCounts:       make(map[EventKey]uint64),
		metricsSourceType: make(map[string]float64),
		sensorPollCount:   make(map[string]uint64),
		sensorPollErrors:  make(map[string]uint64),
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
		metricsSourceDesc: prometheus.NewDesc(
			"redfish_metrics_source_type",
			"Metrics source type for BMC (0=events, 1=polling)",
			[]string{"hostname"},
			nil,
		),
		pollCountDesc: prometheus.NewDesc(
			"redfish_sensor_poll_total",
			"Total count of successful sensor polls",
			[]string{"hostname"},
			nil,
		),
		pollErrorCountDesc: prometheus.NewDesc(
			"redfish_sensor_poll_errors_total",
			"Total count of failed sensor polls",
			[]string{"hostname"},
			nil,
		),
	}
	metrics.Registry.MustRegister(c)
	return c
}

// UpdateFromMetricsReport processes incoming MetricReport events and updates the internal state.
func (c *RedfishEventCollector) UpdateFromMetricsReport(hostname string, report MetricsReport) {
	c.mux.Lock()
	defer c.mux.Unlock()

	for _, entry := range report.MetricValues {
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

	// Mark this host as using events
	c.metricsSourceType[hostname] = 0.0
}

// UpdateFromEvent processes incoming Redfish events and updates the alert counts.
func (c *RedfishEventCollector) UpdateFromEvent(hostname string, data EventData) {
	c.mux.Lock()
	defer c.mux.Unlock()

	events := data.GetEvents() // Use new method to get events from either field
	for _, event := range events {
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
	}

}

// Describe and Collect implement the prometheus.Collector interface to expose metrics.
func (c *RedfishEventCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.sensorDesc
	ch <- c.alertDesc
	ch <- c.metricsSourceDesc
	ch <- c.pollCountDesc
	ch <- c.pollErrorCountDesc
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
	for hostname, sourceType := range c.metricsSourceType {
		ch <- prometheus.MustNewConstMetric(
			c.metricsSourceDesc,
			prometheus.GaugeValue,
			sourceType,
			hostname,
		)
	}
	for hostname, count := range c.sensorPollCount {
		ch <- prometheus.MustNewConstMetric(
			c.pollCountDesc,
			prometheus.CounterValue,
			float64(count),
			hostname,
		)
	}
	for hostname, count := range c.sensorPollErrors {
		ch <- prometheus.MustNewConstMetric(
			c.pollErrorCountDesc,
			prometheus.CounterValue,
			float64(count),
			hostname,
		)
	}
}

// GetLastUpdateTime returns the timestamp of the most recent metric update for the given hostname.
// Returns zero time if no metrics have been recorded for this hostname.
func (c *RedfishEventCollector) GetLastUpdateTime(hostname string) time.Time {
	c.mux.RLock()
	defer c.mux.RUnlock()

	var latestTime time.Time
	for key, entry := range c.lastReadings {
		if strings.HasPrefix(key, hostname) || entry.Source == hostname {
			if entry.Timestamp.After(latestTime) {
				latestTime = entry.Timestamp
			}
		}
	}
	return latestTime
}

// UpdateFromSensorPoll processes sensor data from polling and updates the internal state.
// This allows the collector to work with both event-driven and polling-based metric updates.
func (c *RedfishEventCollector) UpdateFromSensorPoll(hostname string, sensors []bmc.Sensor) {
	c.mux.Lock()
	defer c.mux.Unlock()

	now := time.Now()
	for _, sensor := range sensors {
		// Determine metric type based on the sensor context
		mType := "Gauge"
		if sensor.PhysicalContext != "" {
			if strings.Contains(strings.ToLower(sensor.PhysicalContext), "temp") {
				mType = "Temperature"
			} else if strings.Contains(strings.ToLower(sensor.PhysicalContext), "power") {
				mType = "Power"
			} else if strings.Contains(strings.ToLower(sensor.PhysicalContext), "voltage") {
				mType = "Voltage"
			}
		}

		// Use sensor ID as metric ID and name as origin context
		key := sensor.ID
		c.lastReadings[key] = MetricEntry{
			Value:         sensor.Reading,
			Type:          mType,
			Unit:          sensor.Units,
			MetricID:      sensor.ID,
			OriginContext: sensor.Name,
			Source:        hostname,
			Timestamp:     now,
		}
	}

	// Mark this host as using polling
	c.metricsSourceType[hostname] = 1.0
	c.sensorPollCount[hostname]++
}

// RecordPollError records a failed sensor poll attempt for the given hostname.
func (c *RedfishEventCollector) RecordPollError(hostname string) {
	c.mux.Lock()
	defer c.mux.Unlock()

	c.sensorPollErrors[hostname]++
}

// SetMetricsSourceType sets the metrics source type for a given hostname.
// sourceType should be 0 for events or 1 for polling.
func (c *RedfishEventCollector) SetMetricsSourceType(hostname string, sourceType float64) {
	c.mux.Lock()
	defer c.mux.Unlock()

	c.metricsSourceType[hostname] = sourceType
}

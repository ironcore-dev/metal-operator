// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package serverevents

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// LabelMapping maps a Kubernetes resource label key to a Prometheus label name.
type LabelMapping struct {
	K8sKey    string
	PromLabel string
}

// promLabelPattern is the set of valid Prometheus label name characters.
var promLabelPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// ParseLabelMappings parses a comma-separated list of "kubernetes-label-key=prometheus-label-name"
// pairs into a []LabelMapping.
//
// Format: "some.domain/key=prom_label,other.domain/key2=prom_label2"
//
// Rules:
//   - Empty string returns nil (no mappings, valid).
//   - Each token must contain exactly one "=".
//   - The Prometheus label name must match [a-zA-Z_][a-zA-Z0-9_]*.
//   - Whitespace is trimmed from both sides of each token and each part.
func ParseLabelMappings(s string) ([]LabelMapping, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	tokens := strings.Split(s, ",")
	mappings := make([]LabelMapping, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		parts := strings.SplitN(token, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid label mapping %q: must be 'kubernetes-label=prometheus-label'", token)
		}
		k8sKey := strings.TrimSpace(parts[0])
		promLabel := strings.TrimSpace(parts[1])
		if k8sKey == "" {
			return nil, fmt.Errorf("invalid label mapping %q: Kubernetes label key must not be empty", token)
		}
		if !promLabelPattern.MatchString(promLabel) {
			return nil, fmt.Errorf("invalid label mapping %q: Prometheus label name %q must match [a-zA-Z_][a-zA-Z0-9_]*", token, promLabel)
		}
		mappings = append(mappings, LabelMapping{K8sKey: k8sKey, PromLabel: promLabel})
	}
	return mappings, nil
}

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
	lastReadings map[string]MetricEntry
	alertCounts  map[EventKey]uint64
	mux          sync.RWMutex
	sensorDesc   *prometheus.Desc
	alertDesc    *prometheus.Desc

	k8sClient      client.Client
	bmcMappings    []LabelMapping
	serverMappings []LabelMapping
	allLabelCount  int
}

type EventKey struct {
	Source    string
	Severity  string
	EventID   string
	Component string
}

// NewRedfishEventCollector initializes a new RedfishEventCollector and registers it with Prometheus.
//
// bmcMappings and serverMappings define which Kubernetes resource labels are propagated to Redfish
// metrics as additional Prometheus label dimensions. Pass nil for either to disable enrichment from
// that resource. The k8sClient is used to look up the resources at runtime; pass nil to disable all
// enrichment (e.g. in tests or standalone tooling).
func NewRedfishEventCollector(k8sClient client.Client, bmcMappings, serverMappings []LabelMapping) *RedfishEventCollector {
	allLabelCount := len(bmcMappings) + len(serverMappings)
	allLabels := make([]string, 0, allLabelCount)
	for _, m := range bmcMappings {
		allLabels = append(allLabels, m.PromLabel)
	}
	for _, m := range serverMappings {
		allLabels = append(allLabels, m.PromLabel)
	}
	c := &RedfishEventCollector{
		lastReadings:   make(map[string]MetricEntry),
		alertCounts:    make(map[EventKey]uint64),
		k8sClient:      k8sClient,
		bmcMappings:    bmcMappings,
		serverMappings: serverMappings,
		allLabelCount:  allLabelCount,
		sensorDesc: prometheus.NewDesc(
			"redfish_monitor_reading",
			"Latest value pushed via Redfish MetricReport event",
			append([]string{"hostname", "metric_id", "type", "unit", "origin_context"}, allLabels...),
			nil,
		),
		alertDesc: prometheus.NewDesc(
			"redfish_event_alert_total",
			"Total count of Redfish alerts/events received",
			append([]string{"hostname", "severity", "message_id", "component"}, allLabels...),
			nil,
		),
	}
	metrics.Registry.MustRegister(c)
	return c
}

// getLabels returns the enrichment label values for the given hostname by reading from the
// controller-runtime informer cache. Reads are local in-memory operations — the informer cache
// is watch-based and always reflects the current cluster state without making API server calls.
// Returns empty strings for any label that cannot be resolved.
func (c *RedfishEventCollector) getLabels(hostname string) []string {
	vals := make([]string, c.allLabelCount)
	if c.k8sClient == nil || c.allLabelCount == 0 {
		return vals
	}
	ctx := context.Background()

	// --- BMC labels ---
	if len(c.bmcMappings) > 0 {
		bmc := &metalv1alpha1.BMC{}
		if err := c.k8sClient.Get(ctx, client.ObjectKey{Name: hostname}, bmc); err == nil {
			for i, m := range c.bmcMappings {
				vals[i] = bmc.Labels[m.K8sKey]
			}
		}
	}

	// --- Server labels (looked up via spec.bmcRef.name field index) ---
	if len(c.serverMappings) > 0 {
		serverList := &metalv1alpha1.ServerList{}
		if err := c.k8sClient.List(ctx, serverList, client.MatchingFields{"spec.bmcRef.name": hostname}); err == nil && len(serverList.Items) == 1 {
			for i, m := range c.serverMappings {
				vals[len(c.bmcMappings)+i] = serverList.Items[0].Labels[m.K8sKey]
			}
		}
	}
	return vals
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
}

// UpdateFromEvent processes incoming Redfish events and updates the alert counts.
func (c *RedfishEventCollector) UpdateFromEvent(hostname string, data EventData) {
	c.mux.Lock()
	defer c.mux.Unlock()

	events := data.GetEvents()
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
}

// Collect gathers the latest metrics and sends them to Prometheus.
func (c *RedfishEventCollector) Collect(ch chan<- prometheus.Metric) {
	c.mux.RLock()
	defer c.mux.RUnlock()

	for _, data := range c.lastReadings {
		if time.Since(data.Timestamp) > 10*time.Minute {
			continue
		}
		labelValues := append(
			[]string{data.Source, data.MetricID, data.Type, data.Unit, data.OriginContext},
			c.getLabels(data.Source)...,
		)
		ch <- prometheus.MustNewConstMetric(
			c.sensorDesc,
			prometheus.GaugeValue,
			data.Value,
			labelValues...,
		)
	}
	for key, count := range c.alertCounts {
		labelValues := append(
			[]string{key.Source, key.Severity, key.EventID, key.Component},
			c.getLabels(key.Source)...,
		)
		ch <- prometheus.MustNewConstMetric(
			c.alertDesc,
			prometheus.CounterValue,
			float64(count),
			labelValues...,
		)
	}
}

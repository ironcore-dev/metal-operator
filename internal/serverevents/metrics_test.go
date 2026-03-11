// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package serverevents

import (
	"testing"
	"time"
)

func TestCleanupStaleDataRemovesExpiredAlertsAndMetrics(t *testing.T) {
	now := time.Now()
	alertKeyFresh := EventKey{Source: "node-1", Severity: "Warning", EventID: "FAN001", Component: "Fan1"}
	alertKeyStale := EventKey{Source: "node-2", Severity: "Critical", EventID: "TEMP001", Component: "CPU1"}

	collector := &RedfishEventCollector{
		lastReadings: map[string]MetricEntry{
			"fresh": {Timestamp: now.Add(-staleMetricTTL + time.Minute)},
			"stale": {Timestamp: now.Add(-staleMetricTTL - time.Minute)},
		},
		alertCounts: map[EventKey]AlertEntry{
			alertKeyFresh: {Count: 2, LastSeen: now.Add(-staleAlertTTL + time.Hour)},
			alertKeyStale: {Count: 3, LastSeen: now.Add(-staleAlertTTL - time.Hour)},
		},
	}

	collector.cleanupStaleData()

	if _, ok := collector.lastReadings["fresh"]; !ok {
		t.Fatalf("expected fresh metric reading to remain")
	}
	if _, ok := collector.lastReadings["stale"]; ok {
		t.Fatalf("expected stale metric reading to be removed")
	}
	if _, ok := collector.alertCounts[alertKeyFresh]; !ok {
		t.Fatalf("expected fresh alert to remain")
	}
	if _, ok := collector.alertCounts[alertKeyStale]; ok {
		t.Fatalf("expected stale alert to be removed")
	}
}

func TestUpdateFromEventRefreshesAlertLastSeenAndCount(t *testing.T) {
	staleTime := time.Now().Add(-staleAlertTTL - time.Hour)
	key := EventKey{Source: "node-1", Severity: "Critical", EventID: "TEMP001", Component: "CPU1"}

	collector := &RedfishEventCollector{
		lastReadings: map[string]MetricEntry{},
		alertCounts: map[EventKey]AlertEntry{
			key: {Count: 4, LastSeen: staleTime},
		},
	}

	collector.UpdateFromEvent("node-1", EventData{
		Events: []Event{{
			EventID:           "TEMP001",
			Severity:          "Critical",
			OriginOfCondition: "/redfish/v1/Chassis/1/Thermal#/Temperatures/CPU1",
		}},
	})

	entry, ok := collector.alertCounts[key]
	if !ok {
		t.Fatalf("expected alert entry to exist after update")
	}
	if entry.Count != 5 {
		t.Fatalf("expected alert count to increment to 5, got %d", entry.Count)
	}
	if !entry.LastSeen.After(staleTime) {
		t.Fatalf("expected alert lastSeen to be refreshed")
	}
}

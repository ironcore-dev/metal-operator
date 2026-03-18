// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package serverevents

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
)

func TestHostnameFromPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		prefix   string
		wantHost string
		wantOK   bool
	}{
		{name: "valid metrics path", path: "/serverevents/metricsreport/node-1", prefix: "/serverevents/metricsreport/", wantHost: "node-1", wantOK: true},
		{name: "valid alerts path", path: "/serverevents/alerts/node-2", prefix: "/serverevents/alerts/", wantHost: "node-2", wantOK: true},
		{name: "missing hostname", path: "/serverevents/metricsreport/", prefix: "/serverevents/metricsreport/", wantOK: false},
		{name: "missing trailing slash", path: "/serverevents/metricsreport", prefix: "/serverevents/metricsreport/", wantOK: false},
		{name: "extra path segment", path: "/serverevents/metricsreport/node-1/extra", prefix: "/serverevents/metricsreport/", wantOK: false},
		{name: "wrong prefix", path: "/other/node-1", prefix: "/serverevents/metricsreport/", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHost, gotOK := hostnameFromPath(tt.path, tt.prefix)
			if gotOK != tt.wantOK {
				t.Fatalf("hostnameFromPath() ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotHost != tt.wantHost {
				t.Fatalf("hostnameFromPath() host = %q, want %q", gotHost, tt.wantHost)
			}
		})
	}
}

func TestMetricsReportHandlerRejectsMissingHostname(t *testing.T) {
	t.Parallel()

	server := &Server{log: logr.Discard(), collector: &RedfishEventCollector{}}
	req := httptest.NewRequest(http.MethodPost, "/serverevents/metricsreport/", strings.NewReader(`{"MetricsValues":[]}`))
	rec := httptest.NewRecorder()

	server.metricsreportHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("metricsreportHandler() status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAlertHandlerRejectsExtraPathSegment(t *testing.T) {
	t.Parallel()

	server := &Server{log: logr.Discard(), collector: &RedfishEventCollector{}}
	req := httptest.NewRequest(http.MethodPost, "/serverevents/alerts/node-1/extra", strings.NewReader(`{"Alerts":[]}`))
	rec := httptest.NewRecorder()

	server.alertHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("alertHandler() status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

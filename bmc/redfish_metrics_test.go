// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stmcginnis/gofish"
)

var _ = Describe("Redfish Metrics Methods", func() {
	var (
		server    *httptest.Server
		bmcClient *RedfishBaseBMC
		ctx       context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		if bmcClient != nil && bmcClient.client != nil {
			bmcClient.Logout()
		}
	})

	Describe("GetMetricReport", func() {
		Context("when TelemetryService is available", func() {
			BeforeEach(func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					switch r.URL.Path {
					case "/redfish/v1/":
						json.NewEncoder(w).Encode(map[string]interface{}{
							"@odata.id":         "/redfish/v1/",
							"TelemetryService": map[string]interface{}{"@odata.id": "/redfish/v1/TelemetryService"},
						})
					case "/redfish/v1/TelemetryService":
						json.NewEncoder(w).Encode(map[string]interface{}{
							"@odata.id":     "/redfish/v1/TelemetryService",
							"MetricReports": map[string]interface{}{"@odata.id": "/redfish/v1/TelemetryService/MetricReports"},
						})
					case "/redfish/v1/TelemetryService/MetricReports":
						json.NewEncoder(w).Encode(map[string]interface{}{
							"@odata.id": "/redfish/v1/TelemetryService/MetricReports",
							"Members": []interface{}{
								map[string]interface{}{"@odata.id": "/redfish/v1/TelemetryService/MetricReports/Report1"},
							},
						})
					case "/redfish/v1/TelemetryService/MetricReports/Report1":
						json.NewEncoder(w).Encode(map[string]interface{}{
							"@odata.id":   "/redfish/v1/TelemetryService/MetricReports/Report1",
							"@odata.type": "#MetricReport.v1_0_0.MetricReport",
							"Id":          "Report1",
							"Name":        "Test Metric Report",
							"MetricValues": []interface{}{
								map[string]interface{}{
									"MetricId":       "Temp1",
									"MetricProperty": "/Temperature",
									"MetricValue":    "42.5",
									"Timestamp":      "2024-01-01T00:00:00Z",
								},
								map[string]interface{}{
									"MetricId":       "Fan1",
									"MetricProperty": "/FanSpeed",
									"MetricValue":    "3000",
									"Timestamp":      "2024-01-01T00:00:00Z",
								},
							},
						})
					default:
						w.WriteHeader(http.StatusNotFound)
					}
				}))

				config := gofish.ClientConfig{
					Endpoint: server.URL,
					HTTPClient: &http.Client{
						Transport: &http.Transport{},
					},
					BasicAuth: true,
					Username:  "admin",
					Password:  "password",
				}

				client, err := gofish.Connect(config)
				Expect(err).ToNot(HaveOccurred())

				bmcClient = &RedfishBaseBMC{
					client: client,
				}
			})

			It("should retrieve metrics from TelemetryService", func() {
				report, err := bmcClient.GetMetricReport(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(report.ID).To(Equal("Report1"))
				Expect(report.Name).To(Equal("Test Metric Report"))
				Expect(report.MetricValues).To(HaveLen(2))
				Expect(report.MetricValues[0].MetricID).To(Equal("Metric0"))
				Expect(report.MetricValues[0].MetricValue).To(ContainSubstring("Temp1"))
			})
		})

		Context("when TelemetryService is not available", func() {
			BeforeEach(func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					switch r.URL.Path {
					case "/redfish/v1/":
						json.NewEncoder(w).Encode(map[string]interface{}{
							"@odata.id": "/redfish/v1/",
						})
					default:
						w.WriteHeader(http.StatusNotFound)
					}
				}))

				config := gofish.ClientConfig{
					Endpoint: server.URL,
					HTTPClient: &http.Client{
						Transport: &http.Transport{},
					},
					BasicAuth: true,
					Username:  "admin",
					Password:  "password",
				}

				client, err := gofish.Connect(config)
				Expect(err).ToNot(HaveOccurred())

				bmcClient = &RedfishBaseBMC{
					client: client,
				}
			})

			It("should return empty report", func() {
				report, err := bmcClient.GetMetricReport(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(report.ID).To(Equal("EmptyMetrics"))
				Expect(report.Name).To(Equal("No TelemetryService available"))
				Expect(report.MetricValues).To(BeEmpty())
			})
		})
	})

	Describe("GetEventLog", func() {
		Context("when LogServices are available", func() {
			BeforeEach(func() {
				recentTime := time.Now().Add(-5 * time.Minute).Format(time.RFC3339)
				veryRecentTime := time.Now().Add(-1 * time.Minute).Format(time.RFC3339)

				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					switch r.URL.Path {
					case "/redfish/v1/":
						json.NewEncoder(w).Encode(map[string]interface{}{
							"@odata.id": "/redfish/v1/",
							"Systems":   map[string]interface{}{"@odata.id": "/redfish/v1/Systems"},
						})
					case "/redfish/v1/Systems":
						json.NewEncoder(w).Encode(map[string]interface{}{
							"@odata.id": "/redfish/v1/Systems",
							"Members": []interface{}{
								map[string]interface{}{"@odata.id": "/redfish/v1/Systems/1"},
							},
						})
					case "/redfish/v1/Systems/1":
						json.NewEncoder(w).Encode(map[string]interface{}{
							"@odata.id":   "/redfish/v1/Systems/1",
							"LogServices": map[string]interface{}{"@odata.id": "/redfish/v1/Systems/1/LogServices"},
						})
					case "/redfish/v1/Systems/1/LogServices":
						json.NewEncoder(w).Encode(map[string]interface{}{
							"@odata.id": "/redfish/v1/Systems/1/LogServices",
							"Members": []interface{}{
								map[string]interface{}{"@odata.id": "/redfish/v1/Systems/1/LogServices/SEL"},
							},
						})
					case "/redfish/v1/Systems/1/LogServices/SEL":
						json.NewEncoder(w).Encode(map[string]interface{}{
							"@odata.id": "/redfish/v1/Systems/1/LogServices/SEL",
							"Entries":   map[string]interface{}{"@odata.id": "/redfish/v1/Systems/1/LogServices/SEL/Entries"},
						})
					case "/redfish/v1/Systems/1/LogServices/SEL/Entries":
						json.NewEncoder(w).Encode(map[string]interface{}{
							"@odata.id": "/redfish/v1/Systems/1/LogServices/SEL/Entries",
							"Members": []interface{}{
								map[string]interface{}{"@odata.id": "/redfish/v1/Systems/1/LogServices/SEL/Entries/1"},
								map[string]interface{}{"@odata.id": "/redfish/v1/Systems/1/LogServices/SEL/Entries/2"},
							},
						})
					case "/redfish/v1/Systems/1/LogServices/SEL/Entries/1":
						json.NewEncoder(w).Encode(map[string]interface{}{
							"@odata.id":   "/redfish/v1/Systems/1/LogServices/SEL/Entries/1",
							"Id":          "1",
							"Message":     "Fan 1 speed is below threshold",
							"Severity":    "Warning",
							"Created":     recentTime,
							"EntryType":   "Event",
							"MessageId":   "Fan.1.Warning",
							"MessageArgs": []string{"Fan1"},
						})
					case "/redfish/v1/Systems/1/LogServices/SEL/Entries/2":
						json.NewEncoder(w).Encode(map[string]interface{}{
							"@odata.id":   "/redfish/v1/Systems/1/LogServices/SEL/Entries/2",
							"Id":          "2",
							"Message":     "Temperature sensor reading is critical",
							"Severity":    "Critical",
							"Created":     veryRecentTime,
							"EntryType":   "Event",
							"MessageId":   "Temp.1.Critical",
							"MessageArgs": []string{"Temp1"},
						})
					default:
						w.WriteHeader(http.StatusNotFound)
					}
				}))

				config := gofish.ClientConfig{
					Endpoint: server.URL,
					HTTPClient: &http.Client{
						Transport: &http.Transport{},
					},
					BasicAuth: true,
					Username:  "admin",
					Password:  "password",
				}

				client, err := gofish.Connect(config)
				Expect(err).ToNot(HaveOccurred())

				bmcClient = &RedfishBaseBMC{
					client: client,
				}
			})

			It("should retrieve events from LogServices", func() {
				events, err := bmcClient.GetEventLog(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(events).To(HaveLen(2))

				eventIDs := []string{events[0].EventID, events[1].EventID}
				Expect(eventIDs).To(ContainElements("1", "2"))

				var event1, event2 *Event
				for i := range events {
					if events[i].EventID == "1" {
						event1 = &events[i]
					} else if events[i].EventID == "2" {
						event2 = &events[i]
					}
				}

				Expect(event1).ToNot(BeNil())
				Expect(event1.Message).To(Equal("Fan 1 speed is below threshold"))
				Expect(event1.Severity).To(Equal("Warning"))

				Expect(event2).ToNot(BeNil())
				Expect(event2.Message).To(Equal("Temperature sensor reading is critical"))
				Expect(event2.Severity).To(Equal("Critical"))
			})
		})

		Context("when no systems are available", func() {
			BeforeEach(func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					switch r.URL.Path {
					case "/redfish/v1/":
						json.NewEncoder(w).Encode(map[string]interface{}{
							"@odata.id": "/redfish/v1/",
							"Systems":   map[string]interface{}{"@odata.id": "/redfish/v1/Systems"},
						})
					case "/redfish/v1/Systems":
						json.NewEncoder(w).Encode(map[string]interface{}{
							"@odata.id": "/redfish/v1/Systems",
							"Members":   []interface{}{},
						})
					default:
						w.WriteHeader(http.StatusNotFound)
					}
				}))

				config := gofish.ClientConfig{
					Endpoint: server.URL,
					HTTPClient: &http.Client{
						Transport: &http.Transport{},
					},
					BasicAuth: true,
					Username:  "admin",
					Password:  "password",
				}

				client, err := gofish.Connect(config)
				Expect(err).ToNot(HaveOccurred())

				bmcClient = &RedfishBaseBMC{
					client: client,
				}
			})

			It("should return error for no systems", func() {
				_, err := bmcClient.GetEventLog(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no systems found"))
			})
		})
	})
})

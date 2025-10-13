// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package serverevents

type MetricsReport struct {
	Data Data `json:"data"`
}

type Data struct {
	MetricsValues []MetricsValue `json:"MetricsValues"`
}
type MetricsValue struct {
	MetricId       string      `json:"MetricId"`
	MetricProperty string      `json:"MetricProperty"`
	MetricValue    string      `json:"MetricValue"`
	Timestamp      string      `json:"Timestamp"`
	Oem            interface{} `json:"Oem"`
}

type EventData struct {
	Events []Event `json:"Alerts"`
	Name   string  `json:"Name"`
}

type Event struct {
	EventID        string `json:"EventId"`
	Message        string `json:"Message"`
	Severity       string `json:"Severity"`
	EventTimestamp string `json:"EventTimestamp"`
}

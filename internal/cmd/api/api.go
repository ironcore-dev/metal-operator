// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package api

// ServerInfo represents information about a server in the data center.
type ServerInfo struct {
	Name         string `json:"name"`
	Rack         string `json:"rack"`
	HeightUnit   int    `json:"heightUnit"`
	Power        string `json:"power"`
	IndicatorLED string `json:"indicatorLED"`
	State        string `json:"state"`
}

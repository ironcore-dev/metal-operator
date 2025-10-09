// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package registry

type MemoryDevice struct {
	SizeBytes             int64  `json:"size"`
	DeviceSet             string `json:"deviceSet"`
	DeviceLocator         string `json:"deviceLocator"`
	BankLocator           string `json:"bankLocator"`
	MemoryType            string `json:"memoryType"`
	Speed                 string `json:"speed"`
	Vendor                string `json:"vendor"`
	SerialNumber          string `json:"serialNumber"`
	AssetTag              string `json:"assetTag"`
	PartNumber            string `json:"partNumber"`
	ConfiguredMemorySpeed string `json:"configuredMemorySpeed"`
	MinimumVoltage        string `json:"minimumVoltage"`
	MaximumVoltage        string `json:"maximumVoltage"`
	ConfiguredVoltage     string `json:"configuredVoltage"`
}

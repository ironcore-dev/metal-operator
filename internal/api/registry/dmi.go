// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package registry

type DMI struct {
	BIOSInformation   BIOSInformation   `json:"biosInformation"`
	SystemInformation ServerInformation `json:"systemInformation"`
	BoardInformation  BoardInformation  `json:"boardInformation"`
}

type BIOSInformation struct {
	Vendor  string `json:"vendor"`
	Version string `json:"version"`
	Date    string `json:"date"`
}

type ServerInformation struct {
	Manufacturer string `json:"manufacturer"`
	ProductName  string `json:"productName"`
	Version      string `json:"version"`
	SerialNumber string `json:"serialNumber"`
	UUID         string `json:"uuid"`
	SKUNumber    string `json:"skuNumber"`
	Family       string `json:"family"`
}

type BoardInformation struct {
	Manufacturer string `json:"manufacturer"`
	Product      string `json:"product"`
	Version      string `json:"version"`
	SerialNumber string `json:"serialNumber"`
	AssetTag     string `json:"assetTag"`
}

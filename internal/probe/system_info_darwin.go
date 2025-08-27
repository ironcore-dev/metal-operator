// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

//go:build darwin

package probe

import (
	"github.com/ironcore-dev/metal-operator/internal/api/registry"
	"log"
)

// collectSystemInfoData is the implementation for Darwin.
// It returns hardcoded mock data, allowing the application to run
// for local development and testing without causing a panic.
func collectSystemInfoData() (registry.DMI, error) {
	log.Println("Running on Darwin, providing mock system info for testing.")
	dmi := registry.DMI{
		BIOSInformation: registry.BIOSInformation{
			Vendor:  "MockVendor",
			Version: "D.123",
			Date:    "2023-10-27",
		},
		SystemInformation: registry.ServerInformation{
			Manufacturer: "Apple Inc.",
			ProductName:  "MacBook Pro (Mock)",
			Version:      "1.0",
			SerialNumber: "MOCK-SERIAL-12345",
			UUID:         "00000000-0000-0000-0000-000000000001",
			SKUNumber:    "MOCK-SKU-67890",
			Family:       "MacBookPro",
		},
		BoardInformation: registry.BoardInformation{
			Manufacturer: "Apple Inc.",
			Product:      "Mac-F221BEC8",
			Version:      "1.0",
			SerialNumber: "MOCK-BOARD-SERIAL",
			AssetTag:     "MOCK-ASSET-TAG",
		},
	}
	return dmi, nil
}

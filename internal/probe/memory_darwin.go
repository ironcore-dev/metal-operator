// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"github.com/ironcore-dev/metal-operator/internal/api/registry"
)

func collectMemoryInfoData() ([]registry.MemoryDevice, error) {
	mem := make([]registry.MemoryDevice, 0)

	mem = append(mem, registry.MemoryDevice{
		SizeBytes:             int64(16 * 1024 * 1024 * 1024), // Example: 16 GB
		DeviceSet:             "0",
		DeviceLocator:         "DIMM0",
		BankLocator:           "BANK0",
		MemoryType:            "DDR4",
		Speed:                 "2400 MHz",
		Vendor:                "ExampleVendor",
		SerialNumber:          "1234567890",
		AssetTag:              "AssetTag123",
		PartNumber:            "Part1234",
		ConfiguredMemorySpeed: "2400 MHz",
		MinimumVoltage:        "1.2 V",
		MaximumVoltage:        "1.2 V",
		ConfiguredVoltage:     "1.2 V",
	})
	return mem, nil
}

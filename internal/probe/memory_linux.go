// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"github.com/ironcore-dev/metal-operator/internal/api/registry"
	"github.com/siderolabs/go-smbios/smbios"
)

func collectMemoryInfoData() ([]registry.MemoryDevice, error) {
	sm, err := smbios.New()
	if err != nil {
		return []registry.MemoryDevice{}, err
	}

	mem := make([]registry.MemoryDevice, 0)

	for _, m := range sm.MemoryDevices {
		if m.Size == 0 {
			continue
		}
		mem = append(mem, registry.MemoryDevice{
			SizeBytes:             int64(m.Size.Megabytes() * 1024 * 1024),
			DeviceSet:             m.DeviceSet,
			DeviceLocator:         m.DeviceLocator,
			BankLocator:           m.BankLocator,
			MemoryType:            m.MemoryType.String(),
			Speed:                 m.Speed.String(),
			Vendor:                m.Manufacturer,
			SerialNumber:          m.SerialNumber,
			AssetTag:              m.AssetTag,
			PartNumber:            m.PartNumber,
			ConfiguredMemorySpeed: m.ConfiguredMemorySpeed.String(),
			MinimumVoltage:        m.MinimumVoltage.String(),
			MaximumVoltage:        m.MaximumVoltage.String(),
			ConfiguredVoltage:     m.ConfiguredVoltage.String(),
		})
	}
	return mem, nil
}

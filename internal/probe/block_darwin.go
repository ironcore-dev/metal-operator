// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"github.com/ironcore-dev/metal-operator/internal/api/registry"
)

func collectStorageInfoData() ([]registry.BlockDevice, error) {
	blockDevices := make([]registry.BlockDevice, 0)
	blockDevices = append(blockDevices, registry.BlockDevice{
		Path:              "/dev/disk0",
		Name:              "disk0",
		Rotational:        false,
		Removable:         false,
		ReadOnly:          false,
		Vendor:            "Foo",
		Model:             "Bar",
		Serial:            "1234567890",
		WWID:              "0987654321",
		PhysicalBlockSize: 4096,
		LogicalBlockSize:  4096,
		HWSectorSize:      512,
		SizeBytes:         512000000000,
		NUMANodeID:        0,
	})
	return blockDevices, nil
}

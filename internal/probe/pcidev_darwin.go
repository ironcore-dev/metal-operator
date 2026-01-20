// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"github.com/ironcore-dev/metal-operator/internal/api/registry"
)

func collectPCIDevicesInfoData() ([]registry.PCIDevice, error) {
	return []registry.PCIDevice{
		{
			Address:    "0000:00:00.0",
			Vendor:     "FooVendor",
			VendorID:   "1234",
			Product:    "BarProduct",
			ProductID:  "5678",
			NumaNodeID: 0,
		},
	}, nil
}

// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"github.com/ironcore-dev/metal-operator/internal/api/registry"
)

func collectNICInfoData() ([]registry.NIC, error) {
	var nics []registry.NIC

	nics = append(nics, registry.NIC{
		Name:            "foo",
		MAC:             "00:11:22:33:44:55",
		PCIAddress:      "0000:00:00.0",
		Speed:           "1000",
		LinkModes:       []string{"1000baseT/Full"},
		SupportedPorts:  []string{"TP"},
		FirmwareVersion: "1.0.0",
	})
	return nics, nil
}

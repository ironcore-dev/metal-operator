// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

// HPERedfishBMC is the HPE-specific implementation of the BMC interface.
// It embeds RedfishBaseBMC and inherits all standard Redfish methods.
// HPE-specific overrides (BMC attributes, firmware upgrades) are handled
// via the OEM manager delegation in the base struct.
type HPERedfishBMC struct {
	*RedfishBaseBMC
}

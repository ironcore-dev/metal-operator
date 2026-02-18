// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

// LenovoRedfishBMC is the Lenovo-specific implementation of the BMC interface.
// It embeds RedfishBaseBMC and inherits all standard Redfish methods.
// Lenovo-specific overrides (BMC attributes, firmware upgrades) are handled
// via the OEM manager delegation in the base struct.
type LenovoRedfishBMC struct {
	*RedfishBaseBMC
}

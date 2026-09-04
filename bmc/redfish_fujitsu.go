// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

// FujitsuRedfishBMC is the Fujitsu-specific implementation of the BMC interface.
// Fujitsu iRMC exposes a standard Redfish API, so all operations delegate
// to RedfishBaseBMC. Vendor-specific overrides can be added here as needed.
type FujitsuRedfishBMC struct {
	*RedfishBaseBMC
}

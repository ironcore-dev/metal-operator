// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package registry

type CPUInfo struct {
	ID                   int      `json:"id"`
	TotalCores           uint32   `json:"totalCores"`
	TotalHardwareThreads uint32   `json:"totalHardwareThreads"`
	Vendor               string   `json:"vendor"`
	Model                string   `json:"model"`
	Capabilities         []string `json:"capabilities"`
}

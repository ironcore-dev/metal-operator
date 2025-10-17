// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"fmt"

	"github.com/ironcore-dev/metal-operator/internal/api/registry"
	"github.com/jaypipes/ghw"
)

func collectCPUInfoData() ([]registry.CPUInfo, error) {
	cpuInfos := make([]registry.CPUInfo, 0)
	cpuInfo, err := ghw.CPU()
	if err != nil {
		return cpuInfos, fmt.Errorf("failed to get CPU info: %w", err)
	}
	for _, processor := range cpuInfo.Processors {
		cpuInfos = append(cpuInfos, registry.CPUInfo{
			ID:                   processor.ID,
			TotalCores:           processor.TotalCores,
			TotalHardwareThreads: processor.TotalHardwareThreads,
			Vendor:               processor.Vendor,
			Model:                processor.Model,
			Capabilities:         processor.Capabilities,
		})
	}
	return cpuInfos, nil
}

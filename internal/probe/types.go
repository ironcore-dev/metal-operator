// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"time"

	"github.com/go-logr/logr"
)

// DiskCleaningMode defines the possible disk cleaning modes for the metalprobe.
type DiskCleaningMode string

const (
	// DiskCleaningModeQuick performs quick disk cleaning by wiping partition tables
	// and filesystem signatures using wipefs.
	DiskCleaningModeQuick DiskCleaningMode = "quick"

	// DiskCleaningModeSecure performs secure disk cleaning by wiping the entire disk.
	// Uses blkdiscard for SSDs or dd for HDDs.
	DiskCleaningModeSecure DiskCleaningMode = "secure"
)

// AgentConfig holds configuration for creating a new Agent.
type AgentConfig struct {
	Logger                logr.Logger
	SystemUUID            string
	RegistryURL           string
	Duration              time.Duration
	RegistryClientTimeout time.Duration
	LLDPSyncInterval      time.Duration
	LLDPSyncDuration      time.Duration
	DiskCleaningMode      DiskCleaningMode
}

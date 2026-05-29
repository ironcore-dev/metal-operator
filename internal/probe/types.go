// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe

// DiskCleaningMode defines the possible disk cleaning modes for the metalprobe.
type DiskCleaningMode string

const (
	// DiskCleaningModeQuick performs quick disk cleaning by wiping partition tables
	// and filesystem signatures at the beginning and end of each disk.
	DiskCleaningModeQuick DiskCleaningMode = "quick"

	// DiskCleaningModeSecure performs secure disk cleaning by wiping the entire disk.
	// Uses blkdiscard for SSDs or shred/dd for HDDs.
	DiskCleaningModeSecure DiskCleaningMode = "secure"
)

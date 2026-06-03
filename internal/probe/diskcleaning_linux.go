// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package probe

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/jaypipes/ghw"
	"golang.org/x/sync/errgroup"
)

const diskCleaningMarkerFile = "/var/run/metal-operator/disk-cleaning-complete"

// devicePathRegex validates that device paths match expected format.
// Updated to support multipath/RAID devices (e.g., /dev/mapper/mpatha, /dev/cciss/c0d0)
var devicePathRegex = regexp.MustCompile(`^/dev/[a-zA-Z0-9\-_/]+$`)

// wasDiskCleaningCompleted checks if disk cleaning was already completed.
func wasDiskCleaningCompleted() bool {
	_, err := os.Stat(diskCleaningMarkerFile)
	return err == nil
}

// markDiskCleaningCompleted creates a marker file to indicate disk cleaning was completed.
func markDiskCleaningCompleted() error {
	if err := os.MkdirAll(filepath.Dir(diskCleaningMarkerFile), 0755); err != nil {
		return fmt.Errorf("failed to create marker directory: %w", err)
	}
	content := fmt.Sprintf("Disk cleaning completed at %s\n", time.Now().Format(time.RFC3339))
	if err := os.WriteFile(diskCleaningMarkerFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write marker file: %w", err)
	}
	return nil
}

// validateDevicePath ensures a device path is safe to use in shell commands.
func validateDevicePath(devicePath string) error {
	if !devicePathRegex.MatchString(devicePath) {
		return fmt.Errorf("invalid device path format: %s", devicePath)
	}

	// Additional safety: ensure it's a block device
	fi, err := os.Stat(devicePath)
	if err != nil {
		return fmt.Errorf("device path does not exist: %w", err)
	}

	if fi.Mode()&os.ModeDevice == 0 {
		return fmt.Errorf("path is not a device: %s", devicePath)
	}
	if fi.Mode()&os.ModeCharDevice != 0 {
		return fmt.Errorf("path is a character device, not a block device: %s", devicePath)
	}

	return nil
}

// cleanDisks performs disk cleaning based on the specified mode concurrently.
func cleanDisks(ctx context.Context, mode DiskCleaningMode) error {
	log := logr.FromContextOrDiscard(ctx)

	// Check if disk cleaning was already completed
	if wasDiskCleaningCompleted() {
		log.Info("Disk cleaning already completed, skipping")
		return nil
	}

	log.Info("Starting concurrent disk cleaning", "mode", mode)

	// Get all block devices
	blockStorage, err := ghw.Block()
	if err != nil {
		return fmt.Errorf("failed to enumerate block devices: %w", err)
	}

	if len(blockStorage.Disks) == 0 {
		log.Info("No disks found to clean")
		return nil
	}

	// Filter eligible disks
	var eligibleDisks []*ghw.Disk
	for _, disk := range blockStorage.Disks {
		ro, err := isReadOnly(disk.Name)
		if err != nil {
			log.Error(err, "Failed to check read-only status, skipping disk", "disk", disk.Name)
			continue
		}
		if ro {
			log.Info("Skipping read-only disk", "disk", disk.Name)
			continue
		}

		if disk.IsRemovable {
			log.Info("Skipping removable disk", "disk", disk.Name)
			continue
		}

		devicePath := "/dev/" + disk.Name
		if _, err := os.Stat(devicePath); err != nil {
			log.Error(err, "Device path does not exist, skipping", "disk", disk.Name, "path", devicePath)
			continue
		}

		eligibleDisks = append(eligibleDisks, disk)
	}

	if len(eligibleDisks) == 0 {
		log.Info("No eligible disks to clean (all read-only, removable, or missing)")
		return nil
	}

	log.Info("Found eligible disks to clean", "count", len(eligibleDisks), "total", len(blockStorage.Disks))

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(4) // Limit to 4 concurrent disk wipes

	for _, disk := range eligibleDisks {
		g.Go(func() error {
			return cleanOne(ctx, disk, mode)
		})
	}

	// Wait for all disk wipes to complete
	if err := g.Wait(); err != nil {
		return fmt.Errorf("disk cleaning failed: %w", err)
	}

	// Mark as completed only if all disks were successfully cleaned
	if err := markDiskCleaningCompleted(); err != nil {
		log.Error(err, "Failed to mark disk cleaning as completed (will re-run on restart)")
	}

	log.Info("All disks cleaned successfully", "count", len(eligibleDisks))
	return nil
}

// cleanOne cleans a single disk based on the specified mode.
func cleanOne(ctx context.Context, disk *ghw.Disk, mode DiskCleaningMode) error {
	log := logr.FromContextOrDiscard(ctx)
	devicePath := "/dev/" + disk.Name

	log.Info("Cleaning disk", "disk", disk.Name, "model", disk.Model, "vendor", disk.Vendor,
		"size", disk.SizeBytes, "mode", mode)

	start := time.Now()
	var err error

	switch mode {
	case DiskCleaningModeQuick:
		err = quickCleanDisk(ctx, disk.Name, devicePath)
	case DiskCleaningModeSecure:
		err = secureCleanDisk(ctx, disk.Name, devicePath)
	}

	duration := time.Since(start)

	if err != nil {
		log.Error(err, "Failed to clean disk", "disk", disk.Name, "duration", duration)
		return fmt.Errorf("failed to clean disk %s: %w", disk.Name, err)
	}

	log.Info("Successfully cleaned disk", "disk", disk.Name, "duration", duration)
	return nil
}

func quickCleanDisk(ctx context.Context, diskName, devicePath string) error {
	log := logr.FromContextOrDiscard(ctx)

	// Add timeout for quick clean operations (10 minutes should be enough)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	if err := validateDevicePath(devicePath); err != nil {
		return err
	}

	log.V(1).Info("Using wipefs to remove filesystem signatures", "disk", diskName)
	cmd := exec.CommandContext(ctx, "wipefs", "-a", devicePath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wipefs failed: %w, output: %s", err, string(output))
	}

	if err := rereadPartitionTable(ctx, devicePath); err != nil {
		log.V(1).Info("Warning: failed to re-read partition table", "error", err)
	}

	return nil
}

func secureCleanDisk(ctx context.Context, diskName, devicePath string) error {
	log := logr.FromContextOrDiscard(ctx)

	ctx, cancel := context.WithTimeout(ctx, 24*time.Hour)
	defer cancel()

	if err := validateDevicePath(devicePath); err != nil {
		return err
	}

	// Try blkdiscard for SSDs first
	if !isRotational(diskName) {
		log.V(1).Info("Detected non-rotational flash storage, using blkdiscard", "disk", diskName)
		if err := executeBlkDiscard(ctx, devicePath, true); err != nil {
			log.Error(err, "blkdiscard failed, falling back to dd", "disk", diskName)
		} else {
			return rereadPartitionTable(ctx, devicePath)
		}
	}

	// Use dd for HDDs or when blkdiscard fails
	log.V(1).Info("Using dd for secure wipe", "disk", diskName)
	cmd := exec.CommandContext(ctx, "dd",
		"if=/dev/urandom",
		"of="+devicePath,
		"bs=1M",
		"status=progress",
		"oflag=direct")

	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := string(output)
		// "No space left on device" is expected when dd fills the disk
		if !strings.Contains(outputStr, "No space left on device") {
			return fmt.Errorf("dd failed: %w, output: %s", err, outputStr)
		}
		log.V(1).Info("dd completed (expected 'No space left' at end)", "disk", diskName)
	}

	if err := rereadPartitionTable(ctx, devicePath); err != nil {
		log.V(1).Info("Warning: failed to re-read partition table", "error", err)
	}

	return nil
}

// isRotational checks sysfs to determine if the drive is a spinning HDD (true) or SSD/NVMe (false).
// For multipath/dm devices, checks underlying slaves. Defaults to true (HDD) if unknown for safety.
func isRotational(diskName string) bool {
	// Need just the base name for sysfs, handles paths like /dev/mapper/mpatha properly
	baseName := filepath.Base(diskName)

	// Check if this is a device-mapper/multipath device
	dmPath := fmt.Sprintf("/sys/block/%s/dm/name", baseName)
	if _, err := os.Stat(dmPath); err == nil {
		// This is a dm device - check its slaves
		slavesDir := fmt.Sprintf("/sys/block/%s/slaves", baseName)
		entries, err := os.ReadDir(slavesDir)
		if err == nil && len(entries) > 0 {
			// Check first slave device's rotational status
			slaveName := entries[0].Name()
			slavePath := fmt.Sprintf("/sys/block/%s/queue/rotational", slaveName)
			if data, err := os.ReadFile(slavePath); err == nil {
				return strings.TrimSpace(string(data)) == "1"
			}
		}
	}

	// Not dm device, or couldn't determine from slaves - check direct path
	path := fmt.Sprintf("/sys/block/%s/queue/rotational", baseName)
	data, err := os.ReadFile(path)
	if err != nil {
		// Can't determine - assume rotational (HDD) for safety
		return true
	}

	return strings.TrimSpace(string(data)) == "1"
}

// executeBlkDiscard issues TRIM/UNMAP commands to securely erase flash cells.
func executeBlkDiscard(ctx context.Context, devicePath string, secure bool) error {
	args := []string{}
	if secure {
		args = append(args, "--secure")
	}
	args = append(args, devicePath)

	cmd := exec.CommandContext(ctx, "blkdiscard", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("blkdiscard failed: %w, output: %s", err, string(output))
	}
	return nil
}

// isReadOnly checks if a disk is read-only (hardware write-protected).
func isReadOnly(diskName string) (bool, error) {
	baseName := filepath.Base(diskName)
	roPath := fmt.Sprintf("/sys/class/block/%s/ro", baseName)
	data, err := os.ReadFile(roPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Sysfs file missing (e.g., multipath devices) - assume writable
			return false, nil
		}
		return false, fmt.Errorf("failed to read ro sysfs attribute: %w", err)
	}

	return strings.TrimSpace(string(data)) == "1", nil
}

func rereadPartitionTable(ctx context.Context, devicePath string) error {
	cmd := exec.CommandContext(ctx, "blockdev", "--rereadpt", devicePath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to re-read partition table: %w", err)
	}
	return nil
}

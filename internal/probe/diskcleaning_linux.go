// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package probe

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/jaypipes/ghw"
)

const diskCleaningMarkerFile = "/var/run/metal-operator/disk-cleaning-complete"

// DiskCleaningResult represents the result of cleaning a single disk.
type DiskCleaningResult struct {
	DeviceName string
	Success    bool
	Error      error
	Duration   time.Duration
}

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
		return fmt.Errorf("path is not a block device: %s", devicePath)
	}

	return nil
}

// cleanDisks performs disk cleaning based on the specified mode concurrently.
func cleanDisks(ctx context.Context, log logr.Logger, mode string) error {
	// Check if disk cleaning was already completed
	if wasDiskCleaningCompleted() {
		log.Info("Disk cleaning already completed, skipping")
		return nil
	}

	// Validate mode upfront before launching goroutines
	if mode != "quick" && mode != "secure" {
		return fmt.Errorf("unsupported cleaning mode: %s (must be 'quick' or 'secure')", mode)
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

	var mu sync.Mutex
	results := make([]DiskCleaningResult, 0, len(blockStorage.Disks))
	var wg sync.WaitGroup
	skippedCount := 0

	// Limit concurrent disk wipes to avoid overwhelming the system
	const maxConcurrentWipes = 4
	semaphore := make(chan struct{}, maxConcurrentWipes)

	for _, disk := range blockStorage.Disks {
		ro, err := isReadOnly(disk.Name)
		if err != nil {
			log.Error(err, "Failed to check read-only status, skipping disk", "disk", disk.Name)
			skippedCount++
			continue
		}
		if ro {
			log.Info("Skipping read-only disk", "disk", disk.Name)
			skippedCount++
			continue
		}

		if disk.IsRemovable {
			log.Info("Skipping removable disk", "disk", disk.Name)
			skippedCount++
			continue
		}

		devicePath := "/dev/" + disk.Name
		if _, err := os.Stat(devicePath); err != nil {
			log.Error(err, "Device path does not exist, skipping", "disk", disk.Name, "path", devicePath)
			skippedCount++
			continue
		}

		wg.Add(1)
		// Launch each disk wipe in its own goroutine
		go func(d *ghw.Disk, path string) {
			defer wg.Done()

			// Acquire semaphore to limit concurrency
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				log.Info("Disk cleaning cancelled before starting", "disk", d.Name)
				mu.Lock()
				results = append(results, DiskCleaningResult{
					DeviceName: d.Name,
					Success:    false,
					Error:      ctx.Err(),
					Duration:   0,
				})
				mu.Unlock()
				return
			}

			log.Info("Cleaning disk", "disk", d.Name, "model", d.Model, "vendor", d.Vendor,
				"size", d.SizeBytes, "removable", d.IsRemovable)

			start := time.Now()
			var cleanErr error

			switch mode {
			case "quick":
				cleanErr = quickCleanDisk(ctx, log, d.Name, path)
			case "secure":
				cleanErr = secureCleanDisk(ctx, log, d.Name, path)
			default:
				cleanErr = fmt.Errorf("unsupported cleaning mode: %s", mode)
			}

			duration := time.Since(start)
			result := DiskCleaningResult{
				DeviceName: d.Name,
				Success:    cleanErr == nil,
				Error:      cleanErr,
				Duration:   duration,
			}

			if cleanErr != nil {
				log.Error(cleanErr, "Failed to clean disk", "disk", d.Name, "duration", duration)
			} else {
				log.Info("Successfully cleaned disk", "disk", d.Name, "duration", duration)
			}

			mu.Lock()
			results = append(results, result)
			mu.Unlock()

		}(disk, devicePath)
	}

	// Wait for all disk wipes to complete
	wg.Wait()

	successCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		}
	}

	log.Info("Disk cleaning completed",
		"total", len(blockStorage.Disks),
		"processed", len(results),
		"success", successCount,
		"failed", len(results)-successCount,
		"skipped", skippedCount)

	if successCount < len(results) {
		return fmt.Errorf("failed to clean %d out of %d disks", len(results)-successCount, len(results))
	}

	// Mark disk cleaning as completed to prevent re-runs on agent restart
	if err := markDiskCleaningCompleted(); err != nil {
		log.Error(err, "Failed to mark disk cleaning as completed (will re-run on restart)")
		// Non-fatal - cleaning succeeded, just couldn't write marker
	}

	return nil
}

func quickCleanDisk(ctx context.Context, log logr.Logger, diskName, devicePath string) error {
	// Add timeout for quick clean operations (10 minutes should be enough)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	if err := validateDevicePath(devicePath); err != nil {
		return err
	}

	log.V(1).Info("Wiping disk header", "disk", diskName)
	if err := wipeDiskRangeNative(ctx, devicePath, 0, 10*1024*1024); err != nil {
		return fmt.Errorf("failed to wipe disk header: %w", err)
	}

	sizeBytes, err := getDiskSize(ctx, devicePath)
	if err != nil {
		return fmt.Errorf("failed to get disk size: %w", err)
	}

	if sizeBytes > 10*1024*1024 {
		log.V(1).Info("Wiping disk footer", "disk", diskName)
		offset := sizeBytes - (10 * 1024 * 1024)
		if err := wipeDiskRangeNative(ctx, devicePath, offset, 10*1024*1024); err != nil {
			return fmt.Errorf("failed to wipe disk footer: %w", err)
		}
	}

	if err := rereadPartitionTable(ctx, devicePath); err != nil {
		log.V(1).Info("Warning: failed to re-read partition table", "error", err)
	}

	return nil
}

func secureCleanDisk(ctx context.Context, log logr.Logger, diskName, devicePath string) error {
	// Add timeout for secure clean (24 hours for very large disks)
	ctx, cancel := context.WithTimeout(ctx, 24*time.Hour)
	defer cancel()

	if err := validateDevicePath(devicePath); err != nil {
		return err
	}

	// Check if drive is flash storage (SSD/NVMe). If so, we must use blkdiscard.
	if !isRotational(diskName) {
		log.V(1).Info("Detected non-rotational flash storage. Using blkdiscard.", "disk", diskName)
		if err := executeBlkDiscard(ctx, devicePath, true); err == nil {
			return rereadPartitionTable(ctx, devicePath)
		} else {
			log.Error(err, "blkdiscard failed, falling back to shred/dd", "disk", diskName)
			// Proceed to fallback
		}
	}

	if _, err := exec.LookPath("shred"); err == nil {
		log.V(1).Info("Using shred for secure wipe", "disk", diskName)
		cmd := exec.CommandContext(ctx, "shred", "-v", "-n", "1", "-z", "--force", devicePath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Error(err, "shred failed, falling back to dd", "disk", diskName, "output", string(output))
			// Fall through to dd instead of returning error
		} else {
			// shred succeeded
			if err := rereadPartitionTable(ctx, devicePath); err != nil {
				log.V(1).Info("Warning: failed to re-read partition table", "error", err)
			}
			return nil
		}
	}

	// Use dd as fallback (either shred not found or shred failed)
	log.V(1).Info("Using dd for secure wipe", "disk", diskName)
	cmd := exec.CommandContext(ctx, "dd",
		"if=/dev/urandom",
		"of="+devicePath,
		"bs=1M",
		"status=progress")
	output, err := cmd.CombinedOutput()
	if err != nil {
		if !strings.Contains(err.Error(), "No space left on device") &&
			!strings.Contains(string(output), "No space left on device") {
			return fmt.Errorf("dd failed: %w, output: %s", err, string(output))
		}
		log.V(1).Info("dd completed (expected 'No space left' at end)", "disk", diskName)
	}

	if err := rereadPartitionTable(ctx, devicePath); err != nil {
		log.V(1).Info("Warning: failed to re-read partition table", "error", err)
	}

	return nil
}

// wipeDiskRangeNative uses standard Go I/O to avoid integer truncation bugs with `dd seek`.
func wipeDiskRangeNative(ctx context.Context, devicePath string, offset, size int64) error {
	f, err := os.OpenFile(devicePath, os.O_WRONLY|os.O_SYNC, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek to offset %d: %w", offset, err)
	}

	chunkSize := int64(1024 * 1024) // 1MB
	zeros := make([]byte, chunkSize)
	remaining := size

	for remaining > 0 {
		// Respect context cancellation during long writes
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		writeSize := min(chunkSize, remaining)
		if _, err := f.Write(zeros[:writeSize]); err != nil {
			return fmt.Errorf("failed to write zeros: %w", err)
		}
		remaining -= writeSize
	}

	return nil
}

// isRotational checks sysfs to determine if the drive is a spinning HDD (true) or SSD/NVMe (false).
func isRotational(diskName string) bool {
	// Need just the base name for sysfs, handles paths like /dev/mapper/mpatha properly
	baseName := filepath.Base(diskName)
	path := fmt.Sprintf("/sys/block/%s/queue/rotational", baseName)

	data, err := os.ReadFile(path)
	if err != nil {
		// If we can't tell, assume true (rotational) to be safe and use shred
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
		return false, fmt.Errorf("failed to read ro sysfs attribute: %w", err)
	}

	return strings.TrimSpace(string(data)) == "1", nil
}

func getDiskSize(ctx context.Context, devicePath string) (int64, error) {
	cmd := exec.CommandContext(ctx, "blockdev", "--getsize64", devicePath)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get disk size: %w", err)
	}

	sizeStr := strings.TrimSpace(string(output))
	size, err := strconv.ParseInt(sizeStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse disk size: %w", err)
	}

	return size, nil
}

func rereadPartitionTable(ctx context.Context, devicePath string) error {
	cmd := exec.CommandContext(ctx, "blockdev", "--rereadpt", devicePath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to re-read partition table: %w", err)
	}
	return nil
}

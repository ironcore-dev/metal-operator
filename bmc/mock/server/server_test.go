// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"testing"

	"github.com/go-logr/logr"
)

const testSystemPath = "data/Systems/437XR1138R2/index.json"

func embeddedPowerState(t *testing.T) string {
	t.Helper()
	s := NewMockServer(logr.Discard(), ":0")
	base, err := s.loadResource(testSystemPath)
	if err != nil {
		t.Fatalf("failed to load embedded system resource: %v", err)
	}
	ps, _ := base["PowerState"].(string)
	if ps == "" {
		t.Fatalf("embedded system resource has no PowerState")
	}
	return ps
}

func TestForcePowerStatePinsAndBlocksPowerActions(t *testing.T) {
	s := NewMockServer(logr.Discard(), ":0")

	if err := s.ForcePowerState(testSystemPath, "Unknown"); err != nil {
		t.Fatalf("ForcePowerState: %v", err)
	}

	base, err := s.loadResource(testSystemPath)
	if err != nil {
		t.Fatalf("loadResource: %v", err)
	}
	if got := base["PowerState"]; got != "Unknown" {
		t.Fatalf("PowerState = %v, want Unknown", got)
	}

	// A PowerOff action must not change the pinned value.
	s.doPowerOff(testSystemPath)
	base, err = s.loadResource(testSystemPath)
	if err != nil {
		t.Fatalf("loadResource: %v", err)
	}
	if got := base["PowerState"]; got != "Unknown" {
		t.Fatalf("after doPowerOff PowerState = %v, want Unknown (pinned)", got)
	}
}

// TestForcePowerStateClearPreservesOtherOverrideFields guards the review
// finding: clearing the pin must reset only PowerState to the embedded
// default and must NOT discard other fields a caller PATCHed into the
// override.
func TestForcePowerStateClearPreservesOtherOverrideFields(t *testing.T) {
	s := NewMockServer(logr.Discard(), ":0")
	want := embeddedPowerState(t)

	// Seed an override that carries a custom field plus a non-default
	// PowerState, as a PATCH against the system resource would.
	seeded, err := s.loadResource(testSystemPath)
	if err != nil {
		t.Fatalf("loadResource: %v", err)
	}
	seeded["PowerState"] = "On"
	seeded["CustomPatchedField"] = "keep-me"
	s.saveResource(testSystemPath, seeded)

	// Pin, then release.
	if err := s.ForcePowerState(testSystemPath, "Unknown"); err != nil {
		t.Fatalf("ForcePowerState pin: %v", err)
	}
	if err := s.ForcePowerState(testSystemPath, ""); err != nil {
		t.Fatalf("ForcePowerState clear: %v", err)
	}

	got, err := s.loadResource(testSystemPath)
	if err != nil {
		t.Fatalf("loadResource: %v", err)
	}

	if ps := got["PowerState"]; ps != want {
		t.Errorf("PowerState after clear = %v, want embedded default %q", ps, want)
	}
	if v, ok := got["CustomPatchedField"]; !ok || v != "keep-me" {
		t.Errorf("CustomPatchedField after clear = %v (present=%v), want \"keep-me\" preserved", v, ok)
	}
	if _, pinned := s.stuckPowerState[testSystemPath]; pinned {
		t.Errorf("stuckPowerState still pinned after clear")
	}

	// After release a PowerOff action must once again drive PowerState to Off.
	s.doPowerOff(testSystemPath)
	got, err = s.loadResource(testSystemPath)
	if err != nil {
		t.Fatalf("loadResource: %v", err)
	}
	if ps := got["PowerState"]; ps != PowerOffState {
		t.Errorf("after clear+doPowerOff PowerState = %v, want %q", ps, PowerOffState)
	}
}

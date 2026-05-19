// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParseMgmtIP(t *testing.T) {
	tests := []struct {
		name     string
		input    json.RawMessage
		expected string
	}{
		{
			name:     "string value",
			input:    json.RawMessage(`"192.168.1.1"`),
			expected: "192.168.1.1",
		},
		{
			name:     "array with single value",
			input:    json.RawMessage(`["192.168.1.1"]`),
			expected: "192.168.1.1",
		},
		{
			name:     "array with multiple values returns first",
			input:    json.RawMessage(`["192.168.1.1", "10.0.0.1"]`),
			expected: "192.168.1.1",
		},
		{
			name:     "empty raw message",
			input:    json.RawMessage(``),
			expected: "",
		},
		{
			name:     "empty array",
			input:    json.RawMessage(`[]`),
			expected: "",
		},
		{
			name:     "null",
			input:    json.RawMessage(`null`),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseMgmtIP(tt.input)
			if result != tt.expected {
				t.Errorf("parseMgmtIP(%s) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseLLDPCTL(t *testing.T) {
	// Determine test data directory, search upward from the test file location for a test/data directory
	var baseDir string
	// upward search from test file dir up to repo root
	_, filename, _, _ := runtime.Caller(0)
	dir := filepath.Dir(filename)
	t.Logf("trying test data directory from dir: %s upwards", dir)
	for range 6 { // don't search forever
		cand := filepath.Join(dir, "test", "data")
		t.Logf("trying test data directory: %s", cand)
		if fi, err := os.Stat(cand); err == nil && fi.IsDir() {
			baseDir = cand
			break
		}
		dir = filepath.Dir(dir)
	}
	if baseDir == "" {
		t.Fatalf("could not locate test/data directory; tried GITHUB_WORKSPACE, runtime path, and cwd")
	} else {
		t.Logf("using test data directory: %s", baseDir)
	}

	fixtures := []struct {
		file string
		name string
	}{
		{filepath.Join(baseDir, "lldpctl_complete.json"), "complete"},
		{filepath.Join(baseDir, "lldpctl_incomplete.json"), "incomplete"},
		{filepath.Join(baseDir, "lldpctl_partial.json"), "partial"},
		{filepath.Join(baseDir, "lldpctl_single.json"), "single"},
		{filepath.Join(baseDir, "lldpctl_mgmtip_array.json"), "mgmtip_array"},
	}

	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			data, err := os.ReadFile(f.file)
			if err != nil {
				t.Fatalf("failed to read sample %s: %v", f.file, err)
			}
			parsed, err := ParseLLDPCTL(data)
			if err != nil {
				t.Fatalf("ParseLLDPCTL returned error for %s: %v", f.name, err)
			}
			t.Logf("parsed (%s) interfaces: %d", f.name, len(parsed.Interfaces))

			// Delegate validation to helper functions to reduce cyclomatic complexity
			switch f.name {
			case "complete":
				validateComplete(t, parsed)
			case "incomplete":
				validateIncomplete(t, parsed)
			case "partial":
				validatePartial(t, parsed)
			case "single":
				validateSingle(t, parsed)
			case "mgmtip_array":
				validateMgmtIPArray(t, parsed)
			}
		})
	}
}

func validateComplete(t *testing.T, parsed LLDP) {
	if len(parsed.Interfaces) != 3 {
		t.Fatalf("expected 3 interfaces for complete fixture, got %d", len(parsed.Interfaces))
	}
	expectedNames := map[string]bool{"ens3f0np0": true, "eno12399": true, "ens6f0np0": true}
	for _, iface := range parsed.Interfaces {
		if !expectedNames[iface.Name] {
			t.Errorf("unexpected interface name: %s", iface.Name)
		}
		if len(iface.Neighbors) == 0 {
			t.Errorf("interface %s should have at least one neighbor", iface.Name)
		}
		for _, n := range iface.Neighbors {
			if n.ChassisID == "" {
				t.Errorf("neighbor missing chassis ID for interface %s", iface.Name)
			}
			if n.PortID == "" {
				t.Errorf("neighbor missing port ID for interface %s", iface.Name)
			}
		}
	}
}

func validateIncomplete(t *testing.T, parsed LLDP) {
	if len(parsed.Interfaces) != 0 {
		t.Fatalf("expected 0 interfaces for incomplete fixture, got %d", len(parsed.Interfaces))
	}
}

func validatePartial(t *testing.T, parsed LLDP) {
	if len(parsed.Interfaces) != 1 {
		t.Fatalf("expected 1 interface in partial fixture, got %d", len(parsed.Interfaces))
	}
	if parsed.Interfaces[0].Name != "eth0" {
		t.Fatalf("expected interface name eth0 in partial fixture, got %s", parsed.Interfaces[0].Name)
	}
	if len(parsed.Interfaces[0].Neighbors) != 1 {
		t.Fatalf("expected 1 neighbor for eth0 in partial fixture, got %d", len(parsed.Interfaces[0].Neighbors))
	}
	n := parsed.Interfaces[0].Neighbors[0]
	if n.ChassisID != "xx:xx:xx:xx:xx:xx" {
		t.Errorf("unexpected chassis id in partial fixture: %s", n.ChassisID)
	}
	// missing fields should be empty strings
	if n.PortID != "" {
		t.Errorf("expected empty port id in partial fixture, got %s", n.PortID)
	}
}

func validateSingle(t *testing.T, parsed LLDP) {
	if len(parsed.Interfaces) != 1 {
		t.Fatalf("expected 1 interface in single fixture, got %d", len(parsed.Interfaces))
	}
	if parsed.Interfaces[0].Name != "ens3f0np0" {
		t.Fatalf("expected interface name ens3f0np0 in single fixture, got %s", parsed.Interfaces[0].Name)
	}
	if len(parsed.Interfaces[0].Neighbors) != 1 {
		t.Fatalf("expected 1 neighbor for ens3f0np0 in single fixture, got %d", len(parsed.Interfaces[0].Neighbors))
	}
	n := parsed.Interfaces[0].Neighbors[0]
	if n.ChassisID != "xx:xx:xx:xx:xx:xx" {
		t.Errorf("unexpected chassis id in single fixture: %s", n.ChassisID)
	}
	if n.PortID != "Eth1/17" {
		t.Errorf("unexpected port id in single fixture: %s", n.PortID)
	}
}

func validateMgmtIPArray(t *testing.T, parsed LLDP) {
	if len(parsed.Interfaces) != 1 {
		t.Fatalf("expected 1 interface in mgmtip_array fixture, got %d", len(parsed.Interfaces))
	}
	if parsed.Interfaces[0].Name != "ens3f0np0" {
		t.Fatalf("expected interface name ens3f0np0, got %s", parsed.Interfaces[0].Name)
	}
	if len(parsed.Interfaces[0].Neighbors) != 1 {
		t.Fatalf("expected 1 neighbor, got %d", len(parsed.Interfaces[0].Neighbors))
	}
	n := parsed.Interfaces[0].Neighbors[0]
	if n.ChassisID != "e8:eb:d3:e4:5a:2e" {
		t.Errorf("unexpected chassis id: %s", n.ChassisID)
	}
	if n.MgmtIP != "192.168.30.242" {
		t.Errorf("expected MgmtIP 192.168.30.242, got %q", n.MgmtIP)
	}
}

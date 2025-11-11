// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package registry

import "encoding/json"

// LLDP is the normalized structure we expose.
type LLDP struct {
	Interfaces []LLDPInterface `json:"interfaces"`
}

type LLDPInterface struct {
	Name      string     `json:"name"`
	Neighbors []Neighbor `json:"neighbors"`
}

type Neighbor struct {
	ChassisID         string   `json:"chassisId,omitempty"`
	PortID            string   `json:"portId,omitempty"`
	PortDescription   string   `json:"portDescription,omitempty"`
	SystemName        string   `json:"systemName,omitempty"`
	SystemDescription string   `json:"systemDescription,omitempty"`
	MgmtIP            string   `json:"mgmtIp,omitempty"`
	Capabilities      []string `json:"capabilities,omitempty"`
	VlanID            string   `json:"vlanId,omitempty"`
}

// ParseLLDPCTL converts raw lldpctl JSON (format: {"lldp":{"interface":[{iface:{...}},...]}})
// into the normalized LLDP struct.
func ParseLLDPCTL(data []byte) (LLDP, error) {
	type rawChassisID struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	}
	type rawPortID struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	}
	type rawCapability struct {
		Type    string `json:"type"`
		Enabled bool   `json:"enabled"`
	}
	type rawChassis struct {
		ID         rawChassisID    `json:"id"`
		Descr      string          `json:"descr"`
		MgmtIP     string          `json:"mgmt-ip"`
		MgmtIface  string          `json:"mgmt-iface"`
		Capability []rawCapability `json:"capability"`
	}
	type rawPort struct {
		ID    rawPortID `json:"id"`
		Descr string    `json:"descr"`
		TTL   string    `json:"ttl"`
	}
	type rawVlan struct {
		VlanID string `json:"vlan-id"`
		PVID   bool   `json:"pvid"`
	}
	type rawIfaceDetails struct {
		Via     string                `json:"via"`
		RID     string                `json:"rid"`
		Age     string                `json:"age"`
		Chassis map[string]rawChassis `json:"chassis"`
		Port    rawPort               `json:"port"`
		Vlan    *rawVlan              `json:"vlan,omitempty"`
	}
	type rawLLDPCTL struct {
		LLDP struct {
			Interface json.RawMessage `json:"interface"`
		} `json:"lldp"`
	}
	var raw rawLLDPCTL
	if err := json.Unmarshal(data, &raw); err != nil {
		return LLDP{}, err
	}

	// Try to unmarshal Interface as an array first, then as an object, both forms are possible.
	var entries []map[string]rawIfaceDetails
	if len(raw.LLDP.Interface) > 0 {
		if err := json.Unmarshal(raw.LLDP.Interface, &entries); err != nil {
			// Try object form: map[string]rawIfaceDetails
			var obj map[string]rawIfaceDetails
			if err2 := json.Unmarshal(raw.LLDP.Interface, &obj); err2 != nil {
				return LLDP{}, err // return the original error from array attempt
			}
			// convert object form into array-like entries
			for k, v := range obj {
				entries = append(entries, map[string]rawIfaceDetails{k: v})
			}
		}
	}

	out := LLDP{}
	for _, entry := range entries {
		for ifName, details := range entry {
			iface := LLDPInterface{Name: ifName}
			// details.Chassis may be nil or empty; guard accordingly
			for sysName, ch := range details.Chassis {
				n := Neighbor{
					SystemName:        sysName,
					SystemDescription: ch.Descr,
					ChassisID:         ch.ID.Value,
					PortID:            details.Port.ID.Value,
					PortDescription:   details.Port.Descr,
					MgmtIP:            ch.MgmtIP,
				}
				if details.Vlan != nil {
					n.VlanID = details.Vlan.VlanID
				}
				for _, cap := range ch.Capability {
					if cap.Enabled {
						n.Capabilities = append(n.Capabilities, cap.Type)
					}
				}
				iface.Neighbors = append(iface.Neighbors, n)
			}
			out.Interfaces = append(out.Interfaces, iface)
		}
	}
	return out, nil
}

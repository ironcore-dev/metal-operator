// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package registry

type LLDP struct {
	Interfaces []LLDPInterface `json:"Neighbors"`
}

type LLDPInterface struct {
	InterfaceIndex            int        `json:"InterfaceIndex"`
	InterfaceName             string     `json:"InterfaceName"`
	InterfaceAlternativeNames []string   `json:"InterfaceAlternativeNames"`
	Neighbors                 []Neighbor `json:"Neighbors"`
}

type Neighbor struct {
	ChassisID           string `json:"ChassisID"`
	RawChassisID        []int  `json:"RawChassisID"`
	PortID              string `json:"PortID"`
	RawPortID           []int  `json:"RawPortID"`
	PortDescription     string `json:"PortDescription"`
	SystemName          string `json:"SystemName"`
	SystemDescription   string `json:"SystemDescription"`
	EnabledCapabilities int    `json:"EnabledCapabilities"`
}

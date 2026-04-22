// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServerNetworkConfigSpec defines the expected network topology for a server,
// written by an external source such as argora/NetBox.
type ServerNetworkConfigSpec struct {
	// ServerRef references the Server this config applies to.
	// +required
	ServerRef v1.LocalObjectReference `json:"serverRef"`

	// Interfaces lists the expected network interfaces.
	// +optional
	Interfaces []SpecNetworkInterface `json:"interfaces,omitempty"`
}

// SpecNetworkInterface defines the expected configuration of a single network interface.
type SpecNetworkInterface struct {
	// Name is the interface name as defined in the external source.
	// +required
	Name string `json:"name"`

	// MACAddress is the expected MAC address of the interface.
	// +required
	MACAddress string `json:"macAddress"`

	// Expected holds the expected switch connectivity for this interface.
	// +required
	Expected ExpectedNetworkDetails `json:"expected"`
}

// ExpectedNetworkDetails holds the expected switch connectivity for a network interface.
type ExpectedNetworkDetails struct {
	// Switch is the expected LLDP neighbor system name (i.e. the connected switch).
	// +required
	Switch string `json:"switch"`

	// Port is the expected LLDP neighbor port ID on the connected switch.
	// +required
	Port string `json:"port"`
}

// DiscoveredNeighbor holds LLDP neighbor data observed on a network interface during discovery.
type DiscoveredNeighbor struct {
	// ChassisID is the chassis identifier of the LLDP neighbor.
	// +optional
	ChassisID string `json:"chassisID,omitempty"`

	// SystemName is the system name of the LLDP neighbor.
	// +optional
	SystemName string `json:"systemName,omitempty"`

	// PortID is the port identifier on the connected switch.
	// +optional
	PortID string `json:"portID,omitempty"`

	// PortDescription is the description of the port on the connected switch.
	// +optional
	PortDescription string `json:"portDescription,omitempty"`

	// MgmtIP is the management IP of the LLDP neighbor.
	// +optional
	MgmtIP string `json:"mgmtIP,omitempty"`

	// VlanID is the VLAN ID reported by the LLDP neighbor.
	// +optional
	VlanID string `json:"vlanID,omitempty"`
}

// DiscoveredNetworkDetails holds the observed state of a network interface as collected during discovery.
type DiscoveredNetworkDetails struct {
	// IPAddresses is the list of IP addresses assigned to the interface.
	// +optional
	IPAddresses []string `json:"ipAddresses,omitempty"`

	// CarrierStatus is the operational carrier status of the interface.
	// +optional
	CarrierStatus string `json:"carrierStatus,omitempty"`

	// Speed is the link speed of the interface (e.g. "25000").
	// +optional
	Speed string `json:"speed,omitempty"`

	// LinkModes lists the supported link modes of the interface.
	// +optional
	LinkModes []string `json:"linkModes,omitempty"`

	// Neighbors holds LLDP neighbor data discovered on this interface.
	// +optional
	Neighbors []DiscoveredNeighbor `json:"neighbors,omitempty"`
}

// StatusNetworkInterface holds the observed state of a network interface populated by metal-operator after discovery.
type StatusNetworkInterface struct {
	// Name is the interface name.
	// +required
	Name string `json:"name"`

	// MACAddress is the MAC address of the interface.
	// +optional
	MACAddress string `json:"macAddress,omitempty"`

	// Discovered holds the observed interface details collected during discovery.
	// +optional
	Discovered *DiscoveredNetworkDetails `json:"discovered,omitempty"`
}

// ServerNetworkConfigStatus holds the discovered network state populated by metal-operator after discovery.
type ServerNetworkConfigStatus struct {
	// LastUpdateTime is the timestamp of the last discovery run that updated this status.
	// +optional
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`

	// Interfaces lists the network interfaces observed during discovery.
	// +optional
	Interfaces []StatusNetworkInterface `json:"interfaces,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=snc
// +kubebuilder:printcolumn:name="ServerRef",type=string,JSONPath=`.spec.serverRef.name`
// +kubebuilder:printcolumn:name="Updated",type=date,JSONPath=`.status.lastUpdateTime`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ServerNetworkConfig holds the expected network configuration for a server (written by
// an external source such as argora) and the discovered network state populated by
// metal-operator after each discovery run.
type ServerNetworkConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServerNetworkConfigSpec   `json:"spec,omitempty"`
	Status ServerNetworkConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServerNetworkConfigList contains a list of ServerNetworkConfig.
type ServerNetworkConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServerNetworkConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServerNetworkConfig{}, &ServerNetworkConfigList{})
}

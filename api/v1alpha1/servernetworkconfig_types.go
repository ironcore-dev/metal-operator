// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NetworkCheckPhase indicates the result of the network check.
type NetworkCheckPhase string

const (
	// NetworkCheckPhasePassed indicates all interfaces matched the expected configuration.
	NetworkCheckPhasePassed NetworkCheckPhase = "Passed"
	// NetworkCheckPhaseFailed indicates one or more interfaces did not match.
	NetworkCheckPhaseFailed NetworkCheckPhase = "Failed"
	// NetworkCheckPhasePending indicates the check has not yet run.
	NetworkCheckPhasePending NetworkCheckPhase = "Pending"
)

// ServerNetworkConfigSpec defines the expected network configuration for a server,
// populated by an external source (e.g. argora from NetBox).
type ServerNetworkConfigSpec struct {
	// ServerRef references the Server this config applies to.
	// +required
	ServerRef v1.LocalObjectReference `json:"serverRef"`

	// Interfaces lists the expected network interfaces.
	// +optional
	Interfaces []ExpectedNetworkInterface `json:"interfaces,omitempty"`
}

// ExpectedNetworkInterface defines the expected configuration of a single network interface,
// derived from an external source such as NetBox.
type ExpectedNetworkInterface struct {
	// Name is the interface name as defined in the external source.
	// +required
	Name string `json:"name"`

	// MACAddress is the expected MAC address of the interface.
	// +required
	MACAddress string `json:"macAddress"`

	// Switch is the expected LLDP neighbor system name (i.e. the connected switch).
	// +required
	Switch string `json:"switch"`

	// Port is the expected LLDP neighbor port ID on the connected switch.
	// +required
	Port string `json:"port"`
}

// ServerNetworkConfigStatus defines the observed state of ServerNetworkConfig,
// written by metal-operator after running the comparison at the Discovery→Available transition.
type ServerNetworkConfigStatus struct {
	// Phase is the result of the network check.
	// +optional
	Phase NetworkCheckPhase `json:"phase,omitempty"`

	// Message provides a human-readable summary of the check result.
	// +optional
	Message string `json:"message,omitempty"`

	// LastCheckTime is the timestamp of the last check.
	// +optional
	LastCheckTime *metav1.Time `json:"lastCheckTime,omitempty"`

	// Mismatches lists interfaces that did not match the expected configuration.
	// +optional
	Mismatches []NetworkInterfaceMismatch `json:"mismatches,omitempty"`
}

// NetworkInterfaceMismatch describes a single interface that did not match expectations.
type NetworkInterfaceMismatch struct {
	// Name is the interface name.
	// +required
	Name string `json:"name"`

	// MACAddress is the MAC address used to identify the interface.
	// +required
	MACAddress string `json:"macAddress"`

	// ExpectedSwitch is the switch name expected from the external source.
	// +required
	ExpectedSwitch string `json:"expectedSwitch"`

	// ExpectedPort is the port ID expected from the external source.
	// +required
	ExpectedPort string `json:"expectedPort"`

	// ActualSwitch is the switch name observed via LLDP. Empty if the interface was not found.
	// +optional
	ActualSwitch string `json:"actualSwitch,omitempty"`

	// ActualPort is the port ID observed via LLDP. Empty if the interface was not found.
	// +optional
	ActualPort string `json:"actualPort,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=snc
// +kubebuilder:printcolumn:name="ServerRef",type=string,JSONPath=`.spec.serverRef.name`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ServerNetworkConfig holds the expected network configuration for a server (written by
// an external source such as argora) and the result of the network check performed by
// metal-operator at the Discovery→Available transition.
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

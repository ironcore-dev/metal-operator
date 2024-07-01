// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Power string

const (
	PowerOn  Power = "On"
	PowerOff Power = "Off"
)

type ServerPowerState string

const (
	// ServerOnPowerState the system is powered on.
	ServerOnPowerState ServerPowerState = "On"
	// ServerOffPowerState the system is powered off, although some components may
	// continue to have AUX power such as management controller.
	ServerOffPowerState ServerPowerState = "Off"
	// ServerPausedPowerState the system is paused.
	ServerPausedPowerState ServerPowerState = "Paused"
	// ServerPoweringOnPowerState A temporary state between Off and On. This
	// temporary state can be very short.
	ServerPoweringOnPowerState ServerPowerState = "PoweringOn"
	// ServerPoweringOffPowerState A temporary state between On and Off. The power
	// off action can take time while the OS is in the shutdown process.
	ServerPoweringOffPowerState ServerPowerState = "PoweringOff"
)

type BMCAccess struct {
	Protocol     Protocol                `json:"protocol"`
	Endpoint     string                  `json:"endpoint"`
	BMCSecretRef v1.LocalObjectReference `json:"bmcSecretRef"`
}

// ServerSpec defines the desired state of Server
type ServerSpec struct {
	UUID                 string                   `json:"uuid"`
	Power                Power                    `json:"power,omitempty"`
	IndicatorLED         IndicatorLED             `json:"indicatorLED,omitempty"`
	ServerClaimRef       *v1.ObjectReference      `json:"serverClaimRef,omitempty"`
	BMCRef               *v1.LocalObjectReference `json:"bmcRef,omitempty"`
	BMC                  *BMCAccess               `json:"bmc,omitempty"`
	BootConfigurationRef *v1.ObjectReference      `json:"bootConfigurationRef,omitempty"`
}

type ServerState string

const (
	ServerStateInitial   ServerState = "Initial"
	ServerStateAvailable ServerState = "Available"
	ServerStateReserved  ServerState = "Reserved"
	ServerStateError     ServerState = "Error"
)

// IndicatorLED represents LED indicator states
type IndicatorLED string

const (
	// UnknownIndicatorLED indicates the state of the Indicator LED cannot be
	// determined.
	UnknownIndicatorLED IndicatorLED = "Unknown"
	// LitIndicatorLED indicates the Indicator LED is lit.
	LitIndicatorLED IndicatorLED = "Lit"
	// BlinkingIndicatorLED indicates the Indicator LED is blinking.
	BlinkingIndicatorLED IndicatorLED = "Blinking"
	// OffIndicatorLED indicates the Indicator LED is off.
	OffIndicatorLED IndicatorLED = "Off"
)

// ServerStatus defines the observed state of Server
type ServerStatus struct {
	Manufacturer      string             `json:"manufacturer,omitempty"`
	SKU               string             `json:"sku,omitempty"`
	SerialNumber      string             `json:"serialNumber,omitempty"`
	PowerState        ServerPowerState   `json:"powerState,omitempty"`
	IndicatorLED      IndicatorLED       `json:"indicatorLED,omitempty"`
	State             ServerState        `json:"state,omitempty"`
	NetworkInterfaces []NetworkInterface `json:"networkInterfaces,omitempty"`
	//+patchStrategy=merge
	//+patchMergeKey=type
	//+optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

type NetworkInterface struct {
	Name string `json:"name"`
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Schemaless
	IP         IP     `json:"ip"`
	MACAddress string `json:"macAddress"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster
//+kubebuilder:printcolumn:name="UUID",type=string,JSONPath=`.spec.uuid`
//+kubebuilder:printcolumn:name="Manufacturer",type=string,JSONPath=`.status.manufacturer`
//+kubebuilder:printcolumn:name="SKU",type=string,JSONPath=`.status.sku`,priority=100
//+kubebuilder:printcolumn:name="SerialNumber",type=string,JSONPath=`.status.serialNumber`,priority=100
//+kubebuilder:printcolumn:name="PowerState",type=string,JSONPath=`.status.powerState`
//+kubebuilder:printcolumn:name="IndicatorLED",type=string,JSONPath=`.status.indicatorLED`,priority=100
//+kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Server is the Schema for the servers API
type Server struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServerSpec   `json:"spec,omitempty"`
	Status ServerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ServerList contains a list of Server
type ServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Server `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Server{}, &ServerList{})
}

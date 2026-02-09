// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BootMethod specifies the method used to network boot a server.
type BootMethod string

const (
	// BootMethodPXE boots the server using PXE (Pre-Boot Execution Environment).
	BootMethodPXE BootMethod = "PXE"

	// BootMethodHTTPBoot boots the server using UEFI HTTP Boot.
	BootMethodHTTPBoot BootMethod = "HTTPBoot"
)

// BootMode specifies whether the network boot override is applied once or persistently.
type BootMode string

const (
	// BootModeOnce applies the network boot method for the next boot only.
	// After the server boots once via the specified method, the BIOS boot order takes over.
	// This is the typical mode for OS installation workflows.
	BootModeOnce BootMode = "Once"

	// BootModeContinuous applies the network boot method on every boot cycle.
	// The server will always network-boot using the specified method until changed.
	// This is used for diskless/stateless servers or persistent netboot environments.
	BootModeContinuous BootMode = "Continuous"
)

// ServerBootConfigurationSpec defines the desired state of ServerBootConfiguration.
type ServerBootConfigurationSpec struct {
	// ServerRef is a reference to the server for which this boot configuration is intended.
	// +required
	ServerRef v1.LocalObjectReference `json:"serverRef"`

	// Image specifies the boot image to be used for the server.
	// This field is optional and can be omitted if not specified.
	// +optional
	Image string `json:"image,omitempty"`

	// IgnitionSecretRef is a reference to the Kubernetes Secret object that contains
	// the ignition configuration for the server. This field is optional and can be omitted if not specified.
	// +optional
	IgnitionSecretRef *v1.LocalObjectReference `json:"ignitionSecretRef,omitempty"`

	// BootMethod specifies the network boot method to use for the server.
	// This value can be overridden independently of the originating ServerClaim's preference.
	// Supported values are "PXE" and "HTTPBoot".
	// Defaults to "PXE" if not specified.
	// +kubebuilder:validation:Enum=PXE;HTTPBoot
	// +kubebuilder:default=PXE
	// +optional
	BootMethod BootMethod `json:"bootMethod,omitempty"`

	// BootMode controls whether the boot method override is applied once or on every boot.
	// "Once" boots from the network one time (e.g. for OS installation), then the BIOS boot
	// order takes over. "Continuous" boots from the network on every boot cycle.
	// Defaults to "Once" if not specified.
	// +kubebuilder:validation:Enum=Once;Continuous
	// +kubebuilder:default=Once
	// +optional
	BootMode BootMode `json:"bootMode,omitempty"`
}

// ServerBootConfigurationState defines the possible states of a ServerBootConfiguration.
type ServerBootConfigurationState string

const (
	// ServerBootConfigurationStatePending indicates that the boot configuration is pending and not yet ready.
	ServerBootConfigurationStatePending ServerBootConfigurationState = "Pending"
	// ServerBootConfigurationStateReady indicates that the boot configuration is ready for use.
	ServerBootConfigurationStateReady ServerBootConfigurationState = "Ready"
	// ServerBootConfigurationStateError indicates that there is an error with the boot configuration.
	ServerBootConfigurationStateError ServerBootConfigurationState = "Error"
)

// ServerBootConfigurationStatus defines the observed state of ServerBootConfiguration.
type ServerBootConfigurationStatus struct {
	// State represents the current state of the boot configuration.
	// +optional
	State ServerBootConfigurationState `json:"state,omitempty"`

	// Conditions represents the latest available observations of the ServerBootConfig's current state.
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ServerRef",type=string,JSONPath=`.spec.serverRef.name`
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="IgnitionRef",type=string,JSONPath=`.spec.ignitionSecretRef.name`
// +kubebuilder:printcolumn:name="BootMethod",type=string,JSONPath=`.spec.bootMethod`
// +kubebuilder:printcolumn:name="BootMode",type=string,JSONPath=`.spec.bootMode`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ServerBootConfiguration is the Schema for the serverbootconfigurations API
type ServerBootConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServerBootConfigurationSpec   `json:"spec,omitempty"`
	Status ServerBootConfigurationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServerBootConfigurationList contains a list of ServerBootConfiguration
type ServerBootConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServerBootConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServerBootConfiguration{}, &ServerBootConfigurationList{})
}

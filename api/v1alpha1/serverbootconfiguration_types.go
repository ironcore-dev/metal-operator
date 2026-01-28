// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BootMethod defines the boot method to use for server provisioning.
type BootMethod string

const (
	// BootMethodPXE boots the server using PXE network boot.
	BootMethodPXE BootMethod = "PXE"
	// BootMethodVirtualMedia boots the server using virtual media (ISO mounted via BMC).
	BootMethodVirtualMedia BootMethod = "VirtualMedia"
)

// ServerBootConfigurationSpec defines the desired state of ServerBootConfiguration.
type ServerBootConfigurationSpec struct {
	// ServerRef is a reference to the server for which this boot configuration is intended.
	// +required
	ServerRef v1.LocalObjectReference `json:"serverRef"`

	// Image specifies the boot image to be used for the server.
	// For PXE boot: OCI image reference containing kernel/initrd.
	// For VirtualMedia boot: OCI image reference containing bootable ISO layer.
	// This field is optional and can be omitted if not specified.
	// +optional
	Image string `json:"image,omitempty"`

	// IgnitionSecretRef is a reference to the Kubernetes Secret object that contains
	// the ignition configuration for the server. This field is optional and can be omitted if not specified.
	// +optional
	IgnitionSecretRef *v1.LocalObjectReference `json:"ignitionSecretRef,omitempty"`

	// BootMethod specifies the boot method to use for the server.
	// Valid values are "PXE" (default) and "VirtualMedia".
	// If not specified, defaults to PXE for backwards compatibility.
	// +kubebuilder:validation:Enum=PXE;VirtualMedia
	// +kubebuilder:default=PXE
	// +optional
	BootMethod BootMethod `json:"bootMethod,omitempty"`
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

	// BootISOURL is the URL to the bootable OS ISO provided by boot-operator.
	// This field is populated for VirtualMedia boot type.
	// +optional
	BootISOURL string `json:"bootISOURL,omitempty"`

	// ConfigISOURL is the URL to the config drive ISO containing ignition configuration.
	// This field is populated by boot-operator for VirtualMedia boot type.
	// +optional
	ConfigISOURL string `json:"configISOURL,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="ServerRef",type=string,JSONPath=`.spec.serverRef.name`
//+kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
//+kubebuilder:printcolumn:name="IgnitionRef",type=string,JSONPath=`.spec.ignitionSecretRef.name`
//+kubebuilder:printcolumn:name="BootMethod",type=string,JSONPath=`.spec.bootMethod`
//+kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ServerBootConfiguration is the Schema for the serverbootconfigurations API
type ServerBootConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServerBootConfigurationSpec   `json:"spec,omitempty"`
	Status ServerBootConfigurationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ServerBootConfigurationList contains a list of ServerBootConfiguration
type ServerBootConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServerBootConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServerBootConfiguration{}, &ServerBootConfigurationList{})
}

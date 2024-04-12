// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServerBootConfigurationSpec defines the desired state of ServerBootConfiguration
type ServerBootConfigurationSpec struct {
	ServerRef         v1.LocalObjectReference  `json:"serverRef"`
	Image             string                   `json:"image,omitempty"`
	IgnitionSecretRef *v1.LocalObjectReference `json:"ignitionSecretRef,omitempty"`
}

type ServerBootConfigurationState string

const (
	ServerBootConfigurationStatePending ServerBootConfigurationState = "Pending"
	ServerBootConfigurationStateReady   ServerBootConfigurationState = "Ready"
	ServerBootConfigurationStateError   ServerBootConfigurationState = "Error"
)

// ServerBootConfigurationStatus defines the observed state of ServerBootConfiguration
type ServerBootConfigurationStatus struct {
	State ServerBootConfigurationState `json:"state,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="ServerRef",type=string,JSONPath=`.spec.serverRef`
//+kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
//+kubebuilder:printcolumn:name="IgnitionRef",type=string,JSONPath=`.spec.ingitionRef`
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

// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServerClaimSpec defines the desired state of ServerClaim.
type ServerClaimSpec struct {
	// Power specifies the desired power state of the server.
	Power Power `json:"power"`

	// ServerRef is a reference to a specific server to be claimed.
	// This field is optional and can be omitted if the server is to be selected using ServerSelector.
	ServerRef *v1.LocalObjectReference `json:"serverRef,omitempty"`

	// ServerSelector specifies a label selector to identify the server to be claimed.
	// This field is optional and can be omitted if a specific server is referenced using ServerRef.
	ServerSelector *metav1.LabelSelector `json:"serverSelector,omitempty"`

	// IgnitionSecretRef is a reference to the Kubernetes Secret object that contains
	// the ignition configuration for the server. This field is optional and can be omitted if not specified.
	IgnitionSecretRef *v1.LocalObjectReference `json:"ignitionSecretRef,omitempty"`

	// Image specifies the boot image to be used for the server.
	Image string `json:"image"`
}

// Phase defines the possible phases of a ServerClaim.
type Phase string

const (
	// PhaseBound indicates that the server claim is bound to a server.
	PhaseBound Phase = "Bound"

	// PhaseUnbound indicates that the server claim is not bound to any server.
	PhaseUnbound Phase = "Unbound"
)

// ServerClaimStatus defines the observed state of ServerClaim.
type ServerClaimStatus struct {
	// Phase represents the current phase of the server claim.
	Phase Phase `json:"phase,omitempty"`

	// ClaimedServerRef is a reference to a specific server which is claimed.
	// This field is optional and can be omitted if the server is to be selected using ServerSelector.
	ClaimedServerRef *v1.LocalObjectReference `json:"claimedServerRef,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Server",type="string",JSONPath=".status.claimedServerRef.name"
// +kubebuilder:printcolumn:name="Ignition",type="string",JSONPath=".spec.ignitionSecretRef.name"
// +kubebuilder:printcolumn:name="Image",type="string",JSONPath=".spec.image"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ServerClaim is the Schema for the serverclaims API
type ServerClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServerClaimSpec   `json:"spec,omitempty"`
	Status ServerClaimStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServerClaimList contains a list of ServerClaim
type ServerClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServerClaim `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServerClaim{}, &ServerClaimList{})
}

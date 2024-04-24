/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServerClaimSpec defines the desired state of ServerClaim
type ServerClaimSpec struct {
	Power             Power                    `json:"power"`
	ServerRef         *v1.LocalObjectReference `json:"serverRef,omitempty"`
	ServerSelector    *metav1.LabelSelector    `json:"serverSelector,omitempty"`
	IgnitionSecretRef *v1.LocalObjectReference `json:"ignitionSecretRef,omitempty"`
	Image             string                   `json:"image"`
}

type Phase string

const (
	PhaseBound   Phase = "Bound"
	PhaseUnbound Phase = "Unbound"
)

// ServerClaimStatus defines the observed state of ServerClaim
type ServerClaimStatus struct {
	Phase Phase `json:"phase,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Server",type="string",JSONPath=".spec.serverRef.name"
// +kubebuilder:printcolumn:name="Ignition",type="string",JSONPath=".spec.ignitionRef.name"
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

//+kubebuilder:object:root=true

// ServerClaimList contains a list of ServerClaim
type ServerClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServerClaim `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServerClaim{}, &ServerClaimList{})
}

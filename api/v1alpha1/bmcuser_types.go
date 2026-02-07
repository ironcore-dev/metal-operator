// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BMCUserSpec defines the desired state of BMCUser.
type BMCUserSpec struct {
	BMCUserTemplate `json:",inline"`

	// BMCRef references the BMC this user should be created on.
	BMCRef *v1.LocalObjectReference `json:"bmcRef,omitempty"`
}

// BMCUserStatus defines the observed state of BMCUser.
type BMCUserStatus struct {
	// EffectiveBMCSecretRef references the BMCSecret currently used for this user.
	// This may differ from Spec.BMCSecretRef if the operator generated a password.
	EffectiveBMCSecretRef *v1.LocalObjectReference `json:"effectiveBMCSecretRef,omitempty"`
	// LastRotation is the timestamp of the last password rotation.
	LastRotation *metav1.Time `json:"lastRotation,omitempty"`
	// PasswordExpiration is the timestamp when the password will expire.
	PasswordExpiration *metav1.Time `json:"passwordExpiration,omitempty"`
	// ID of the user in the BMC system
	ID string `json:"id,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// BMCUser is the Schema for the bmcusers API.
type BMCUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BMCUserSpec   `json:"spec,omitempty"`
	Status BMCUserStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BMCUserList contains a list of BMCUser.
type BMCUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BMCUser `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BMCUser{}, &BMCUserList{})
}

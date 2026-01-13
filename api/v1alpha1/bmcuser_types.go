// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BMCUserSpec defines the desired state of BMCUser.
type BMCUserSpec struct {
	UserName       string           `json:"userName"`
	RoleID         string           `json:"roleID"`
	Description    string           `json:"description,omitempty"`
	RotationPeriod *metav1.Duration `json:"rotationPeriod,omitempty"`
	// if not set, the operator will generate a secure password based on BMC manufacturer requirements.
	BMCSecretRef *v1.LocalObjectReference `json:"bmcSecretRef,omitempty"`
	BMCRef       *v1.LocalObjectReference `json:"bmcRef,omitempty"`
	Enabled      bool                     `json:"enabled"`
	// set if the user should be used by the BMC controller to access the system.
	// +kubebuilder:default=false
	BMCControllerUser bool `json:"bmcControllerUser"`
}

// BMCUserStatus defines the observed state of BMCUser.
type BMCUserStatus struct {
	EffectiveBMCSecretRef *v1.LocalObjectReference `json:"effectiveBMCSecretRef,omitempty"`
	LastRotation          *metav1.Time             `json:"lastRotation,omitempty"`
	PasswordExpiration    *metav1.Time             `json:"passwordExpiration,omitempty"`
	ID                    string                   `json:"id,omitempty"` // ID of the user in the BMC system
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

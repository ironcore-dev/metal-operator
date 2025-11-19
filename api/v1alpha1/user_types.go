// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UserSpec defines the desired state of User
type UserSpec struct {
	UserName       string           `json:"userName"`
	RoleID         string           `json:"roleID"`
	Description    string           `json:"description,omitempty"`
	RotationPolicy *metav1.Duration `json:"rotationPeriod,omitempty"`
	// if not set, the operator will generate a secure password based on BMC manufacturer requirements.
	BMCSecretRef *v1.LocalObjectReference `json:"bmcSecretRef,omitempty"`
	BMCRef       *v1.LocalObjectReference `json:"bmcRef,omitempty"`
	Enabled      bool                     `json:"enabled"`
	// set if the user should be used by the BMC reconciler to access the system.
	UseForBMCAccess bool `json:"useForBMCAccess,omitempty"`
}

// UserStatus defines the observed state of User
type UserStatus struct {
	EffectiveBMCSecretRef *v1.LocalObjectReference `json:"effectiveBMCSecretRef,omitempty"`
	LastRotation          *metav1.Time             `json:"lastRotation,omitempty"`
	PasswordExpiration    string                   `json:"passwordExpiration,omitempty"`
	ID                    string                   `json:"id,omitempty"` // ID of the user in the BMC system
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status

// User is the Schema for the users API
type User struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UserSpec   `json:"spec,omitempty"`
	Status UserStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// UserList contains a list of User
type UserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []User `json:"items"`
}

func init() {
	SchemeBuilder.Register(&User{}, &UserList{})
}

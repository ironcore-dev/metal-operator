// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AccountState is the state of an account.
type AccountState string

const (
	// AccountStateActive is the state of an account that is active and can be used.
	AccountStateActive AccountState = "Active"
	// AccountStateInactive is the state of an account that is inactive and cannot be used.
	AccountStateInactive AccountState = "Inactive"
	// AccountStateLocked is the state of an account that is locked and cannot be used.
	AccountStateLocked AccountState = "Locked"
	// AccountStateUnknown is the state of an account that is unknown and cannot be used.
	AccountStateUnknown AccountState = "Unknown"
	// AccountStateDisabled is the state of an account that is disabled and cannot be used.
	AccountStateDisabled AccountState = "Disabled"
	// AccountStateEnabled is the state of an account that is enabled and can be used.
	AccountStateEnabled AccountState = "Enabled"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// UserSpec defines the desired state of User
type UserSpec struct {
	UserName       string                   `json:"userName"`
	RoleID         string                   `json:"roleID"`
	Description    string                   `json:"description,omitempty"`
	RotationPeriod *metav1.Duration         `json:"rotationPeriod,omitempty"`
	BMCSecretRef   *v1.LocalObjectReference `json:"bmcSecretRef,omitempty"`
	BMCRef         *v1.LocalObjectReference `json:"bmcRef,omitempty"`
	Enabled        bool                     `json:"enabled"`
	IsAdmin        bool                     `json:"isAdmin"`
}

// UserStatus defines the observed state of User
type UserStatus struct {
	EffectiveBMCSecretRef *v1.LocalObjectReference `json:"effectiveBMCSecretRef,omitempty"`
	LastRotation          *metav1.Time             `json:"lastRotation,omitempty"`
	State                 AccountState
	ID                    string
}

// +kubebuilder:object:root=true
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

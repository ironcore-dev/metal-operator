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

// AccountSpec defines the desired state of Account
type AccountSpec struct {
	Name               string
	RoleID             string
	Description        string
	PasswordExpiration metav1.Time
	BMCSecretRef       v1.LocalObjectReference
	BMCSelector        *metav1.LabelSelector
	Enabled            bool
	GeneratePassword   bool
	MetalUser          bool
	//"PasswordChangeRequired": false,
}

// AccountStatus defines the observed state of Account
type AccountStatus struct {
	State AccountState
	ID    string
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Account is the Schema for the accounts API
type Account struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AccountSpec   `json:"spec,omitempty"`
	Status AccountStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AccountList contains a list of Account
type AccountList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Account `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Account{}, &AccountList{})
}

// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AccountState string

const (
	AccountStateActive   AccountState = "Active"
	AccountStateInactive AccountState = "Inactive"
	AccountStateLocked   AccountState = "Locked"
	AccountStateUnknown  AccountState = "Unknown"
	AccountStateDisabled AccountState = "Disabled"
	AccountStateEnabled  AccountState = "Enabled"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AccountSpec defines the desired state of Account
type AccountSpec struct {
	Name               string
	RoleID             string
	Description        string
	PasswordExpiration metav1.Time
	SecretRef          v1.LocalObjectReference
	BMCSelector        *metav1.LabelSelector
	//"PasswordChangeRequired": false,
	//"PasswordExpiration": null,
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

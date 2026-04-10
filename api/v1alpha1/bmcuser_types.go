// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BMCUserSpec defines the desired state of BMCUser.
// +kubebuilder:validation:XValidation:rule="!(has(self.ttl) && has(self.expiresAt))",message="ttl and expiresAt are mutually exclusive"
type BMCUserSpec struct {
	// UserName is the username of the BMC user.
	UserName string `json:"userName"`

	// RoleID is the ID of the role to assign to the user.
	RoleID string `json:"roleID"`

	// Description is a description for the BMC user.
	Description string `json:"description,omitempty"`

	// RotationPeriod defines how often the password should be rotated.
	// If not set, the password will not be rotated.
	RotationPeriod *metav1.Duration `json:"rotationPeriod,omitempty"`

	// BMCSecretRef references the BMCSecret containing the credentials for this user.
	// If not set, the operator will generate a secure password based on BMC manufacturer requirements.
	BMCSecretRef *v1.LocalObjectReference `json:"bmcSecretRef,omitempty"`

	// BMCRef references the BMC this user should be created on.
	BMCRef *v1.LocalObjectReference `json:"bmcRef,omitempty"`

	// TTL specifies the time-to-live duration for this user.
	// When set, the user will be automatically deleted after this duration from creation.
	// This is useful for temporary debugging users.
	// If not set, the user is permanent (no automatic deletion).
	// Mutually exclusive with ExpiresAt - only one should be set.
	// +optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern="^([0-9]+(\\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$"
	TTL *metav1.Duration `json:"ttl,omitempty"`

	// ExpiresAt specifies an absolute timestamp when this user should be deleted.
	// This is useful for users that need to expire at a specific time.
	// If not set along with TTL, the user is permanent.
	// Mutually exclusive with TTL - only one should be set.
	// +optional
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`
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

	// ID is the identifier of the user in the BMC system.
	ID string `json:"id,omitempty"`

	// ExpiresAt is the calculated absolute time when this user will be deleted.
	// Set by the controller based on TTL or spec.ExpiresAt.
	// Only set for temporary users (when spec.TTL or spec.ExpiresAt is set).
	// +optional
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`

	// Conditions represents the latest available observations of the BMCUser's current state.
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=bmcu
// +kubebuilder:printcolumn:name="ID",type=string,JSONPath=`.status.id`
// +kubebuilder:printcolumn:name="UserName",type=string,JSONPath=`.spec.userName`
// +kubebuilder:printcolumn:name="RoleID",type=string,JSONPath=`.spec.roleID`
// +kubebuilder:printcolumn:name="ExpiresAt",type=date,JSONPath=`.status.expiresAt`,priority=1
// +kubebuilder:printcolumn:name="LastRotation",type=date,JSONPath=`.status.lastRotation`
// +kubebuilder:printcolumn:name="PasswordExpiration",type=date,JSONPath=`.status.passwordExpiration`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

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

const (
	// BMCUserConditionTypeActive indicates whether the user is currently active and not expired.
	BMCUserConditionTypeActive = "Active"
)

// Condition Reasons for Active condition
const (
	// BMCUserReasonActive indicates the user is active and within its lifetime
	BMCUserReasonActive = "Active"

	// BMCUserReasonExpiringSoon indicates the user will expire within the warning period
	BMCUserReasonExpiringSoon = "ExpiringSoon"

	// BMCUserReasonExpired indicates the user has reached its expiration time
	BMCUserReasonExpired = "Expired"
)

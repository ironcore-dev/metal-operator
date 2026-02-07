// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BMCUserTemplate defines the template for the BMCUser Resource to be applied to the BMCs.
type BMCUserTemplate struct {
	// Username of the BMC user.
	UserName string `json:"userName"`
	// RoleID is the ID of the role to assign to the user.
	// The available roles depend on the BMC implementation.
	// For Redfish, common role IDs are "Administrator", "Operator", "ReadOnly".
	RoleID string `json:"roleID"`
	// Description is an optional description for the BMC user.
	Description string `json:"description,omitempty"`
	// RotationPeriod defines how often the password should be rotated.
	// if not set, the password will not be rotated.
	RotationPeriod *metav1.Duration `json:"rotationPeriod,omitempty"`
	// BMCSecretRef references the BMCSecret containing the credentials for this user.
	// If not set, the operator will generate a secure password based on BMC manufacturer requirements.
	BMCSecretRef *corev1.LocalObjectReference `json:"bmcSecretRef,omitempty"`
}

// BMCUserSetSpec defines the desired state of BMCUserSet.
type BMCUserSetSpec struct {
	// BMCSelector specifies a label selector to identify the BMCs that are to be selected.
	// +required
	BMCSelector metav1.LabelSelector `json:"bmcSelector"`

	// BMCUserTemplate defines the template for the BMCUser Resource to be applied to the BMCs.
	// +required
	BMCUserTemplate BMCUserTemplate `json:"bmcUserTemplate,omitempty"`
}

// BMCUserSetStatus defines the observed state of BMCUserSet.
type BMCUserSetStatus struct {
	// FullyLabeledBMCs is the number of BMC in the set.
	FullyLabeledBMCs int32 `json:"fullyLabeledBMCs,omitempty"`
	// AvailableBMCUsers is the number of BMCUsers currently created by the set.
	AvailableBMCUsers int32 `json:"availableBMCUsers,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="UserName",type=string,JSONPath=`.spec.bmcUserTemplate.userName`
// +kubebuilder:printcolumn:name="RoleID",type=string,JSONPath=`.spec.bmcUserTemplate.roleID`
// +kubebuilder:printcolumn:name="TotalBMCs",type="integer",JSONPath=`.status.fullyLabeledBMCs`
// +kubebuilder:printcolumn:name="AvailableBMCUsers",type="integer",JSONPath=`.status.availableBMCUsers`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// BMCUserSet is the Schema for the bmcusersets API.
type BMCUserSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BMCUserSetSpec   `json:"spec,omitempty"`
	Status BMCUserSetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BMCUserSetList contains a list of BMCUserSet.
type BMCUserSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BMCUserSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BMCUserSet{}, &BMCUserSetList{})
}

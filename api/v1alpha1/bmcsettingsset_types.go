// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BMCSettingsSetSpec defines the desired state of BMCSettingsSet.
type BMCSettingsSetSpec struct {
	// BMCSettingsTemplate defines the template for the BMCSettings resource to be applied to the BMCs.
	// +required
	BMCSettingsTemplate BMCSettingsTemplate `json:"bmcSettingsTemplate,omitempty"`
	// DynamicVariables defines the dynamic variables for the BMCSettingsSet which allows users to specify variables in the BMC settings and their sources which will be resolved by the controller at runtime and injected into the BMC settings before applying them to the BMCs.
	// +optional
	DynamicVariables []DynamicVariables `json:"dynamicVariables,omitempty"`
	// BMCSelector specifies a label selector to identify the BMCs to be selected.
	// +required
	BMCSelector metav1.LabelSelector `json:"bmcSelector"`
}

// +kubebuilder:validation:XValidation:rule="!has(self.objectKeyRef) || (has(self.key) && size(self.key) > 0)",message="key must be provided when objectKeyRef is set"
// +kubebuilder:validation:XValidation:rule="!has(self.objectKeyRef) || size(self.objectKeyRef.kind) > 0",message="objectKeyRef.kind must be provided when objectKeyRef is set"
// +kubebuilder:validation:XValidation:rule="has(self.objectKeyRef) || (has(self.bmcLabel) && size(self.bmcLabel) > 0)",message="either objectKeyRef or bmcLabel must be provided"
type DynamicVariables struct {
	// ObjectKeyRef is used to specify the reference to the object which contains the value of the variable.
	// +optional
	ObjectKeyRef *ObjectReference `json:"objectKeyRef,omitempty"`
	// Key is used to specify the key of the value in data from the object defined in 'ObjectKeyRef'.
	// For example, if the variable is supposed to get its value from a ConfigMap, 'ObjectKeyRef' contains the reference to the ConfigMap and 'Key' can be used to specify the key of the value in the ConfigMap's data.
	// +optional
	Key string `json:"key,omitempty"`

	// BMCLabel is used to specify the label of the BMC from which the variable value will be sourced. The controller will look for the label in the BMC's labels and use its value as the variable's value.
	// +optional
	BMCLabel string `json:"bmcLabel,omitempty"`

	// Name specifies the variable name, referenced in BMC settings using the `{{ .Name }}` syntax.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	// +required
	Name string `json:"name"`
}

// BMCSettingsSetStatus defines the observed state of BMCSettingsSet.
type BMCSettingsSetStatus struct {
	// FullyLabeledBMCs is the number of BMCs in the set.
	FullyLabeledBMCs int32 `json:"fullyLabeledBMCs,omitempty"`
	// AvailableBMCSettings is the number of BMCSettings currently created by the set.
	AvailableBMCSettings int32 `json:"availableBMCSettings,omitempty"`
	// PendingBMCSettings is the total number of pending BMCSettings in the set.
	PendingBMCSettings int32 `json:"pendingBMCSettings,omitempty"`
	// InProgressBMCSettings is the total number of BMCSettings in the set that are currently in progress.
	InProgressBMCSettings int32 `json:"inProgressBMCSettings,omitempty"`
	// CompletedBMCSettings is the total number of completed BMCSettings in the set.
	CompletedBMCSettings int32 `json:"completedBMCSettings,omitempty"`
	// FailedBMCSettings is the total number of failed BMCSettings in the set.
	FailedBMCSettings int32 `json:"failedBMCSettings,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="BMCVersion",type=string,JSONPath=`.spec.bmcSettingsTemplate.version`
// +kubebuilder:printcolumn:name="TotalBMCs",type="integer",JSONPath=`.status.fullyLabeledBMCs`
// +kubebuilder:printcolumn:name="AvailableBMCSettings",type="integer",JSONPath=`.status.availableBMCSettings`
// +kubebuilder:printcolumn:name="Pending",type="integer",JSONPath=`.status.pendingBMCSettings`
// +kubebuilder:printcolumn:name="InProgress",type="integer",JSONPath=`.status.inProgressBMCSettings`
// +kubebuilder:printcolumn:name="Completed",type="integer",JSONPath=`.status.completedBMCSettings`
// +kubebuilder:printcolumn:name="Failed",type="integer",JSONPath=`.status.failedBMCSettings`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// BMCSettingsSet is the Schema for the bmcsettingssets API.
type BMCSettingsSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BMCSettingsSetSpec   `json:"spec,omitempty"`
	Status BMCSettingsSetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BMCSettingsSetList contains a list of BMCSettingsSet.
type BMCSettingsSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BMCSettingsSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BMCSettingsSet{}, &BMCSettingsSetList{})
}

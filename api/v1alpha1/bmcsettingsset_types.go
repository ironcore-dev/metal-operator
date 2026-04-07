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
	BMCSettingsTemplate BMCSettingsTemplate `json:"bmcSettingsTemplate"`
	// DynamicSettings defines dynamic settings to resolve per BMC when creating BMCSettings resources.
	// +optional
	DynamicSettings []DynamicSetting `json:"dynamicSettings,omitempty"`
	// BMCSelector specifies a label selector to identify the BMCs to be selected.
	// +required
	BMCSelector metav1.LabelSelector `json:"bmcSelector"`
}

// +kubebuilder:validation:XValidation:rule="(has(self.valueFrom) ? 1 : 0) + (has(self.format) && size(self.format) > 0 ? 1 : 0) == 1",message="exactly one of valueFrom or non-empty format must be provided"
// +kubebuilder:validation:XValidation:rule="!has(self.format) || size(self.format) == 0 || (has(self.variables) && size(self.variables) > 0)",message="variables must be provided when format is set"
// +kubebuilder:validation:XValidation:rule="!has(self.variables) || (has(self.format) && size(self.format) > 0)",message="variables can only be provided when format is set"
type DynamicSetting struct {
	// Key is the BMC setting key to set.
	// +kubebuilder:validation:MinLength=1
	// +required
	Key string `json:"key"`

	// ValueFrom defines a simple single source for the setting value.
	// +optional
	ValueFrom *DynamicSettingSource `json:"valueFrom,omitempty"`

	// Format defines a composite Go template with variable references like '{{.name}}'.
	// +optional
	Format string `json:"format,omitempty"`

	// Variables maps format placeholder names to their sources.
	// +optional
	Variables map[string]DynamicSettingSource `json:"variables,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="(has(self.bmcLabel) ? 1 : 0) + (has(self.configMapKeyRef) ? 1 : 0) + (has(self.secretKeyRef) ? 1 : 0) == 1",message="exactly one of bmcLabel, configMapKeyRef, or secretKeyRef must be provided"
type DynamicSettingSource struct {
	// BMCLabel is sourced from a label on the selected BMC.
	// +optional
	BMCLabel string `json:"bmcLabel,omitempty"`

	// ConfigMapKeyRef points to a namespaced ConfigMap key.
	// +optional
	ConfigMapKeyRef *NamespacedKeySelector `json:"configMapKeyRef,omitempty"`

	// SecretKeyRef points to a namespaced Secret key.
	// +optional
	SecretKeyRef *NamespacedKeySelector `json:"secretKeyRef,omitempty"`
}

type NamespacedKeySelector struct {
	// Name is the referenced object name.
	// +required
	Name string `json:"name"`

	// Namespace is the referenced object namespace.
	// +required
	Namespace string `json:"namespace"`

	// Key is the key within the referenced object.
	// +required
	Key string `json:"key"`
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

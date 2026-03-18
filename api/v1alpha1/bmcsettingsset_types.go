// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DynamicSettingSourceType string

const (
	// ConfigMap specifies that the dynamic setting source is a ConfigMap.
	ConfigMap DynamicSettingSourceType = "ConfigMap"
	// Labels specifies that the dynamic setting source is labels.
	Labels DynamicSettingSourceType = "labels"
	// Secret specifies that the dynamic setting source is a Secret.
	Secret DynamicSettingSourceType = "Secret"
)

// BMCSettingsSetSpec defines the desired state of BMCSettingsSet.
type BMCSettingsSetSpec struct {
	// BMCSettingsTemplate defines the template for the BMCSettings resource to be applied to the BMCs.
	// +required
	BMCSettingsTemplate BMCSettingsTemplate `json:"bmcSettingsTemplate,omitempty"`
	// DynamicSettings defines the dynamic settings for the BMCSettingsSet which allows users to specify variables in the BMC settings and their sources which will be resolved by the controller at runtime and injected into the BMC settings before applying them to the BMCs.
	// +optional
	DynamicSettings DynamicBMCSettings `json:"dynamicSettings,omitempty"`
	// BMCSelector specifies a label selector to identify the BMCs to be selected.
	// +required
	BMCSelector metav1.LabelSelector `json:"bmcSelector"`
}

type DynamicBMCSettings struct {
	ObjectKeyRefs []ObjectReference  `json:"objectKeyRefs,omitempty"`
	Variables     []DynamicVariables `json:"variables,omitempty"`
}

type DynamicVariables struct {
	// ObjectName is used to specify the identifier of the object which contains the value of the variable.
	// For example, if the variable is supposed to get its value from a Secret object, this field along with ObjectKind can be used to identify the Secret in 'ObjectKeyRefs' and fetch the value of the variable from the Secret's data using the key specified in 'Key' field.
	// This is not needed for the Type "labels" as the value can be fetched directly from the labels of the BMC.
	ObjectName string `json:"objectName,omitempty"`

	// ObjectKind specifies the type of the Object, which determines where the controller should look for the value of the variable.
	// example: if ObjectKind is "ConfigMap", 'ObjectName' along ObjectKind  is used to identify the object in 'ObjectKeyRefs' and fetch ConfigMap and use the value of the 'key' specified in Key to fetch the value of the required variable.
	// In case of "labels" data for variable is fetched directly from the labels of the BMC.
	// +kubebuilder:validation:Enum=ConfigMap;labels;Secret
	// +required
	ObjectKind DynamicSettingSourceType `json:"objectKind"`

	// Key is used to specify the key of the value in data from the object identified by 'ObjectName' and 'ObjectKind'  in 'ObjectKeyRefs'.
	// For example, if the variable is supposed to get its value from a ConfigMap, 'ObjectName' along with ObjectKind can be used to identify the ConfigMap object in 'ObjectKeyRefs' and 'Key' can be used to specify the key of the value in the ConfigMap's data.
	// +required
	Key string `json:"key"`

	// Name specifies the variable name, referenced in BMC settings using the `{{ .Name }}` syntax.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	// +required
	Name string `json:"name"`

	// RegexReplace defines regex replacement rules applied to the resolved variable.
	// +optional
	RegexReplace RegexPattern `json:"regexReplace,omitempty"`
}

// RegexPattern defines a single regex replacement rule.
type RegexPattern struct {
	// Pattern is the regular expression to match against the variable value.
	// +required
	Pattern string `json:"pattern"`
	// Replacement is the string to replace the matched pattern with.
	// +required
	Replacement string `json:"replacement"`
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

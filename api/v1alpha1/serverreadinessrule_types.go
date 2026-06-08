// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EnforcementMode specifies how the controller maintains the desired state.
// +kubebuilder:validation:Enum=BootstrapOnly;Continuous
type EnforcementMode string

const (
	// EnforcementModeBootstrapOnly applies configuration only during the first reconcile.
	EnforcementModeBootstrapOnly EnforcementMode = "BootstrapOnly"

	// EnforcementModeContinuous continuously monitors and enforces the configuration.
	EnforcementModeContinuous EnforcementMode = "Continuous"
)

// TaintStatus specifies status of the Taint on Server.
// +kubebuilder:validation:Enum=Present;Absent
type TaintStatus string

const (
	// TaintStatusPresent represent the taint present on the Server.
	TaintStatusPresent TaintStatus = "Present"

	// TaintStatusAbsent represent the taint absent on the Server.
	TaintStatusAbsent TaintStatus = "Absent"
)

// ServerReadinessRuleSpec defines the desired state of ServerReadinessRule
type ServerReadinessRuleSpec struct {
	// Conditions contains a list of the Server conditions that defines the specific
	// criteria that must be met for taints to be managed on the target Server.
	// The presence or status of these conditions directly triggers the application or removal of Server taints.
	//
	// +required
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="conditions is immutable"
	Conditions []ConditionRequirement `json:"conditions"` //nolint:kubeapilinter

	// EnforcementMode specifies how the controller maintains the desired state.
	// enforcementMode is one of BootstrapOnly, Continuous.
	// "BootstrapOnly" applies the configuration once during initial setup.
	// "Continuous" ensures the state is monitored and corrected throughout the resource lifecycle.
	//
	// +required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="enforcementMode is immutable"
	EnforcementMode EnforcementMode `json:"enforcementMode,omitempty"`

	// Taint defines the specific Taint (Key, Value, and Effect) to be managed
	// on Servers that meet the defined condition criteria.
	//
	// +required
	// +kubebuilder:validation:XValidation:rule="self.key.size() <= 253",message="taint key length must be at most 253 characters"
	// +kubebuilder:validation:XValidation:rule="!has(oldSelf.key) || self.key == oldSelf.key",message="taint key is immutable"
	// +kubebuilder:validation:XValidation:rule="!has(oldSelf.effect) || self.effect == oldSelf.effect",message="taint effect is immutable"
	// +kubebuilder:validation:XValidation:rule="!has(oldSelf.value) || self.value == oldSelf.value",message="taint value is immutable"
	Taint Taint `json:"taint,omitempty,omitzero"`

	// ServerSelector limits the scope of this rule to a specific subset of Servers.
	//
	// +required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serverSelector is immutable"
	ServerSelector metav1.LabelSelector `json:"serverSelector,omitempty,omitzero"`
}

// ConditionRequirement defines a specific Server condition and the status value
// required to trigger the controller's action.
type ConditionRequirement struct {
	// Type of server condition
	//
	// Following kubebuilder validation is referred from https://pkg.go.dev/k8s.io/apimachinery/pkg/apis/meta/v1#Condition
	//
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=316
	Type string `json:"type,omitempty"`

	// RequiredStatus is status of the condition, one of True, False, Unknown.
	//
	// +required
	// +kubebuilder:validation:Enum=True;False;Unknown
	RequiredStatus metav1.ConditionStatus `json:"requiredStatus,omitempty"`
}

// ServerReadinessRuleStatus defines the observed state of ServerReadinessRule.
type ServerReadinessRuleStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the ServerReadinessRule resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// ServerReadinessRule is the Schema for the serverreadinessrules API
type ServerReadinessRule struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of ServerReadinessRule
	// +required
	Spec ServerReadinessRuleSpec `json:"spec"`

	// status defines the observed state of ServerReadinessRule
	// +optional
	Status ServerReadinessRuleStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ServerReadinessRuleList contains a list of ServerReadinessRule
type ServerReadinessRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []ServerReadinessRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &ServerReadinessRule{}, &ServerReadinessRuleList{})
		return nil
	})
}

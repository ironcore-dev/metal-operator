// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BMCSettingsFlowStatus tracks the per-step state within settingsFlow.
type BMCSettingsFlowStatus struct {
	// Name is the name of the flow step.
	Name string `json:"name"`

	// State is the current state of this flow step.
	State BMCSettingsState `json:"state"`
}

// BMCSettingsTemplate defines the template for BMC settings to be applied.
// +kubebuilder:validation:XValidation:rule="!has(self.variables) || self.variables.all(v, self.variables.filter(w, w.key == v.key).size() == 1)",message="variable keys must be unique"
type BMCSettingsTemplate struct {
	// Version is the firmware version these settings are applicable for.
	// Empty means version-agnostic. When created from a BMCSettingsSet with a versionSelector,
	// the controller sets this to the matched version for traceability.
	// +optional
	Version string `json:"version,omitempty"`

	// SettingsFlow is the ordered list of setting groups to apply.
	// Items are applied in ascending Priority order.
	// +optional
	SettingsFlow []SettingsFlowItem `json:"settingsFlow,omitempty"`

	// ReadinessGates blocks this BMCSettings in Pending until all gates are satisfied.
	// +optional
	ReadinessGates []ReadinessGate `json:"readinessGates,omitempty"`

	// Variables is a list of variables that can be used in the settings for templating.
	// +kubebuilder:validation:MaxItems=64
	// +optional
	Variables []Variable `json:"variables,omitempty"`

	// RetryPolicy defines the retry behavior for automatic retries on transient failures.
	// +optional
	RetryPolicy *RetryPolicy `json:"retryPolicy,omitempty"`

	// ServerMaintenancePolicy is a maintenance policy to be applied on the server.
	// +optional
	ServerMaintenancePolicy ServerMaintenancePolicy `json:"serverMaintenancePolicy,omitempty"`
}

// BMCSettingsSpec defines the desired state of BMCSettings.
type BMCSettingsSpec struct {
	BMCSettingsTemplate `json:",inline"`

	// ServerMaintenanceRefs are references to ServerMaintenance objects which are created by the controller for each
	// server that needs to be updated with the BMC settings.
	// +optional
	ServerMaintenanceRefs []ServerMaintenanceRefItem `json:"serverMaintenanceRefs,omitempty"`

	// BMCRef is a reference to a specific BMC to apply settings to.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="BMCRef is immutable"
	// +required
	BMCRef *corev1.LocalObjectReference `json:"BMCRef,omitempty"`
}

// ServerMaintenanceRefItem is a reference to a ServerMaintenance object.
type ServerMaintenanceRefItem struct {
	// ServerMaintenanceRef is a reference to a ServerMaintenance object that the BMCSettings has requested for the referred server.
	// +optional
	ServerMaintenanceRef *ObjectReference `json:"serverMaintenanceRef,omitempty"`
}

// BMCSettingsState specifies the current state of the server maintenance.
type BMCSettingsState string

const (
	// BMCSettingsStatePending specifies that the BMC settings update is waiting.
	BMCSettingsStatePending BMCSettingsState = "Pending"
	// BMCSettingsStateInProgress specifies that the BMC settings changes are in progress.
	BMCSettingsStateInProgress BMCSettingsState = "InProgress"
	// BMCSettingsStateApplied specifies that the BMC settings have been applied.
	BMCSettingsStateApplied BMCSettingsState = "Applied"
	// BMCSettingsStateFailed specifies that the BMC settings update has failed.
	BMCSettingsStateFailed BMCSettingsState = "Failed"
)

// BMCSettingsStatus defines the observed state of BMCSettings.
type BMCSettingsStatus struct {
	// State represents the current state of the BMC configuration task.
	// +optional
	State BMCSettingsState `json:"state,omitempty"`

	// FlowState tracks per-step state within settingsFlow.
	// +optional
	FlowState []BMCSettingsFlowStatus `json:"flowState,omitempty"`

	// LastAppliedTime is when the BMCSettings last transitioned to Applied.
	// +optional
	LastAppliedTime *metav1.Time `json:"lastAppliedTime,omitempty"`

	// FailedAttempts is the number of automatic retry attempts made after failure.
	// +optional
	FailedAttempts int32 `json:"failedAttempts,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represents the latest available observations of the BMC Settings Resource state.
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="BMCRef",type=string,JSONPath=`.spec.BMCRef.name`
// +kubebuilder:printcolumn:name="LastAppliedTime",type=date,JSONPath=`.status.lastAppliedTime`

// BMCSettings is the Schema for the BMCSettings API.
type BMCSettings struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BMCSettingsSpec   `json:"spec,omitempty"`
	Status BMCSettingsStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BMCSettingsList contains a list of BMCSettings.
type BMCSettingsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BMCSettings `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BMCSettings{}, &BMCSettingsList{})
}

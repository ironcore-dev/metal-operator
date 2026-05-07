// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:XValidation:rule="self.version != \"\"",message="version is required"
// +kubebuilder:validation:XValidation:rule="!has(self.variables)",message="variables is not supported for BIOSSettings"
type BIOSSettingsTemplate struct {
	BaseTemplate     `json:",inline"`
	SettingsTemplate `json:",inline"`
}

type SettingsFlowItem struct {
	// Name is the name of the flow item.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1000
	Name string `json:"name"`

	// Settings contains software (e.g. BIOS, BMC) settings as a map.
	// +optional
	Settings map[string]string `json:"settings,omitempty"`

	// Priority defines the order of applying the settings. Lower numbers have higher priority (i.e. lower numbers are applied first).
	// +required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=2147483645
	Priority int32 `json:"priority"`
}

// BIOSSettingsSpec defines the desired state of BIOSSettings.
type BIOSSettingsSpec struct {
	// BIOSSettingsTemplate defines the template for BIOS Settings to be applied on the servers.
	BIOSSettingsTemplate `json:",inline"`

	// DriftPolicy controls the controller's reconcile mode when owned by a MaintenancePipelineRun.
	// Empty (default): active. Observe: drift detection only. Suspend: completely frozen.
	// +optional
	DriftPolicy DriftPolicy `json:"driftPolicy,omitempty"`

	// ServerMaintenanceRef is a reference to a ServerMaintenance object that BIOSSettings has requested for the referred server.
	// +optional
	ServerMaintenanceRef *ObjectReference `json:"serverMaintenanceRef,omitempty"`

	// ServerRef is a reference to a specific server to apply the BIOS settings on.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serverRef is immutable"
	ServerRef *corev1.LocalObjectReference `json:"serverRef,omitempty"`
}

// BIOSSettingsState specifies the current state of the BIOS Settings update.
type BIOSSettingsState string

const (
	// BIOSSettingsStatePending specifies that the BIOS settings update is waiting.
	BIOSSettingsStatePending BIOSSettingsState = "Pending"
	// BIOSSettingsStateInProgress specifies that the BIOS settings update is in progress.
	BIOSSettingsStateInProgress BIOSSettingsState = "InProgress"
	// BIOSSettingsStateApplied specifies that the BIOS settings have been applied.
	BIOSSettingsStateApplied BIOSSettingsState = "Applied"
	// BIOSSettingsStateFailed specifies that the BIOS settings update has failed.
	BIOSSettingsStateFailed BIOSSettingsState = "Failed"
)

type BIOSSettingsFlowState string

const (
	// BIOSSettingsFlowStatePending specifies that the BIOS settings update for the current priority is pending.
	BIOSSettingsFlowStatePending BIOSSettingsFlowState = "Pending"
	// BIOSSettingsFlowStateInProgress specifies that the BIOS settings update for the current priority is in progress.
	BIOSSettingsFlowStateInProgress BIOSSettingsFlowState = "InProgress"
	// BIOSSettingsFlowStateApplied specifies that the BIOS settings for the current priority have been applied.
	BIOSSettingsFlowStateApplied BIOSSettingsFlowState = "Applied"
	// BIOSSettingsFlowStateFailed specifies that the BIOS settings update has failed.
	BIOSSettingsFlowStateFailed BIOSSettingsFlowState = "Failed"
)

// BIOSSettingsStatus defines the observed state of BIOSSettings.
type BIOSSettingsStatus struct {
	// State represents the current state of the BIOS settings update.
	// +optional
	State BIOSSettingsState `json:"state,omitempty"`

	// FlowState is a list of individual BIOSSettings operation flows.
	FlowState []BIOSSettingsFlowStatus `json:"flowState,omitempty"`

	// LastAppliedTime represents the timestamp when the last setting was successfully applied.
	// +optional
	LastAppliedTime *metav1.Time `json:"lastAppliedTime,omitempty"`

	// FailedAttempts is the number of automatic retry attempts made after failure.
	// +optional
	FailedAttempts int32 `json:"failedAttempts,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represents the latest available observations of the BIOSSettings's current state.
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

type BIOSSettingsFlowStatus struct {
	// State represents the current state of the BIOS settings update for the current priority.
	// +optional
	State BIOSSettingsFlowState `json:"flowState,omitempty"`

	// Name identifies the current priority settings from the spec.
	// +optional
	Name string `json:"name,omitempty"`

	// Priority identifies the settings priority from the spec.
	// +optional
	Priority int32 `json:"priority"`

	// Conditions represents the latest available observations of the BIOSSettings's current Flowstate.
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// LastAppliedTime represents the timestamp when the last setting was successfully applied.
	// +optional
	LastAppliedTime *metav1.Time `json:"lastAppliedTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=bioss
// +kubebuilder:printcolumn:name="BIOSVersion",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="ServerRef",type=string,JSONPath=`.spec.serverRef.name`
// +kubebuilder:printcolumn:name="ServerMaintenanceRef",type=string,JSONPath=`.spec.serverMaintenanceRef.name`
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="AppliedOn",type=date,JSONPath=`.status.lastAppliedTime`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// BIOSSettings is the Schema for the biossettings API.
type BIOSSettings struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BIOSSettingsSpec   `json:"spec,omitempty"`
	Status BIOSSettingsStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BIOSSettingsList contains a list of BIOSSettings.
type BIOSSettingsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BIOSSettings `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BIOSSettings{}, &BIOSSettingsList{})
}

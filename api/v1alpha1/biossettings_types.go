// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type BIOSSettingsTemplate struct {
	// Version contains software (eg: BIOS, BMC) version this settings applies to
	// +required
	Version string `json:"version"`

	// SettingsFlow contains BIOS settings sequence to apply on the BIOS in given order
	// +optional
	SettingsFlow []SettingsFlowItem `json:"settingsFlow,omitempty"`

	// FailedAutoRetryCount is the number of times the controller should automatically retry the bios upgrade in case of failure before giving up.
	// +optional
	FailedAutoRetryCount *int32 `json:"failedAutoRetryCount,omitempty"`

	// ServerMaintenancePolicy is a maintenance policy to be enforced on the server.
	// +optional
	ServerMaintenancePolicy ServerMaintenancePolicy `json:"serverMaintenancePolicy,omitempty"`
}

type SettingsFlowItem struct {
	// Name is the name of the flow item
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1000
	Name string `json:"name"`

	// Settings contains software (eg: BIOS, BMC) settings as map
	// +optional
	Settings map[string]string `json:"settings,omitempty"`

	// Priority defines the order of applying the settings
	// any int greater than 0. lower number have higher Priority (ie; lower number is applied first)
	// +required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=2147483645
	Priority int32 `json:"priority"`
}

// BIOSSettingsSpec defines the desired state of BIOSSettings.
type BIOSSettingsSpec struct {
	// BIOSSettingsTemplate defines the template for BIOS Settings to be applied on the servers.
	BIOSSettingsTemplate `json:",inline"`

	// ServerMaintenanceRef is a reference to a ServerMaintenance object that BiosSetting has requested for the referred server.
	// +optional
	ServerMaintenanceRef *ObjectReference `json:"serverMaintenanceRef,omitempty"`

	// ServerRef is a reference to a specific server to apply bios setting on.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serverRef is immutable"
	ServerRef *corev1.LocalObjectReference `json:"serverRef,omitempty"`
}

// BIOSSettingsState specifies the current state of the BIOS Settings update.
type BIOSSettingsState string

const (
	// BIOSSettingsStatePending specifies that the bios setting update is waiting
	BIOSSettingsStatePending BIOSSettingsState = "Pending"
	// BIOSSettingsStateInProgress specifies that the BIOSSetting Controller is updating the settings
	BIOSSettingsStateInProgress BIOSSettingsState = "InProgress"
	// BIOSSettingsStateApplied specifies that the bios setting update has been completed.
	BIOSSettingsStateApplied BIOSSettingsState = "Applied"
	// BIOSSettingsStateFailed specifies that the bios setting update has failed.
	BIOSSettingsStateFailed BIOSSettingsState = "Failed"
)

type BIOSSettingsFlowState string

const (
	// BIOSSettingsFlowStatePending specifies that the BIOSSetting Controller is updating the settings for current Priority
	BIOSSettingsFlowStatePending BIOSSettingsFlowState = "Pending"
	// BIOSSettingsFlowStateInProgress specifies that the BIOSSetting Controller is updating the settings for current Priority
	BIOSSettingsFlowStateInProgress BIOSSettingsFlowState = "InProgress"
	// BIOSSettingsFlowStateApplied specifies that the bios setting has been completed for current Priority
	BIOSSettingsFlowStateApplied BIOSSettingsFlowState = "Applied"
	// BIOSSettingsFlowStateFailed specifies that the bios setting update has failed.
	BIOSSettingsFlowStateFailed BIOSSettingsFlowState = "Failed"
)

// BIOSSettingsStatus defines the observed state of BIOSSettings.
type BIOSSettingsStatus struct {
	// State represents the current state of the bios configuration task.
	// +optional
	State BIOSSettingsState `json:"state,omitempty"`

	// FlowState is a list of individual BIOSSettings operation flows.
	FlowState []BIOSSettingsFlowStatus `json:"flowState,omitempty"`

	// LastAppliedTime represents the timestamp when the last setting was successfully applied.
	// +optional
	LastAppliedTime *metav1.Time `json:"lastAppliedTime,omitempty"`

	// AutoRetryCountRemaining is the number of remaining times the controller will automatically retry the bios upgrade in case of failure before giving up.
	// +optional
	AutoRetryCountRemaining *int32 `json:"autoRetryCountRemaining,omitempty"`

	// Conditions represents the latest available observations of the BIOSSettings's current state.
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

type BIOSSettingsFlowStatus struct {
	// State represents the current state of the bios configuration task for current priority.
	// +optional
	State BIOSSettingsFlowState `json:"flowState,omitempty"`

	// Name identifies current priority settings from the Spec
	// +optional
	Name string `json:"name,omitempty"`

	// Priority identifies the settings priority from the Spec
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
// +kubebuilder:resource:scope=Cluster
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

// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BIOSSettingsSpec defines the desired state of BIOSSettings.
type BIOSSettingsSpec struct {
	// Version contains software (eg: BIOS, BMC) version this settings applies to
	// +required
	Version string `json:"version"`

	// SettingsFlow contains BIOS settings sequence to apply on the BIOS in given order
	// +optional
	SettingsFlow []SettingsFlowItem `json:"settingsFlow,omitempty"`

	// ServerRef is a reference to a specific server to apply bios setting on.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serverRef is immutable"
	ServerRef *corev1.LocalObjectReference `json:"serverRef,omitempty"`

	// CurrentSettingPriority specifies the priority of the current settings in sequence of settings (Flow) which currently being applied.
	// This is used in conjunction with and BIOSSettingFlow.
	// value above 0 indicates that the settings are part of a sequence of settings (Flow) to be applied in a specific order.
	// If the value is 0, it means that the settings are not part of a sequence and can be applied at one shot.
	// +optional
	CurrentSettingPriority int32 `json:"currentSettingPriority,omitempty"`

	// ServerMaintenancePolicy is a maintenance policy to be enforced on the server.
	// +optional
	ServerMaintenancePolicy ServerMaintenancePolicy `json:"serverMaintenancePolicy,omitempty"`

	// ServerMaintenanceRef is a reference to a ServerMaintenance object that BiosSetting has requested for the referred server.
	// +optional
	ServerMaintenanceRef *corev1.ObjectReference `json:"serverMaintenanceRef,omitempty"`
}

type SettingsFlowItem struct {
	SettingsMap map[string]string `json:"settings,omitempty"`
	// Priority defines the order of applying the settings
	// any int greater than 0. lower number have higher Priority (ie; lower number is applied first)
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=2147483645
	Priority int32 `json:"priority"`
}

// BIOSSettingsState specifies the current state of the BIOS maintenance.
type BIOSSettingsState string

const (
	// BIOSSettingsStatePending specifies that the bios setting maintenance is waiting
	BIOSSettingsStatePending BIOSSettingsState = "Pending"
	// BIOSSettingsStateInProgress specifies that the BIOSSetting Controller is updating the settings
	BIOSSettingsStateInProgress BIOSSettingsState = "InProgress"
	// BIOSSettingsStateApplied specifies that the bios setting maintenance has been completed.
	BIOSSettingsStateApplied BIOSSettingsState = "Applied"
	// BIOSSettingsStateFailed specifies that the bios setting maintenance has failed.
	BIOSSettingsStateFailed BIOSSettingsState = "Failed"
)

// BIOSSettingsStatus defines the observed state of BIOSSettings.
type BIOSSettingsStatus struct {
	// State represents the current state of the bios configuration task.
	// +optional
	State BIOSSettingsState `json:"state,omitempty"`

	// LastAppliedTime represents the timestamp when the last setting was successfully applied.
	// +optional
	LastAppliedTime *metav1.Time `json:"lastAppliedTime,omitempty"`

	// AppliedSettingPriority specifies the priority of the current settings in sequence of settings (Flow) which has been applied.
	// used in conjunction with BIOSSettingFlow Resource
	// value above 0 indicates that the settings was applied at one shot.
	// +optional
	AppliedSettingPriority int32 `json:"appliedSettingPriority,omitempty"`

	// Conditions represents the latest available observations of the BIOSSettings's current state.
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
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

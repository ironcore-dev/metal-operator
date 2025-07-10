// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BIOSSettingsState specifies the current state of the BIOS maintenance.
type BIOSSettingsFlowState string

const (
	// BIOSSettingsFlowStatePending specifies that the bios setting maintenance is waiting
	BIOSSettingsFlowStatePending BIOSSettingsFlowState = "Pending"
	// BIOSSettingsFlowStateInProgress specifies that the BIOSSetting Controller is updating the settings
	BIOSSettingsFlowStateInProgress BIOSSettingsFlowState = "InProgress"
	// BIOSSettingsFlowStateApplied specifies that the bios setting maintenance has been completed.
	BIOSSettingsFlowStateApplied BIOSSettingsFlowState = "Applied"
	// BIOSSettingsFlowStateFailed specifies that the bios setting maintenance has failed.
	BIOSSettingsFlowStateFailed BIOSSettingsFlowState = "Failed"
)

// BIOSSettingsFlowSpec defines the desired state of BIOSSettingsFlow.
type BIOSSettingsFlowSpec struct {
	// Version contains software (eg: BIOS, BMC) version this settings applies to
	// +required
	Version string `json:"version"`

	// SettingsFlow contains BIOS settings sequence to apply on the BIOS in given order
	// +optional
	SettingsFlow []SettingsFlowItem `json:"settingsFlow,omitempty"`

	// ServerRef is a reference to a specific server to apply bios setting on.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serverRef is immutable"
	ServerRef *corev1.LocalObjectReference `json:"serverRef,omitempty"`

	// ServerMaintenancePolicy is maintenance policy to be enforced on the server.
	ServerMaintenancePolicy ServerMaintenancePolicy `json:"serverMaintenancePolicy,omitempty"`
}

type SettingsFlowItem struct {
	Settings map[string]string `json:"settings,omitempty"`
	// Priority defines the order of applying the settings
	// any int greater than 0. lower number have higher Priority (ie; lower number is applied first)
	Priority int32 `json:"priority"`
}

// BIOSSettingsFlowStatus defines the observed state of BIOSSettingsFlow.
type BIOSSettingsFlowStatus struct {
	// State represents the current state of the bios configuration task.
	State BIOSSettingsFlowState `json:"state,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="BIOSVersion",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="ServerRef",type=string,JSONPath=`.spec.serverRef.name`
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// BIOSSettingsFlow is the Schema for the biossettingsflows API.
type BIOSSettingsFlow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BIOSSettingsFlowSpec   `json:"spec,omitempty"`
	Status BIOSSettingsFlowStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BIOSSettingsFlowList contains a list of BIOSSettingsFlow.
type BIOSSettingsFlowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BIOSSettingsFlow `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BIOSSettingsFlow{}, &BIOSSettingsFlowList{})
}

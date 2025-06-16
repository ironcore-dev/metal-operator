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

	// SettingsMap contains software (eg: BIOS, BMC) settings as map
	// +optional
	SettingsMap map[string]string `json:"settings,omitempty"`

	// SettingUpdatePolicy dictates how the settings are applied.
	// if 'Sequence', the BIOSSettings resource will enter 'Waiting' state after applying the settings
	// if 'OneShotUpdate' the BIOSSettings resource will enter 'Completed' state after applying the settings
	SettingUpdatePolicy SettingUpdatePolicy `json:"settingUpdatePolicy,omitempty"`

	// CurrentSettingPriority specifies the number of sequence left to complete the settings workflow
	// used in conjunction with SettingUpdatePolicy and BIOSSettingFlow
	CurrentSettingPriority int32 `json:"currentSettingPriority,omitempty"`

	// ServerRef is a reference to a specific server to apply bios setting on.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serverRef is immutable"
	ServerRef *corev1.LocalObjectReference `json:"serverRef,omitempty"`

	// ServerMaintenancePolicy is maintenance policy to be enforced on the server.
	ServerMaintenancePolicy ServerMaintenancePolicy `json:"serverMaintenancePolicy,omitempty"`

	// ServerMaintenanceRef is a reference to a ServerMaintenance object that BiosSetting has requested for the referred server.
	ServerMaintenanceRef *corev1.ObjectReference `json:"serverMaintenanceRef,omitempty"`
}

// BIOSSettingsState specifies the current state of the BIOS maintenance.
type BIOSSettingsState string

const (
	// BIOSSettingsStatePending specifies that the bios setting maintenance is waiting
	BIOSSettingsStatePending BIOSSettingsState = "Pending"
	// BIOSSettingsStateInWaiting specifies that the BIOSSetting Controller is updating the settings
	BIOSSettingsStateInWaiting BIOSSettingsState = "Waiting"
	// BIOSSettingsStateInProgress specifies that the BIOSSetting Controller is updating the settings
	BIOSSettingsStateInProgress BIOSSettingsState = "InProgress"
	// BIOSSettingsStateApplied specifies that the bios setting maintenance has been completed.
	BIOSSettingsStateApplied BIOSSettingsState = "Applied"
	// BIOSSettingsStateFailed specifies that the bios setting maintenance has failed.
	BIOSSettingsStateFailed BIOSSettingsState = "Failed"
)

type SettingUpdatePolicy string

const (
	SequencialUpdate SettingUpdatePolicy = "Sequence"
	OneShotUpdate    SettingUpdatePolicy = "OneShotUpdate"
)

type BIOSSettingUpdateState string

const (
	// BIOSSettingUpdateWaitOnServerRebootPowerOff specifies that the bios setting state is waiting on server to turn off during Reboot.
	BIOSSettingUpdateWaitOnServerRebootPowerOff BIOSSettingUpdateState = "WaitOnServerRebootPowerOff"
	// BIOSSettingUpdateWaitOnServerRebootPowerOn specifies that the bios setting state is waiting on server to turn on during Reboot.
	BIOSSettingUpdateWaitOnServerRebootPowerOn BIOSSettingUpdateState = "WaitOnServerRebootPowerOn"
	// BIOSSettingUpdateStateIssue specifies that the bios new setting was posted to server's RedFish API
	BIOSSettingUpdateStateIssue BIOSSettingUpdateState = "IssueSettingUpdate"
	// BIOSSettingUpdateStateVerification specifies that the bios setting is beening verified.
	BIOSSettingUpdateStateVerification BIOSSettingUpdateState = "VerifySettingUpdate"
)

// BIOSSettingsStatus defines the observed state of BIOSSettings.
type BIOSSettingsStatus struct {

	// State represents the current state of the bios configuration task.
	State BIOSSettingsState `json:"state,omitempty"`
	// UpdateSettingState represents the current state of the bios setting update task.
	UpdateSettingState BIOSSettingUpdateState `json:"updateSettingState,omitempty"`
	// AppliedSettingPriority specifies the number of sequence left to complete the settings workflow
	// used in conjunction with SettingUpdatePolicy and BIOSSettingFlow Resource
	AppliedSettingPriority int32 `json:"appliedSettingPriority,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="BIOSVersion",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="ServerRef",type=string,JSONPath=`.spec.serverRef.name`
// +kubebuilder:printcolumn:name="ServerMaintenanceRef",type=string,JSONPath=`.spec.serverMaintenanceRef.name`
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=`.status.state`
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

// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// BiosSettingsSpec defines the desired state of BiosSettings.
type BiosSettingsSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// BiosSettings specifies the BIOS settings for the selected serverRef or serverSelector.
	BiosSettings Settings `json:"biosSettings,omitempty"`

	// ServerRef is a reference to a specific server to apply bios setting on.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serverRef is immutable"
	ServerRef *corev1.LocalObjectReference `json:"serverRef,omitempty"`

	// ServerMaintenancePolicy is maintenance policy to be enforced on the server.
	ServerMaintenancePolicy ServerMaintenancePolicy `json:"ServerMaintenancePolicyTemplate,omitempty"`

	// ServerMaintenanceRef is a reference to a ServerMaintenance object that that BIOS has requested for the referred server.
	ServerMaintenanceRef *corev1.ObjectReference `json:"serverMaintenanceRef,omitempty"`
}

type Settings struct {
	// Version contains version this settings applies to
	// +required
	Version string `json:"version"`

	// SettingsMap contains settings as map
	// +optional
	SettingsMap map[string]string `json:"settings,omitempty"`
}

// BiosSettingsState specifies the current state of the BIOS maintenance.
type BiosSettingsState string

const (
	// BiosSettingsStateInProgress specifies that the server bios is in setting update path.
	BiosSettingsStateInProgress BiosSettingsState = "InProgress"
	// BiosSettingsStateSynced specifies that the server bios maintenance has been completed.
	BiosSettingsStateSynced BiosSettingsState = "SettingsSyncCompleted"
	// BiosSettingsStateFailed specifies that the server maintenance has failed.
	BiosSettingsStateFailed BiosSettingsState = "Failed"
)

type BiosSettingUpdateState string

const (
	// BiosSettingUpdateWaitOnServerRebootPowerOff specifies that the bios setting state is waiting on server to turn off during Reboot.
	BiosSettingUpdateWaitOnServerRebootPowerOff BiosSettingUpdateState = "WaitOnServerRebootPowerOff"
	// BiosSettingUpdateWaitOnServerRebootPowerOn specifies that the bios setting state is waiting on server to turn on during Reboot.
	BiosSettingUpdateWaitOnServerRebootPowerOn BiosSettingUpdateState = "WaitOnServerRebootPowerOn"
	// BiosSettingUpdateStateIssue specifies that the bios new setting was posted to RedFish
	BiosSettingUpdateStateIssue BiosSettingUpdateState = "IssueSettingUpdate"
	// BiosSettingUpdateStateVerification specifies that the bios setting has been completed.
	BiosSettingUpdateStateVerification BiosSettingUpdateState = "VerifySettingUpdate"
)

// BiosSettingsStatus defines the observed state of BiosSettings.
type BiosSettingsStatus struct {

	// State represents the current state of the bios configuration task.
	State BiosSettingsState `json:"state,omitempty"`
	// UpdateSettingState represents the current state of the bios setting update task.
	UpdateSettingState BiosSettingUpdateState `json:"updateSettingState,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// BiosSettings is the Schema for the biossettings API.
type BiosSettings struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BiosSettingsSpec   `json:"spec,omitempty"`
	Status BiosSettingsStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BiosSettingsList contains a list of BiosSettings.
type BiosSettingsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BiosSettings `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BiosSettings{}, &BiosSettingsList{})
}

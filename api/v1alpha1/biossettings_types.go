// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// BIOSSettingsSpec defines the desired state of BIOSSettings.
type BIOSSettingsSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// BIOSSettings specifies the BIOS settings for the selected serverRef or serverSelector.
	BIOSSettings Settings `json:"biosSettings,omitempty"`

	// ServerRef is a reference to a specific server to apply bios setting on.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serverRef is immutable"
	ServerRef *corev1.LocalObjectReference `json:"serverRef,omitempty"`

	// ServerMaintenancePolicy is maintenance policy to be enforced on the server.
	ServerMaintenancePolicy ServerMaintenancePolicy `json:"serverMaintenancePolicy,omitempty"`

	// ServerMaintenanceRef is a reference to a ServerMaintenance object that BiosSetting has requested for the referred server.
	ServerMaintenanceRef *corev1.ObjectReference `json:"serverMaintenanceRef,omitempty"`
}

type Settings struct {
	// Version contains BIOS version this settings applies to
	// +required
	Version string `json:"version"`

	// SettingsMap contains bios settings as map
	// +optional
	SettingsMap map[string]string `json:"settings,omitempty"`
}

// BIOSSettingsState specifies the current state of the BIOS maintenance.
type BIOSSettingsState string

const (
	// BIOSSettingsStatePending specifies that the server bios is in setting update path.
	BIOSSettingsStatePending BIOSSettingsState = "Pending"
	// BIOSSettingsStateInProgress specifies that the server bios is in setting update path.
	BIOSSettingsStateInProgress BIOSSettingsState = "InProgress"
	// BIOSSettingsStateApplied specifies that the server bios maintenance has been completed.
	BIOSSettingsStateApplied BIOSSettingsState = "Applied"
	// BIOSSettingsStateFailed specifies that the server maintenance has failed.
	BIOSSettingsStateFailed BIOSSettingsState = "Failed"
)

type BIOSSettingUpdateState string

const (
	// BIOSSettingUpdateWaitOnServerRebootPowerOff specifies that the bios setting state is waiting on server to turn off during Reboot.
	BIOSSettingUpdateWaitOnServerRebootPowerOff BIOSSettingUpdateState = "WaitOnServerRebootPowerOff"
	// BIOSSettingUpdateWaitOnServerRebootPowerOn specifies that the bios setting state is waiting on server to turn on during Reboot.
	BIOSSettingUpdateWaitOnServerRebootPowerOn BIOSSettingUpdateState = "WaitOnServerRebootPowerOn"
	// BIOSSettingUpdateStateIssue specifies that the bios new setting was posted to RedFish
	BIOSSettingUpdateStateIssue BIOSSettingUpdateState = "IssueSettingUpdate"
	// BIOSSettingUpdateStateVerification specifies that the bios setting has been completed.
	BIOSSettingUpdateStateVerification BIOSSettingUpdateState = "VerifySettingUpdate"
)

// BIOSSettingsStatus defines the observed state of BIOSSettings.
type BIOSSettingsStatus struct {

	// State represents the current state of the bios configuration task.
	State BIOSSettingsState `json:"state,omitempty"`
	// UpdateSettingState represents the current state of the bios setting update task.
	UpdateSettingState BIOSSettingUpdateState `json:"updateSettingState,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="BIOSVersion",type=string,JSONPath=`.spec.biosSettings.version`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="ServerRef",type=string,JSONPath=`.spec.serverRef.name`
// +kubebuilder:printcolumn:name="ServerMaintenanceRef",type=string,JSONPath=`.spec.serverMaintenanceRef.name`
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

// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ServerBIOSSpec defines the desired state of ServerBIOS
type ServerBIOSSpec struct {
	// ScanPeriodMinutes defines the period in minutes after which scanned data is considered obsolete.
	// +kubebuilder:default=30
	// +optional
	ScanPeriodMinutes int32 `json:"scanPeriodMinutes,omitempty"`

	// ServerRef is a reference to Server object
	// +optional
	ServerRef v1.LocalObjectReference `json:"serverRef,omitempty"`

	// BIOS contains a bios version and settings.
	// +optional
	BIOS BIOSSettings `json:"bios,omitempty"`
}

// BIOSSettings contains a version, settings and a flag defining whether it is a current version
type BIOSSettings struct {
	// Version contains BIOS version
	// +required
	Version string `json:"version"`

	// Settings contains BIOS settings as map
	// +optional
	Settings map[string]string `json:"settings,omitempty"`
}

// ServerBIOSStatus defines the observed state of ServerBIOS
type ServerBIOSStatus struct {
	// LastScanTime reflects the timestamp when the scanning for installed firmware was performed
	// +optional
	LastScanTime metav1.Time `json:"lastScanTime,omitempty"`

	// BIOS contains a bios version and settings.
	// +optional
	BIOS BIOSSettings `json:"bios,omitempty"`

	// RunningJob reflects the invoked scan or update job running
	// +optional
	RunningJob RunningJobRef `json:"runningJob,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Server",type=string,JSONPath=`.spec.serverRef.name`,description="Server name"
// +kubebuilder:printcolumn:name="BIOS Version",type=string,JSONPath=`.status.version`,description="Installed BIOS Version"

// ServerBIOS is the Schema for the serverbios API
type ServerBIOS struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServerBIOSSpec   `json:"spec,omitempty"`
	Status ServerBIOSStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServerBIOSList contains a list of ServerBIOS
type ServerBIOSList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServerBIOS `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServerBIOS{}, &ServerBIOSList{})
}

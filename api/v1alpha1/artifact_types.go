// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ArtifactSpec defines the desired state of Artifact
type ArtifactSpec struct {
	// Builder is the builder responsible for building this artifact.
	Builder string `json:"builder"`

	// Config is the configuration to use for building the artifact.
	Config ArtifactConfig `json:"config"`
}

type ArtifactConfig struct {
	// Direct is the configuration to build a direct artifact.
	Direct *DirectArtifactConfig `json:"direct,omitempty"`

	// ConfigDrive is the configuration to build a config drive.
	ConfigDrive *ConfigDriveArtifactConfig `json:"configDrive,omitempty"`
}

type DirectArtifactConfig struct {
	ImageRef         string                     `json:"imageRef"`
	Cmdline          string                     `json:"cmdline,omitempty"`
	Args             string                     `json:"args,omitempty"`
	OSRelease        string                     `json:"osRelease,omitempty"`
	EmbedConfigDrive *ConfigDriveArtifactConfig `json:"embedConfigDrive,omitempty"`
}

type ConfigDriveArtifactConfig struct {
	ServerID       string                   `json:"serverID"`
	Hostname       string                   `json:"hostname,omitempty"`
	NetworkDataRef *LocalObjectKeyReference `json:"networkDataRef,omitempty"`
	UserDataRef    *LocalObjectKeyReference `json:"userDataRef,omitempty"`
}

type DirectArtifactOverrides struct {
	Cmdline   string `json:"cmdline,omitempty"`
	Args      string `json:"args,omitempty"`
	OSRelease string `json:"osRelease,omitempty"`
}

// ArtifactStatus defines the observed state of Artifact.
type ArtifactStatus struct {
	// conditions represent the current state of the Artifact resource.
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

const (
	ArtifactAvailable = "Available"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Artifact is the Schema for the artifacts API
type Artifact struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Artifact
	// +required
	Spec ArtifactSpec `json:"spec"`

	// status defines the observed state of Artifact
	// +optional
	Status ArtifactStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ArtifactList contains a list of Artifact
type ArtifactList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Artifact `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &Artifact{}, &ArtifactList{})
		return nil
	})
}

// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// GlobalExposureSpec defines the desired state of GlobalExposure
type GlobalExposureSpec struct {
	// ArtifactRef references the artifact to expose.
	ArtifactRef LocalObjectReference `json:"artifactRef"`

	// Policy sets the policy to expose the artifact with.
	Policy ExposurePolicy `json:"policy"`
}

// GlobalExposureStatus defines the observed state of GlobalExposure.
type GlobalExposureStatus struct {
	// Conditions represent the current state of the GlobalExposure resource.
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

	// URL is the URL the artifact is served.
	URL string `json:"url,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// GlobalExposure is the Schema for the globalexposures API
type GlobalExposure struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of GlobalExposure
	// +required
	Spec GlobalExposureSpec `json:"spec"`

	// status defines the observed state of GlobalExposure
	// +optional
	Status GlobalExposureStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// GlobalExposureList contains a list of GlobalExposure
type GlobalExposureList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []GlobalExposure `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &GlobalExposure{}, &GlobalExposureList{})
		return nil
	})
}

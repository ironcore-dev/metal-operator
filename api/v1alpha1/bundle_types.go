// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"github.com/opencontainers/go-digest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// BundleSpec defines the desired state of Bundle
type BundleSpec struct {
	// Entry is the file to serve if the bundle is accessed at its root.
	Entry string `json:"entry,omitempty"`

	// Files are the files of this bundle.
	// +listType=map
	// +listMapKey=path
	// +optional
	Files []BundleFile `json:"files,omitempty"`
}

type BundleFile struct {
	// Path is the path to serve the file at.
	Path string `json:"path"`
	// ContentType is the content type of the file.
	ContentType string `json:"contentType"`
	// Digest is the digest to retrieve the file.
	Digest digest.Digest `json:"digest"`
	// Size is the size of the file.
	Size int64 `json:"size"`
}

// BundleStatus defines the observed state of Bundle.
type BundleStatus struct {
	// conditions represent the current state of the Bundle resource.
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

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=bdl
// +kubebuilder:subresource:status

// Bundle is the Schema for the bundles API
type Bundle struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Bundle
	// +required
	Spec BundleSpec `json:"spec"`

	// status defines the observed state of Bundle
	// +optional
	Status BundleStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// BundleList contains a list of Bundle
type BundleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Bundle `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &Bundle{}, &BundleList{})
		return nil
	})
}

// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Records is the Schema for the records API
type Records struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// Image is the resolved image.
	Image *Image `json:"image,omitempty"`

	// AgentTokenSecretRef references the agent token secret, if any.
	AgentTokenSecretRef *LocalObjectReference `json:"agentTokenSecretRef,omitempty"`

	// Markers are markers that denote certain events.
	// +listType=map
	// +listMapKey=name
	// +optional
	Markers []Marker `json:"markers,omitempty"`
}

type Image struct {
	Ref  string    `json:"ref"`
	Kind ImageKind `json:"kind"`
}

type ImageKind string

const (
	ImageKindDirect ImageKind = "Direct"
	ImageKindDisk   ImageKind = "Disk"
)

type Marker struct {
	Name MarkerName `json:"name"`
	// Time marks the time the marker was created.
	Time metav1.Time `json:"time"`
}

type MarkerName string

const (
	// MarkerBooted indicates whether a server has been fully booted.
	MarkerBooted MarkerName = "booted"
	// MarkerDirectRebooted denotes whether a server has been direct-rebooted.
	MarkerDirectRebooted MarkerName = "direct-rebooted"
	// MarkerDiskRebooted denotes whether a server has been disk-rebooted.
	MarkerDiskRebooted MarkerName = "disk-rebooted"
	// MarkerProvisioned indicates whether a server has been provisioned.
	MarkerProvisioned MarkerName = "provisioned"
)

func HasMarker(markers []Marker, markerName MarkerName) bool {
	return slices.IndexFunc(markers, func(marker Marker) bool {
		return marker.Name == markerName
	}) >= 0
}

func SetMarker(markers *[]Marker, markerName MarkerName) (changed bool) {
	if HasMarker(*markers, markerName) {
		return false
	}
	*markers = append(*markers, Marker{
		Name: markerName,
		Time: metav1.Now(),
	})
	return true
}

func DeleteMarker(markers *[]Marker, markerName MarkerName) (changed bool) {
	idx := slices.IndexFunc(*markers, func(marker Marker) bool { return marker.Name == markerName })
	if idx < 0 {
		return false
	}
	*markers = slices.Delete(*markers, idx, idx+1)
	return true
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=rec

// RecordsList contains a list of Records
type RecordsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Records `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &Records{}, &RecordsList{})
		return nil
	})
}

package records

import (
	"context"
	"fmt"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func New(c client.Client, records *metalv1alpha1.Records) *Records {
	return &Records{
		client:  c,
		records: records,
	}
}

type Records struct {
	client  client.Client
	records *metalv1alpha1.Records
}

type Markers interface {
	HasMarker(markerName metalv1alpha1.MarkerName) bool
	UpdateMarkers(ctx context.Context, opts ...MarkersUpdateOption) error
}

type MarkersUpdate struct {
	Add    []metalv1alpha1.MarkerName
	Delete []metalv1alpha1.MarkerName
}

type MarkersUpdateOption func(*MarkersUpdate)

func AddMarker(markerNames ...metalv1alpha1.MarkerName) MarkersUpdateOption {
	return func(o *MarkersUpdate) {
		o.Add = append(o.Add, markerNames...)
	}
}

func DeleteMarker(markerName metalv1alpha1.MarkerName) MarkersUpdateOption {
	return func(o *MarkersUpdate) {
		o.Delete = append(o.Delete, markerName)
	}
}

func (r *Records) HasMarker(markerName metalv1alpha1.MarkerName) bool {
	return metalv1alpha1.HasMarker(r.records.Markers, markerName)
}

func (r *Records) UpdateMarkers(ctx context.Context, opts ...MarkersUpdateOption) error {
	update := &MarkersUpdate{}
	for _, opt := range opts {
		opt(update)
	}

	base := r.records.DeepCopy()

	var changed bool
	for _, name := range update.Add {
		added := metalv1alpha1.SetMarker(&r.records.Markers, name)
		changed = changed || added
	}
	for _, name := range update.Delete {
		deleted := metalv1alpha1.DeleteMarker(&r.records.Markers, name)
		changed = changed || deleted
	}
	if !changed {
		return nil
	}

	if err := r.client.Patch(ctx, r.records,
		client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}),
	); err != nil {
		return fmt.Errorf("patching records: %w", err)
	}
	return nil
}

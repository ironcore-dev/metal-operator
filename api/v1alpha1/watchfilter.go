// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

// WatchFilterSelector returns the label selector matching exactly the
// resources an operator instance with the given watch filter value is
// responsible for. An empty filter matches resources without the watch-filter
// label, a non-empty filter matches resources labeled
// WatchFilterLabel=<filter>.
func WatchFilterSelector(filter string) (labels.Selector, error) {
	operator, values := selection.DoesNotExist, []string(nil)
	if filter != "" {
		operator, values = selection.Equals, []string{filter}
	}
	requirement, err := labels.NewRequirement(WatchFilterLabel, operator, values)
	if err != nil {
		return nil, fmt.Errorf("invalid watch filter %q: %w", filter, err)
	}
	return labels.NewSelector().Add(*requirement), nil
}

// MatchesWatchFilter reports whether the object belongs to the given watch
// filter. An empty filter only owns unlabeled objects.
func MatchesWatchFilter(obj metav1.Object, filter string) bool {
	value, ok := obj.GetLabels()[WatchFilterLabel]
	if filter == "" {
		return !ok
	}
	return ok && value == filter
}

// PropagateWatchFilterLabel copies the watch-filter label from src to dst if
// src carries one and removes it from dst otherwise. Controllers that create
// child resources use this to keep children visible to the same operator
// instance as their parent, including when the parent's label is removed or
// changed.
func PropagateWatchFilterLabel(dst, src metav1.Object) {
	filter, ok := src.GetLabels()[WatchFilterLabel]
	objLabels := dst.GetLabels()
	if !ok {
		delete(objLabels, WatchFilterLabel)
		dst.SetLabels(objLabels)
		return
	}
	if objLabels == nil {
		objLabels = map[string]string{}
	}
	objLabels[WatchFilterLabel] = filter
	dst.SetLabels(objLabels)
}

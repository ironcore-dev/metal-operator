// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func TestWatchFilterSelector(t *testing.T) {
	filtered, err := WatchFilterSelector("experimental")
	if err != nil {
		t.Fatal(err)
	}
	unfiltered, err := WatchFilterSelector("")
	if err != nil {
		t.Fatal(err)
	}

	unlabeled := labels.Set{"foo": "bar"}
	if filtered.Matches(unlabeled) {
		t.Error("filtered selector must not match unlabeled objects")
	}
	if !unfiltered.Matches(unlabeled) {
		t.Error("unfiltered selector must match unlabeled objects")
	}

	experimental := labels.Set{WatchFilterLabel: "experimental"}
	if !filtered.Matches(experimental) {
		t.Error("filtered selector must match its own filter value")
	}
	if unfiltered.Matches(experimental) {
		t.Error("unfiltered selector must not match labeled objects")
	}

	if filtered.Matches(labels.Set{WatchFilterLabel: "other"}) {
		t.Error("filtered selector must not match another filter value")
	}
}

func TestMatchesWatchFilter(t *testing.T) {
	obj := &metav1.PartialObjectMetadata{}
	if !MatchesWatchFilter(obj, "") || MatchesWatchFilter(obj, "experimental") {
		t.Error("unlabeled object must belong to the default instance only")
	}
	obj.SetLabels(map[string]string{WatchFilterLabel: "experimental"})
	if !MatchesWatchFilter(obj, "experimental") || MatchesWatchFilter(obj, "") {
		t.Error("labeled object must belong to its filter value only")
	}
}

func TestPropagateWatchFilterLabel(t *testing.T) {
	src := &metav1.PartialObjectMetadata{}
	dst := &metav1.PartialObjectMetadata{}
	PropagateWatchFilterLabel(dst, src)
	if _, ok := dst.GetLabels()[WatchFilterLabel]; ok {
		t.Error("propagation must not invent a watch-filter label")
	}

	src.SetLabels(map[string]string{WatchFilterLabel: "experimental"})
	PropagateWatchFilterLabel(dst, src)
	if dst.GetLabels()[WatchFilterLabel] != "experimental" {
		t.Error("propagation must copy the watch-filter label")
	}

	src.SetLabels(map[string]string{WatchFilterLabel: "other"})
	PropagateWatchFilterLabel(dst, src)
	if dst.GetLabels()[WatchFilterLabel] != "other" {
		t.Error("propagation must follow watch-filter label changes")
	}

	src.SetLabels(nil)
	PropagateWatchFilterLabel(dst, src)
	if _, ok := dst.GetLabels()[WatchFilterLabel]; ok {
		t.Error("propagation must remove the watch-filter label when the source lost it")
	}
}

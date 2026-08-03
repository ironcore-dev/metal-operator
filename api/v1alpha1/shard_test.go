// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func TestShardSelector(t *testing.T) {
	sharded, err := ShardSelector("experimental")
	if err != nil {
		t.Fatal(err)
	}
	unsharded, err := ShardSelector("")
	if err != nil {
		t.Fatal(err)
	}

	unlabeled := labels.Set{"foo": "bar"}
	if sharded.Matches(unlabeled) {
		t.Error("sharded selector must not match unlabeled objects")
	}
	if !unsharded.Matches(unlabeled) {
		t.Error("unsharded selector must match unlabeled objects")
	}

	experimental := labels.Set{ShardLabel: "experimental"}
	if !sharded.Matches(experimental) {
		t.Error("sharded selector must match its own shard")
	}
	if unsharded.Matches(experimental) {
		t.Error("unsharded selector must not match labeled objects")
	}

	if sharded.Matches(labels.Set{ShardLabel: "other"}) {
		t.Error("sharded selector must not match another shard")
	}
}

func TestInShard(t *testing.T) {
	obj := &metav1.PartialObjectMetadata{}
	if !InShard(obj, "") || InShard(obj, "experimental") {
		t.Error("unlabeled object must belong to the default shard only")
	}
	obj.SetLabels(map[string]string{ShardLabel: "experimental"})
	if !InShard(obj, "experimental") || InShard(obj, "") {
		t.Error("labeled object must belong to its shard only")
	}
}

func TestPropagateShardLabel(t *testing.T) {
	src := &metav1.PartialObjectMetadata{}
	dst := &metav1.PartialObjectMetadata{}
	PropagateShardLabel(dst, src)
	if _, ok := dst.GetLabels()[ShardLabel]; ok {
		t.Error("propagation must not invent a shard label")
	}

	src.SetLabels(map[string]string{ShardLabel: "experimental"})
	PropagateShardLabel(dst, src)
	if dst.GetLabels()[ShardLabel] != "experimental" {
		t.Error("propagation must copy the shard label")
	}
}

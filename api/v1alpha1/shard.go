// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

// ShardSelector returns the label selector matching exactly the resources an
// operator instance with the given shard is responsible for. An empty shard
// matches resources without the shard label, a non-empty shard matches
// resources labeled ShardLabel=<shard>.
func ShardSelector(shard string) (labels.Selector, error) {
	operator, values := selection.DoesNotExist, []string(nil)
	if shard != "" {
		operator, values = selection.Equals, []string{shard}
	}
	requirement, err := labels.NewRequirement(ShardLabel, operator, values)
	if err != nil {
		return nil, fmt.Errorf("invalid shard %q: %w", shard, err)
	}
	return labels.NewSelector().Add(*requirement), nil
}

// InShard reports whether the object belongs to the given shard. An empty
// shard only owns unlabeled objects.
func InShard(obj metav1.Object, shard string) bool {
	return obj.GetLabels()[ShardLabel] == shard
}

// PropagateShardLabel copies the shard label from src to dst if src carries one.
// Controllers that create child resources use this to keep children visible to
// the same shard instance as their parent.
func PropagateShardLabel(dst, src metav1.Object) {
	shard, ok := src.GetLabels()[ShardLabel]
	if !ok {
		return
	}
	objLabels := dst.GetLabels()
	if objLabels == nil {
		objLabels = map[string]string{}
	}
	objLabels[ShardLabel] = shard
	dst.SetLabels(objLabels)
}

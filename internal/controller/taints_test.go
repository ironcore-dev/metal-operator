// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("tolerates", func() {
	DescribeTable("taint/toleration matching",
		func(taints []metalv1alpha1.Taint, tolerations []metalv1alpha1.Toleration, expected bool) {
			Expect(tolerates(taints, tolerations)).To(Equal(expected))
		},
		Entry("empty taints always tolerated",
			nil, nil, true,
		),
		Entry("single NoBind taint matched by Exists toleration",
			[]metalv1alpha1.Taint{
				{Key: "dedicated", Value: "gpu", Effect: metalv1alpha1.TaintEffectNoBind},
			},
			[]metalv1alpha1.Toleration{
				{Key: "dedicated", Operator: metalv1alpha1.TolerationOperatorExists},
			},
			true,
		),
		Entry("single NoBind taint matched by Equal toleration with correct value",
			[]metalv1alpha1.Taint{
				{Key: "dedicated", Value: "gpu", Effect: metalv1alpha1.TaintEffectNoBind},
			},
			[]metalv1alpha1.Toleration{
				{Key: "dedicated", Operator: metalv1alpha1.TolerationOperatorEqual, Value: "gpu"},
			},
			true,
		),
		Entry("Equal toleration with wrong value does not match",
			[]metalv1alpha1.Taint{
				{Key: "dedicated", Value: "gpu", Effect: metalv1alpha1.TaintEffectNoBind},
			},
			[]metalv1alpha1.Toleration{
				{Key: "dedicated", Operator: metalv1alpha1.TolerationOperatorEqual, Value: "cpu"},
			},
			false,
		),
		Entry("single NoBind taint with no toleration returns false",
			[]metalv1alpha1.Taint{
				{Key: "dedicated", Value: "gpu", Effect: metalv1alpha1.TaintEffectNoBind},
			},
			nil,
			false,
		),
		Entry("toleration with empty effect matches any taint effect",
			[]metalv1alpha1.Taint{
				{Key: "dedicated", Value: "gpu", Effect: metalv1alpha1.TaintEffectNoBind},
			},
			[]metalv1alpha1.Toleration{
				{Key: "dedicated", Operator: metalv1alpha1.TolerationOperatorEqual, Value: "gpu", Effect: ""},
			},
			true,
		),
		Entry("toleration with specific effect must match taint effect",
			[]metalv1alpha1.Taint{
				{Key: "dedicated", Value: "gpu", Effect: metalv1alpha1.TaintEffectNoBind},
			},
			[]metalv1alpha1.Toleration{
				{Key: "dedicated", Operator: metalv1alpha1.TolerationOperatorEqual, Value: "gpu", Effect: metalv1alpha1.TaintEffectEvict},
			},
			false,
		),
		Entry("Evict taint is always tolerated even without matching toleration",
			[]metalv1alpha1.Taint{
				{Key: "evictme", Value: "yes", Effect: metalv1alpha1.TaintEffectEvict},
			},
			nil,
			true,
		),
		Entry("multiple taints, all tolerated",
			[]metalv1alpha1.Taint{
				{Key: "key1", Value: "val1", Effect: metalv1alpha1.TaintEffectNoBind},
				{Key: "key2", Value: "val2", Effect: metalv1alpha1.TaintEffectNoBind},
			},
			[]metalv1alpha1.Toleration{
				{Key: "key1", Operator: metalv1alpha1.TolerationOperatorEqual, Value: "val1"},
				{Key: "key2", Operator: metalv1alpha1.TolerationOperatorExists},
			},
			true,
		),
		Entry("multiple taints, one not tolerated returns false",
			[]metalv1alpha1.Taint{
				{Key: "key1", Value: "val1", Effect: metalv1alpha1.TaintEffectNoBind},
				{Key: "key2", Value: "val2", Effect: metalv1alpha1.TaintEffectNoBind},
			},
			[]metalv1alpha1.Toleration{
				{Key: "key1", Operator: metalv1alpha1.TolerationOperatorEqual, Value: "val1"},
			},
			false,
		),
		Entry("empty operator treated as Equal, matching value",
			[]metalv1alpha1.Taint{
				{Key: "rack", Value: "1", Effect: metalv1alpha1.TaintEffectNoBind},
			},
			[]metalv1alpha1.Toleration{
				{Key: "rack", Value: "1"},
			},
			true,
		),
		Entry("empty operator treated as Equal, non-matching value",
			[]metalv1alpha1.Taint{
				{Key: "rack", Value: "1", Effect: metalv1alpha1.TaintEffectNoBind},
			},
			[]metalv1alpha1.Toleration{
				{Key: "rack", Value: "2"},
			},
			false,
		),
	)
})

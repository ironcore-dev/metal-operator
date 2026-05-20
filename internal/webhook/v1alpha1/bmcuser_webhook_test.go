// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

var _ = Describe("BMCUser Webhook", func() {
	var (
		validator BMCUserCustomValidator
		ctx       context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		validator = BMCUserCustomValidator{
			Client: k8sClient,
		}
	})

	Context("ValidateCreate", func() {
		It("should accept valid TTL", func() {
			user := &metalv1alpha1.BMCUser{
				Spec: metalv1alpha1.BMCUserSpec{
					TTL: &metav1.Duration{Duration: 8 * time.Hour},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, user)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should accept valid ExpiresAt in future", func() {
			futureTime := metav1.NewTime(time.Now().Add(24 * time.Hour))
			user := &metalv1alpha1.BMCUser{
				Spec: metalv1alpha1.BMCUserSpec{
					ExpiresAt: &futureTime,
				},
			}

			warnings, err := validator.ValidateCreate(ctx, user)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should reject both TTL and ExpiresAt set", func() {
			futureTime := metav1.NewTime(time.Now().Add(24 * time.Hour))
			user := &metalv1alpha1.BMCUser{
				Spec: metalv1alpha1.BMCUserSpec{
					TTL:       &metav1.Duration{Duration: 8 * time.Hour},
					ExpiresAt: &futureTime,
				},
			}

			warnings, err := validator.ValidateCreate(ctx, user)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("mutually exclusive"))
			Expect(warnings).To(BeEmpty())
		})

		It("should reject ExpiresAt in the past", func() {
			pastTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
			user := &metalv1alpha1.BMCUser{
				Spec: metalv1alpha1.BMCUserSpec{
					ExpiresAt: &pastTime,
				},
			}

			warnings, err := validator.ValidateCreate(ctx, user)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must be in the future"))
			Expect(warnings).To(BeEmpty())
		})

		It("should reject negative TTL", func() {
			user := &metalv1alpha1.BMCUser{
				Spec: metalv1alpha1.BMCUserSpec{
					TTL: &metav1.Duration{Duration: -1 * time.Hour},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, user)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must be positive"))
			Expect(warnings).To(BeEmpty())
		})

		It("should reject TTL exceeding 1 week", func() {
			user := &metalv1alpha1.BMCUser{
				Spec: metalv1alpha1.BMCUserSpec{
					TTL: &metav1.Duration{Duration: 200 * time.Hour}, // More than 1 week
				},
			}

			warnings, err := validator.ValidateCreate(ctx, user)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exceeds maximum"))
			Expect(warnings).To(BeEmpty())
		})
	})

	Context("ValidateUpdate", func() {
		It("should accept valid update with future ExpiresAt", func() {
			oldUser := &metalv1alpha1.BMCUser{
				Spec: metalv1alpha1.BMCUserSpec{
					TTL: &metav1.Duration{Duration: 8 * time.Hour},
				},
			}

			futureTime := metav1.NewTime(time.Now().Add(24 * time.Hour))
			newUser := &metalv1alpha1.BMCUser{
				Spec: metalv1alpha1.BMCUserSpec{
					ExpiresAt: &futureTime,
				},
			}

			warnings, err := validator.ValidateUpdate(ctx, oldUser, newUser)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should reject update with past ExpiresAt", func() {
			oldUser := &metalv1alpha1.BMCUser{}
			pastTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
			newUser := &metalv1alpha1.BMCUser{
				Spec: metalv1alpha1.BMCUserSpec{
					ExpiresAt: &pastTime,
				},
			}

			_, err := validator.ValidateUpdate(ctx, oldUser, newUser)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must be in the future"))
		})

		It("should reject update with invalid TTL", func() {
			oldUser := &metalv1alpha1.BMCUser{}
			newUser := &metalv1alpha1.BMCUser{
				Spec: metalv1alpha1.BMCUserSpec{
					TTL: &metav1.Duration{Duration: -1 * time.Hour},
				},
			}

			_, err := validator.ValidateUpdate(ctx, oldUser, newUser)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must be positive"))
		})

		It("should warn when TTL changes after expiration calculated", func() {
			expiresAt := metav1.NewTime(time.Now().Add(24 * time.Hour))
			oldUser := &metalv1alpha1.BMCUser{
				Spec: metalv1alpha1.BMCUserSpec{
					TTL: &metav1.Duration{Duration: 8 * time.Hour},
				},
				Status: metalv1alpha1.BMCUserStatus{
					ExpiresAt: &expiresAt,
				},
			}

			newUser := &metalv1alpha1.BMCUser{
				Spec: metalv1alpha1.BMCUserSpec{
					TTL: &metav1.Duration{Duration: 12 * time.Hour},
				},
				Status: metalv1alpha1.BMCUserStatus{
					ExpiresAt: &expiresAt,
				},
			}

			warnings, err := validator.ValidateUpdate(ctx, oldUser, newUser)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(HaveLen(1))
			Expect(warnings[0]).To(ContainSubstring("TTL was changed"))
		})

		It("should warn when ExpiresAt changes after expiration calculated", func() {
			expiresAt := metav1.NewTime(time.Now().Add(24 * time.Hour))
			oldExpiresAt := metav1.NewTime(time.Now().Add(24 * time.Hour))
			newExpiresAt := metav1.NewTime(time.Now().Add(48 * time.Hour))

			oldUser := &metalv1alpha1.BMCUser{
				Spec: metalv1alpha1.BMCUserSpec{
					ExpiresAt: &oldExpiresAt,
				},
				Status: metalv1alpha1.BMCUserStatus{
					ExpiresAt: &expiresAt,
				},
			}

			newUser := &metalv1alpha1.BMCUser{
				Spec: metalv1alpha1.BMCUserSpec{
					ExpiresAt: &newExpiresAt,
				},
				Status: metalv1alpha1.BMCUserStatus{
					ExpiresAt: &expiresAt,
				},
			}

			warnings, err := validator.ValidateUpdate(ctx, oldUser, newUser)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(HaveLen(1))
			Expect(warnings[0]).To(ContainSubstring("ExpiresAt was changed"))
		})

		It("should warn when TTL is removed after expiration calculated", func() {
			expiresAt := metav1.NewTime(time.Now().Add(24 * time.Hour))
			oldUser := &metalv1alpha1.BMCUser{
				Spec: metalv1alpha1.BMCUserSpec{
					TTL: &metav1.Duration{Duration: 8 * time.Hour},
				},
				Status: metalv1alpha1.BMCUserStatus{
					ExpiresAt: &expiresAt,
				},
			}

			newUser := &metalv1alpha1.BMCUser{
				Spec: metalv1alpha1.BMCUserSpec{
					TTL: nil,
				},
				Status: metalv1alpha1.BMCUserStatus{
					ExpiresAt: &expiresAt,
				},
			}

			warnings, err := validator.ValidateUpdate(ctx, oldUser, newUser)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(HaveLen(1))
			Expect(warnings[0]).To(ContainSubstring("TTL was changed"))
		})

		It("should warn when switching from TTL to ExpiresAt after expiration calculated", func() {
			expiresAt := metav1.NewTime(time.Now().Add(24 * time.Hour))
			newExpiresAt := metav1.NewTime(time.Now().Add(48 * time.Hour))

			oldUser := &metalv1alpha1.BMCUser{
				Spec: metalv1alpha1.BMCUserSpec{
					TTL: &metav1.Duration{Duration: 8 * time.Hour},
				},
				Status: metalv1alpha1.BMCUserStatus{
					ExpiresAt: &expiresAt,
				},
			}

			newUser := &metalv1alpha1.BMCUser{
				Spec: metalv1alpha1.BMCUserSpec{
					ExpiresAt: &newExpiresAt,
				},
				Status: metalv1alpha1.BMCUserStatus{
					ExpiresAt: &expiresAt,
				},
			}

			warnings, err := validator.ValidateUpdate(ctx, oldUser, newUser)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(HaveLen(2)) // Both TTL and ExpiresAt changed
			Expect(warnings[0]).To(ContainSubstring("TTL was changed"))
			Expect(warnings[1]).To(ContainSubstring("ExpiresAt was changed"))
		})
	})

	Context("Helper functions", func() {
		It("ttlChanged should detect nil to non-nil change", func() {
			Expect(ttlChanged(nil, &metav1.Duration{Duration: 1 * time.Hour})).To(BeTrue())
		})

		It("ttlChanged should detect non-nil to nil change", func() {
			Expect(ttlChanged(&metav1.Duration{Duration: 1 * time.Hour}, nil)).To(BeTrue())
		})

		It("ttlChanged should detect value change", func() {
			Expect(ttlChanged(
				&metav1.Duration{Duration: 1 * time.Hour},
				&metav1.Duration{Duration: 2 * time.Hour},
			)).To(BeTrue())
		})

		It("ttlChanged should return false for same value", func() {
			Expect(ttlChanged(
				&metav1.Duration{Duration: 1 * time.Hour},
				&metav1.Duration{Duration: 1 * time.Hour},
			)).To(BeFalse())
		})

		It("ttlChanged should return false for both nil", func() {
			Expect(ttlChanged(nil, nil)).To(BeFalse())
		})

		It("expiresAtChanged should detect nil to non-nil change", func() {
			t := metav1.NewTime(time.Now())
			Expect(expiresAtChanged(nil, &t)).To(BeTrue())
		})

		It("expiresAtChanged should detect non-nil to nil change", func() {
			t := metav1.NewTime(time.Now())
			Expect(expiresAtChanged(&t, nil)).To(BeTrue())
		})

		It("expiresAtChanged should detect time change", func() {
			t1 := metav1.NewTime(time.Now())
			t2 := metav1.NewTime(time.Now().Add(1 * time.Hour))
			Expect(expiresAtChanged(&t1, &t2)).To(BeTrue())
		})

		It("expiresAtChanged should return false for same time", func() {
			t := metav1.NewTime(time.Now())
			Expect(expiresAtChanged(&t, &t)).To(BeFalse())
		})

		It("expiresAtChanged should return false for both nil", func() {
			Expect(expiresAtChanged(nil, nil)).To(BeFalse())
		})
	})

	Context("Integration with BMCRef", func() {
		It("should validate BMCUser with valid TTL and BMCRef", func() {
			user := &metalv1alpha1.BMCUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					Namespace: "default",
				},
				Spec: metalv1alpha1.BMCUserSpec{
					UserName: "admin",
					RoleID:   "Administrator",
					TTL:      &metav1.Duration{Duration: 8 * time.Hour},
					BMCRef: &corev1.LocalObjectReference{
						Name: "test-bmc",
					},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, user)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})
})

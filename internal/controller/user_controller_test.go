// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"time"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("User Controller", func() {
	ns := SetupTest()

	var bmc *metalv1alpha1.BMC

	BeforeEach(func(ctx SpecContext) {
		By("Ensuring clean state")
		endpoint := &metalv1alpha1.Endpoint{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-server-",
			},
			Spec: metalv1alpha1.EndpointSpec{
				// emulator BMC mac address
				MACAddress: "23:11:8A:33:CF:EA",
				IP:         metalv1alpha1.MustParseIP("127.0.0.1"),
			},
		}
		Expect(k8sClient.Create(ctx, endpoint)).To(Succeed())
		By("Ensuring that the BMC resource has been created for an endpoint")
		bmc = &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: endpoint.Name,
			},
		}
		Eventually(Get(bmc)).Should(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		DeleteAllMetalResources(ctx, ns.Name)
	})

	It("Should initialeze user", func(ctx SpecContext) {
		By("Creating a User resource")
		user := &metalv1alpha1.User{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-user",
			},
			Spec: metalv1alpha1.UserSpec{
				UserName: "foo",
				RoleID:   "Administrator",
				BMCRef: &v1.LocalObjectReference{
					Name: bmc.Name,
				},
				BMCSecretRef: &v1.LocalObjectReference{
					Name: "test-user-secret",
				},
			},
		}
		By("Creating a User resource")
		Expect(k8sClient.Create(ctx, user)).To(Succeed())
		By("Ensuring that the User resource has been created")
		Eventually(Get(user)).Should(Succeed())

		By("Creating a BMCSecret for the User")

		bmcSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-user-secret",
			},
			Data: map[string][]byte{
				"username": []byte("foo"),
				"password": []byte("bar"),
			},
		}
		Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())
		By("Ensuring that the BMCSecret has been created")
		Eventually(Get(bmcSecret)).Should(Succeed())

		// update bmc spec to use tbe user password
		Eventually(Update(bmc, func() {
			bmc.Spec.AdminUserRef = &v1.LocalObjectReference{
				Name: user.Name,
			}
		})).Should(Succeed())

		By("Ensuring that the User resource has been patched with the BMC secret reference")
		Eventually(Object(user)).Should(SatisfyAll(
			HaveField("Status.EffectiveBMCSecretRef", Not(BeNil())),
		))

		By("Ensuring the effective bmcSecret has been created")
		effectiveSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name: user.Status.EffectiveBMCSecretRef.Name,
			},
		}
		Eventually(Get(effectiveSecret)).Should(Succeed())
	})
	It("Should create a new bmcSecret if missing", func(ctx SpecContext) {
		By("Creating an Endpoint object")
		endpoint := &metalv1alpha1.Endpoint{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-server-",
			},
			Spec: metalv1alpha1.EndpointSpec{
				// emulator BMC mac address
				MACAddress: "23:11:8A:33:CF:EA",
				IP:         metalv1alpha1.MustParseIP("127.0.0.1"),
			},
		}
		Expect(k8sClient.Create(ctx, endpoint)).To(Succeed())

		By("Ensuring that the BMC resource has been created for an endpoint")
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: endpoint.Name,
			},
		}
		Eventually(Get(bmc)).Should(Succeed())

		By("Creating a BMCSecret for the User")

		bmcSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-bmc-secret",
			},
			Data: map[string][]byte{
				"username": []byte("foo"),
				"password": []byte("bar"),
			},
		}
		Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())
		By("Ensuring that the BMCSecret has been created")
		Eventually(Get(bmcSecret)).Should(Succeed())

		adminUser := &metalv1alpha1.User{
			ObjectMeta: metav1.ObjectMeta{
				Name: "admin-user",
			},
			Spec: metalv1alpha1.UserSpec{
				UserName: "foo",
				RoleID:   "Administrator",
				BMCRef: &v1.LocalObjectReference{
					Name: bmc.Name,
				},
				BMCSecretRef: &v1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
			},
		}
		By("Creating a User resource")
		Expect(k8sClient.Create(ctx, adminUser)).To(Succeed())
		By("Ensuring that the User resource has been created")
		Eventually(Get(adminUser)).Should(Succeed())

		// update bmc spec to use tbe user password
		Eventually(Update(bmc, func() {
			bmc.Spec.AdminUserRef = &v1.LocalObjectReference{
				Name: adminUser.Name,
			}
		})).Should(Succeed())

		By("Creating a read only user without password")
		readonlyUser := &metalv1alpha1.User{
			ObjectMeta: metav1.ObjectMeta{
				Name: "readonly-user",
			},
			Spec: metalv1alpha1.UserSpec{
				UserName: "readonly",
				RoleID:   "ReadOnly",
				BMCRef: &v1.LocalObjectReference{
					Name: bmc.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, readonlyUser)).To(Succeed())
		Eventually(Get(readonlyUser)).Should(Succeed())

		By("Ensuring that the readonly User resource has EffectiveBMCSecretRef")
		Eventually(Object(readonlyUser)).Should(SatisfyAll(
			HaveField("Status.EffectiveBMCSecretRef", Not(BeNil())),
			HaveField("Spec.BMCSecretRef", Not(BeNil())),
		))
	})
	It("Should rotate password if rotationPeriod is set", func(ctx SpecContext) {
		By("Creating a BMCSecret for the User")
		bmcSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-user-secret",
			},
			Data: map[string][]byte{
				"username": []byte("foo"),
				"password": []byte("bar"),
			},
		}
		Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())
		By("Ensuring that the BMCSecret has been created")
		Eventually(Get(bmcSecret)).Should(Succeed())

		adminUser := &metalv1alpha1.User{
			ObjectMeta: metav1.ObjectMeta{
				Name: "admin-user",
			},
			Spec: metalv1alpha1.UserSpec{
				UserName: "foo",
				RoleID:   "Administrator",
				BMCRef: &v1.LocalObjectReference{
					Name: bmc.Name,
				},
				BMCSecretRef: &v1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
				RotationPolicy: &metav1.Duration{
					Duration: 1 * time.Second,
				},
			},
		}

		By("Creating a User resource")
		Expect(k8sClient.Create(ctx, adminUser)).To(Succeed())
		By("Ensuring that the User resource has been created")
		Eventually(Get(adminUser)).Should(Succeed())
		// update bmc spec to use tbe user password
		Eventually(Update(bmc, func() {
			bmc.Spec.AdminUserRef = &v1.LocalObjectReference{
				Name: adminUser.Name,
			}
		})).Should(Succeed())

		By("Ensuring that a new secret with new password has been rotated and set to EffectiveBMCSecretRef")
		Eventually(Object(adminUser), "2s").Should(SatisfyAll(
			HaveField("Status.LastRotation", Not(BeNil())),
			HaveField("Status.EffectiveBMCSecretRef", Not(BeNil())),
			HaveField("Status.EffectiveBMCSecretRef.Name", Not(Equal(bmcSecret.Name))),
		))
		newSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name: adminUser.Status.EffectiveBMCSecretRef.Name,
			},
		}
		Eventually(Get(newSecret)).Should(Succeed())
		Expect(newSecret.Data).To(Not(HaveKeyWithValue("password", []byte("bar"))))

	})

	It("Should just create additional bmc users", func(ctx SpecContext) {
		user01 := &metalv1alpha1.User{
			ObjectMeta: metav1.ObjectMeta{
				Name: "user01",
			},
			Spec: metalv1alpha1.UserSpec{
				UserName: "user01",
				RoleID:   "Readonly",
				BMCRef: &v1.LocalObjectReference{
					Name: bmc.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, user01)).To(Succeed())
		Eventually(Get(user01)).Should(Succeed())
		By("Ensuring that the User resource has EffectiveBMCSecretRef")
		Eventually(Object(user01), "4s").Should(SatisfyAll(
			HaveField("Status.EffectiveBMCSecretRef", Not(BeNil())),
			HaveField("Status.ID", Not(BeEmpty())),
		))
		By("Ensuring that the BMCSecret has been created")
		effectiveSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name: user01.Status.EffectiveBMCSecretRef.Name,
			},
		}
		Eventually(Get(effectiveSecret)).Should(Succeed())
		Expect(effectiveSecret.Data).To(HaveKeyWithValue("username", []byte("user01")))
		Expect(effectiveSecret.Data).To(HaveKeyWithValue("password", Not(BeEmpty())))

		By("Creating a second user with the same BMCRef")
		user02 := &metalv1alpha1.User{
			ObjectMeta: metav1.ObjectMeta{
				Name: "user02",
			},
			Spec: metalv1alpha1.UserSpec{
				UserName: "user02",
				RoleID:   "Readonly",
				BMCRef: &v1.LocalObjectReference{
					Name: bmc.Name,
				},
				BMCSecretRef: &v1.LocalObjectReference{
					Name: user01.Status.EffectiveBMCSecretRef.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, user02)).To(Succeed())
		Eventually(Get(user02)).Should(Succeed())
		By("Ensuring that the User resource has EffectiveBMCSecretRef")
		Eventually(Object(user02)).Should(SatisfyAll(
			HaveField("Status.EffectiveBMCSecretRef", Not(BeNil())),
			HaveField("Status.ID", Not(BeEmpty())),
		))
		By("Ensuring that the BMCSecret has been created")
		effectiveSecret02 := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name: user02.Status.EffectiveBMCSecretRef.Name,
			},
		}
		Eventually(Get(effectiveSecret02)).Should(Succeed())
	})
})

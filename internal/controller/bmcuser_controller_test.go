// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"time"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	metalBMC "github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("BMCUser Controller", func() {
	_ = SetupTest(nil)

	var bmc *metalv1alpha1.BMC
	var bmcSecret *metalv1alpha1.BMCSecret

	BeforeEach(func(ctx SpecContext) {
		By("Creating a BMCSecret for the User")
		bmcSecret = &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-user-secret",
			},
			Data: map[string][]byte{
				"username": []byte("admin"),
				"password": []byte("adminpass"),
			},
		}
		Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())
		By("Ensuring that the BMCSecret has been created")
		Eventually(Get(bmcSecret)).Should(Succeed())

		By("Creating a BMC resource")
		bmc = &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-",
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP("127.0.0.1"),
					MACAddress: "23:11:8A:33:CF:EA",
				},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolRedfishLocal,
					Port: 8000,
				},
				BMCSecretRef: v1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
		By("Ensuring that the BMC resource has been created")
		Eventually(Get(bmc)).Should(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: bmcutils.GetServerNameFromBMCandIndex(0, bmc),
			},
		}
		Eventually(Get(server)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, server)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, bmc)).Should(Succeed())
		EnsureCleanState()
	})

	It("Should create a bmc user and secret", func(ctx SpecContext) {
		By("Creating a User resource")
		user := &metalv1alpha1.BMCUser{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-user",
			},
			Spec: metalv1alpha1.BMCUserSpec{
				BMCUserTemplate: metalv1alpha1.BMCUserTemplate{
					UserName: "user",
					RoleID:   "ReadOnly",
				},
				BMCRef: &v1.LocalObjectReference{
					Name: bmc.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, user)).To(Succeed())
		By("Ensuring that the User resource has been created")
		Eventually(Get(user)).Should(Succeed())

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
		Eventually(Object(effectiveSecret)).Should(SatisfyAll(
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "BMCUser",
				Name:               user.Name,
				UID:                user.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
		))

		By("Ensuring the effective bmcSecret has the correct data")
		Expect(effectiveSecret.Data).To(HaveKeyWithValue("username", []byte("user")))
		password := string(effectiveSecret.Data["password"])
		// make sure that the password has a length of 30 (default max length for redfish mock server)
		Expect(password).To(HaveLen(30))

		// set delete option to Background to ensure the secret is deleted when the user is deleted
		By("Deleting the User resource and ensuring the effective secret is deleted")

		Expect(k8sClient.Delete(ctx, user)).To(Succeed())
		By("Ensuring that the User resource has been deleted")
		Eventually(Get(user)).Should(Satisfy(apierrors.IsNotFound))
		By("Ensuring that the effective BMCSecret has been deleted")
		Expect(k8sClient.Delete(ctx, effectiveSecret)).To(Succeed())
		Eventually(Get(effectiveSecret)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("Should just create additional bmc users", func(ctx SpecContext) {
		user01 := &metalv1alpha1.BMCUser{
			ObjectMeta: metav1.ObjectMeta{
				Name: "user01",
			},
			Spec: metalv1alpha1.BMCUserSpec{
				BMCUserTemplate: metalv1alpha1.BMCUserTemplate{
					UserName: "user01",
					RoleID:   "Readonly",
				},
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
		user02 := &metalv1alpha1.BMCUser{
			ObjectMeta: metav1.ObjectMeta{
				Name: "user02",
			},
			Spec: metalv1alpha1.BMCUserSpec{
				BMCUserTemplate: metalv1alpha1.BMCUserTemplate{
					UserName: "user02",
					RoleID:   "Readonly",
				},
				BMCRef: &v1.LocalObjectReference{
					Name: bmc.Name,
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

		user03Secret := metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "user03-secret",
			},
			Data: map[string][]byte{
				"username": []byte("user03"),
				"password": []byte("userpass"),
			},
		}

		By("Creating a BMCSecret for the third User")
		Expect(k8sClient.Create(ctx, &user03Secret)).To(Succeed())
		Eventually(Get(&user03Secret)).Should(Succeed())

		By("Creating a second user with the same BMCRef")
		user03 := &metalv1alpha1.BMCUser{
			ObjectMeta: metav1.ObjectMeta{
				Name: "user03",
			},
			Spec: metalv1alpha1.BMCUserSpec{
				BMCUserTemplate: metalv1alpha1.BMCUserTemplate{
					UserName: "user03",
					RoleID:   "Readonly",
					BMCSecretRef: &v1.LocalObjectReference{
						Name: user03Secret.Name,
					},
				},
				BMCRef: &v1.LocalObjectReference{
					Name: bmc.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, user03)).To(Succeed())
		Eventually(Get(user03)).Should(Succeed())
		By("Ensuring that the User resource has EffectiveBMCSecretRef")
		Eventually(Object(user03)).Should(SatisfyAll(
			HaveField("Status.EffectiveBMCSecretRef", Equal(&v1.LocalObjectReference{
				Name: user03Secret.Name,
			})),
			HaveField("Status.ID", Not(BeEmpty())),
		))
		By("Ensuring that the BMCSecret has been created")
		Expect(k8sClient.Delete(ctx, user01)).To(Succeed())
		Expect(k8sClient.Delete(ctx, user02)).To(Succeed())
		Expect(k8sClient.Delete(ctx, user03)).To(Succeed())
		Expect(k8sClient.Delete(ctx, &user03Secret)).To(Succeed())
		Expect(k8sClient.Delete(ctx, effectiveSecret)).To(Succeed())
		Expect(k8sClient.Delete(ctx, effectiveSecret02)).To(Succeed())
	})

	It("Should rotate password if rotationPeriod is set", func(ctx SpecContext) {
		adminUser := &metalv1alpha1.BMCUser{
			ObjectMeta: metav1.ObjectMeta{
				Name: "admin-user",
			},
			Spec: metalv1alpha1.BMCUserSpec{
				BMCUserTemplate: metalv1alpha1.BMCUserTemplate{
					UserName: "admin-user",
					RoleID:   "Administrator",
					BMCSecretRef: &v1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
					RotationPeriod: &metav1.Duration{
						Duration: 1 * time.Second,
					},
				},
				BMCRef: &v1.LocalObjectReference{
					Name: bmc.Name,
				},
			},
		}
		By("Creating a User resource")
		Expect(k8sClient.Create(ctx, adminUser)).To(Succeed())
		By("Ensuring that the User resource has been created")
		Eventually(Get(adminUser)).Should(Succeed())

		By("Ensuring that a new secret with new password has been rotated and set to EffectiveBMCSecretRef")
		Eventually(Object(adminUser), "4s").Should(SatisfyAll(
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
		Expect(k8sClient.Delete(ctx, adminUser)).To(Succeed())
		Expect(k8sClient.Delete(ctx, newSecret)).To(Succeed())
	})

	It("Should delete bmc user and secret on User deletion", func(ctx SpecContext) {
		metalBMC.UnitTestMockUps.InitializeDefaults()
		By("Creating a User resource")
		user := &metalv1alpha1.BMCUser{
			ObjectMeta: metav1.ObjectMeta{
				Name: "delete-user",
			},
			Spec: metalv1alpha1.BMCUserSpec{
				BMCUserTemplate: metalv1alpha1.BMCUserTemplate{
					UserName: "deleteUser",
					RoleID:   "ReadOnly",
				},
				BMCRef: &v1.LocalObjectReference{
					Name: bmc.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, user)).To(Succeed())
		By("Ensuring that the User resource has been created")
		Eventually(Get(user)).Should(Succeed())
		Eventually(metalBMC.UnitTestMockUps.Accounts).Should(HaveKey("deleteUser"))

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

		By("Deleting the User resource")
		Expect(k8sClient.Delete(ctx, user)).To(Succeed())

		By("Ensuring that the User resource has been deleted")
		Eventually(Get(user)).ShouldNot(Succeed())

		By("Ensuring that the BMC user has been deleted")
		Eventually(metalBMC.UnitTestMockUps.Accounts).ShouldNot(HaveKey("deleteUser"))

		By("Ensuring that the effective BMCSecret has been deleted")
		Expect(k8sClient.Delete(ctx, effectiveSecret)).To(Succeed())
	})

	It("Should rotate password if OperationAnnotationRotateCredentials is set", func(ctx SpecContext) {
		By("Creating a User resource")
		user := &metalv1alpha1.BMCUser{
			ObjectMeta: metav1.ObjectMeta{
				Name: "annotated-user",
			},
			Spec: metalv1alpha1.BMCUserSpec{
				BMCUserTemplate: metalv1alpha1.BMCUserTemplate{
					UserName: "annotated-user",
					RoleID:   "ReadOnly",
				},
				BMCRef: &v1.LocalObjectReference{
					Name: bmc.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, user)).To(Succeed())
		By("Ensuring that the User resource has been created")
		Eventually(Get(user)).Should(Succeed())

		By("Ensuring that the User resource has EffectiveBMCSecretRef")
		Eventually(Object(user)).Should(SatisfyAll(
			HaveField("Status.EffectiveBMCSecretRef", Not(BeNil())),
		))

		initialSecretName := ""
		By("Getting the initial effective secret name")
		Eventually(Object(user)).Should(WithTransform(func(u *metalv1alpha1.BMCUser) string {
			initialSecretName = u.Status.EffectiveBMCSecretRef.Name
			return initialSecretName
		}, Not(BeEmpty())))

		By("Adding the rotation annotation to the User resource")
		Eventually(Object(user)).Should(SatisfyAll(
			HaveField("ObjectMeta.Annotations", BeNil()),
		))
		Eventually(Update(user, func() {
			user.Annotations = map[string]string{
				metalv1alpha1.OperationAnnotation: metalv1alpha1.OperationAnnotationRotateCredentials,
			}
		})).Should(Succeed())

		By("Ensuring that a new secret with new password has been rotated and set to EffectiveBMCSecretRef")
		Eventually(Object(user), "4s").Should(SatisfyAll(
			HaveField("Status.EffectiveBMCSecretRef", Not(BeNil())),
			HaveField("Status.EffectiveBMCSecretRef.Name", Not(Equal(initialSecretName))),
		))

		newSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name: user.Status.EffectiveBMCSecretRef.Name,
			},
		}
		Eventually(Get(newSecret)).Should(Succeed())

		By("Checking that the rotation annotation has been removed")
		Eventually(Object(user)).Should(SatisfyAll(
			HaveField("ObjectMeta.Annotations", BeNil()),
		))

		Expect(k8sClient.Delete(ctx, user)).To(Succeed())
		Expect(k8sClient.Delete(ctx, newSecret)).To(Succeed())
		Expect(k8sClient.Delete(ctx, &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name: initialSecretName,
			},
		})).To(Succeed())
	})

})

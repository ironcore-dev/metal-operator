// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"
	"net/http"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/controller/testdata"
	"github.com/ironcore-dev/metal-operator/internal/probe"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("Server Controller", func() {
	ns := SetupTest()

	var endpoint *metalv1alpha1.Endpoint
	var bmc *metalv1alpha1.BMC

	It("Should initialize a Server from Endpoint", func(ctx SpecContext) {
		By("Creating an Endpoint object")
		endpoint = &metalv1alpha1.Endpoint{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.EndpointSpec{
				// emulator BMC mac address
				MACAddress: "23:11:8A:33:CF:EA",
				IP:         metalv1alpha1.MustParseIP("127.0.0.1"),
			},
		}
		Expect(k8sClient.Create(ctx, endpoint)).To(Succeed())
		DeferCleanup(k8sClient.Delete, endpoint)

		By("Ensuring that the BMC resource has been created for an endpoint")
		bmc = &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: endpoint.Name,
			},
		}
		Eventually(Get(bmc)).Should(Succeed())
		DeferCleanup(k8sClient.Delete, bmc)

		By("Ensuring that the BMCSecret will be removed")
		bmcSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name: bmc.Name,
			},
		}
		Eventually(Get(bmcSecret)).Should(Succeed())
		DeferCleanup(k8sClient.Delete, bmcSecret)

		By("Ensuring that the Server resource has been created")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("%s-system-0", bmc.Name),
			},
		}
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Finalizers", ContainElement(ServerFinalizer)),
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "BMC",
				Name:               bmc.Name,
				UID:                bmc.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
			HaveField("Spec.UUID", "38947555-7742-3448-3784-823347823834"),
			HaveField("Spec.Power", metalv1alpha1.PowerOff),
			HaveField("Spec.IndicatorLED", metalv1alpha1.IndicatorLED("")),
			HaveField("Spec.ServerClaimRef", BeNil()),
			HaveField("Status.Manufacturer", "Contoso"),
			HaveField("Status.Model", "3500"),
			HaveField("Status.SKU", "8675309"),
			HaveField("Status.SerialNumber", "437XR1138R2"),
			HaveField("Status.IndicatorLED", metalv1alpha1.OffIndicatorLED),
			HaveField("Status.State", metalv1alpha1.ServerStateDiscovery),
			HaveField("Status.PowerState", metalv1alpha1.ServerOffPowerState),
			HaveField("Status.Storages", ContainElement(metalv1alpha1.Storage{
				Name:       "SATA Bay 1",
				Rotational: false,
				Capacity:   *resource.NewQuantity(8000000000000, resource.BinarySI),
				Vendor:     "Contoso",
				Model:      "3000GT8",
				State:      metalv1alpha1.StorageStateEnabled,
			})),
			HaveField("Status.Storages", HaveLen(4)),
		))
		DeferCleanup(k8sClient.Delete, server)

		By("Ensuring the boot configuration has been created")
		bootConfig := &metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      server.Name,
			},
		}
		Eventually(Object(bootConfig)).Should(SatisfyAll(
			HaveField("Annotations", HaveKeyWithValue(InternalAnnotationTypeKeyName, InternalAnnotationTypeValue)),
			HaveField("Spec.ServerRef", v1.LocalObjectReference{Name: server.Name}),
			HaveField("Spec.Image", "fooOS:latest"),
			HaveField("Spec.IgnitionSecretRef", &v1.LocalObjectReference{Name: server.Name}),
			HaveField("Status.State", metalv1alpha1.ServerBootConfigurationStatePending),
		))

		By("Ensuring that the default ignition configuration has been created")
		ignitionSecret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      bootConfig.Name,
			},
		}
		Eventually(Object(ignitionSecret)).Should(SatisfyAll(
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "ServerBootConfiguration",
				Name:               bootConfig.Name,
				UID:                bootConfig.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
			HaveField("Data", HaveKeyWithValue("ignition", MatchYAML(testdata.DefaultIgnition))),
		))

		By("Patching the boot configuration to a Ready state")
		Eventually(UpdateStatus(bootConfig, func() {
			bootConfig.Status.State = metalv1alpha1.ServerBootConfigurationStateReady
		})).Should(Succeed())

		By("Ensuring that the Server is set to discovery and powered on")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Finalizers", ContainElement(ServerFinalizer)),
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "BMC",
				Name:               bmc.Name,
				UID:                bmc.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
			HaveField("Spec.Power", metalv1alpha1.Power("On")),
			HaveField("Spec.BootConfigurationRef", &v1.ObjectReference{
				Kind:       "ServerBootConfiguration",
				Namespace:  ns.Name,
				Name:       server.Name,
				UID:        bootConfig.UID,
				APIVersion: "metal.ironcore.dev/v1alpha1",
			}),
			HaveField("Status.State", metalv1alpha1.ServerStateDiscovery),
		))

		By("Starting the probe agent")
		probeAgent := probe.NewAgent(server.Spec.UUID, registryURL, 100*time.Millisecond)
		go func() {
			defer GinkgoRecover()
			Expect(probeAgent.Start(ctx)).To(Succeed(), "failed to start probe agent")
		}()

		By("Ensuring that the server is set to available and powered off")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BootConfigurationRef", BeNil()),
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
			HaveField("Status.PowerState", metalv1alpha1.ServerOffPowerState),
			HaveField("Status.NetworkInterfaces", Not(BeEmpty())),
		))

		By("Ensuring that the boot configuration has been removed")
		Consistently(Get(bootConfig)).Should(Satisfy(apierrors.IsNotFound))

		By("Ensuring that the server is removed from the registry")
		response, err := http.Get(registryURL + "/systems/" + server.Spec.UUID)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusNotFound))
	})

	It("Should initialize a Server with inline BMC configuration", func(ctx SpecContext) {
		By("Creating a BMCSecret")
		bmcSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Data: map[string][]byte{
				"username": []byte("foo"),
				"password": []byte("bar"),
			},
		}
		Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())
		DeferCleanup(k8sClient.Delete, bmcSecret)

		By("Creating a Server with inline BMC configuration")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "server-",
			},
			Spec: metalv1alpha1.ServerSpec{
				UUID: "38947555-7742-3448-3784-823347823834",
				BMC: &metalv1alpha1.BMCAccess{
					Protocol: metalv1alpha1.Protocol{
						Name: metalv1alpha1.ProtocolRedfishLocal,
						Port: 8000,
					},
					Endpoint: "127.0.0.1",
					BMCSecretRef: v1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())
		DeferCleanup(k8sClient.Delete, server)

		By("Ensuring the boot configuration has been created")
		bootConfig := &metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      server.Name,
			},
		}
		Eventually(Object(bootConfig)).Should(SatisfyAll(
			HaveField("Annotations", HaveKeyWithValue(InternalAnnotationTypeKeyName, InternalAnnotationTypeValue)),
			HaveField("Spec.ServerRef", v1.LocalObjectReference{Name: server.Name}),
			HaveField("Spec.Image", "fooOS:latest"),
			HaveField("Spec.IgnitionSecretRef", &v1.LocalObjectReference{Name: server.Name}),
			HaveField("Status.State", metalv1alpha1.ServerBootConfigurationStatePending),
		))

		By("Ensuring that the default ignition configuration has been created")
		ignitionSecret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      bootConfig.Name,
			},
		}
		Eventually(Object(ignitionSecret)).Should(SatisfyAll(
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "ServerBootConfiguration",
				Name:               bootConfig.Name,
				UID:                bootConfig.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
			HaveField("Data", HaveKeyWithValue("ignition", MatchYAML(testdata.DefaultIgnition))),
		))

		By("Patching the boot configuration to a Ready state")
		Eventually(UpdateStatus(bootConfig, func() {
			bootConfig.Status.State = metalv1alpha1.ServerBootConfigurationStateReady
		})).Should(Succeed())

		By("Ensuring that the Server resource has been created")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Finalizers", ContainElement(ServerFinalizer)),
			HaveField("Spec.UUID", "38947555-7742-3448-3784-823347823834"),
			HaveField("Spec.Power", metalv1alpha1.Power("On")),
			HaveField("Spec.IndicatorLED", metalv1alpha1.IndicatorLED("")),
			HaveField("Spec.ServerClaimRef", BeNil()),
			HaveField("Spec.BootConfigurationRef", &v1.ObjectReference{
				Kind:       "ServerBootConfiguration",
				Namespace:  ns.Name,
				Name:       server.Name,
				UID:        bootConfig.UID,
				APIVersion: "metal.ironcore.dev/v1alpha1",
			}),
			HaveField("Status.Manufacturer", "Contoso"),
			HaveField("Status.SKU", "8675309"),
			HaveField("Status.SerialNumber", "437XR1138R2"),
			HaveField("Status.IndicatorLED", metalv1alpha1.OffIndicatorLED),
			HaveField("Status.State", metalv1alpha1.ServerStateDiscovery),
		))

		By("Ensuring that the server is set back to initial due to the discovery check timing out")
		Eventually(Object(server), "500ms").Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateInitial),
		))

		By("Starting the probe agent")
		probeAgent := probe.NewAgent(server.Spec.UUID, registryURL, 100*time.Millisecond)
		go func() {
			defer GinkgoRecover()
			Expect(probeAgent.Start(ctx)).To(Succeed(), "failed to start probe agent")
		}()

		By("Ensuring that the server is set to available and powered off")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BootConfigurationRef", BeNil()),
			HaveField("Spec.Power", metalv1alpha1.PowerOff),
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
			HaveField("Status.PowerState", metalv1alpha1.ServerOffPowerState),
			HaveField("Status.NetworkInterfaces", Not(BeEmpty())),
		))

		By("Ensuring that the boot configuration has been removed")
		Consistently(Get(bootConfig)).Should(Satisfy(apierrors.IsNotFound))

		By("Ensuring that the server is removed from the registry")
		response, err := http.Get(registryURL + "/systems/" + server.Spec.UUID)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusNotFound))
	})
})

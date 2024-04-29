/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	metalv1alpha1 "github.com/afritzler/metal-operator/api/v1alpha1"
	"github.com/afritzler/metal-operator/internal/controller/testdata"
	"github.com/afritzler/metal-operator/internal/probe"
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

	It("should initialize a server", func(ctx SpecContext) {
		By("Creating an Endpoints object")
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
				Name: fmt.Sprintf("bmc-%s", endpoint.Name),
			},
		}
		Eventually(Get(bmc)).Should(Succeed())

		By("Ensuring the boot configuration has been created")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("compute-0-%s", bmc.Name),
			},
		}
		bootConfig := &metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      server.Name,
			},
		}
		Eventually(Object(bootConfig)).Should(SatisfyAll(
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

		By("Ensuring that the Server resource has been created")
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
			HaveField("Spec.Power", metalv1alpha1.Power("")),
			HaveField("Spec.IndicatorLED", metalv1alpha1.IndicatorLED("")),
			HaveField("Spec.ServerClaimRef", BeNil()),
			HaveField("Spec.BMCRef", &v1.LocalObjectReference{Name: bmc.Name}),
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
			HaveField("Status.State", metalv1alpha1.ServerStateInitial),
		))

		By("Starting the probe agent")
		probeAgent := probe.NewAgent(server.Spec.UUID, registryURL)
		go func() {
			defer GinkgoRecover()
			Expect(probeAgent.Start(ctx)).To(Succeed(), "failed to start probe agent")
		}()

		By("Patching the boot configuration to a Ready state")
		Eventually(UpdateStatus(bootConfig, func() {
			bootConfig.Status.State = metalv1alpha1.ServerBootConfigurationStateReady
		})).Should(Succeed())

		By("Ensuring that the server is set to available and powered off")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BootConfigurationRef", BeNil()),
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
			HaveField("Status.PowerState", metalv1alpha1.ServerOffPowerState),
			HaveField("Status.NetworkInterfaces", Not(BeEmpty())),
		))

		By("Ensuring that the boot configuration has been removed")
		config := &metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      server.Name,
			},
		}
		Eventually(Get(config)).Should(Satisfy(apierrors.IsNotFound))
	})

	// TODO: test server with manual BMC registration
})

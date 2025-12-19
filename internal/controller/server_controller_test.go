// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v3"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/api/registry"
	"github.com/ironcore-dev/metal-operator/internal/ignition"
	"github.com/ironcore-dev/metal-operator/internal/probe"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("Server Controller", func() {
	ns := SetupTest(nil)

	AfterEach(func(ctx SpecContext) {
		EnsureCleanState()
	})

	It("Should initialize a Server from Endpoint", func(ctx SpecContext) {
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

		By("Ensuring that the BMCSecret will be removed")
		bmcSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name: bmc.Name,
			},
		}
		Eventually(Get(bmcSecret)).Should(Succeed())

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
			HaveField("Spec.SystemUUID", "38947555-7742-3448-3784-823347823834"),
			HaveField("Spec.SystemURI", "/redfish/v1/Systems/437XR1138R2"),
			HaveField("Spec.Power", metalv1alpha1.PowerOff),
			HaveField("Spec.IndicatorLED", metalv1alpha1.IndicatorLED("")),
			HaveField("Spec.ServerClaimRef", BeNil()),
			HaveField("Status.Manufacturer", "Contoso"),
			HaveField("Status.BIOSVersion", "P79 v1.45 (12/06/2017)"),
			HaveField("Status.Model", "3500"),
			HaveField("Status.SKU", "8675309"),
			HaveField("Status.SerialNumber", "437XR1138R2"),
			HaveField("Status.IndicatorLED", metalv1alpha1.OffIndicatorLED),
			HaveField("Status.State", metalv1alpha1.ServerStateDiscovery),
			HaveField("Status.PowerState", metalv1alpha1.ServerOffPowerState),
			HaveField("Status.Processors", ConsistOf(
				metalv1alpha1.Processor{
					ID:             "CPU1",
					Type:           "CPU",
					Architecture:   "x86",
					InstructionSet: "x86-64",
					Manufacturer:   "Intel(R) Corporation",
					Model:          "Multi-Core Intel(R) Xeon(R) processor 7xxx Series",
					MaxSpeedMHz:    3700,
					TotalCores:     8,
					TotalThreads:   16,
				},
				metalv1alpha1.Processor{
					ID:   "CPU2",
					Type: "CPU",
				},
				metalv1alpha1.Processor{
					ID:             "FPGA1",
					Type:           "FPGA",
					Architecture:   "OEM",
					InstructionSet: "OEM",
					Manufacturer:   "Intel(R) Corporation",
					Model:          "Stratix 10",
				},
			)),
		))

		By("Ensuring the boot configuration has been created")
		bootConfig := &metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      server.Name,
			},
		}
		Eventually(Object(bootConfig)).Should(SatisfyAll(
			HaveField("Annotations", HaveKeyWithValue(InternalAnnotationTypeKeyName, InternalAnnotationTypeValue)),
			HaveField("Annotations", HaveKeyWithValue(IsDefaultServerBootConfigOSImageKeyName, "true")),
			HaveField("Spec.ServerRef", v1.LocalObjectReference{Name: server.Name}),
			HaveField("Spec.Image", "fooOS:latest"),
			HaveField("Spec.IgnitionSecretRef", &v1.LocalObjectReference{Name: server.Name}),
			HaveField("Status.State", metalv1alpha1.ServerBootConfigurationStatePending),
		))

		By("Ensuring that the SSH keypair has been created")
		sshSecret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      bootConfig.Name + "-ssh",
			},
		}
		Eventually(Object(sshSecret)).Should(SatisfyAll(
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "ServerBootConfiguration",
				Name:               bootConfig.Name,
				UID:                bootConfig.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
			HaveField("Data", HaveKeyWithValue(SSHKeyPairSecretPrivateKeyName, Not(BeNil()))),
			HaveField("Data", HaveKeyWithValue(SSHKeyPairSecretPublicKeyName, Not(BeEmpty()))),
			HaveField("Data", HaveKeyWithValue(SSHKeyPairSecretPasswordKeyName, Not(BeNil()))),
		))
		_, err := ssh.ParsePrivateKey(sshSecret.Data[SSHKeyPairSecretPrivateKeyName])
		Expect(err).NotTo(HaveOccurred())
		_, _, _, _, err = ssh.ParseAuthorizedKey(sshSecret.Data[SSHKeyPairSecretPublicKeyName])
		Expect(err).NotTo(HaveOccurred())

		By("Ensuring that the default ignition configuration has been created")
		ignitionSecret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      bootConfig.Name,
			},
		}
		Eventually(Get(ignitionSecret)).Should(Succeed())

		// Since the bycrypted password hash is not deterministic, we will extract if from the actual secret and
		// add it to our ignition data which we want to compare the rest with.
		var parsedData map[string]interface{}
		Expect(yaml.Unmarshal(ignitionSecret.Data[DefaultIgnitionSecretKeyName], &parsedData)).ToNot(HaveOccurred())

		passwd, ok := parsedData["passwd"].(map[string]interface{})
		Expect(ok).To(BeTrue())

		users, _ := passwd["users"].([]interface{})
		Expect(users).To(HaveLen(1))

		user, ok := users[0].(map[string]interface{})
		Expect(ok).To(BeTrue())
		Expect(user).To(HaveKeyWithValue("name", "metal"))

		passwordHash, ok := user["password_hash"].(string)
		Expect(ok).To(BeTrue(), "password_hash should be a string")

		err = bcrypt.CompareHashAndPassword([]byte(passwordHash), sshSecret.Data[SSHKeyPairSecretPasswordKeyName])
		Expect(err).ToNot(HaveOccurred(), "passwordHash should match the expected password")

		ignitionData, err := ignition.GenerateDefaultIgnitionData(ignition.Config{
			Image:        "foo:latest",
			Flags:        "--registry-url=http://localhost:30000 --server-uuid=38947555-7742-3448-3784-823347823834",
			SSHPublicKey: string(sshSecret.Data[SSHKeyPairSecretPublicKeyName]),
			PasswordHash: passwordHash,
		})
		Expect(err).NotTo(HaveOccurred())

		Eventually(Object(ignitionSecret)).Should(SatisfyAll(
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "ServerBootConfiguration",
				Name:               bootConfig.Name,
				UID:                bootConfig.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
			HaveField("Data", HaveKeyWithValue(DefaultIgnitionFormatKey, []byte("fcos"))),
			HaveField("Data", HaveKeyWithValue(DefaultIgnitionSecretKeyName, MatchYAML(ignitionData))),
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
			HaveField("Spec.Power", metalv1alpha1.PowerOn),
			HaveField("Spec.BootConfigurationRef", &metalv1alpha1.ObjectReference{
				Kind:       "ServerBootConfiguration",
				Namespace:  ns.Name,
				Name:       server.Name,
				UID:        bootConfig.UID,
				APIVersion: "metal.ironcore.dev/v1alpha1",
			}),
			HaveField("Status.State", metalv1alpha1.ServerStateDiscovery),
			HaveField("Status.Conditions", ContainElement(
				HaveField("Type", PoweringOnCondition),
			)),
		))

		By("Starting the probe agent")
		probeAgent := probe.NewAgent(GinkgoLogr, server.Spec.SystemUUID, registryURL, 100*time.Millisecond, 50*time.Millisecond, 250*time.Millisecond)
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
		response, err := http.Get(registryURL + "/systems/" + server.Spec.SystemUUID)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusNotFound))

		biosSettings := &metalv1alpha1.BIOSSettings{ObjectMeta: metav1.ObjectMeta{
			Name: "bios-settings",
		}}
		Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())
		Eventually(Update(server, func() {
			server.Spec.BIOSSettingsRef = &v1.LocalObjectReference{Name: biosSettings.Name}
		})).Should(Succeed())

		// cleanup
		Expect(k8sClient.Delete(ctx, endpoint)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, bmc)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, server)).Should(Succeed())
		Eventually(Get(biosSettings)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("Should initialize a Server with inline BMC configuration", func(ctx SpecContext) {
		By("Creating a BMCSecret")
		bmcSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-server-",
			},
			Data: map[string][]byte{
				"username": []byte("foo"),
				"password": []byte("bar"),
			},
		}
		Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())

		By("Creating a Server with inline BMC configuration")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "server-",
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "38947555-7742-3448-3784-823347823834",
				BMC: &metalv1alpha1.BMCAccess{
					Protocol: metalv1alpha1.Protocol{
						Name: metalv1alpha1.ProtocolRedfishLocal,
						Port: 8000,
					},
					Address: "127.0.0.1",
					BMCSecretRef: v1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())

		By("Ensuring the boot configuration has been created")
		bootConfig := &metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      server.Name,
			},
		}
		Eventually(Object(bootConfig)).Should(SatisfyAll(
			HaveField("Annotations", HaveKeyWithValue(InternalAnnotationTypeKeyName, InternalAnnotationTypeValue)),
			HaveField("Annotations", HaveKeyWithValue(IsDefaultServerBootConfigOSImageKeyName, "true")),
			HaveField("Spec.ServerRef", v1.LocalObjectReference{Name: server.Name}),
			HaveField("Spec.Image", "fooOS:latest"),
			HaveField("Spec.IgnitionSecretRef", &v1.LocalObjectReference{Name: server.Name}),
			HaveField("Status.State", metalv1alpha1.ServerBootConfigurationStatePending),
		))

		By("Ensuring that the SSH keypair has been created")
		sshSecret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      bootConfig.Name + "-ssh",
			},
		}
		Eventually(Object(sshSecret)).Should(SatisfyAll(
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "ServerBootConfiguration",
				Name:               bootConfig.Name,
				UID:                bootConfig.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
			HaveField("Data", HaveKeyWithValue(SSHKeyPairSecretPublicKeyName, Not(BeEmpty()))),
			HaveField("Data", HaveKeyWithValue(SSHKeyPairSecretPrivateKeyName, Not(BeEmpty()))),
			HaveField("Data", HaveKeyWithValue(SSHKeyPairSecretPasswordKeyName, Not(BeEmpty()))),
		))
		_, err := ssh.ParsePrivateKey(sshSecret.Data[SSHKeyPairSecretPrivateKeyName])
		Expect(err).NotTo(HaveOccurred())
		_, _, _, _, err = ssh.ParseAuthorizedKey(sshSecret.Data[SSHKeyPairSecretPublicKeyName])
		Expect(err).NotTo(HaveOccurred())

		By("Ensuring that the default ignition configuration has been created")
		ignitionSecret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      bootConfig.Name,
			},
		}
		Eventually(Get(ignitionSecret)).Should(Succeed())

		// Since the bycrypted password hash is not deterministic we will extract if from the actual secret and
		// add it to our ignition data which we want to compare the rest with.
		var parsedData map[string]interface{}
		Expect(yaml.Unmarshal(ignitionSecret.Data[DefaultIgnitionSecretKeyName], &parsedData)).ToNot(HaveOccurred())

		passwd, ok := parsedData["passwd"].(map[string]interface{})
		Expect(ok).To(BeTrue())

		users, _ := passwd["users"].([]interface{})
		Expect(users).To(HaveLen(1))

		user, ok := users[0].(map[string]interface{})
		Expect(ok).To(BeTrue())
		Expect(user).To(HaveKeyWithValue("name", "metal"))

		passwordHash, ok := user["password_hash"].(string)
		Expect(ok).To(BeTrue(), "password_hash should be a string")

		Expect(bcrypt.CompareHashAndPassword([]byte(passwordHash), sshSecret.Data[SSHKeyPairSecretPasswordKeyName])).Should(Succeed())

		ignitionData, err := ignition.GenerateDefaultIgnitionData(ignition.Config{
			Image:        "foo:latest",
			Flags:        "--registry-url=http://localhost:30000 --server-uuid=38947555-7742-3448-3784-823347823834",
			SSHPublicKey: string(sshSecret.Data[SSHKeyPairSecretPublicKeyName]),
			PasswordHash: passwordHash,
		})
		Expect(err).NotTo(HaveOccurred())

		Eventually(Object(ignitionSecret)).Should(SatisfyAll(
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "ServerBootConfiguration",
				Name:               bootConfig.Name,
				UID:                bootConfig.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
			HaveField("Data", HaveKeyWithValue(DefaultIgnitionFormatKey, []byte("fcos"))),
			HaveField("Data", HaveKeyWithValue(DefaultIgnitionSecretKeyName, MatchYAML(ignitionData))),
		))

		By("Patching the boot configuration to a Ready state")
		Eventually(UpdateStatus(bootConfig, func() {
			bootConfig.Status.State = metalv1alpha1.ServerBootConfigurationStateReady
		})).Should(Succeed())

		By("Ensuring that the Server resource has been created")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Finalizers", ContainElement(ServerFinalizer)),
			HaveField("Spec.SystemUUID", "38947555-7742-3448-3784-823347823834"),
			HaveField("Spec.SystemURI", "/redfish/v1/Systems/437XR1138R2"),
			HaveField("Spec.Power", metalv1alpha1.PowerOn),
			HaveField("Spec.IndicatorLED", metalv1alpha1.IndicatorLED("")),
			HaveField("Spec.ServerClaimRef", BeNil()),
			HaveField("Spec.BootConfigurationRef", &metalv1alpha1.ObjectReference{
				Kind:       "ServerBootConfiguration",
				Namespace:  ns.Name,
				Name:       server.Name,
				UID:        bootConfig.UID,
				APIVersion: "metal.ironcore.dev/v1alpha1",
			}),
			HaveField("Status.Manufacturer", "Contoso"),
			HaveField("Status.BIOSVersion", "P79 v1.45 (12/06/2017)"),
			HaveField("Status.SKU", "8675309"),
			HaveField("Status.SerialNumber", "437XR1138R2"),
			HaveField("Status.IndicatorLED", metalv1alpha1.OffIndicatorLED),
			HaveField("Status.State", metalv1alpha1.ServerStateDiscovery),
			HaveField("Status.Conditions", ContainElement(
				HaveField("Type", PoweringOnCondition),
			)),
		))

		By("Starting the probe agent")
		probeAgent := probe.NewAgent(GinkgoLogr, server.Spec.SystemUUID, registryURL, 50*time.Millisecond, 50*time.Millisecond, 250*time.Millisecond)
		go func() {
			defer GinkgoRecover()
			Expect(probeAgent.Start(ctx)).To(Succeed(), "failed to start probe agent")
		}()

		By("Ensuring that the server is set to available and powered off")
		// check that the available state is set first, as that is as part of handling
		// the discovery state. The ServerBootConfig deletion happens in a later
		// reconciliation as part of handling the available state.
		Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateAvailable))

		zeroCapacity := resource.NewQuantity(0, resource.DecimalSI)
		// force calculation of zero capacity string
		_ = zeroCapacity.String()
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BootConfigurationRef", BeNil()),
			HaveField("Spec.Power", metalv1alpha1.PowerOff),
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
			HaveField("Status.PowerState", metalv1alpha1.ServerOffPowerState),
			HaveField("Status.NetworkInterfaces", Not(BeEmpty())),
			HaveField("Status.Storages", ContainElement(metalv1alpha1.Storage{
				Name: "Simple Storage Controller",
				Drives: []metalv1alpha1.StorageDrive{
					{
						Name:     "SATA Bay 1",
						Capacity: resource.NewQuantity(8000000000000, resource.BinarySI),
						Vendor:   "Contoso",
						Model:    "3000GT8",
						State:    metalv1alpha1.StorageStateEnabled,
					},
					{
						Name:     "SATA Bay 2",
						Capacity: resource.NewQuantity(4000000000000, resource.BinarySI),
						Vendor:   "Contoso",
						Model:    "3000GT7",
						State:    metalv1alpha1.StorageStateEnabled,
					},
					{
						Name:     "SATA Bay 3",
						State:    metalv1alpha1.StorageStateAbsent,
						Capacity: zeroCapacity,
					},
					{
						Name:     "SATA Bay 4",
						State:    metalv1alpha1.StorageStateAbsent,
						Capacity: zeroCapacity,
					},
				},
			})),
			HaveField("Status.Storages", HaveLen(1)),
		))

		By("Ensuring that the boot configuration has been removed")
		Consistently(Get(bootConfig)).Should(Satisfy(apierrors.IsNotFound))

		By("Ensuring that the server is removed from the registry")
		response, err := http.Get(registryURL + "/systems/" + server.Spec.SystemUUID)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusNotFound))

		// cleanup
		Expect(k8sClient.Delete(ctx, server)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).Should(Succeed())
	})

	It("Should reset a Server into initial state on discovery failure", func(ctx SpecContext) {
		By("Creating a BMCSecret")
		bmcSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-server-",
			},
			Data: map[string][]byte{
				"username": []byte("foo"),
				"password": []byte("bar"),
			},
		}
		Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())

		By("Creating a Server with inline BMC configuration")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "server-",
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "38947555-7742-3448-3784-823347823834",
				BMC: &metalv1alpha1.BMCAccess{
					Protocol: metalv1alpha1.Protocol{
						Name: metalv1alpha1.ProtocolRedfishLocal,
						Port: 8000,
					},
					Address: "127.0.0.1",
					BMCSecretRef: v1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())

		Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateInitial))
		Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateDiscovery))

		By("Ensuring the boot configuration has been created")
		bootConfig := &metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      server.Name,
			},
		}
		Eventually(Object(bootConfig)).Should(SatisfyAll(
			HaveField("Annotations", HaveKeyWithValue(InternalAnnotationTypeKeyName, InternalAnnotationTypeValue)),
			HaveField("Annotations", HaveKeyWithValue(IsDefaultServerBootConfigOSImageKeyName, "true")),
			HaveField("Spec.ServerRef", v1.LocalObjectReference{Name: server.Name}),
			HaveField("Spec.Image", "fooOS:latest"),
			HaveField("Spec.IgnitionSecretRef", &v1.LocalObjectReference{Name: server.Name}),
			HaveField("Status.State", metalv1alpha1.ServerBootConfigurationStatePending),
		))

		go func(ctx SpecContext) {
			for {
				select {
				case <-ctx.Done():
					return
				default:
					deleteRegistrySystemIfExists(server.Spec.SystemUUID)
				}
			}
		}(ctx)

		By("Patching the boot configuration to a Ready state")
		Eventually(UpdateStatus(bootConfig, func() {
			bootConfig.Status.State = metalv1alpha1.ServerBootConfigurationStateReady
		})).Should(Succeed())

		Eventually(Object(server)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.ServerStateInitial),
			HaveField("Status.State", metalv1alpha1.ServerStateDiscovery),
		))
		Consistently(Object(server)).WithTimeout(6 * time.Second).WithPolling(2 * time.Second).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.ServerStateInitial),
			HaveField("Status.State", metalv1alpha1.ServerStateDiscovery),
		))

		// cleanup
		Expect(k8sClient.Delete(ctx, server)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).Should(Succeed())
	})

	It("Should reset a Server into initial state after maintenance is removed", func(ctx SpecContext) {
		By("Creating a BMCSecret")
		bmcSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-server-",
			},
			Data: map[string][]byte{
				"username": []byte("foo"),
				"password": []byte("bar"),
			},
		}
		Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())

		By("Creating a Server with inline BMC configuration")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "server-",
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "38947555-7742-3448-3784-823347823834",
				BMC: &metalv1alpha1.BMCAccess{
					Protocol: metalv1alpha1.Protocol{
						Name: metalv1alpha1.ProtocolRedfishLocal,
						Port: 8000,
					},
					Address: "127.0.0.1",
					BMCSecretRef: v1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())

		By("Ensuring that the Server is set to discovery")
		Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateDiscovery))

		By("Creating the server maintenance")
		maintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      server.Name,
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
			},
		}
		Expect(k8sClient.Create(ctx, maintenance)).To(Succeed())

		By("Ensuring that the server is set to maintenance")
		Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateMaintenance))

		By("Deleting the server maintenance")
		Expect(k8sClient.Delete(ctx, maintenance)).To(Succeed())

		By("Ensuring that the server is reset to initial state")
		Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateInitial))

		// cleanup
		Expect(k8sClient.Delete(ctx, server)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
	})

	It("Should reset a claimed Server into Reserved state after maintenance is removed", func(ctx SpecContext) {
		By("Creating a BMCSecret")
		bmcSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-server-",
			},
			Data: map[string][]byte{
				"username": []byte("foo"),
				"password": []byte("bar"),
			},
		}
		Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())

		By("Creating a Server with inline BMC configuration and claiming it")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "server-",
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "38947555-7742-3448-3784-823347823834",
				BMC: &metalv1alpha1.BMCAccess{
					Protocol: metalv1alpha1.Protocol{
						Name: metalv1alpha1.ProtocolRedfishLocal,
						Port: 8000,
					},
					Address: "127.0.0.1",
					BMCSecretRef: v1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())

		By("Updating the Server to available state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
		})).Should(Succeed())

		By("Creating a ServerClaim for the server")
		claim := &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-claim",
				Namespace: ns.Name,
			},
			Spec: metalv1alpha1.ServerClaimSpec{
				ServerRef: &v1.LocalObjectReference{
					Name: server.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, claim)).To(Succeed())

		By("Ensuring that the Server is set to reserved")
		Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateReserved))

		By("Ensuring the server maintenance is created and set to server")
		maintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      server.Name,
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenanceTemplate: metalv1alpha1.ServerMaintenanceTemplate{
					Policy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
			},
		}
		Expect(k8sClient.Create(ctx, maintenance)).To(Succeed())

		By("Ensuring that the server is set to maintenance")
		Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateMaintenance))

		By("Deleting the server maintenance")
		Expect(k8sClient.Delete(ctx, maintenance)).To(Succeed())

		By("Ensuring that the server is reset to reserved state")
		Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateReserved))

		// cleanup
		Expect(k8sClient.Delete(ctx, claim)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, server)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
	})

	It("Should updated the BootStateReceived condition when the bootstate endpoint is called", func(ctx SpecContext) {
		By("Creating a BMCSecret")
		bmcSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-server-",
			},
			Data: map[string][]byte{
				"username": []byte("foo"),
				"password": []byte("bar"),
			},
		}
		Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())

		By("Creating a Server with inline BMC configuration")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "server-",
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "38947555-7742-3448-3784-823347823834",
				BMC: &metalv1alpha1.BMCAccess{
					Protocol: metalv1alpha1.Protocol{
						Name: metalv1alpha1.ProtocolRedfishLocal,
						Port: 8000,
					},
					Address: "127.0.0.1",
					BMCSecretRef: v1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())
		Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateDiscovery))

		var bootstateRequest registry.BootstatePayload
		bootstateRequest.SystemUUID = server.Spec.SystemUUID
		bootstateRequest.Booted = true
		marshaled, err := json.Marshal(bootstateRequest)
		Expect(err).NotTo(HaveOccurred())
		response, err := http.Post(registryURL+"/bootstate", "application/json", bytes.NewBuffer(marshaled))
		Expect(err).NotTo(HaveOccurred())
		Expect(response.Body.Close()).To(Succeed())
		Expect(response.StatusCode).To(Equal(http.StatusOK))

		bootConfig := metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      server.Spec.BootConfigurationRef.Name,
				Namespace: server.Spec.BootConfigurationRef.Namespace,
			},
		}
		Eventually(Object(&bootConfig)).Should(HaveField("Status.Conditions", ContainElement(HaveField("Type", registry.BootStateReceivedCondition))))
		Expect(k8sClient.Delete(ctx, server)).To(Succeed())
		Eventually(Get(server)).Should(Satisfy(apierrors.IsNotFound))
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		Eventually(Get(bmcSecret)).Should(Satisfy(apierrors.IsNotFound))
		Eventually(Get(&bootConfig)).Should(Satisfy(apierrors.IsNotFound))
	})
})

func deleteRegistrySystemIfExists(systemUUID string) {
	response, err := http.Get(registryURL + "/systems/" + systemUUID)
	if err != nil {
		return
	}
	if response.StatusCode == http.StatusOK {
		req, err := http.NewRequest(http.MethodDelete, registryURL+"/systems/"+systemUUID, nil)
		if err != nil {
			return
		}
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close() //nolint:errcheck
	}
}

// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("Server Controller", func() {
	ns := SetupTest(nil)

	AfterEach(func(ctx SpecContext) {
		EnsureCleanState(ctx)
	})

	It("should transition a server to released state and back to available", func(ctx SpecContext) {
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
						Port: MockServerPort,
					},
					Address: MockServerIP,
					BMCSecretRef: v1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
				},
				ReclaimPolicy: metalv1alpha1.ServerReclaimPolicyRetain,
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())

		By("Updating the Server to available state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
		})).Should(Succeed())

		By("creating a server claim")
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

		By("waiting for the server to be reserved")
		claimRef := metalv1alpha1.ImmutableObjectReference{
			Namespace: claim.Namespace,
			Name:      claim.Name,
		}
		Eventually(Object(server)).To(SatisfyAll(
			HaveField("Spec.ServerClaimRef", Equal(&claimRef)),
			HaveField("Status.State", metalv1alpha1.ServerStateReserved),
		))

		By("deleting the server claim")
		Expect(k8sClient.Delete(ctx, claim)).To(Succeed())

		By("waiting for the server claim to be gone")
		Eventually(Get(claim)).To(Satisfy(apierrors.IsNotFound))

		By("waiting for the server be released")
		Eventually(Object(server)).To(SatisfyAll(
			HaveField("Spec.ServerClaimRef", &claimRef),
			HaveField("Status.State", metalv1alpha1.ServerStateReleased),
		))

		By("deleting the claim ref of the server")
		Eventually(Update(server, func() {
			server.Spec.ServerClaimRef = nil
		})).To(Succeed())

		By("waiting for the server to be available again")
		Eventually(Object(server)).To(HaveField("Status.State", metalv1alpha1.ServerStateAvailable))

		// cleanup
		Expect(k8sClient.Delete(ctx, server)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).Should(Succeed())
	})

	It("should resume a claimed server with wiped status directly in reserved state", func(ctx SpecContext) {
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

		By("Creating a claim for the server")
		claim := &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-claim-",
				Namespace:    ns.Name,
			},
			Spec: metalv1alpha1.ServerClaimSpec{
				Power: metalv1alpha1.PowerOn,
			},
		}
		Expect(k8sClient.Create(ctx, claim)).To(Succeed())

		By("Creating a Server with a claim ref and wiped status")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "server-",
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "47947555-7742-3448-3784-823347823835",
				BMC: &metalv1alpha1.BMCAccess{
					Protocol: metalv1alpha1.Protocol{
						Name: metalv1alpha1.ProtocolRedfishLocal,
						Port: MockServerPort,
					},
					Address: MockServerIP,
					BMCSecretRef: v1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
				},
				ServerClaimRef: &metalv1alpha1.ImmutableObjectReference{
					Namespace: claim.Namespace,
					Name:      claim.Name,
				},
				ReclaimPolicy: metalv1alpha1.ServerReclaimPolicyRetain,
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())

		By("waiting for the server to be reserved without going through discovery")
		Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateReserved))
		Consistently(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateReserved))

		// cleanup
		Expect(k8sClient.Delete(ctx, claim)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, server)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).Should(Succeed())
	})

	It("should initialize a Server from Endpoint", func(ctx SpecContext) {
		By("Creating an Endpoint object")
		endpoint := &metalv1alpha1.Endpoint{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-server-",
			},
			Spec: metalv1alpha1.EndpointSpec{
				// emulator BMC mac address
				MACAddress: "23:11:8A:33:CF:EA",
				IP:         metalv1alpha1.MustParseIP(MockServerIP),
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
			HaveField("OwnerReferences", BeEmpty()),
			HaveField("Spec.SystemUUID", "38947555-7742-3448-3784-823347823834"),
			HaveField("Spec.SystemURI", "/redfish/v1/Systems/437XR1138R2"),
			HaveField("Spec.IndicatorLED", metalv1alpha1.IndicatorLED("")),
			HaveField("Spec.ServerClaimRef", BeNil()),
			HaveField("Status.Manufacturer", "Contoso"),
			HaveField("Status.BIOSVersion", "P79 v1.45 (12/06/2017)"),
			HaveField("Status.Model", "3500"),
			HaveField("Status.SKU", "8675309"),
			HaveField("Status.SerialNumber", "437XR1138R2"),
			HaveField("Status.IndicatorLED", metalv1alpha1.OffIndicatorLED),
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
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

		// cleanup
		Expect(k8sClient.Delete(ctx, endpoint)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, bmc)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, server)).Should(Succeed())
	})

	It("should initialize a Server with inline BMC configuration", func(ctx SpecContext) {
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
						Port: MockServerPort,
					},
					Address: MockServerIP,
					BMCSecretRef: v1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())

		By("Ensuring that the Server transitions directly to available and is powered off")
		zeroCapacity := resource.NewQuantity(0, resource.DecimalSI)
		// force calculation of zero capacity string
		_ = zeroCapacity.String()
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Finalizers", ContainElement(ServerFinalizer)),
			HaveField("Spec.SystemUUID", "38947555-7742-3448-3784-823347823834"),
			HaveField("Spec.SystemURI", "/redfish/v1/Systems/437XR1138R2"),
			HaveField("Spec.ServerClaimRef", BeNil()),
			HaveField("Status.Manufacturer", "Contoso"),
			HaveField("Status.BIOSVersion", "P79 v1.45 (12/06/2017)"),
			HaveField("Status.SKU", "8675309"),
			HaveField("Status.SerialNumber", "437XR1138R2"),
			HaveField("Status.IndicatorLED", metalv1alpha1.OffIndicatorLED),
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
			HaveField("Status.PowerState", metalv1alpha1.ServerOffPowerState),
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
		Consistently(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateAvailable))

		// cleanup
		Expect(k8sClient.Delete(ctx, server)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).Should(Succeed())
	})

	It("should move Server out of reserved state on missing serverClaim and BootConfig", func(ctx SpecContext) {
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
						Port: MockServerPort,
					},
					Address: MockServerIP,
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

		By("Creating an Ignition secret")
		ignitionSecret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Data: map[string][]byte{
				"foo": []byte("bar"),
			},
		}
		Expect(k8sClient.Create(ctx, ignitionSecret)).To(Succeed())

		By("Creating a ServerClaim")
		claim := &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-deleted-serverclaim-",
			},
			Spec: metalv1alpha1.ServerClaimSpec{
				Power:             metalv1alpha1.PowerOn,
				ServerRef:         &v1.LocalObjectReference{Name: server.Name},
				IgnitionSecretRef: &v1.LocalObjectReference{Name: ignitionSecret.Name},
				Image:             "foo:bar",
			},
		}
		Expect(k8sClient.Create(ctx, claim)).To(Succeed())

		By("Ensuring that the ServerBootConfiguration has been created")
		config := &metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      claim.Name,
			},
		}
		Eventually(Object(config)).Should(SatisfyAll(
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "ServerClaim",
				Name:               claim.Name,
				UID:                claim.UID,
				Controller:         new(true),
				BlockOwnerDeletion: new(true),
			})),
			HaveField("Spec.ServerRef.Name", server.Name),
			HaveField("Spec.Image", "foo:bar"),
			HaveField("Spec.IgnitionSecretRef.Name", ignitionSecret.Name),
		))

		By("Patching the boot configuration to a Ready state")
		Eventually(UpdateStatus(config, func() {
			config.Status.State = metalv1alpha1.ServerBootConfigurationStateReady
		})).Should(Succeed())

		By("Ensuring that the Server is set to reserved")
		Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateReserved))

		// this is needed to catch the case where the server is reconciled after deletion of the claim and config.
		By("Patching the server with ignore annotation")
		Eventually(Update(server, func() {
			metav1.SetMetaDataAnnotation(&server.ObjectMeta, metalv1alpha1.OperationAnnotation, metalv1alpha1.OperationAnnotationIgnore)
		})).Should(Succeed())

		By("Ensuring that the boot configuration has been removed")
		Expect(k8sClient.Delete(ctx, config)).To(Succeed())

		By("Ensuring that the serverClaim has been removed")
		Expect(k8sClient.Delete(ctx, claim)).To(Succeed())

		Eventually(Get(config)).Should(Satisfy(apierrors.IsNotFound))
		Eventually(Get(claim)).Should(Satisfy(apierrors.IsNotFound))

		By("Remove ignore annotation on server")
		Eventually(Update(server, func() {
			delete(server.Annotations, metalv1alpha1.OperationAnnotation)
		})).Should(Succeed())

		By("Ensuring that the Server is set not at reserved")
		Eventually(Object(server)).Should(HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateReserved))))

		// cleanup
		Expect(k8sClient.Delete(ctx, server)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, ignitionSecret)).Should(Succeed())
	})

	Describe("Parked state", func() {
		park := func(server *metalv1alpha1.Server) {
			By("Requesting the server to be parked")
			Eventually(Update(server, func() {
				metav1.SetMetaDataAnnotation(&server.ObjectMeta, metalv1alpha1.OperationAnnotation, metalv1alpha1.OperationAnnotationPark)
			})).Should(Succeed())
		}

		It("should park a free server and resume to available", func(ctx SpecContext) {
			server, bmcSecret := createServerAndSecret(ctx)

			By("Driving the Server to available state")
			Eventually(UpdateStatus(server, func() {
				server.Status.State = metalv1alpha1.ServerStateAvailable
			})).Should(Succeed())

			park(server)

			By("Ensuring the server reaches Parked, is off, and the request annotation is consumed")
			Eventually(Object(server)).Should(SatisfyAll(
				HaveField("Status.State", metalv1alpha1.ServerStateParked),
				HaveField("Status.PowerState", metalv1alpha1.ServerOffPowerState),
				HaveField("Annotations", HaveKeyWithValue(metalv1alpha1.ParkedAnnotation, Not(BeEmpty()))),
				HaveField("Annotations", Not(HaveKey(metalv1alpha1.OperationAnnotation))),
			))

			By("Requesting unpark to resume")
			Eventually(Update(server, func() {
				metav1.SetMetaDataAnnotation(&server.ObjectMeta, metalv1alpha1.OperationAnnotation, metalv1alpha1.OperationAnnotationUnpark)
			})).Should(Succeed())

			By("Ensuring the server resumes to available and the unpark request is consumed")
			Eventually(Object(server)).Should(SatisfyAll(
				HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
				HaveField("Annotations", Not(HaveKey(metalv1alpha1.OperationAnnotation))),
			))

			Expect(k8sClient.Delete(ctx, server)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		})

		It("should park a claimed server, keep the claim bound, and resume to reserved", func(ctx SpecContext) {
			server, bmcSecret := createServerAndSecret(ctx)

			By("Driving the Server to available state")
			Eventually(UpdateStatus(server, func() {
				server.Status.State = metalv1alpha1.ServerStateAvailable
			})).Should(Succeed())

			By("Creating a ServerClaim for the server")
			claim := &metalv1alpha1.ServerClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "parked-claim",
					Namespace: ns.Name,
				},
				Spec: metalv1alpha1.ServerClaimSpec{
					ServerRef: &v1.LocalObjectReference{Name: server.Name},
				},
			}
			Expect(k8sClient.Create(ctx, claim)).To(Succeed())

			By("Ensuring that the Server is set to reserved")
			Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateReserved))

			park(server)

			By("Ensuring the server is parked and the claim stays bound")
			Eventually(Object(server)).Should(SatisfyAll(
				HaveField("Status.State", metalv1alpha1.ServerStateParked),
				HaveField("Status.PowerState", metalv1alpha1.ServerOffPowerState),
				HaveField("Annotations", HaveKeyWithValue(metalv1alpha1.ParkedAnnotation, Not(BeEmpty()))),
			))
			Eventually(Object(claim)).Should(HaveField("Status.Phase", metalv1alpha1.PhaseBound))

			By("Requesting unpark to resume")
			Eventually(Update(server, func() {
				metav1.SetMetaDataAnnotation(&server.ObjectMeta, metalv1alpha1.OperationAnnotation, metalv1alpha1.OperationAnnotationUnpark)
			})).Should(Succeed())

			By("Ensuring the server resumes to reserved")
			Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateReserved))

			Expect(k8sClient.Delete(ctx, claim)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, server)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		})

		It("should reconstruct the parked state from the annotation if status.state is lost", func(ctx SpecContext) {
			server, bmcSecret := createServerAndSecret(ctx)

			By("Driving the Server to available state")
			Eventually(UpdateStatus(server, func() {
				server.Status.State = metalv1alpha1.ServerStateAvailable
			})).Should(Succeed())

			park(server)
			Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateParked))

			By("Clearing status.state while the parked annotation remains")
			Eventually(UpdateStatus(server, func() {
				server.Status.State = ""
			})).Should(Succeed())

			By("Ensuring the reconciler reconstructs the Parked state from the annotation")
			Eventually(Object(server)).Should(SatisfyAll(
				HaveField("Status.State", metalv1alpha1.ServerStateParked),
				HaveField("Annotations", HaveKeyWithValue(metalv1alpha1.ParkedAnnotation, Not(BeEmpty()))),
			))

			By("Requesting unpark to resume")
			Eventually(Update(server, func() {
				metav1.SetMetaDataAnnotation(&server.ObjectMeta, metalv1alpha1.OperationAnnotation, metalv1alpha1.OperationAnnotationUnpark)
			})).Should(Succeed())
			Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateAvailable))

			Expect(k8sClient.Delete(ctx, server)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		})

		It("should fall back to available on resume if the ServerClaimRef is gone while parked", func(ctx SpecContext) {
			server, bmcSecret := createServerAndSecret(ctx)

			By("Driving the Server to available state and claiming it")
			Eventually(UpdateStatus(server, func() {
				server.Status.State = metalv1alpha1.ServerStateAvailable
			})).Should(Succeed())

			claim := &metalv1alpha1.ServerClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "parked-fallback-claim",
					Namespace: ns.Name,
				},
				Spec: metalv1alpha1.ServerClaimSpec{
					ServerRef: &v1.LocalObjectReference{Name: server.Name},
				},
			}
			Expect(k8sClient.Create(ctx, claim)).To(Succeed())
			Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateReserved))

			park(server)
			Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateParked))

			By("Deleting the claim while parked and clearing the ServerClaimRef (claim removed during the procedure)")
			Expect(k8sClient.Delete(ctx, claim)).Should(Succeed())
			Eventually(Get(claim)).Should(Satisfy(apierrors.IsNotFound))
			Eventually(Update(server, func() {
				server.Spec.ServerClaimRef = nil
			})).Should(Succeed())

			By("Requesting unpark to resume")
			Eventually(Update(server, func() {
				metav1.SetMetaDataAnnotation(&server.ObjectMeta, metalv1alpha1.OperationAnnotation, metalv1alpha1.OperationAnnotationUnpark)
			})).Should(Succeed())

			By("Ensuring the server resumes to available (not reserved) since the claim ref is gone")
			Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateAvailable))

			Expect(k8sClient.Delete(ctx, server)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		})

		It("should not deadlock on deletion while parked", func(ctx SpecContext) {
			server, bmcSecret := createServerAndSecret(ctx)

			By("Driving the Server to available state")
			Eventually(UpdateStatus(server, func() {
				server.Status.State = metalv1alpha1.ServerStateAvailable
			})).Should(Succeed())

			park(server)
			Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateParked))

			By("Deleting the parked server")
			Expect(k8sClient.Delete(ctx, server)).To(Succeed())

			By("Ensuring the server is gone (finalizer removed despite being parked)")
			Eventually(Get(server)).Should(Satisfy(apierrors.IsNotFound))

			Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		})

		It("should not power off a parked server that the external actor powered on", func(ctx SpecContext) {
			server, bmcSecret := createServerAndSecret(ctx)

			By("Driving the Server to available state and parking it")
			Eventually(UpdateStatus(server, func() {
				server.Status.State = metalv1alpha1.ServerStateAvailable
			})).Should(Succeed())
			park(server)
			Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateParked))

			mockPowerState := func() string {
				req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s:%d%s", MockServerIP, MockServerPort, server.Spec.SystemURI), nil)
				Expect(err).NotTo(HaveOccurred())
				req.SetBasicAuth("foo", "bar")
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					return ""
				}
				defer func() { _ = resp.Body.Close() }()
				out := struct {
					PowerState string `json:"PowerState"`
				}{}
				Expect(json.NewDecoder(resp.Body).Decode(&out)).To(Succeed())
				return out.PowerState
			}

			By("Simulating the external actor powering the server on out of band")
			resetReq, err := http.NewRequest(http.MethodPost,
				fmt.Sprintf("http://%s:%d%s/Actions/ComputerSystem.Reset", MockServerIP, MockServerPort, server.Spec.SystemURI),
				bytes.NewBufferString(`{"ResetType":"On"}`),
			)
			Expect(err).NotTo(HaveOccurred())
			resetReq.SetBasicAuth("foo", "bar")
			resetReq.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(resetReq)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusAccepted))
			Expect(resp.Body.Close()).To(Succeed())
			Eventually(mockPowerState).Should(Equal("On"))

			By("Nudging a reconciliation while parked (dummy annotation)")
			Eventually(Update(server, func() {
				metav1.SetMetaDataAnnotation(&server.ObjectMeta, "metal.ironcore.dev/parked-tickle", "true")
			})).Should(Succeed())

			By("Ensuring the operator does not power the parked server back off")
			Consistently(mockPowerState, "2s", "100ms").Should(Equal("On"))

			By("Resuming the server via an unpark request")
			Eventually(Update(server, func() {
				metav1.SetMetaDataAnnotation(&server.ObjectMeta, metalv1alpha1.OperationAnnotation, metalv1alpha1.OperationAnnotationUnpark)
			})).Should(Succeed())
			Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateAvailable))

			Expect(k8sClient.Delete(ctx, server)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		})

		It("should defer a park request on a non-parkable state and park once parkable", func(ctx SpecContext) {
			server, bmcSecret := createServerAndSecret(ctx)

			By("Driving the server into the Error state (a non-parkable state)")
			Eventually(UpdateStatus(server, func() {
				server.Status.State = metalv1alpha1.ServerStateError
			})).Should(Succeed())

			By("Requesting the server to be parked while in Error")
			Eventually(Update(server, func() {
				metav1.SetMetaDataAnnotation(&server.ObjectMeta, metalv1alpha1.OperationAnnotation, metalv1alpha1.OperationAnnotationPark)
			})).Should(Succeed())

			By("Ensuring the server is not parked and the request is left in place")
			Consistently(Object(server)).Should(SatisfyAll(
				HaveField("Status.State", metalv1alpha1.ServerStateError),
				HaveField("Annotations", HaveKeyWithValue(metalv1alpha1.OperationAnnotation, metalv1alpha1.OperationAnnotationPark)),
				HaveField("Annotations", Not(HaveKey(metalv1alpha1.ParkedAnnotation))),
			))

			By("Driving the Server to available state (now parkable)")
			Eventually(UpdateStatus(server, func() {
				server.Status.State = metalv1alpha1.ServerStateAvailable
			})).Should(Succeed())

			By("Ensuring the deferred request is now honored and the server is parked")
			Eventually(Object(server)).Should(SatisfyAll(
				HaveField("Status.State", metalv1alpha1.ServerStateParked),
				HaveField("Annotations", HaveKeyWithValue(metalv1alpha1.ParkedAnnotation, Not(BeEmpty()))),
				HaveField("Annotations", Not(HaveKey(metalv1alpha1.OperationAnnotation))),
			))

			By("Requesting unpark to resume")
			Eventually(Update(server, func() {
				metav1.SetMetaDataAnnotation(&server.ObjectMeta, metalv1alpha1.OperationAnnotation, metalv1alpha1.OperationAnnotationUnpark)
			})).Should(Succeed())
			Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateAvailable))

			Expect(k8sClient.Delete(ctx, server)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		})

		It("should consume a reset operation annotation while parked without leaving it linger", func(ctx SpecContext) {
			server, bmcSecret := createServerAndSecret(ctx)

			By("Driving the Server to available state and parking it")
			Eventually(UpdateStatus(server, func() {
				server.Status.State = metalv1alpha1.ServerStateAvailable
			})).Should(Succeed())
			park(server)
			Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateParked))

			By("Issuing a reset operation against the parked server")
			Eventually(Update(server, func() {
				metav1.SetMetaDataAnnotation(&server.ObjectMeta, metalv1alpha1.OperationAnnotation, metalv1alpha1.GracefulRestartServerPower)
			})).Should(Succeed())

			By("Ensuring the server stays parked and the reset request is consumed (not lingered)")
			Eventually(Object(server)).Should(SatisfyAll(
				HaveField("Status.State", metalv1alpha1.ServerStateParked),
				HaveField("Annotations", HaveKeyWithValue(metalv1alpha1.ParkedAnnotation, Not(BeEmpty()))),
				HaveField("Annotations", Not(HaveKey(metalv1alpha1.OperationAnnotation))),
			))

			By("Resuming the server via an unpark request")
			Eventually(Update(server, func() {
				metav1.SetMetaDataAnnotation(&server.ObjectMeta, metalv1alpha1.OperationAnnotation, metalv1alpha1.OperationAnnotationUnpark)
			})).Should(Succeed())
			Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateAvailable))

			Expect(k8sClient.Delete(ctx, server)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		})
	})

	Describe("Operation annotations", func() {
		It("should clear an unsupported operation annotation", func(ctx SpecContext) {
			server, bmcSecret := createServerAndSecret(ctx)

			By("Driving the Server to available state")
			Eventually(UpdateStatus(server, func() {
				server.Status.State = metalv1alpha1.ServerStateAvailable
			})).Should(Succeed())

			By("Setting an unsupported operation annotation")
			Eventually(Update(server, func() {
				metav1.SetMetaDataAnnotation(&server.ObjectMeta, metalv1alpha1.OperationAnnotation, "bogus-operation")
			})).Should(Succeed())

			By("Ensuring the unsupported operation annotation is consumed")
			Eventually(Object(server)).Should(SatisfyAll(
				HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
				HaveField("Annotations", Not(HaveKey(metalv1alpha1.OperationAnnotation))),
			))

			Expect(k8sClient.Delete(ctx, server)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, bmcSecret)).Should(Succeed())
		})

		It("should clear an unsupported operation annotation while parked", func(ctx SpecContext) {
			server, bmcSecret := createServerAndSecret(ctx)

			By("Driving the Server to available state and parking it")
			Eventually(UpdateStatus(server, func() {
				server.Status.State = metalv1alpha1.ServerStateAvailable
			})).Should(Succeed())
			Eventually(Update(server, func() {
				metav1.SetMetaDataAnnotation(&server.ObjectMeta, metalv1alpha1.OperationAnnotation, metalv1alpha1.OperationAnnotationPark)
			})).Should(Succeed())
			Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateParked))

			By("Setting an unsupported operation annotation on the parked server")
			Eventually(Update(server, func() {
				metav1.SetMetaDataAnnotation(&server.ObjectMeta, metalv1alpha1.OperationAnnotation, "bogus-operation")
			})).Should(Succeed())

			By("Ensuring the unsupported operation annotation is consumed and the server stays parked")
			Eventually(Object(server)).Should(SatisfyAll(
				HaveField("Status.State", metalv1alpha1.ServerStateParked),
				HaveField("Annotations", HaveKeyWithValue(metalv1alpha1.ParkedAnnotation, Not(BeEmpty()))),
				HaveField("Annotations", Not(HaveKey(metalv1alpha1.OperationAnnotation))),
			))

			Expect(k8sClient.Delete(ctx, server)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, bmcSecret)).Should(Succeed())
		})
	})
})

func createServerAndSecret(ctx SpecContext) (*metalv1alpha1.Server, *metalv1alpha1.BMCSecret) {
	By("Creating a BMCSecret")
	bmcSecret := &metalv1alpha1.BMCSecret{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "test-server-"},
		Data: map[string][]byte{
			"username": []byte("foo"),
			"password": []byte("bar"),
		},
	}
	Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())

	By("Creating a Server with inline BMC configuration")
	server := &metalv1alpha1.Server{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "server-"},
		Spec: metalv1alpha1.ServerSpec{
			SystemUUID: "38947555-7742-3448-3784-823347823834",
			BMC: &metalv1alpha1.BMCAccess{
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolRedfishLocal,
					Port: MockServerPort,
				},
				Address: MockServerIP,
				BMCSecretRef: v1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
			},
		},
	}
	Expect(k8sClient.Create(ctx, server)).To(Succeed())
	return server, bmcSecret
}

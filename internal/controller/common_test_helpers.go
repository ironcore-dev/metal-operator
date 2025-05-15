// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v3"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/ignition"
	"github.com/ironcore-dev/metal-operator/internal/probe"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var LongTimeoutForPowerChecks = 3 * time.Second

// TransistionServerFromInitialToAvailableState transistions the server to AvailableState
func TransistionServerFromInitialToAvailableState(
	ctx context.Context,
	k8sClient client.Client,
	server *metalv1alpha1.Server,
	BootConfigNameSpace string,
) {
	Expect(k8sClient.Create(ctx, server)).To(Succeed())

	By("Ensuring the boot configuration has been created")
	bootConfig := &metalv1alpha1.ServerBootConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: BootConfigNameSpace,
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
			Namespace: BootConfigNameSpace,
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
			Namespace: BootConfigNameSpace,
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
		HaveField("Spec.UUID", "38947555-7742-3448-3784-823347823834"),
		HaveField("Spec.Power", metalv1alpha1.Power("On")),
		HaveField("Spec.IndicatorLED", metalv1alpha1.IndicatorLED("")),
		HaveField("Spec.ServerClaimRef", BeNil()),
		HaveField("Spec.BootConfigurationRef", &v1.ObjectReference{
			Kind:       "ServerBootConfiguration",
			Namespace:  BootConfigNameSpace,
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

	By("Starting the probe agent")
	probeAgent := probe.NewAgent(server.Spec.SystemUUID, "http://localhost:30000", 50*time.Millisecond)
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
	response, err := http.Get("http://localhost:30000" + "/systems/" + server.Spec.SystemUUID)
	Expect(err).NotTo(HaveOccurred())
	Expect(response.StatusCode).To(Equal(http.StatusNotFound))
}

// TransistionServerToReserveredState transistions the server to Reserved
func TransistionServerToReserveredState(
	ctx context.Context,
	k8sClient client.Client,
	serverClaim *metalv1alpha1.ServerClaim,
	server *metalv1alpha1.Server,
	nameSpace string,
) error {
	if server.Status.State == metalv1alpha1.ServerStateReserved {
		By("Ensuring the server in Reserevd state consistently")
		Consistently(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateReserved),
		), fmt.Sprintf("Expected server to be consistently in Reserved State %v", server.Status.State))
		return nil
	}
	TransistionServerFromInitialToAvailableState(ctx, k8sClient, server, nameSpace)

	if serverClaim.ResourceVersion == "" && serverClaim.Name == "" {
		Expect(k8sClient.Create(ctx, serverClaim)).Should(SatisfyAny(
			BeNil(),
			Satisfy(apierrors.IsAlreadyExists),
		), fmt.Sprintf("serverClaim is not created %v", serverClaim))
	}
	Eventually(Get(serverClaim)).Should(Succeed())

	By("Ensuring that the Server has the correct State and Claim ref")
	Eventually(Object(server)).Should(SatisfyAll(
		HaveField("Spec.ServerClaimRef", Not(BeNil())),
		HaveField("Spec.ServerClaimRef.Name", serverClaim.Name),
	), fmt.Sprintf("Expected Server %v to be referenced by serverClaim %v", server, serverClaim))
	Eventually(Object(server)).Should(SatisfyAll(
		HaveField("Status.State", metalv1alpha1.ServerStateReserved),
	), fmt.Sprintf("Expected Server to be in Reserved state %v", server))

	By("Ensuring that the ServerClaim is bound")
	Eventually(Object(serverClaim)).Should(SatisfyAll(
		HaveField("Finalizers", ContainElement(ServerClaimFinalizer)),
		HaveField("Status.Phase", metalv1alpha1.PhaseBound),
		HaveField("Spec.ServerRef", Not(BeNil())),
		HaveField("Spec.ServerRef.Name", server.Name),
	), fmt.Sprintf("Expected serverClaim %v to  be bound", serverClaim))

	By("Ensuring that the ServerBootConfiguration has been created")
	claimConfig := &metalv1alpha1.ServerBootConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: serverClaim.Namespace,
			Name:      serverClaim.Name,
		},
	}
	Eventually(Get(claimConfig)).Should(Succeed())

	By("Ensuring that the server has a correct boot configuration ref")
	Eventually(Object(server)).Should(SatisfyAll(
		HaveField("Spec.BootConfigurationRef", &v1.ObjectReference{
			APIVersion: "metal.ironcore.dev/v1alpha1",
			Kind:       "ServerBootConfiguration",
			Namespace:  claimConfig.Namespace,
			Name:       claimConfig.Name,
			UID:        claimConfig.UID,
		}),
	), fmt.Sprintf("Expected Server to have ref for BootConfig %v created by serverClaim %v", claimConfig, serverClaim))

	By("Patching the boot configuration to a Ready state")
	Eventually(UpdateStatus(claimConfig, func() {
		claimConfig.Status.State = metalv1alpha1.ServerBootConfigurationStateReady
	})).Should(Succeed(), fmt.Sprintf("Unable to set the bootconfig %v to Ready State", claimConfig))

	By("Ensuring that the Server has the correct PowerState")
	Eventually(Object(server)).WithTimeout(LongTimeoutForPowerChecks).Should(SatisfyAll(
		HaveField("Spec.Power", serverClaim.Spec.Power),
	), fmt.Sprintf("Expected Server to be in Power %v in Reserved state %v", serverClaim.Spec.Power, server.Status))
	Eventually(Object(server)).WithTimeout(LongTimeoutForPowerChecks).Should(SatisfyAll(
		HaveField("Status.PowerState", metalv1alpha1.ServerPowerState(serverClaim.Spec.Power)),
	), fmt.Sprintf("Expected Server to be in PowerState %v in Reserved state %v", serverClaim.Spec.Power, server.Status))
	return nil
}

func GetServerClaim(
	ctx context.Context,
	k8sClient client.Client,
	server metalv1alpha1.Server,
	nameSpace string,
	ignitionData map[string][]byte,
	requiredPower metalv1alpha1.Power,
	claimImage string,
) *metalv1alpha1.ServerClaim {
	if ignitionData == nil {
		ignitionData = make(map[string][]byte, 1)
	}
	By("Creating an Ignition secret")
	ignitionSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    nameSpace,
			GenerateName: "test-",
		},
		Data: ignitionData,
	}
	Expect(k8sClient.Create(ctx, ignitionSecret)).To(Succeed())

	By("Creating a ServerClaim object")
	serverClaim := &metalv1alpha1.ServerClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    nameSpace,
			GenerateName: "test-",
		},
		Spec: metalv1alpha1.ServerClaimSpec{
			Power:             requiredPower,
			ServerRef:         &v1.LocalObjectReference{Name: server.Name},
			IgnitionSecretRef: &v1.LocalObjectReference{Name: ignitionSecret.Name},
			Image:             claimImage,
		},
	}
	return serverClaim
}

func CheckServerPowerStateTransistionsDuringMaintenance(
	ctx context.Context,
	k8sClient client.Client,
	serverMaintaince *metalv1alpha1.ServerMaintenance,
	server *metalv1alpha1.Server,
	requiredPower metalv1alpha1.Power,
) error {
	By("Ensuring all the objects are available")
	Eventually(Get(serverMaintaince)).Should(Succeed())
	Eventually(Get(server)).Should(Succeed())

	if requiredPower == server.Spec.Power && serverMaintaince.Spec.ServerPower == server.Spec.Power && requiredPower == metalv1alpha1.Power(requiredPower) {
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.PowerState", metalv1alpha1.ServerPowerState(server.Spec.Power)),
		), fmt.Sprintf("Expected Server to be consistently in PowerState %v", requiredPower))
		return nil
	}

	By("Ensuring that the ServerMaintenance has the correct Power Spec")
	Eventually(Object(serverMaintaince)).Should(SatisfyAll(
		HaveField("Spec.ServerPower", requiredPower),
	), fmt.Sprintf("Expected ServerMaintenance to have ServerPower %v", requiredPower))

	By("Ensuring that the Server has the correct Power Spec")
	Eventually(Object(server)).WithTimeout(LongTimeoutForPowerChecks).Should(SatisfyAll(
		HaveField("Spec.Power", serverMaintaince.Spec.ServerPower),
	), fmt.Sprintf("Expected server to have power Spec %v", requiredPower))

	By("Ensuring that the Server has the correct PowerState")
	Eventually(Object(server)).WithTimeout(LongTimeoutForPowerChecks).Should(SatisfyAll(
		HaveField("Status.PowerState", metalv1alpha1.ServerPowerState(server.Spec.Power)),
	), fmt.Sprintf("Expected Server to be in PowerState %v", requiredPower))

	return nil
}

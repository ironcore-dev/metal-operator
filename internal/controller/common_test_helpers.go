// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"net/http"
	"time"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/probe"
	. "github.com/onsi/ginkgo/v2" // nolint: staticcheck
	. "github.com/onsi/gomega"    // nolint: staticcheck
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega" // nolint: staticcheck
)

// TransistionServerFromInitialToAvailableState transistions the server to AvailableState
func TransistionServerFromInitialToAvailableState(
	ctx context.Context,
	k8sClient client.Client,
	server *metalv1alpha1.Server,
	BootConfigNameSpace string,
) {
	By("Creating the server")
	server.Spec.Power = metalv1alpha1.PowerOff
	Expect(k8sClient.Create(ctx, server)).To(Succeed())

	Eventually(Object(server)).Should(SatisfyAny(
		HaveField("Status.State", metalv1alpha1.ServerStateInitial),
		HaveField("Status.State", metalv1alpha1.ServerStateDiscovery),
	))

	Eventually(Object(server)).WithPolling(100 * time.Millisecond).Should(
		HaveField("Status.State", metalv1alpha1.ServerStateDiscovery),
	)

	By("Ensuring the boot configuration has been created")
	bootConfig := &metalv1alpha1.ServerBootConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: BootConfigNameSpace,
			Name:      server.Name,
		},
	}
	Eventually(Object(bootConfig)).WithPolling(100 * time.Millisecond).Should(SatisfyAll(
		HaveField("Annotations", HaveKeyWithValue(InternalAnnotationTypeKeyName, InternalAnnotationTypeValue)),
		HaveField("Annotations", HaveKeyWithValue(IsDefaultServerBootConfigOSImageKeyName, "true")),
		HaveField("Spec.ServerRef", v1.LocalObjectReference{Name: server.Name}),
		HaveField("Spec.Image", "fooOS:latest"),
		HaveField("Spec.IgnitionSecretRef", &v1.LocalObjectReference{Name: server.Name}),
		HaveField("Status.State", metalv1alpha1.ServerBootConfigurationStatePending),
	))

	By("Patching the boot configuration to a Ready state")
	Eventually(UpdateStatus(bootConfig, func() {
		bootConfig.Status.State = metalv1alpha1.ServerBootConfigurationStateReady
	})).Should(Succeed())

	By("Starting the probe agent")
	registryURL := "http://localhost:30000"

	resp, err := http.Get(fmt.Sprintf("%s/systems/%s", registryURL, server.Spec.SystemUUID))
	if err != nil || resp.StatusCode != 200 {
		probeAgent := probe.NewAgent(server.Spec.SystemUUID, registryURL, 50*time.Millisecond)
		go func() {
			defer GinkgoRecover()
			Expect(probeAgent.Start(ctx)).To(Succeed(), "failed to start probe agent")
		}()
	}

	// give enough time to transistion to availalbe and then to power off.
	// sometime the transition happen from
	// discovery -> initial -> discovery -> available timeout needs to accomadate this as well
	By("ensuring the status of the server")
	Eventually(Object(server)).WithTimeout(9*time.Second).WithPolling(100*time.Millisecond).Should(SatisfyAll(
		HaveField("Finalizers", ContainElement(ServerFinalizer)),
		HaveField("Spec.ServerClaimRef", BeNil()),
		HaveField("Spec.UUID", "38947555-7742-3448-3784-823347823834"),
		HaveField("Spec.Power", metalv1alpha1.PowerOff),
		HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
		HaveField("Status.PowerState", metalv1alpha1.ServerOffPowerState),
		HaveField("Status.Manufacturer", "Contoso"),
		HaveField("Status.SKU", "8675309"),
		HaveField("Status.SerialNumber", "437XR1138R2"),
		HaveField("Status.IndicatorLED", metalv1alpha1.OffIndicatorLED),
	), fmt.Sprintf("server not in extected status %v and spec %v", server.Status, server.Spec))
}

// TransistionServerToReserveredState transistions the server to Reserved
func TransistionServerToReserveredState(
	ctx context.Context,
	k8sClient client.Client,
	serverClaim *metalv1alpha1.ServerClaim,
	server *metalv1alpha1.Server,
	nameSpace string,
) {
	if server.Status.State == metalv1alpha1.ServerStateReserved {
		By("Ensuring the server in Reserevd state consistently")
		Consistently(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateReserved),
		), fmt.Sprintf("Expected server to be consistently in Reserved State %v", server.Status.State))
		return
	}
	//TransistionServerFromInitialToAvailableState(ctx, k8sClient, server, nameSpace)

	By("Patching the server to a Available state")
	Eventually(UpdateStatus(server, func() {
		server.Status.State = metalv1alpha1.ServerStateAvailable
	})).Should(Succeed())

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
	Eventually(Object(server)).WithPolling(150*time.Millisecond).Should(SatisfyAll(
		HaveField("Spec.Power", serverClaim.Spec.Power),
		HaveField("Status.PowerState", metalv1alpha1.ServerPowerState(serverClaim.Spec.Power)),
	), fmt.Sprintf("Expected Server to be in PowerState %v in Reserved state %v", serverClaim.Spec.Power, server.Status))
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
) {
	By("Ensuring all the objects are available")
	Eventually(Get(serverMaintaince)).Should(Succeed())
	Eventually(Get(server)).Should(Succeed())

	if requiredPower == server.Spec.Power && serverMaintaince.Spec.ServerPower == server.Spec.Power && server.Status.PowerState == metalv1alpha1.ServerPowerState(requiredPower) {
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.PowerState", metalv1alpha1.ServerPowerState(server.Spec.Power)),
		), fmt.Sprintf("Expected Server to be consistently in PowerState %v", requiredPower))
		return
	}

	By("Ensuring that the ServerMaintenance has the correct Power Spec")
	Eventually(Object(serverMaintaince)).Should(SatisfyAll(
		HaveField("Spec.ServerPower", requiredPower),
	), fmt.Sprintf("Expected ServerMaintenance to have ServerPower %v", requiredPower))

	By("Ensuring that the Server has the correct PowerState")
	Eventually(Object(server)).WithPolling(150*time.Millisecond).Should(SatisfyAll(
		HaveField("Spec.Power", serverMaintaince.Spec.ServerPower),
		HaveField("Status.PowerState", metalv1alpha1.ServerPowerState(server.Spec.Power)),
	), fmt.Sprintf("Expected Server to be in PowerState %v", requiredPower))
}

// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"                         // nolint: staticcheck
	. "github.com/onsi/gomega"                            // nolint: staticcheck
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega" // nolint: staticcheck

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/probe"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TransistionServerFromInitialToAvailableState transistions the server to AvailableState
func TransistionServerFromInitialToAvailableState(
	ctx context.Context,
	k8sClient client.Client,
	server *metalv1alpha1.Server,
	BootConfigNameSpace string,
) {
	if server.ResourceVersion == "" && server.Name == "" {
		By("Ensuring the server has been created")
		Expect(k8sClient.Create(ctx, server)).Should(SatisfyAny(
			BeNil(),
			Satisfy(apierrors.IsAlreadyExists),
		), fmt.Sprintf("server is not created %v", server))
	}
	Eventually(Get(server)).Should(Succeed())

	if server.Status.State == metalv1alpha1.ServerStateAvailable {
		By("Ensuring the server in Available state consistently")
		Consistently(Object(server)).Should(
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
			fmt.Sprintf("Expected server to be consistently in Available State %v", server.Status.State))
		Eventually(Object(server)).Should(
			HaveField("Status.PowerState", metalv1alpha1.ServerOffPowerState),
			fmt.Sprintf("Expected Server to be in PowerState 'off' in Available state %v", server))
		return
	}

	By("Ensuring the server's Initial state")
	Eventually(Object(server)).Should(SatisfyAny(
		HaveField("Status.State", metalv1alpha1.ServerStateInitial),
		HaveField("Status.State", metalv1alpha1.ServerStateDiscovery),
	), fmt.Sprintf("Expected server to be in Initial State and transisitiong to powerOff %v", server.Status.State))
	if server.Status.State == metalv1alpha1.ServerStateInitial {
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.Power", metalv1alpha1.PowerOff),
			HaveField("Status.PowerState", metalv1alpha1.ServerOffPowerState),
		), fmt.Sprintf("Expected Server to be in PowerState 'off' in Initial state %v", server))
	}

	By("Ensuring the server's bootconfig is ref")
	Eventually(Object(server)).Should(
		HaveField("Spec.BootConfigurationRef", Not(BeNil())),
	)

	// need long time to create boot config
	// as we go through multiple reconcile before creating the boot config
	By("Ensuring the boot configuration has been created")
	bootConfig := &metalv1alpha1.ServerBootConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: BootConfigNameSpace,
			Name:      server.Name,
		},
	}
	Eventually(Get(bootConfig)).Should(
		Succeed(),
		fmt.Sprintf("Expected to get the bootConfig %v, created by Server %v", bootConfig, server.Name),
	)
	Eventually(Object(bootConfig)).Should(
		HaveField("Status.State", metalv1alpha1.ServerBootConfigurationStatePending),
		"Expected to get the bootConfig to reach pending state")

	By("Patching the boot configuration to a Ready state")
	Eventually(UpdateStatus(bootConfig, func() {
		bootConfig.Status.State = metalv1alpha1.ServerBootConfigurationStateReady
	})).Should(Succeed(), fmt.Sprintf("Unable to set the bootconfig %v to Ready State", bootConfig))

	Eventually(Object(bootConfig)).Should(SatisfyAll(
		HaveField("Status.State", metalv1alpha1.ServerBootConfigurationStateReady),
		HaveField("Spec.IgnitionSecretRef", Not(BeNil())),
	), "Expected the bootConfig to reach ready state")

	By("Ensuring that the Server is set to discovery and powered on")
	Eventually(Object(server)).Should(
		HaveField("Status.State", metalv1alpha1.ServerStateDiscovery),
		fmt.Sprintf("Expected Server to be in Discovery state %v", server))
	Eventually(Object(server)).Should(SatisfyAll(
		HaveField("Spec.Power", metalv1alpha1.PowerOn),
		HaveField("Status.PowerState", metalv1alpha1.ServerOnPowerState),
	), fmt.Sprintf("Expected Server to be in PowerState 'on' in discovery state %v", server))

	By("Starting the probe agent")
	probeAgent := probe.NewAgent(GinkgoLogr, server.Spec.SystemUUID, "http://localhost:30000", 50*time.Millisecond)
	go func() {
		defer GinkgoRecover()
		Expect(probeAgent.Start(ctx)).To(Succeed(), "failed to start probe agent")
	}()

	By("Ensuring that the server is set to available and powered off")
	// here, the Registry agent check sometimes fails (checkLastStatusUpdateAfter), need longer wait time,
	// to give chance to reach available incase it was reset to Initial state
	Eventually(Object(server)).Should(SatisfyAny(
		HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
		HaveField("Status.State", metalv1alpha1.ServerStateInitial),
	), fmt.Sprintf("Expected Server to be in Available or Initial State %v", server))

	// give it one more chance to reach Available state before declaring an error
	if server.Status.State == metalv1alpha1.ServerStateInitial {
		Eventually(Object(server)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.ServerStateDiscovery),
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
		), fmt.Sprintf("Expected Server to be in Available or Discovery State %v", server))
	}
	Eventually(Object(server)).Should(
		HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
		fmt.Sprintf("Expected Server to be in Available State %v", server))
	Eventually(Object(server)).Should(SatisfyAll(
		HaveField("Spec.Power", metalv1alpha1.PowerOff),
		HaveField("Status.PowerState", metalv1alpha1.ServerOffPowerState),
	), fmt.Sprintf("Expected Server to be Powered Off in Available State %v", server))
}

// TransitionServerToReservedState transitions the server to Reserved
func TransitionServerToReservedState(
	ctx context.Context,
	k8sClient client.Client,
	serverClaim *metalv1alpha1.ServerClaim,
	server *metalv1alpha1.Server,
	nameSpace string,
) {
	if server.Status.State == metalv1alpha1.ServerStateReserved {
		By("Ensuring the server in Reserved state consistently")
		Consistently(Object(server)).Should(
			HaveField("Status.State", metalv1alpha1.ServerStateReserved),
			fmt.Sprintf("Expected server to be consistently in Reserved State %v", server.Status.State))
		return
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
	Eventually(Object(server)).Should(
		HaveField("Status.State", metalv1alpha1.ServerStateReserved),
		fmt.Sprintf("Expected Server to be in Reserved state %v", server))

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
	Eventually(Object(server)).Should(
		HaveField("Spec.BootConfigurationRef", &v1.ObjectReference{
			APIVersion: "metal.ironcore.dev/v1alpha1",
			Kind:       "ServerBootConfiguration",
			Namespace:  claimConfig.Namespace,
			Name:       claimConfig.Name,
			UID:        claimConfig.UID,
		}),
		fmt.Sprintf("Expected Server to have ref for BootConfig %v created by serverClaim %v",
			claimConfig,
			serverClaim),
	)

	By("Patching the boot configuration to a Ready state")
	Eventually(UpdateStatus(claimConfig, func() {
		claimConfig.Status.State = metalv1alpha1.ServerBootConfigurationStateReady
	})).Should(Succeed(), fmt.Sprintf("Unable to set the bootconfig %v to Ready State", claimConfig))

	By("Ensuring that the Server has the correct PowerState")
	Eventually(Object(server)).Should(
		HaveField("Spec.Power", serverClaim.Spec.Power),
		fmt.Sprintf("Expected Server to be in Power %v in Reserved state %v", serverClaim.Spec.Power, server.Status),
	)
	Eventually(Object(server)).Should(
		HaveField("Status.PowerState", metalv1alpha1.ServerPowerState(serverClaim.Spec.Power)),
		fmt.Sprintf("Expected Server to be in PowerState %v in Reserved state %v",
			serverClaim.Spec.Power,
			server.Status),
	)
}

func BuildServerClaim(
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

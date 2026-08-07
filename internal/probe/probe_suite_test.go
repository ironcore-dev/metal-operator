// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/ironcore-dev/metal-operator/internal/probe"
	"github.com/ironcore-dev/metal-operator/internal/registry"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	probeAgent     *probe.Agent
	registryServer *registry.Server

	systemUUID = "1234-5678"
)

var (
	registryAddr string
	registryURL  string
)

func TestRegistry(t *testing.T) {
	RegisterFailHandler(Fail)
	// Offset the port per parallel ginkgo process to avoid bind collisions.
	// Flags are parsed at this point, so GinkgoParallelProcess is reliable.
	port := 30001 + GinkgoParallelProcess()
	registryAddr = fmt.Sprintf(":%d", port)
	registryURL = fmt.Sprintf("http://localhost:%d", port)
	RunSpecs(t, "Probe Suite")
}

var _ = BeforeSuite(func() {
	ctx, cancel := context.WithCancel(context.Background())
	DeferCleanup(cancel)

	// Initialize the registry
	registryServer = registry.NewServer(GinkgoLogr, registryAddr, nil)
	go func() {
		defer GinkgoRecover()
		Expect(registryServer.Start(ctx)).To(Succeed(), "failed to start registry agent")
	}()

	Eventually(func() error {
		_, err := http.Get(registryURL)
		return err
	}).Should(Succeed())

	// Initialize your probe server
	probeAgent = probe.NewAgent(GinkgoLogr, systemUUID, registryURL, 100*time.Millisecond, 1*time.Second, 50*time.Millisecond, 250*time.Millisecond)
	go func() {
		defer GinkgoRecover()
		Expect(probeAgent.Start(ctx)).To(Succeed(), "failed to start probe agent")
	}()
})

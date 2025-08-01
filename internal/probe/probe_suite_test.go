// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/ironcore-dev/metal-operator/internal/probe"
	"github.com/ironcore-dev/metal-operator/internal/registry"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	probeAgent     *probe.Agent
	registryServer *registry.Server

	registryAddr = ":30001"
	registryURL  = "http://localhost:30001"
	systemUUID   = "1234-5678"
)

func TestRegistry(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Registry Suite")
}

var _ = BeforeSuite(func() {
	log := zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true))
	logf.SetLogger(log)

	ctx, cancel := context.WithCancel(context.Background())
	DeferCleanup(cancel)

	// Initialize the registry
	registryServer = registry.NewServer(log, registryAddr)
	go func() {
		defer GinkgoRecover()
		Expect(registryServer.Start(ctx)).To(Succeed(), "failed to start registry agent")
	}()

	Eventually(func() error {
		_, err := http.Get(registryURL)
		return err
	}).Should(Succeed())

	// Initialize your probe server
	probeAgent = probe.NewAgent(log, systemUUID, registryURL, 100*time.Millisecond)
	go func() {
		defer GinkgoRecover()
		Expect(probeAgent.Start(ctx)).To(Succeed(), "failed to start probe agent")
	}()
})

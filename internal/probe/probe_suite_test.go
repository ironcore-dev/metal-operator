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

package probe_test

import (
	"context"
	"testing"

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

	registryAddr = ":5432"
	registryURL  = "http://localhost:5432"
	systemUUID   = "1234-5678"
)

func TestRegistry(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Registry Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel := context.WithCancel(context.Background())
	DeferCleanup(cancel)

	// Initialize the registry
	registryServer = registry.NewServer(registryAddr)
	go func() {
		defer GinkgoRecover()
		Expect(registryServer.Start(ctx)).To(Succeed(), "failed to start registry agent")
	}()

	// Initialize your probe server
	probeAgent = probe.NewAgent(systemUUID, registryURL)
	go func() {
		defer GinkgoRecover()
		Expect(probeAgent.Start(ctx)).To(Succeed(), "failed to start probe agent")
	}()
})

// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package registry_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/ironcore-dev/metal-operator/internal/registry"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	server         *registry.Server
	testServerURL  = "http://localhost:30002"
	testServerAddr = ":30002"
)

func TestRegistry(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Registry Suite")
}

var _ = BeforeSuite(func() {
	ctx, cancel := context.WithCancel(context.Background())
	DeferCleanup(cancel)

	server = registry.NewServer(GinkgoLogr, testServerAddr)
	go func() {
		defer GinkgoRecover()
		Expect(server.Start(ctx)).To(Succeed(), "failed to start registry server")
	}()

	Eventually(func() error {
		_, err := http.Get(testServerURL)
		return err
	}).Should(Succeed())
})

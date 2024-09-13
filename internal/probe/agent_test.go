// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ironcore-dev/metal-operator/internal/probe"

	"github.com/ironcore-dev/metal-operator/internal/api/registry"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ProbeAgent", func() {
	BeforeEach(func() {
		By("Starting the probe agent")
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)

		// Initialize your probe agent
		probeAgent = probe.NewAgent(systemUUID, registryURL)
		go func() {
			defer GinkgoRecover()
			Expect(probeAgent.Start(ctx)).To(Succeed(), "failed to start probe agent")
		}()
	})

	It("should ensure the correct endpoints have been registered", func() {
		By("performing a GET request to the /systems/{uuid} endpoint")
		var resp *http.Response
		var err error
		Eventually(func(g Gomega) {
			resp, err = http.Get(fmt.Sprintf("%s/systems/%s", registryURL, systemUUID))
			g.Expect(resp).NotTo(BeNil())
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(resp.StatusCode).To(Equal(http.StatusOK))
		}).Should(Succeed())

		By("ensuring that the endpoints are not empty")
		server := &registry.Server{}
		Expect(json.NewDecoder(resp.Body).Decode(server)).NotTo(HaveOccurred())
		Expect(server.NetworkInterfaces).NotTo(BeEmpty())
	})
})

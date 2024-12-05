// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe_test

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ironcore-dev/metal-operator/internal/api/registry"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ProbeAgent", func() {
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

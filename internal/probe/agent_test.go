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
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/afritzler/metal-operator/internal/api/registry"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ProbeServer", func() {
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

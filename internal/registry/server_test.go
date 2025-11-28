// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package registry_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ironcore-dev/metal-operator/internal/api/registry"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RegistryServer", func() {
	It("should store the system information successfully", func() {
		By("registering a system with it's endpoints")
		systemRegistrationPayload := registry.RegistrationPayload{
			SystemUUID: "test-uuid",
			Data: registry.Server{
				NetworkInterfaces: []registry.NetworkInterface{
					{
						Name:        "foo",
						IPAddresses: []string{"1.1.1.1"},
						MACAddress:  "abcd",
					},
				},
			},
		}
		payload, err := json.Marshal(systemRegistrationPayload)
		Expect(err).NotTo(HaveOccurred())

		By("performing a POST request to the /register endpoint")
		response, err := http.Post(fmt.Sprintf("%s/register", testServerURL), "application/json", bytes.NewBuffer(payload))
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusCreated))

		By("performing a GET request to the /systems/{uuid} endpoint")
		resp, err := http.Get(fmt.Sprintf("%s/systems/%s", testServerURL, systemRegistrationPayload.SystemUUID))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		By("ensuring that the endpoints are correct")
		server := &registry.Server{}
		Expect(json.NewDecoder(resp.Body).Decode(server)).NotTo(HaveOccurred())
		Expect(server).To(Equal(&registry.Server{
			NetworkInterfaces: []registry.NetworkInterface{
				{
					Name:        "foo",
					IPAddresses: []string{"1.1.1.1"},
					MACAddress:  "abcd",
				},
			},
		}))

		By("Ensuring that the server is removed from the registry")
		request, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/delete/%s", testServerURL, systemRegistrationPayload.SystemUUID), nil)
		Expect(err).NotTo(HaveOccurred())
		c := &http.Client{}
		response, err = c.Do(request)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))

		By("Ensuring that the server is removed from the registry")
		response, err = http.Get(fmt.Sprintf("%s/systems/%s", testServerURL, systemRegistrationPayload.SystemUUID))
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusNotFound))
	})
})

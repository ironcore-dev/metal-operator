// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"io"
	"net/http"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stmcginnis/gofish/schemas"
)

func TestDellOEM(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dell OEM Suite")
}

var _ = Describe("Dell OEM", func() {
	var dell *DellRedfishBMC

	BeforeEach(func() {
		dell = &DellRedfishBMC{}
	})

	Describe("GetUpdateRequestBody", func() {
		It("should create request body with correct parameters", func() {
			params := &schemas.UpdateServiceSimpleUpdateParameters{
				ImageURI:    "http://example.com/firmware.bin",
				Username:    "admin",
				Password:    "password",
				ForceUpdate: true,
				Targets:     []string{"/redfish/v1/UpdateService/Actions/UpdateService.SimpleUpdate"},
			}

			body := dell.dellBuildRequestBody(params)

			Expect(body.ImageURI).To(Equal(params.ImageURI))
			Expect(body.Username).To(Equal(params.Username))
			Expect(body.Password).To(Equal(params.Password))
			Expect(body.ForceUpdate).To(Equal(params.ForceUpdate))
			Expect(body.Targets).To(Equal(params.Targets))
			Expect(body.RedfishOperationApplyTime).To(Equal(schemas.ImmediateOperationApplyTime))
		})
	})

	Describe("GetUpdateTaskMonitorURI", func() {
		It("should extract URI from Location header", func() {
			resp := &http.Response{
				Header: make(http.Header),
				Body:   io.NopCloser(strings.NewReader("")),
			}
			resp.Header.Set("Location", "/redfish/v1/TaskService/Tasks/1")

			uri, err := dell.dellExtractTaskMonitorURI(resp)
			Expect(err).ToNot(HaveOccurred())
			Expect(uri).To(Equal("/redfish/v1/TaskService/Tasks/1"))
		})

		It("should extract URI from response body", func() {
			resp := &http.Response{
				Header: make(http.Header),
				Body:   io.NopCloser(strings.NewReader(`{"@odata.id": "/redfish/v1/TaskService/Tasks/2"}`)),
			}

			uri, err := dell.dellExtractTaskMonitorURI(resp)
			Expect(err).ToNot(HaveOccurred())
			Expect(uri).To(Equal("/redfish/v1/TaskService/Tasks/2"))
		})

		It("should return error when no URI found", func() {
			resp := &http.Response{
				Header: make(http.Header),
				Body:   io.NopCloser(strings.NewReader("{}")),
			}

			_, err := dell.dellExtractTaskMonitorURI(resp)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to extract task monitor URI"))
		})
	})

})

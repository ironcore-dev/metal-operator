// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package oem

import (
	"io"
	"net/http"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stmcginnis/gofish/redfish"
)

func TestDellOEM(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dell OEM Suite")
}

var _ = Describe("Dell OEM", func() {
	var dell *Dell

	BeforeEach(func() {
		dell = &Dell{}
	})

	Describe("GetUpdateRequestBody", func() {
		It("should create request body with correct parameters", func() {
			params := &redfish.SimpleUpdateParameters{
				ImageURI:    "http://example.com/firmware.bin",
				Username:    "admin",
				Passord:     "password",
				ForceUpdate: true,
				Targets:     []string{"/redfish/v1/UpdateService/Actions/UpdateService.SimpleUpdate"},
			}

			body := dell.GetUpdateRequestBody(params)

			Expect(body.ImageURI).To(Equal(params.ImageURI))
			Expect(body.Username).To(Equal(params.Username))
			Expect(body.Passord).To(Equal(params.Passord))
			Expect(body.ForceUpdate).To(Equal(params.ForceUpdate))
			Expect(body.Targets).To(Equal(params.Targets))
			Expect(body.RedfishOperationApplyTime).To(Equal(redfish.ImmediateOperationApplyTime))
		})
	})

	Describe("GetUpdateTaskMonitorURI", func() {
		It("should extract URI from Location header", func() {
			resp := &http.Response{
				Header: make(http.Header),
				Body:   io.NopCloser(strings.NewReader("")),
			}
			resp.Header.Set("Location", "/redfish/v1/TaskService/Tasks/1")

			uri, err := dell.GetUpdateTaskMonitorURI(resp)
			Expect(err).ToNot(HaveOccurred())
			Expect(uri).To(Equal("/redfish/v1/TaskService/Tasks/1"))
		})

		It("should extract URI from response body", func() {
			resp := &http.Response{
				Header: make(http.Header),
				Body:   io.NopCloser(strings.NewReader(`{"@odata.id": "/redfish/v1/TaskService/Tasks/2"}`)),
			}

			uri, err := dell.GetUpdateTaskMonitorURI(resp)
			Expect(err).ToNot(HaveOccurred())
			Expect(uri).To(Equal("/redfish/v1/TaskService/Tasks/2"))
		})

		It("should return error when no URI found", func() {
			resp := &http.Response{
				Header: make(http.Header),
				Body:   io.NopCloser(strings.NewReader("{}")),
			}

			_, err := dell.GetUpdateTaskMonitorURI(resp)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to extract task monitor URI"))
		})
	})

	Describe("Dell Common BMC Attributes", func() {
		It("should include Dell-specific BMC attributes", func() {
			Expect(dellCommonBMCAttributes).ToNot(BeEmpty())

			// Check that common Dell attributes are defined
			Expect(dellCommonBMCAttributes).To(HaveKey("SysLog.1.SysLogEnable"))
			Expect(dellCommonBMCAttributes).To(HaveKey("NTPConfigGroup.1.NTPEnable"))
			Expect(dellCommonBMCAttributes).To(HaveKey("EmailAlert.1.Enable"))
			Expect(dellCommonBMCAttributes).To(HaveKey("SNMP.1.AgentEnable"))
		})

		It("should have correct attribute types", func() {
			syslogAttr := dellCommonBMCAttributes["SysLog.1.SysLogEnable"]
			Expect(syslogAttr.Type).To(Equal(redfish.BooleanAttributeType))
			Expect(syslogAttr.ReadOnly).To(BeFalse())

			snmpAttr := dellCommonBMCAttributes["SNMP.1.AgentCommunity"]
			Expect(snmpAttr.Type).To(Equal(redfish.StringAttributeType))
			Expect(snmpAttr.ReadOnly).To(BeFalse())
		})
	})
})

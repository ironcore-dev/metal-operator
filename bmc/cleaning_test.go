// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCleaning(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cleaning Suite")
}

var _ = Describe("Server Cleaning Operations", func() {
	Describe("Vendor-Specific Disk Wipe Configuration", func() {
		Describe("Dell Disk Wipe Passes", func() {
			It("should return correct pass count for quick wipe", func() {
				passes := getDellWipePasses(DiskWipeMethodQuick)
				Expect(passes).To(Equal(1))
			})

			It("should return correct pass count for secure wipe", func() {
				passes := getDellWipePasses(DiskWipeMethodSecure)
				Expect(passes).To(Equal(3))
			})

			It("should return correct pass count for DoD wipe", func() {
				passes := getDellWipePasses(DiskWipeMethodDoD)
				Expect(passes).To(Equal(7))
			})

			It("should default to 1 pass for unknown method", func() {
				passes := getDellWipePasses("unknown")
				Expect(passes).To(Equal(1))
			})
		})

		Describe("HPE Wipe Type", func() {
			It("should return correct type for quick wipe", func() {
				wipeType := getHPEWipeType(DiskWipeMethodQuick)
				Expect(wipeType).To(Equal("BlockErase"))
			})

			It("should return correct type for secure wipe", func() {
				wipeType := getHPEWipeType(DiskWipeMethodSecure)
				Expect(wipeType).To(Equal("Overwrite"))
			})

			It("should return correct type for DoD wipe", func() {
				wipeType := getHPEWipeType(DiskWipeMethodDoD)
				Expect(wipeType).To(Equal("CryptographicErase"))
			})

			It("should default to BlockErase for unknown method", func() {
				wipeType := getHPEWipeType("unknown")
				Expect(wipeType).To(Equal("BlockErase"))
			})
		})

		Describe("Lenovo Wipe Method", func() {
			It("should return correct method for quick wipe", func() {
				method := getLenovoWipeMethod(DiskWipeMethodQuick)
				Expect(method).To(Equal("Simple"))
			})

			It("should return correct method for secure wipe", func() {
				method := getLenovoWipeMethod(DiskWipeMethodSecure)
				Expect(method).To(Equal("Cryptographic"))
			})

			It("should return correct method for DoD wipe", func() {
				method := getLenovoWipeMethod(DiskWipeMethodDoD)
				Expect(method).To(Equal("Sanitize"))
			})

			It("should default to Simple for unknown method", func() {
				method := getLenovoWipeMethod("unknown")
				Expect(method).To(Equal("Simple"))
			})
		})
	})

	Describe("DiskWipeMethod Constants", func() {
		It("should have expected constant values", func() {
			Expect(DiskWipeMethodQuick).To(Equal(DiskWipeMethod("quick")))
			Expect(DiskWipeMethodSecure).To(Equal(DiskWipeMethod("secure")))
			Expect(DiskWipeMethodDoD).To(Equal(DiskWipeMethod("dod")))
		})
	})

	Describe("Manufacturer Constants", func() {
		It("should have expected manufacturer values", func() {
			Expect(ManufacturerDell).To(Equal(Manufacturer("Dell Inc.")))
			Expect(ManufacturerHPE).To(Equal(Manufacturer("HPE")))
			Expect(ManufacturerLenovo).To(Equal(Manufacturer("Lenovo")))
			Expect(ManufacturerSupermicro).To(Equal(Manufacturer("Supermicro")))
		})
	})
})

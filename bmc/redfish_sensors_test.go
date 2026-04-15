// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GetSensors", func() {
	var (
		server     *httptest.Server
		redfishBMC BMC
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create a mock HTTP server to simulate Redfish API
		mux := http.NewServeMux()

		// Mock chassis endpoint
		mux.HandleFunc("/redfish/v1/Chassis/1U", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{
				"@odata.type": "#Chassis.v1_25_1.Chassis",
				"Id": "1U",
				"Name": "Computer System Chassis",
				"Sensors": {
					"@odata.id": "/redfish/v1/Chassis/1U/Sensors"
				}
			}`)
		})

		// Mock sensors collection endpoint
		mux.HandleFunc("/redfish/v1/Chassis/1U/Sensors", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{
				"@odata.type": "#SensorCollection.SensorCollection",
				"Name": "Chassis sensors",
				"Members@odata.count": 3,
				"Members": [
					{"@odata.id": "/redfish/v1/Chassis/1U/Sensors/AmbientTemp"},
					{"@odata.id": "/redfish/v1/Chassis/1U/Sensors/CPUFan1"},
					{"@odata.id": "/redfish/v1/Chassis/1U/Sensors/TotalPower"}
				]
			}`)
		})

		// Mock individual sensor endpoints
		mux.HandleFunc("/redfish/v1/Chassis/1U/Sensors/AmbientTemp", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{
				"@odata.type": "#Sensor.v1_8_1.Sensor",
				"Id": "AmbientTemp",
				"Name": "Ambient Temperature",
				"PhysicalContext": "Room",
				"Status": {
					"State": "Enabled",
					"Health": "OK"
				},
				"Reading": 22.5,
				"ReadingUnits": "Cel"
			}`)
		})

		mux.HandleFunc("/redfish/v1/Chassis/1U/Sensors/CPUFan1", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{
				"@odata.type": "#Sensor.v1_8_1.Sensor",
				"Id": "CPUFan1",
				"Name": "CPU #1 Fan Speed",
				"PhysicalContext": "CPU",
				"Reading": 80.0,
				"ReadingUnits": "%%",
				"Status": {
					"Health": "OK",
					"State": "Enabled"
				}
			}`)
		})

		mux.HandleFunc("/redfish/v1/Chassis/1U/Sensors/TotalPower", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{
				"@odata.type": "#Sensor.v1_8_1.Sensor",
				"Id": "TotalPower",
				"Name": "Total Power",
				"PhysicalContext": "Chassis",
				"Reading": 450.0,
				"ReadingUnits": "W",
				"Status": {
					"Health": "OK",
					"State": "Enabled"
				}
			}`)
		})

		// Mock chassis with no sensors (404)
		mux.HandleFunc("/redfish/v1/Chassis/NoSensors", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{
				"@odata.type": "#Chassis.v1_25_1.Chassis",
				"Id": "NoSensors",
				"Name": "Chassis without sensors"
			}`)
		})

		// Mock empty sensors collection
		mux.HandleFunc("/redfish/v1/Chassis/EmptySensors/Sensors", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{
				"@odata.type": "#SensorCollection.SensorCollection",
				"Name": "Empty sensors",
				"Members@odata.count": 0,
				"Members": []
			}`)
		})

		mux.HandleFunc("/redfish/v1/Chassis/EmptySensors", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{
				"@odata.type": "#Chassis.v1_25_1.Chassis",
				"Id": "EmptySensors",
				"Name": "Chassis with empty sensors",
				"Sensors": {
					"@odata.id": "/redfish/v1/Chassis/EmptySensors/Sensors"
				}
			}`)
		})

		// Mock sensor with nil reading
		mux.HandleFunc("/redfish/v1/Chassis/NilReading/Sensors", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{
				"@odata.type": "#SensorCollection.SensorCollection",
				"Name": "Sensors with nil reading",
				"Members@odata.count": 1,
				"Members": [
					{"@odata.id": "/redfish/v1/Chassis/NilReading/Sensors/BrokenSensor"}
				]
			}`)
		})

		mux.HandleFunc("/redfish/v1/Chassis/NilReading/Sensors/BrokenSensor", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{
				"@odata.type": "#Sensor.v1_8_1.Sensor",
				"Id": "BrokenSensor",
				"Name": "Broken Sensor",
				"PhysicalContext": "Chassis",
				"ReadingUnits": "Cel",
				"Status": {
					"Health": "Critical",
					"State": "Disabled"
				}
			}`)
		})

		mux.HandleFunc("/redfish/v1/Chassis/NilReading", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{
				"@odata.type": "#Chassis.v1_25_1.Chassis",
				"Id": "NilReading",
				"Name": "Chassis with nil reading sensor",
				"Sensors": {
					"@odata.id": "/redfish/v1/Chassis/NilReading/Sensors"
				}
			}`)
		})

		// Mock Redfish root
		mux.HandleFunc("/redfish/v1/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{
				"@odata.type": "#ServiceRoot.v1_18_0.ServiceRoot",
				"Id": "RootService",
				"Name": "Root Service"
			}`)
		})

		server = httptest.NewServer(mux)

		// Create RedfishBMC using NewRedfishBMCClient
		var err error
		redfishBMC, err = NewRedfishBMCClient(ctx, Options{
			Endpoint:   server.URL,
			Username:   "admin",
			Password:   "admin",
			InsecureTLS: true,
			BasicAuth:   true,
		})
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if redfishBMC != nil {
			redfishBMC.Logout()
		}
		if server != nil {
			server.Close()
		}
	})

	Describe("Successful sensor retrieval", func() {
		It("should retrieve all sensors from a chassis", func() {
			sensors, err := redfishBMC.GetSensors(ctx, "/redfish/v1/Chassis/1U")

			Expect(err).ToNot(HaveOccurred())
			Expect(sensors).To(HaveLen(3))
		})

		It("should correctly map sensor fields", func() {
			sensors, err := redfishBMC.GetSensors(ctx, "/redfish/v1/Chassis/1U")

			Expect(err).ToNot(HaveOccurred())
			Expect(sensors).To(HaveLen(3))

			// Find AmbientTemp sensor
			var ambientTemp *Sensor
			for i := range sensors {
				if sensors[i].ID == "AmbientTemp" {
					ambientTemp = &sensors[i]
					break
				}
			}

			Expect(ambientTemp).ToNot(BeNil())
			Expect(ambientTemp.ID).To(Equal("AmbientTemp"))
			Expect(ambientTemp.Name).To(Equal("Ambient Temperature"))
			Expect(ambientTemp.Reading).To(BeNumerically("==", 22.5))
			Expect(ambientTemp.Units).To(Equal("Cel"))
			Expect(ambientTemp.State).To(Equal("Enabled"))
			Expect(ambientTemp.PhysicalContext).To(Equal("Room"))
		})

		It("should correctly map CPU fan sensor", func() {
			sensors, err := redfishBMC.GetSensors(ctx, "/redfish/v1/Chassis/1U")

			Expect(err).ToNot(HaveOccurred())

			// Find CPUFan1 sensor
			var cpuFan *Sensor
			for i := range sensors {
				if sensors[i].ID == "CPUFan1" {
					cpuFan = &sensors[i]
					break
				}
			}

			Expect(cpuFan).ToNot(BeNil())
			Expect(cpuFan.ID).To(Equal("CPUFan1"))
			Expect(cpuFan.Name).To(Equal("CPU #1 Fan Speed"))
			Expect(cpuFan.Reading).To(BeNumerically("==", 80.0))
			Expect(cpuFan.Units).To(Equal("%"))
			Expect(cpuFan.State).To(Equal("Enabled"))
			Expect(cpuFan.PhysicalContext).To(Equal("CPU"))
		})

		It("should correctly map power sensor", func() {
			sensors, err := redfishBMC.GetSensors(ctx, "/redfish/v1/Chassis/1U")

			Expect(err).ToNot(HaveOccurred())

			// Find TotalPower sensor
			var totalPower *Sensor
			for i := range sensors {
				if sensors[i].ID == "TotalPower" {
					totalPower = &sensors[i]
					break
				}
			}

			Expect(totalPower).ToNot(BeNil())
			Expect(totalPower.ID).To(Equal("TotalPower"))
			Expect(totalPower.Name).To(Equal("Total Power"))
			Expect(totalPower.Reading).To(BeNumerically("==", 450.0))
			Expect(totalPower.Units).To(Equal("W"))
			Expect(totalPower.State).To(Equal("Enabled"))
			Expect(totalPower.PhysicalContext).To(Equal("Chassis"))
		})
	})

	Describe("Empty sensor collection", func() {
		It("should return empty slice for chassis with no sensors", func() {
			sensors, err := redfishBMC.GetSensors(ctx, "/redfish/v1/Chassis/EmptySensors")

			Expect(err).ToNot(HaveOccurred())
			Expect(sensors).To(BeEmpty())
		})
	})

	Describe("Chassis without sensors endpoint", func() {
		It("should return empty slice when sensors endpoint is not supported", func() {
			sensors, err := redfishBMC.GetSensors(ctx, "/redfish/v1/Chassis/NoSensors")

			Expect(err).ToNot(HaveOccurred())
			Expect(sensors).To(BeEmpty())
		})
	})

	Describe("Sensor with nil reading", func() {
		It("should handle sensor with nil reading by setting Reading to 0.0", func() {
			sensors, err := redfishBMC.GetSensors(ctx, "/redfish/v1/Chassis/NilReading")

			Expect(err).ToNot(HaveOccurred())
			Expect(sensors).To(HaveLen(1))

			brokenSensor := sensors[0]
			Expect(brokenSensor.ID).To(Equal("BrokenSensor"))
			Expect(brokenSensor.Name).To(Equal("Broken Sensor"))
			Expect(brokenSensor.Reading).To(BeNumerically("==", 0.0))
			Expect(brokenSensor.Units).To(Equal("Cel"))
			Expect(brokenSensor.State).To(Equal("Disabled"))
			Expect(brokenSensor.PhysicalContext).To(Equal("Chassis"))
		})
	})

	Describe("Error handling", func() {
		It("should handle missing chassis gracefully", func() {
			// Note: The actual error handling depends on the gofish library's behavior
			// when a chassis doesn't exist. In this test, we verify that GetSensors
			// doesn't panic and returns a reasonable result.
			sensors, err := redfishBMC.GetSensors(ctx, "/redfish/v1/Chassis/NonExistent")

			// The implementation may return either an error or an empty slice
			// depending on how gofish handles 404 responses
			if err != nil {
				Expect(err.Error()).To(ContainSubstring("failed to get chassis"))
			} else {
				Expect(sensors).To(BeEmpty())
			}
		})
	})
})

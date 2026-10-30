// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"

	"github.com/ironcore-dev/metal-operator/bmc/mock/server"
)

const lenovoRepoActionPath = "/redfish/v1/UpdateService/Actions/Oem/LenovoUpdateService.UpdateFromRepository"

// lenovoServiceRootJSON returns a service root that links the UpdateService.
func lenovoServiceRootJSON() []byte {
	b, _ := json.Marshal(map[string]any{
		"@odata.id":      "/redfish/v1/",
		"Id":             "ServiceRoot",
		"Name":           "Service Root",
		"RedfishVersion": "1.0.0",
		"UpdateService":  map[string]string{"@odata.id": "/redfish/v1/UpdateService"},
	})
	return b
}

// lenovoFirmwareSlot describes a FirmwareInventory member served by newLenovoInventoryMux.
// Version is `any` so that a nil value is served as JSON null, as the XCC does for some slots.
type lenovoFirmwareSlot struct {
	ID      string
	State   string
	Version any
}

// newLenovoInventoryMux serves a service root, UpdateService and a FirmwareInventory with the given slots.
func newLenovoInventoryMux(slots []lenovoFirmwareSlot) *http.ServeMux {
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		Expect(json.NewEncoder(w).Encode(v)).To(Succeed())
	}
	mux.HandleFunc("/redfish/v1/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(lenovoServiceRootJSON()) //nolint:errcheck
	})
	mux.HandleFunc("/redfish/v1/UpdateService", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"@odata.id":         "/redfish/v1/UpdateService",
			"Id":                "UpdateService",
			"FirmwareInventory": map[string]string{"@odata.id": "/redfish/v1/UpdateService/FirmwareInventory"},
		})
	})
	members := make([]map[string]string, 0, len(slots))
	for _, slot := range slots {
		odataID := "/redfish/v1/UpdateService/FirmwareInventory/" + slot.ID
		members = append(members, map[string]string{"@odata.id": odataID})
		mux.HandleFunc(odataID, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, map[string]any{
				"@odata.id": odataID,
				"Id":        slot.ID,
				"Version":   slot.Version,
				"Status":    map[string]string{"State": slot.State},
			})
		})
	}
	mux.HandleFunc("/redfish/v1/UpdateService/FirmwareInventory", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"@odata.id":           "/redfish/v1/UpdateService/FirmwareInventory",
			"Members":             members,
			"Members@odata.count": len(members),
		})
	})
	return mux
}

var _ = Describe("Lenovo OEM", func() {
	var lenovo *LenovoRedfishBMC

	BeforeEach(func() {
		lenovo = &LenovoRedfishBMC{}
	})

	Describe("lenovoExtractRepositoryTaskURI", func() {
		newResponse := func(body, location string) *http.Response {
			resp := &http.Response{Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
			if location != "" {
				resp.Header.Set("Location", location)
			}
			return resp
		}

		It("should prefer the task @odata.id from the body", func() {
			uri, err := lenovoExtractRepositoryTaskURI(newResponse(
				`{"@odata.id":"/redfish/v1/TaskService/Tasks/7","TaskState":"Running"}`,
				"/redfish/v1/TaskService/TaskMonitors/7"))
			Expect(err).NotTo(HaveOccurred())
			Expect(uri).To(Equal("/redfish/v1/TaskService/Tasks/7"))
		})

		It("should fall back to the Location header", func() {
			uri, err := lenovoExtractRepositoryTaskURI(newResponse(`{"TaskState":"Running"}`, "/redfish/v1/TaskService/TaskMonitors/7"))
			Expect(err).NotTo(HaveOccurred())
			Expect(uri).To(Equal("/redfish/v1/TaskService/TaskMonitors/7"))
		})

		It("should fall back to the Location header for a non-JSON body", func() {
			uri, err := lenovoExtractRepositoryTaskURI(newResponse("accepted", "/redfish/v1/TaskService/TaskMonitors/7"))
			Expect(err).NotTo(HaveOccurred())
			Expect(uri).To(Equal("/redfish/v1/TaskService/TaskMonitors/7"))
		})

		It("should fail without body URI and Location header", func() {
			_, err := lenovoExtractRepositoryTaskURI(newResponse("", ""))
			Expect(err).To(HaveOccurred())
		})

		DescribeTable("should normalize the task URI",
			func(body, location, expected string) {
				uri, err := lenovoExtractRepositoryTaskURI(newResponse(body, location))
				Expect(err).NotTo(HaveOccurred())
				Expect(uri).To(Equal(expected))
			},
			Entry("absolute @odata.id",
				`{"@odata.id":"https://10.0.0.1/redfish/v1/TaskService/Tasks/7"}`, "", "/redfish/v1/TaskService/Tasks/7"),
			Entry("absolute Location header",
				"", "https://10.0.0.1:443/redfish/v1/TaskService/TaskMonitors/7", "/redfish/v1/TaskService/TaskMonitors/7"),
			Entry("query is kept",
				"", "/redfish/v1/TaskService/TaskMonitors/7?a=b", "/redfish/v1/TaskService/TaskMonitors/7?a=b"),
		)

		DescribeTable("should reject task URIs outside the TaskService",
			func(body, location string) {
				_, err := lenovoExtractRepositoryTaskURI(newResponse(body, location))
				Expect(err).To(HaveOccurred())
			},
			Entry("other resource in body", `{"@odata.id":"/redfish/v1/Systems/1"}`, ""),
			Entry("other resource in Location header", "", "https://10.0.0.1/redfish/v1/Managers/1"),
			Entry("path traversal", "", "/redfish/v1/TaskService/../Systems/1"),
			Entry("TaskService root", "", "/redfish/v1/TaskService"),
		)
	})

	Describe("InstallFirmwareFromRepository", func() {
		newMux := func(handler http.HandlerFunc) *http.ServeMux {
			mux := http.NewServeMux()
			mux.HandleFunc("/redfish/v1/", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write(lenovoServiceRootJSON()) //nolint:errcheck
			})
			mux.HandleFunc(lenovoRepoActionPath, handler)
			return mux
		}

		It("should send the RepoURI and return the task URI from the body", func() {
			srv := httptest.NewServer(newMux(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodPost))
				var body map[string]any
				Expect(json.NewDecoder(r.Body).Decode(&body)).To(Succeed())
				Expect(body).To(Equal(map[string]any{"RepoURI": "https://repo.example.com/fw.zip"}))

				// Response recorded on an SR650 V4 (XCC 28L-2.00).
				w.Header().Set("Location", "/redfish/v1/TaskService/TaskMonitors/64")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusAccepted)
				_, _ = w.Write([]byte(`{
					"@odata.context": "/redfish/v1/$metadata#Task.Task",
					"@odata.id": "/redfish/v1/TaskService/Tasks/64",
					"@odata.type": "#Task.v1_7_4.Task",
					"HidePayload": true,
					"Id": "64",
					"Messages": [{"Message": "The task with Id '64' has started.", "MessageArgs": ["64"],
						"MessageId": "TaskEvent.1.0.3.TaskStarted", "MessageSeverity": "OK", "Resolution": "None."}],
					"Name": "Task 64",
					"StartTime": "2026-10-29T13:55:22+00:00",
					"TaskMonitor": "/redfish/v1/TaskService/TaskMonitors/64",
					"TaskState": "Running",
					"TaskStatus": "OK"
				}`))
			}))
			defer srv.Close()

			lenovo.RedfishBaseBMC = newTestRedfishBMC(srv)
			taskURI, isFatal, err := lenovo.InstallFirmwareFromRepository(context.Background(), &LenovoRepositoryUpdateParameters{RepoURI: "https://repo.example.com/fw.zip"})
			Expect(err).NotTo(HaveOccurred())
			Expect(isFatal).To(BeFalse())
			Expect(taskURI).To(Equal("/redfish/v1/TaskService/Tasks/64"))
		})

		It("should return the task monitor URI from the Location header if the body has none", func() {
			srv := httptest.NewServer(newMux(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", "/redfish/v1/TaskService/TaskMonitors/12")
				w.WriteHeader(http.StatusAccepted)
			}))
			defer srv.Close()

			lenovo.RedfishBaseBMC = newTestRedfishBMC(srv)
			taskURI, isFatal, err := lenovo.InstallFirmwareFromRepository(context.Background(), &LenovoRepositoryUpdateParameters{RepoURI: "https://repo.example.com/fw.zip"})
			Expect(err).NotTo(HaveOccurred())
			Expect(isFatal).To(BeFalse())
			Expect(taskURI).To(Equal("/redfish/v1/TaskService/TaskMonitors/12"))
		})

		It("should return a fatal error if no task URI is returned", func() {
			srv := httptest.NewServer(newMux(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusAccepted)
			}))
			defer srv.Close()

			lenovo.RedfishBaseBMC = newTestRedfishBMC(srv)
			_, isFatal, err := lenovo.InstallFirmwareFromRepository(context.Background(), &LenovoRepositoryUpdateParameters{RepoURI: "https://repo.example.com/fw.zip"})
			Expect(err).To(HaveOccurred())
			Expect(isFatal).To(BeTrue())
		})

		It("should return a fatal error on an unexpected status code", func() {
			srv := httptest.NewServer(newMux(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			defer srv.Close()

			lenovo.RedfishBaseBMC = newTestRedfishBMC(srv)
			_, isFatal, err := lenovo.InstallFirmwareFromRepository(context.Background(), &LenovoRepositoryUpdateParameters{RepoURI: "https://repo.example.com/fw.zip"})
			Expect(err).To(HaveOccurred())
			Expect(isFatal).To(BeTrue())
		})

		It("should return a fatal error on an error response", func() {
			srv := httptest.NewServer(newMux(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "bad request", http.StatusBadRequest)
			}))
			defer srv.Close()

			lenovo.RedfishBaseBMC = newTestRedfishBMC(srv)
			_, isFatal, err := lenovo.InstallFirmwareFromRepository(context.Background(), &LenovoRepositoryUpdateParameters{RepoURI: "https://repo.example.com/fw.zip"})
			Expect(err).To(HaveOccurred())
			Expect(isFatal).To(BeTrue())
		})

		DescribeTable("should return a non-fatal error for missing parameters without calling the BMC",
			func(parameters *LenovoRepositoryUpdateParameters) {
				srv := httptest.NewServer(newMux(func(_ http.ResponseWriter, _ *http.Request) {
					Fail("UpdateFromRepository must not be called")
				}))
				defer srv.Close()

				lenovo.RedfishBaseBMC = newTestRedfishBMC(srv)
				_, isFatal, err := lenovo.InstallFirmwareFromRepository(context.Background(), parameters)
				Expect(err).To(HaveOccurred())
				Expect(isFatal).To(BeFalse())
			},
			Entry("nil parameters", nil),
			Entry("empty RepoURI", &LenovoRepositoryUpdateParameters{}),
		)
	})

	Describe("GetRepositoryUpdateTask", func() {
		DescribeTable("should parse the task",
			func(taskJSON string, expectedState schemas.TaskState, expectedStatus schemas.Health, expectedMessageID string) {
				mux := http.NewServeMux()
				mux.HandleFunc("/redfish/v1/", func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Write(lenovoServiceRootJSON()) //nolint:errcheck
				})
				mux.HandleFunc("/redfish/v1/TaskService/Tasks/12", func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(taskJSON)) //nolint:errcheck
				})
				srv := httptest.NewServer(mux)
				defer srv.Close()

				lenovo.RedfishBaseBMC = newTestRedfishBMC(srv)
				task, err := lenovo.GetRepositoryUpdateTask(context.Background(), "/redfish/v1/TaskService/Tasks/12")
				Expect(err).NotTo(HaveOccurred())
				Expect(task.TaskState).To(Equal(expectedState))
				Expect(task.TaskStatus).To(Equal(expectedStatus))
				if expectedMessageID != "" {
					Expect(task.Messages).To(HaveLen(1))
					Expect(task.Messages[0].MessageID).To(Equal(expectedMessageID))
				}
			},
			Entry("running",
				`{"@odata.id":"/redfish/v1/TaskService/Tasks/12","Id":"12","TaskState":"Running","TaskStatus":"OK"}`,
				schemas.RunningTaskState, schemas.OKHealth, ""),
			Entry("completed",
				`{"@odata.id":"/redfish/v1/TaskService/Tasks/12","Id":"12","TaskState":"Completed","TaskStatus":"OK","PercentComplete":100}`,
				schemas.CompletedTaskState, schemas.OKHealth, ""),
			Entry("completed (recorded on an SR650 V4, XCC 28L-2.00)",
				`{"@odata.context": "/redfish/v1/$metadata#Task.Task", "@odata.etag": "W/\"326066BB\"",
				  "@odata.id": "/redfish/v1/TaskService/Tasks/64", "@odata.type": "#Task.v1_7_4.Task",
				  "EndTime": "2026-10-29T13:58:12+00:00", "HidePayload": true, "Id": "64",
				  "Messages": [{"Message": "The task with Id '64' has started.", "MessageArgs": ["64"],
				    "MessageId": "TaskEvent.1.0.3.TaskStarted", "MessageSeverity": "OK", "Resolution": "None."}],
				  "Name": "Task 64", "StartTime": "2026-10-29T13:55:22+00:00",
				  "TaskMonitor": "/redfish/v1/TaskService/TaskMonitors/64", "TaskState": "Completed", "TaskStatus": "OK"}`,
				schemas.CompletedTaskState, schemas.OKHealth, "TaskEvent.1.0.3.TaskStarted"),
			Entry("exception",
				`{"@odata.id":"/redfish/v1/TaskService/Tasks/12","Id":"12","TaskState":"Exception","TaskStatus":"Warning",
				  "Messages":[{"MessageId":"LenovoFirmwareUpdate.1.0.0.SyncRepoSubstateFailed","Message":"failed"}]}`,
				schemas.ExceptionTaskState, schemas.WarningHealth, "LenovoFirmwareUpdate.1.0.0.SyncRepoSubstateFailed"),
		)

		It("should return an error if the task cannot be fetched", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/redfish/v1/", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write(lenovoServiceRootJSON()) //nolint:errcheck
			})
			mux.HandleFunc("/redfish/v1/TaskService/Tasks/12", func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
			})
			srv := httptest.NewServer(mux)
			defer srv.Close()

			lenovo.RedfishBaseBMC = newTestRedfishBMC(srv)
			_, err := lenovo.GetRepositoryUpdateTask(context.Background(), "/redfish/v1/TaskService/Tasks/12")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("GetRepositoryUpdateDetail", func() {
		const detailPath = "/redfish/v1/UpdateService/Actions/Oem/LenovoUpdateService.GetRepoUpdateDetail"

		newMux := func(handler http.HandlerFunc) *http.ServeMux {
			mux := http.NewServeMux()
			mux.HandleFunc("/redfish/v1/", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write(lenovoServiceRootJSON()) //nolint:errcheck
			})
			mux.HandleFunc(detailPath, handler)
			return mux
		}

		It("should POST an empty body and parse the finished components", func() {
			srv := httptest.NewServer(newMux(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodPost))
				var body map[string]any
				Expect(json.NewDecoder(r.Body).Decode(&body)).To(Succeed())
				Expect(body).To(BeEmpty())

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"BundleErrorCode": 0,
					"FinishedList": [
						{"category": "LXPM", "devices": [], "method": "Core",
						 "payload": "payloads/lnvgy_fw_lxpm_dfl110m-5.03.00_anyos_comp.uxz", "reboot": "noreboot", "ret": 0},
						{"category": "DISK", "method": "OOB", "reboot": "reboot", "ret": 0,
						 "payload": "payloads/firmware/lnvgy_fw_drives_all.mu.7450-e2mu300-e2mu300-02e_anyos_comp.lvt",
						 "devices": [{
							"drive": {"ctrladdr": 2293760, "method": "Storelib", "model": "", "name": "M.2 Drive 1"},
							"location": "boot_drive_bay_1", "name": "", "oob": true, "partnum": "MTFDKBA960TFR",
							"serialnum": "24114777955E", "swid": "DISK-MTFDKBA960TFR-3-10", "version": "E2MU300"}]}
					]
				}`))
			}))
			defer srv.Close()

			lenovo.RedfishBaseBMC = newTestRedfishBMC(srv)
			detail, err := lenovo.GetRepositoryUpdateDetail(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(detail.BundleErrorCode).To(Equal(0))
			Expect(detail.FinishedList).To(Equal([]LenovoRepositoryUpdateComponent{
				{
					Category: "LXPM",
					Method:   "Core",
					Payload:  "payloads/lnvgy_fw_lxpm_dfl110m-5.03.00_anyos_comp.uxz",
					Reboot:   "noreboot",
					Devices:  []LenovoRepositoryUpdateDevice{},
				},
				{
					Category: "DISK",
					Method:   "OOB",
					Payload:  "payloads/firmware/lnvgy_fw_drives_all.mu.7450-e2mu300-e2mu300-02e_anyos_comp.lvt",
					Reboot:   "reboot",
					Devices: []LenovoRepositoryUpdateDevice{{
						Location:     "boot_drive_bay_1",
						PartNumber:   "MTFDKBA960TFR",
						SerialNumber: "24114777955E",
						SoftwareID:   "DISK-MTFDKBA960TFR-3-10",
						Version:      "E2MU300",
						OOB:          true,
					}},
				},
			}))
		})

		It("should return non-zero result codes as-is (recorded on an SR650 V4, XCC 28L-2.00)", func() {
			srv := httptest.NewServer(newMux(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"BundleErrorCode": 2,
					"FinishedList": [
						{"category": "DEVICE", "method": "OOB", "reboot": "reboot", "ret": 2,
						 "payload": "payloads/brcm-lnvgy_fw_nic_nxe-238.1.169.0-ja9ua-064_anyos_comp.uxz",
						 "devices": [{"location": "14", "name": "Broadcom 57508 100GbE QSFP56 2-Port OCP Ethernet Adapter",
							"oob": true, "partnum": "", "serialnum": "", "swid": "14E4-1750-17AA-4103", "version": "232.1.132.8"}]}
					]
				}`))
			}))
			defer srv.Close()

			lenovo.RedfishBaseBMC = newTestRedfishBMC(srv)
			detail, err := lenovo.GetRepositoryUpdateDetail(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(detail.BundleErrorCode).To(Equal(2))
			Expect(detail.FinishedList).To(HaveLen(1))
			Expect(detail.FinishedList[0].Ret).To(Equal(2))
			Expect(detail.FinishedList[0].Devices[0].SoftwareID).To(Equal("14E4-1750-17AA-4103"))
		})

		It("should return an error on an error response", func() {
			srv := httptest.NewServer(newMux(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "bad request", http.StatusBadRequest)
			}))
			defer srv.Close()

			lenovo.RedfishBaseBMC = newTestRedfishBMC(srv)
			_, err := lenovo.GetRepositoryUpdateDetail(context.Background())
			Expect(err).To(HaveOccurred())
		})

		It("should return an error on a malformed body", func() {
			srv := httptest.NewServer(newMux(func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte("not json")) //nolint:errcheck
			}))
			defer srv.Close()

			lenovo.RedfishBaseBMC = newTestRedfishBMC(srv)
			_, err := lenovo.GetRepositoryUpdateDetail(context.Background())
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("GetPendingFirmwareComponents", func() {
		It("should return only staged non-BMC/UEFI pending slots", func() {
			srv := httptest.NewServer(newLenovoInventoryMux([]lenovoFirmwareSlot{
				{ID: "HPM-FPGA", State: "Enabled", Version: "1.0.0"},
				{ID: "HPM-FPGA-Pending", State: "StandbyOffline", Version: "1.1.0"},
				{ID: "LXPM", State: "Enabled", Version: "4.0"},
				{ID: "LXPM-Pending", State: "StandbyOffline", Version: "4.0"}, // same version as active still counts
				{ID: "PSU1-Pending", State: "Disabled", Version: ""},          // activated
				{ID: "PSU2-Pending", State: "StandbyOffline", Version: ""},    // no version
				{ID: "UEFI", State: "Enabled", Version: "3.0"},
				{ID: "UEFI-Pending", State: "StandbyOffline", Version: "3.1"},        // managed by BIOSVersion
				{ID: "BMC-Primary-Pending", State: "StandbyOffline", Version: "5.0"}, // managed by BMCVersion
				{ID: "XCC-Pending", State: "StandbyOffline", Version: "5.0"},         // managed by BMCVersion
				{ID: "Slot_1.Bundle", State: "Enabled", Version: "1.0"},              // no pending slot
			}))
			defer srv.Close()

			lenovo.RedfishBaseBMC = newTestRedfishBMC(srv)
			pending, err := lenovo.GetPendingFirmwareComponents(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(pending).To(ConsistOf("HPM-FPGA-Pending", "LXPM-Pending"))
		})

		It("should return nothing if all pending slots are activated", func() {
			srv := httptest.NewServer(newLenovoInventoryMux([]lenovoFirmwareSlot{
				{ID: "HPM-FPGA", State: "Enabled", Version: "1.1.0"},
				{ID: "HPM-FPGA-Pending", State: "Disabled", Version: ""},
			}))
			defer srv.Close()

			lenovo.RedfishBaseBMC = newTestRedfishBMC(srv)
			pending, err := lenovo.GetPendingFirmwareComponents(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(pending).To(BeEmpty())
		})

		It("should ignore pending slots with a null Version", func() {
			srv := httptest.NewServer(newLenovoInventoryMux([]lenovoFirmwareSlot{
				{ID: "Drive_Backplane_SR17B71473_L1HF4BM008A_PSoC_0xBA-Pending", State: "Disabled", Version: nil},
				{ID: "Drive_Backplane_STA7B78556_L1HF53C01S2_PSoC_0xBB-Pending", State: "StandbyOffline", Version: nil},
			}))
			defer srv.Close()

			lenovo.RedfishBaseBMC = newTestRedfishBMC(srv)
			pending, err := lenovo.GetPendingFirmwareComponents(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(pending).To(BeEmpty())
		})

		// Inventory recorded on an SR650 V4 (XCC 28L-2.00) right after a repository
		// update that staged a NIC firmware: network adapters have no "-Pending"
		// slot, so the staged NIC update is not reported.
		recordedInventory := func(hpmPending lenovoFirmwareSlot) []lenovoFirmwareSlot {
			return []lenovoFirmwareSlot{
				{ID: "BMC-Primary", State: "Enabled", Version: "28L-2.00"},
				{ID: "BMC-Primary-Pending", State: "Disabled", Version: ""},
				{ID: "BMC-Backup", State: "StandbySpare", Version: "28L-2.00"},
				{ID: "HPM-FPGA", State: "Enabled", Version: "06H-4.38"},
				hpmPending,
				{ID: "UEFI", State: "Enabled", Version: "16I-1.40"},
				{ID: "UEFI-Pending", State: "Disabled", Version: ""},
				{ID: "LXPM", State: "Enabled", Version: "10M-5.03.00"},
				{ID: "LXPMWindowsDriver", State: "Enabled", Version: "10K-5.03.00"},
				{ID: "LXPMLinuxDriver", State: "Enabled", Version: "10K-5.03.00"},
				{ID: "LXUM", State: "Enabled", Version: "06J-1.12"},
				{ID: "SCM-FPGA", State: "Enabled", Version: "02Q-2.47"},
				{ID: "SCM-FPGA-Pending", State: "Disabled", Version: ""},
				{ID: "Bundle.Current", State: "Enabled", Version: "238.1.169.0"},
				{ID: "boot_drive_bay_0", State: "Enabled", Version: "E2MU300"},
				{ID: "boot_drive_bay_1", State: "Enabled", Version: "E2MU300"},
				{ID: "drive_bay_0", State: "Enabled", Version: "LDDM7U2Q"},
				{ID: "PCIeDevice_slot13_0xd0000.Bundle", State: "Enabled", Version: "233.0.4.3"},
				{ID: "PCIeDevice_slot14_0xb40000.Bundle", State: "Enabled", Version: "232.1.132.8"},
				{ID: "PCIeDevice_slot25_0x230000.Bundle", State: "Enabled", Version: "52.32.0-5988"},
				{ID: "Drive_Backplane_STA7B78556_L1HF53C01S2_PSoC_0xBB", State: "Enabled", Version: "26"},
				{ID: "Drive_Backplane_STA7B78556_L1HF53C01S2_PSoC_0xBB-Pending", State: "Disabled", Version: nil},
				{ID: "Drive_Backplane_SR17B71473_L1HF4BM008A_PSoC_0xBA", State: "Enabled", Version: "8"},
				{ID: "Drive_Backplane_SR17B71473_L1HF4BM008A_PSoC_0xBA-Pending", State: "Disabled", Version: nil},
				{ID: "PSU1", State: "Enabled", Version: "2.55"},
				{ID: "PSU2", State: "Enabled", Version: "2.55"},
			}
		}

		DescribeTable("with a recorded XCC inventory",
			func(hpmPending lenovoFirmwareSlot, expected []string) {
				srv := httptest.NewServer(newLenovoInventoryMux(recordedInventory(hpmPending)))
				defer srv.Close()

				lenovo.RedfishBaseBMC = newTestRedfishBMC(srv)
				pending, err := lenovo.GetPendingFirmwareComponents(context.Background())
				Expect(err).NotTo(HaveOccurred())
				Expect(pending).To(ConsistOf(expected))
			},
			Entry("nothing staged in a pending slot (staged NIC is not visible)",
				lenovoFirmwareSlot{ID: "HPM-FPGA-Pending", State: "Disabled", Version: ""}, []string{}),
			Entry("HPM FPGA staged",
				lenovoFirmwareSlot{ID: "HPM-FPGA-Pending", State: "StandbyOffline", Version: "06H-4.38"}, []string{"HPM-FPGA-Pending"}),
		)
	})

	Describe("with the mock server", func() {
		var (
			ms     *server.MockServer
			cancel context.CancelFunc
		)

		BeforeEach(func() {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			Expect(err).NotTo(HaveOccurred())
			addr := listener.Addr().String()
			Expect(listener.Close()).To(Succeed())

			ms = server.NewMockServer(GinkgoLogr, addr, server.WithLenovoFirmwareInventory())
			var ctx context.Context
			ctx, cancel = context.WithCancel(context.Background())
			go func() {
				defer GinkgoRecover()
				Expect(ms.Start(ctx)).To(Succeed())
			}()
			DeferCleanup(func() {
				ms.ResetLenovoRepositoryUpdate()
				cancel()
			})

			srvURL := "http://" + addr
			Eventually(func() error {
				resp, err := http.Get(srvURL + "/redfish/v1/")
				if err != nil {
					return err
				}
				return resp.Body.Close()
			}).Should(Succeed())

			client, err := gofish.ConnectContext(context.Background(), gofish.ClientConfig{
				Endpoint:  srvURL,
				Username:  "admin",
				Password:  "admin",
				BasicAuth: true,
			})
			Expect(err).NotTo(HaveOccurred())
			lenovo.RedfishBaseBMC = &RedfishBaseBMC{client: client}
		})

		getPending := func() []string {
			pending, err := lenovo.GetPendingFirmwareComponents(context.Background())
			Expect(err).NotTo(HaveOccurred())
			return pending
		}

		getVersion := func(id string) string {
			updateService, err := lenovo.client.GetService().UpdateService()
			Expect(err).NotTo(HaveOccurred())
			inventory, err := updateService.FirmwareInventory()
			Expect(err).NotTo(HaveOccurred())
			for _, fw := range inventory {
				if fw.ID == id {
					return fw.Version
				}
			}
			Fail("firmware inventory entry not found: " + id)
			return ""
		}

		It("should stage firmware on success and activate it on host reset", func(ctx SpecContext) {
			Expect(getPending()).To(BeEmpty(), "staged UEFI must be ignored")

			taskURI, isFatal, err := lenovo.InstallFirmwareFromRepository(ctx, &LenovoRepositoryUpdateParameters{RepoURI: "https://repo.example.com/fw.zip"})
			Expect(err).NotTo(HaveOccurred())
			Expect(isFatal).To(BeFalse())
			Expect(taskURI).To(Equal("/redfish/v1/TaskService/Tasks/lenovo-repo"))

			Eventually(func(g Gomega) {
				task, err := lenovo.GetRepositoryUpdateTask(ctx, taskURI)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(task.TaskState).To(Equal(schemas.CompletedTaskState))
				g.Expect(task.TaskStatus).To(Equal(schemas.OKHealth))
			}).Should(Succeed())
			Expect(getPending()).To(ConsistOf("HPM-FPGA-Pending"), "staged NIC has no pending slot")
			Expect(getVersion("PCIeDevice_slot14_0xb40000.Bundle")).To(Equal("232.1.132.8"), "NIC is only staged")

			detail, err := lenovo.GetRepositoryUpdateDetail(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(detail.BundleErrorCode).To(Equal(2))
			Expect(detail.FinishedList).To(HaveLen(2))
			for _, c := range detail.FinishedList {
				Expect(c.Ret).To(Equal(2))
				Expect(c.Reboot).To(Equal("reboot"))
				Expect(c.Devices).To(HaveLen(1))
			}
			Expect(detail.FinishedList[1].Category).To(Equal("DEVICE"))
			Expect(detail.FinishedList[1].Devices[0].Version).To(Equal("232.1.132.8"), "version before the run")

			// The task monitor URI returns the same task.
			task, err := lenovo.GetRepositoryUpdateTask(ctx, "/redfish/v1/TaskService/TaskMonitors/lenovo-repo")
			Expect(err).NotTo(HaveOccurred())
			Expect(task.TaskState).To(Equal(schemas.CompletedTaskState))

			Expect(lenovo.PowerOff(ctx, "/redfish/v1/Systems/437XR1138R2")).To(Succeed())
			Eventually(func(g Gomega) {
				system, err := lenovo.getSystemFromUri(ctx, "/redfish/v1/Systems/437XR1138R2")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(system.PowerState).To(Equal(schemas.OffPowerState))
			}).Should(Succeed())
			Expect(lenovo.PowerOn(ctx, "/redfish/v1/Systems/437XR1138R2")).To(Succeed())
			Eventually(getPending).Should(BeEmpty())
			Expect(getVersion("HPM-FPGA")).To(Equal("https://repo.example.com/fw.zip"))
			Expect(getVersion("PCIeDevice_slot14_0xb40000.Bundle")).To(Equal("https://repo.example.com/fw.zip"))
		}, SpecTimeout(10*time.Second))

		It("should report an exception for an invalid repository", func(ctx SpecContext) {
			taskURI, isFatal, err := lenovo.InstallFirmwareFromRepository(ctx, &LenovoRepositoryUpdateParameters{RepoURI: "https://repo.example.com/fail.zip"})
			Expect(err).NotTo(HaveOccurred())
			Expect(isFatal).To(BeFalse())

			Eventually(func(g Gomega) {
				task, err := lenovo.GetRepositoryUpdateTask(ctx, taskURI)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(task.TaskState).To(Equal(schemas.ExceptionTaskState))
				g.Expect(task.TaskStatus).To(Equal(schemas.WarningHealth))
				g.Expect(slices.ContainsFunc(task.Messages, func(m schemas.Message) bool {
					return m.MessageID == "LenovoFirmwareUpdate.1.0.0.SyncRepoSubstateFailed"
				})).To(BeTrue())
			}).Should(Succeed())
			Expect(getPending()).To(BeEmpty())

			detail, err := lenovo.GetRepositoryUpdateDetail(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(detail.FinishedList).To(BeEmpty())
		}, SpecTimeout(10*time.Second))
	})
})

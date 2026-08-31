// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stmcginnis/gofish/schemas"
)

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

	Describe("dellBuildRepositoryUpdateRequestBody", func() {
		It("should build the request body with correct field encodings", func() {
			params := &RepositoryUpdateParameters{
				ShareType:         "HTTPS",
				IPAddress:         "example.com",
				ShareName:         "share",
				CatalogFile:       "Catalog.xml",
				UserName:          "admin",
				Password:          "secret",
				Workgroup:         "WORKGROUP",
				IgnoreCertWarning: true,
				ApplyUpdate:       true,
				RebootNeeded:      true,
			}

			body := dellBuildRepositoryUpdateRequestBody(params)

			Expect(body.ShareType).To(Equal("HTTPS"))
			Expect(body.IPAddress).To(Equal("example.com"))
			Expect(body.ShareName).To(Equal("share"))
			Expect(body.CatalogFile).To(Equal("Catalog.xml"))
			Expect(body.UserName).To(Equal("admin"))
			Expect(body.Password).To(Equal("secret"))
			Expect(body.Workgroup).To(Equal("WORKGROUP"))
			Expect(body.IgnoreCertWarning).To(Equal("On"))
			Expect(body.ApplyUpdate).To(Equal("True"))
			Expect(body.RebootNeeded).To(BeTrue())
		})

		It("should render false booleans as \"False\"", func() {
			body := dellBuildRepositoryUpdateRequestBody(&RepositoryUpdateParameters{})
			Expect(body.IgnoreCertWarning).To(Equal("Off"))
			Expect(body.ApplyUpdate).To(Equal("False"))
			Expect(body.ApplySameVersions).To(Equal("False"))
			Expect(body.ApplyDowngradeVersions).To(Equal("False"))
		})

		It("should encode ApplySameVersions and ApplyDowngradeVersions", func() {
			body := dellBuildRepositoryUpdateRequestBody(&RepositoryUpdateParameters{
				ApplySameVersions:      true,
				ApplyDowngradeVersions: true,
			})
			Expect(body.ApplySameVersions).To(Equal("True"))
			Expect(body.ApplyDowngradeVersions).To(Equal("True"))
		})
	})

	Describe("dellRepositoryActionTarget", func() {
		It("should build the action URI under the system's OEM software installation service", func() {
			target := dellRepositoryActionTarget("/redfish/v1/Systems/1", "InstallFromRepository")
			Expect(target).To(Equal("/redfish/v1/Systems/1/Oem/Dell/DellSoftwareInstallationService/Actions/DellSoftwareInstallationService.InstallFromRepository"))
		})

		It("should normalize a trailing slash on the system URI", func() {
			target := dellRepositoryActionTarget("/redfish/v1/Systems/1/", "GetRepoBasedUpdateList")
			Expect(target).To(Equal("/redfish/v1/Systems/1/Oem/Dell/DellSoftwareInstallationService/Actions/DellSoftwareInstallationService.GetRepoBasedUpdateList"))
		})
	})

	Describe("dellExtractJobID", func() {
		It("should extract the job ID from the Location header", func() {
			resp := &http.Response{
				Header: make(http.Header),
				Body:   io.NopCloser(strings.NewReader("")),
			}
			resp.Header.Set("Location", "/redfish/v1/Managers/BMC/Oem/Dell/Jobs/JID_1234")

			jobID, err := dellExtractJobID(resp)
			Expect(err).ToNot(HaveOccurred())
			Expect(jobID).To(Equal("JID_1234"))
		})

		It("should return an error when no Location header is present", func() {
			resp := &http.Response{
				Header: make(http.Header),
				Body:   io.NopCloser(strings.NewReader("")),
			}

			_, err := dellExtractJobID(resp)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no Location header"))
		})
	})

	Describe("dellPackageListHasPendingPackages", func() {
		It("should return false for an empty package list", func() {
			Expect(dellPackageListHasPendingPackages("")).To(BeFalse())
			Expect(dellPackageListHasPendingPackages("   ")).To(BeFalse())
		})

		It("should return true when a PACKAGE element is present", func() {
			Expect(dellPackageListHasPendingPackages("<PACKAGELIST><PACKAGE NAME=\"BIOS\"/></PACKAGELIST>")).To(BeTrue())
		})

		It("should return true when an INSTANCE element is present", func() {
			Expect(dellPackageListHasPendingPackages("<INSTANCE CLASSNAME=\"foo\"/>")).To(BeTrue())
		})

		It("should return false when no package/instance element is present", func() {
			Expect(dellPackageListHasPendingPackages("<UpdateList></UpdateList>")).To(BeFalse())
		})
	})

	Describe("DellJob classification", func() {
		DescribeTable("IsCompleted",
			func(job *DellJob, expected bool) {
				Expect(job.IsCompleted()).To(Equal(expected))
			},
			Entry("Completed state", &DellJob{State: "Completed"}, true),
			Entry("Running state", &DellJob{State: "Running"}, false),
			Entry("CompletedWithErrors state", &DellJob{State: "CompletedWithErrors"}, false),
		)

		DescribeTable("IsFailed",
			func(job *DellJob, expected bool) {
				Expect(job.IsFailed()).To(Equal(expected))
			},
			Entry("Failed state", &DellJob{State: "Failed"}, true),
			Entry("CompletedWithErrors state", &DellJob{State: "CompletedWithErrors"}, true),
			Entry("RebootFailed state", &DellJob{State: "RebootFailed"}, true),
			Entry("Running state with no failure message", &DellJob{State: "Running", Message: "Job execution in progress."}, false),
			Entry("Running state with a failure message", &DellJob{State: "Running", Message: "Unable to apply the update."}, false),
			Entry("Completed state", &DellJob{State: "Completed"}, false),
		)

		DescribeTable("IsTerminal",
			func(job *DellJob, expected bool) {
				Expect(job.IsTerminal()).To(Equal(expected))
			},
			Entry("Completed state", &DellJob{State: "Completed"}, true),
			Entry("Failed state", &DellJob{State: "Failed"}, true),
			Entry("Running state", &DellJob{State: "Running"}, false),
			Entry("Scheduled state", &DellJob{State: "Scheduled"}, false),
		)
	})

	Describe("InstallFirmwareFromRepository", func() {
		It("should return the job ID on a successful 202 response", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/redfish/v1/", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write(serviceRootJSON()) //nolint:errcheck
			})
			mux.HandleFunc("/redfish/v1/Systems/1/Oem/Dell/DellSoftwareInstallationService/Actions/DellSoftwareInstallationService.InstallFromRepository",
				func(w http.ResponseWriter, r *http.Request) {
					Expect(r.Method).To(Equal(http.MethodPost))
					w.Header().Set("Location", "/redfish/v1/Managers/BMC/Oem/Dell/Jobs/JID_5678")
					w.WriteHeader(http.StatusAccepted)
				})

			server := httptest.NewServer(mux)
			defer server.Close()

			dell.RedfishBaseBMC = newTestRedfishBMC(server)
			jobID, isFatal, err := dell.InstallFirmwareFromRepository(context.Background(), "/redfish/v1/Systems/1", &RepositoryUpdateParameters{})
			Expect(err).ToNot(HaveOccurred())
			Expect(isFatal).To(BeFalse())
			Expect(jobID).To(Equal("JID_5678"))
		})

		It("should return a fatal error on a non-202 response", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/redfish/v1/", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write(serviceRootJSON()) //nolint:errcheck
			})
			mux.HandleFunc("/redfish/v1/Systems/1/Oem/Dell/DellSoftwareInstallationService/Actions/DellSoftwareInstallationService.InstallFromRepository",
				func(w http.ResponseWriter, _ *http.Request) {
					http.Error(w, "bad request", http.StatusBadRequest)
				})

			server := httptest.NewServer(mux)
			defer server.Close()

			dell.RedfishBaseBMC = newTestRedfishBMC(server)
			_, isFatal, err := dell.InstallFirmwareFromRepository(context.Background(), "/redfish/v1/Systems/1", &RepositoryUpdateParameters{})
			Expect(err).To(HaveOccurred())
			Expect(isFatal).To(BeTrue())
		})

		It("should return a fatal error when the connection is dropped mid-response", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/redfish/v1/", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write(serviceRootJSON()) //nolint:errcheck
			})
			mux.HandleFunc("/redfish/v1/Systems/1/Oem/Dell/DellSoftwareInstallationService/Actions/DellSoftwareInstallationService.InstallFromRepository",
				func(w http.ResponseWriter, r *http.Request) {
					// Hijack and close the connection immediately to simulate a dropped response.
					hj, ok := w.(http.Hijacker)
					if !ok {
						http.Error(w, "hijack not supported", http.StatusInternalServerError)
						return
					}
					conn, _, _ := hj.Hijack()
					_ = conn.Close()
				})

			server := httptest.NewServer(mux)
			defer server.Close()

			dell.RedfishBaseBMC = newTestRedfishBMC(server)
			_, isFatal, err := dell.InstallFirmwareFromRepository(context.Background(), "/redfish/v1/Systems/1", &RepositoryUpdateParameters{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to issue repository firmware install"))
			Expect(isFatal).To(BeTrue())
		})
	})

	Describe("GetRepositoryUpdateList", func() {
		It("should report pending packages from a 200 response", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/redfish/v1/", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write(serviceRootJSON()) //nolint:errcheck
			})
			mux.HandleFunc("/redfish/v1/Systems/1/Oem/Dell/DellSoftwareInstallationService/Actions/DellSoftwareInstallationService.GetRepoBasedUpdateList",
				func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{"PackageList": "<PACKAGELIST><PACKAGE NAME=\"BIOS\"/></PACKAGELIST>"}`)) //nolint:errcheck
				})

			server := httptest.NewServer(mux)
			defer server.Close()

			dell.RedfishBaseBMC = newTestRedfishBMC(server)
			hasPending, packageList, err := dell.GetRepositoryUpdateList(context.Background(), "/redfish/v1/Systems/1")
			Expect(err).ToNot(HaveOccurred())
			Expect(hasPending).To(BeTrue())
			Expect(packageList).To(ContainSubstring("<PACKAGE"))
		})

		It("should treat a \"no match catalog\" error response as no pending packages", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/redfish/v1/", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write(serviceRootJSON()) //nolint:errcheck
			})
			mux.HandleFunc("/redfish/v1/Systems/1/Oem/Dell/DellSoftwareInstallationService/Actions/DellSoftwareInstallationService.GetRepoBasedUpdateList",
				func(w http.ResponseWriter, _ *http.Request) {
					http.Error(w, "Unable to find match catalog file.", http.StatusBadRequest)
				})

			server := httptest.NewServer(mux)
			defer server.Close()

			dell.RedfishBaseBMC = newTestRedfishBMC(server)
			hasPending, packageList, err := dell.GetRepositoryUpdateList(context.Background(), "/redfish/v1/Systems/1")
			Expect(err).ToNot(HaveOccurred())
			Expect(hasPending).To(BeFalse())
			Expect(packageList).To(BeEmpty())
		})

		It("should return an error on an unrelated failure response", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/redfish/v1/", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write(serviceRootJSON()) //nolint:errcheck
			})
			mux.HandleFunc("/redfish/v1/Systems/1/Oem/Dell/DellSoftwareInstallationService/Actions/DellSoftwareInstallationService.GetRepoBasedUpdateList",
				func(w http.ResponseWriter, _ *http.Request) {
					http.Error(w, "internal error", http.StatusInternalServerError)
				})

			server := httptest.NewServer(mux)
			defer server.Close()

			dell.RedfishBaseBMC = newTestRedfishBMC(server)
			_, _, err := dell.GetRepositoryUpdateList(context.Background(), "/redfish/v1/Systems/1")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ListJobs and GetJob", func() {
		It("should list job IDs and retrieve a job's details", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/redfish/v1/", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write(serviceRootJSON()) //nolint:errcheck
			})
			mux.HandleFunc("/redfish/v1/Managers", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write(managersCollectionJSON([]string{"/redfish/v1/Managers/BMC"})) //nolint:errcheck
			})
			mux.HandleFunc("/redfish/v1/Managers/BMC", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write(managerJSON("BMC", 0, nil)) //nolint:errcheck
			})
			mux.HandleFunc("/redfish/v1/Managers/BMC/Oem/Dell/Jobs", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"Members": [{"@odata.id": "/redfish/v1/Managers/BMC/Oem/Dell/Jobs/JID_1234"}]}`)) //nolint:errcheck
			})
			mux.HandleFunc("/redfish/v1/Managers/BMC/Oem/Dell/Jobs/JID_1234", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"Id": "JID_1234", "Name": "Repository Update", "JobType": "RepositoryUpdate", "JobState": "Running", "Message": "Job execution in progress.", "PercentComplete": 50}`)) //nolint:errcheck
			})

			server := httptest.NewServer(mux)
			defer server.Close()

			dell.RedfishBaseBMC = newTestRedfishBMC(server)

			jobIDs, err := dell.ListJobs(context.Background(), "")
			Expect(err).ToNot(HaveOccurred())
			Expect(jobIDs).To(Equal([]string{"JID_1234"}))

			job, err := dell.GetJob(context.Background(), "", "JID_1234")
			Expect(err).ToNot(HaveOccurred())
			Expect(job.ID).To(Equal("JID_1234"))
			Expect(job.Name).To(Equal("Repository Update"))
			Expect(job.JobType).To(Equal("RepositoryUpdate"))
			Expect(job.State).To(Equal("Running"))
			Expect(job.Message).To(Equal("Job execution in progress."))
			Expect(job.PercentComplete).To(Equal(int32(50)))
		})
	})

})

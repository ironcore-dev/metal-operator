// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"net/netip"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ironcore-dev/controller-utils/conditionutils"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/bmc/mock/server"
	"github.com/ironcore-dev/metal-operator/internal/api/macdb"
	"github.com/ironcore-dev/metal-operator/internal/cmd/dns"
	"github.com/ironcore-dev/metal-operator/internal/registry"

	// Blank-imported so that github.com/ironcore-dev/metal-maintenance-operator stays a
	// real (non-test-pruned) go.mod dependency. This lets maintenanceOperatorCRDDir
	// resolve the module's on-disk location via `go list -m`, instead of assuming a
	// sibling checkout of that repository at a fixed relative path.
	_ "github.com/ironcore-dev/metal-maintenance-operator/api/servermaintenance/v1alpha1"
	// +kubebuilder:scaffold:imports
)

const (
	pollingInterval      = 50 * time.Millisecond
	eventuallyTimeout    = 5 * time.Second
	consistentlyDuration = 1 * time.Second
	MockServerIP         = "127.0.0.1"
	MockServerPort       = 8000
)

var (
	cfg                     *rest.Config
	k8sClient               client.Client
	testEnv                 *envtest.Environment
	registryURL             = "http://localhost:30000"
	mockUpServerBiosVersion = "P79 v1.45 (12/06/2017)"
	mockUpServerBMCVersion  = "1.45.455b66-rev4"
	mockServers             []*server.MockServer
)

func TestControllers(t *testing.T) {
	SetDefaultConsistentlyPollingInterval(pollingInterval)
	SetDefaultEventuallyPollingInterval(pollingInterval)
	SetDefaultEventuallyTimeout(eventuallyTimeout)
	SetDefaultConsistentlyDuration(consistentlyDuration)
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

// maintenanceOperatorCRDDir returns the config/crd/bases directory of the
// github.com/ironcore-dev/metal-maintenance-operator module (e.g. for the
// ServerMaintenance CRD), resolved from the local Go module cache/proxy via
// the go.mod requirement rather than a relative path to a sibling checkout.
func maintenanceOperatorCRDDir() (string, error) {
	const modulePath = "github.com/ironcore-dev/metal-maintenance-operator"

	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", modulePath).Output()
	if err != nil {
		return "", fmt.Errorf("failed to resolve module dir for %s: %w", modulePath, err)
	}

	dir := strings.TrimSpace(string(out))
	if dir == "" {
		return "", fmt.Errorf("empty module dir for %s", modulePath)
	}

	return filepath.Join(dir, "config", "crd", "bases"), nil
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	maintenanceCRDDir, err := maintenanceOperatorCRDDir()
	Expect(err).NotTo(HaveOccurred())

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
			maintenanceCRDDir,
		},
		ErrorIfCRDPathMissing: true,

		// The BinaryAssetsDirectory is only required if you want to run the tests directly
		// without call the makefile target test. If not informed it will look for the
		// default path defined in controller-runtime which is /usr/local/kubebuilder/.
		// Note that you must have the required binaries setup under the bin directory to perform
		// the tests directly. When we run make test it will be setup and used automatically.
		BinaryAssetsDirectory: filepath.Join("..", "..", "bin", "k8s",
			fmt.Sprintf("1.36.0-%s-%s", runtime.GOOS, runtime.GOARCH)),
	}

	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	DeferCleanup(testEnv.Stop)

	Expect(metalv1alpha1.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// set komega client
	SetClient(k8sClient)
})

func SetupTest(redfishMockServers []netip.AddrPort) *corev1.Namespace {
	ns := &corev1.Namespace{}

	BeforeEach(func(ctx SpecContext) {
		var mgrCtx context.Context
		mgrCtx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)

		*ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed(), "failed to create test namespace")
		DeferCleanup(k8sClient.Delete, ns)

		k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme: scheme.Scheme,
			Controller: config.Controller{
				// need to skip unique controller name validation
				// since all tests need a dedicated controller
				SkipNameValidation: new(true),
			},
			Metrics: metricsserver.Options{
				BindAddress: "0",
			},
			Cache: cache.Options{
				ByObject: map[client.Object]cache.ByObject{
					func() client.Object {
						u := &unstructured.Unstructured{}
						u.SetGroupVersionKind(k8sschema.GroupVersionKind{
							Group:   "servermaintenance.metal.ironcore.dev",
							Version: "v1alpha1",
							Kind:    "ServerMaintenance",
						})
						return u
					}(): {},
				},
			},
			Client: client.Options{
				Cache: &client.CacheOptions{
					// Required so unstructured ServerMaintenance List calls go through
					// the cache (and its field indexer) instead of the live API server.
					Unstructured: true,
				},
			},
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(RegisterIndexFields(mgrCtx, k8sManager.GetFieldIndexer())).To(Succeed())

		prefixDB := &macdb.MacPrefixes{
			MacPrefixes: []macdb.MacPrefix{
				{
					MacPrefix:    "23",
					Manufacturer: "Foo",
					Protocol:     "RedfishLocal",
					Port:         MockServerPort,
					Type:         "bmc",
					DefaultCredentials: []macdb.Credential{
						{
							Username: "foo",
							Password: "bar",
						},
					},
					Console: macdb.Console{
						Type: string(metalv1alpha1.ConsoleProtocolNameSSH),
						Port: 22,
					},
				},
			},
		}

		accessor := conditionutils.NewAccessor(conditionutils.AccessorOptions{})

		// register reconciler here
		// NOTE: The test suite uses HTTP protocol with SkipCertValidation=true because
		// the mock Redfish server only supports HTTP (no TLS). Full HTTPS + certificate
		// verification testing should be performed in E2E tests against real BMC hardware
		// or production-like environments with valid TLS certificates.
		// TODO: Consider adding HTTPS support to the mock server for more comprehensive
		// unit test coverage of the TLS certificate validation path.
		Expect((&EndpointReconciler{
			Client:             k8sManager.GetClient(),
			Scheme:             k8sManager.GetScheme(),
			MACPrefixes:        prefixDB,
			DefaultProtocol:    metalv1alpha1.HTTPProtocolScheme,
			SkipCertValidation: true,
		}).SetupWithManager(k8sManager)).To(Succeed())

		dnsTemplate, err := dns.LoadTemplate("../../test/data/dns_record_template.yaml")
		Expect(err).NotTo(HaveOccurred())

		Expect((&BMCReconciler{
			Client:                 k8sManager.GetClient(),
			Scheme:                 k8sManager.GetScheme(),
			DefaultProtocol:        metalv1alpha1.HTTPProtocolScheme,
			SkipCertValidation:     true,
			ManagerNamespace:       ns.Name,
			BMCResetWaitTime:       400 * time.Millisecond,
			BMCClientRetryInterval: 25 * time.Millisecond,
			EventURL:               "http://localhost:8008",
			DNSRecordTemplate:      dnsTemplate,
			Conditions:             accessor,
			BMCOptions: bmc.Options{
				PowerPollingInterval: 50 * time.Millisecond,
				PowerPollingTimeout:  200 * time.Millisecond,
				BasicAuth:            true,
			},
		}).SetupWithManager(k8sManager)).To(Succeed())

		Expect((&ServerReconciler{
			Client:                  k8sManager.GetClient(),
			APIReader:               k8sManager.GetAPIReader(),
			Scheme:                  k8sManager.GetScheme(),
			DefaultProtocol:         metalv1alpha1.HTTPProtocolScheme,
			SkipCertValidation:      true,
			ManagerNamespace:        ns.Name,
			ProbeImage:              "foo:latest",
			ProbeOSImage:            "fooOS:latest",
			RegistryURL:             registryURL,
			RegistryClientTimeout:   5 * time.Second,
			RegistryDataMaxAge:      2 * time.Minute,
			RegistryResyncInterval:  50 * time.Millisecond,
			ResyncInterval:          50 * time.Millisecond,
			EnforceFirstBoot:        true,
			MaxConcurrentReconciles: 5,
			Conditions:              accessor,
			BMCOptions: bmc.Options{
				PowerPollingInterval: 50 * time.Millisecond,
				PowerPollingTimeout:  200 * time.Millisecond,
				BasicAuth:            true,
			},
			DiscoveryTimeout:      30 * time.Second, // Set a short discovery timeout for testing
			DiscoveryIgnitionPath: filepath.Join("..", "..", "config", "manager", "ignition-template.yaml"),
		}).SetupWithManager(k8sManager)).To(Succeed())

		Expect((&ServerClaimReconciler{
			Client:                  k8sManager.GetClient(),
			APIReader:               k8sManager.GetAPIReader(),
			Cache:                   k8sManager.GetCache(),
			Scheme:                  k8sManager.GetScheme(),
			MaxConcurrentReconciles: 5,
		}).SetupWithManager(k8sManager)).To(Succeed())

		Expect((&ServerBootConfigurationReconciler{
			Client: k8sManager.GetClient(),
			Scheme: k8sManager.GetScheme(),
		}).SetupWithManager(k8sManager)).To(Succeed())

		Expect((&ServerMaintenanceReconciler{
			Client: k8sManager.GetClient(),
			Scheme: k8sManager.GetScheme(),
		}).SetupWithManager(k8sManager)).To(Succeed())

		// BIOSSettings, BIOSVersion, BIOSVersionSet, BIOSSettingsSet, BMCSettings, BMCVersion,
		// BMCVersionSet, BMCSettingsSet controllers are disabled — their tests are skipped
		// (//go:build ignore) because they depend on ServerMaintenance which has moved to
		// maintenance-operator (servermaintenance.metal.ironcore.dev).

		Expect((&BMCUserReconciler{
			Client:             k8sManager.GetClient(),
			Scheme:             k8sManager.GetScheme(),
			DefaultProtocol:    metalv1alpha1.HTTPProtocolScheme,
			SkipCertValidation: true,
			BMCOptions: bmc.Options{
				PowerPollingInterval: 50 * time.Millisecond,
				PowerPollingTimeout:  200 * time.Millisecond,
				BasicAuth:            true,
			},
		}).SetupWithManager(k8sManager)).To(Succeed())

		Expect((&ServerReadinessRuleReconciler{
			Client: k8sManager.GetClient(),
		}).SetupWithManager(k8sManager)).To(Succeed())

		Expect((&ServerReadinessRuleServerReconciler{
			Client: k8sManager.GetClient(),
		}).SetupWithManager(k8sManager)).To(Succeed())

		By("Starting the registry server")
		Expect(k8sManager.Add(manager.RunnableFunc(func(ctx context.Context) error {
			registryServer := registry.NewServer(GinkgoLogr, ":30000", k8sManager.GetClient())
			if err := registryServer.Start(ctx); err != nil {
				return fmt.Errorf("failed to start registry server: %w", err)
			}
			<-ctx.Done()
			return nil
		}))).Should(Succeed())

		if len(redfishMockServers) > 0 {
			mockServers = make([]*server.MockServer, 0, len(redfishMockServers))
			for _, serverAddr := range redfishMockServers {
				By(fmt.Sprintf("Starting the mock Redfish servers %v", serverAddr))
				ms := server.NewMockServer(GinkgoLogr, serverAddr.String(), server.WithAuth())
				mockServers = append(mockServers, ms)
				Expect(k8sManager.Add(manager.RunnableFunc(func(ctx context.Context) error {
					if err := ms.Start(ctx); err != nil {
						return fmt.Errorf("failed to start mock Redfish server %v", serverAddr)
					}
					<-ctx.Done()
					return nil
				}))).Should(Succeed())
			}
		} else {
			By("Starting the default mock Redfish server")
			ms := server.NewMockServer(GinkgoLogr, fmt.Sprintf(":%d", MockServerPort), server.WithAuth())
			mockServers = []*server.MockServer{ms}
			Expect(k8sManager.Add(manager.RunnableFunc(func(ctx context.Context) error {
				if err := ms.Start(ctx); err != nil {
					return fmt.Errorf("failed to start mock Redfish server: %w", err)
				}
				<-ctx.Done()
				return nil
			}))).Should(Succeed())
		}

		go func() {
			defer GinkgoRecover()
			Expect(k8sManager.Start(mgrCtx)).To(Succeed(), "failed to start manager")
		}()
	})

	return ns
}

// EnsureCleanState ensures that all ServerClaims and cluster scoped objects are removed from the API server.
func EnsureCleanState() {
	GinkgoHelper()

	objectLists := []client.ObjectList{
		&metalv1alpha1.EndpointList{},
		&metalv1alpha1.BMCList{},
		&metalv1alpha1.BMCSecretList{},
		&metalv1alpha1.ServerClaimList{},
		&metalv1alpha1.BMCSettingsSetList{},
		&metalv1alpha1.BMCSettingsList{},
		&metalv1alpha1.BMCVersionSetList{},
		&metalv1alpha1.BMCVersionList{},
		&metalv1alpha1.BIOSVersionList{},
		&metalv1alpha1.BIOSSettingsSetList{},
		&metalv1alpha1.BIOSSettingsList{},
		&metalv1alpha1.ServerMaintenanceList{},
		&metalv1alpha1.ServerList{},
	}

	for _, list := range objectLists {
		Eventually(ObjectList(list)).Should(HaveField("Items", HaveLen(0)))
	}
}

// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"net/netip"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/bmc/mock/server"
	"github.com/ironcore-dev/metal-operator/internal/api/macdb"
	"github.com/ironcore-dev/metal-operator/internal/registry"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	//+kubebuilder:scaffold:imports
)

const (
	pollingInterval      = 50 * time.Millisecond
	eventuallyTimeout    = 5 * time.Second
	consistentlyDuration = 1 * time.Second
)

var (
	cfg                            *rest.Config
	k8sClient                      client.Client
	testEnv                        *envtest.Environment
	registryURL                    = "http://localhost:30000"
	defaultMockUpServerBiosVersion = "P79 v1.45 (12/06/2017)"
	defaultMockUpServerBMCVersion  = "1.45.455b66-rev4"
)

func TestControllers(t *testing.T) {
	SetDefaultConsistentlyPollingInterval(pollingInterval)
	SetDefaultEventuallyPollingInterval(pollingInterval)
	SetDefaultEventuallyTimeout(eventuallyTimeout)
	SetDefaultConsistentlyDuration(consistentlyDuration)
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,

		// The BinaryAssetsDirectory is only required if you want to run the tests directly
		// without call the makefile target test. If not informed it will look for the
		// default path defined in controller-runtime which is /usr/local/kubebuilder/.
		// Note that you must have the required binaries setup under the bin directory to perform
		// the tests directly. When we run make test it will be setup and used automatically.
		BinaryAssetsDirectory: filepath.Join("..", "..", "bin", "k8s",
			fmt.Sprintf("1.34.0-%s-%s", runtime.GOOS, runtime.GOARCH)),
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	DeferCleanup(testEnv.Stop)

	Expect(metalv1alpha1.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// set komega client
	SetClient(k8sClient)

	bmc.InitMockUp()
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
				SkipNameValidation: ptr.To(true),
			},
			Metrics: metricsserver.Options{
				BindAddress: "0",
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
					Port:         8000,
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

		// register reconciler here
		Expect((&EndpointReconciler{
			Client:      k8sManager.GetClient(),
			Scheme:      k8sManager.GetScheme(),
			MACPrefixes: prefixDB,
			Insecure:    true,
		}).SetupWithManager(k8sManager)).To(Succeed())

		Expect((&BMCReconciler{
			Client:                 k8sManager.GetClient(),
			Scheme:                 k8sManager.GetScheme(),
			Insecure:               true,
			ManagerNamespace:       ns.Name,
			BMCResetWaitTime:       400 * time.Millisecond,
			BMCClientRetryInterval: 25 * time.Millisecond,
			DNSRecordTemplatePath:  "../../test/data/dns_record_template.yaml",
		}).SetupWithManager(k8sManager)).To(Succeed())

		Expect((&ServerReconciler{
			Client:                  k8sManager.GetClient(),
			Scheme:                  k8sManager.GetScheme(),
			Insecure:                true,
			ManagerNamespace:        ns.Name,
			ProbeImage:              "foo:latest",
			ProbeOSImage:            "fooOS:latest",
			RegistryURL:             registryURL,
			RegistryResyncInterval:  50 * time.Millisecond,
			ResyncInterval:          50 * time.Millisecond,
			EnforceFirstBoot:        true,
			MaxConcurrentReconciles: 5,
			BMCOptions: bmc.Options{
				PowerPollingInterval: 50 * time.Millisecond,
				PowerPollingTimeout:  200 * time.Millisecond,
				BasicAuth:            true,
			},
			DiscoveryTimeout: time.Second, // Force timeout to be quick for tests
		}).SetupWithManager(k8sManager)).To(Succeed())

		Expect((&ServerClaimReconciler{
			Client:                  k8sManager.GetClient(),
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

		Expect((&BiosSettingsReconciler{
			Client:           k8sManager.GetClient(),
			ManagerNamespace: ns.Name,
			Insecure:         true,
			Scheme:           k8sManager.GetScheme(),
			ResyncInterval:   10 * time.Millisecond,
			BMCOptions: bmc.Options{
				PowerPollingInterval: 50 * time.Millisecond,
				PowerPollingTimeout:  200 * time.Millisecond,
				BasicAuth:            true,
			},
			TimeoutExpiry: 6 * time.Second,
		}).SetupWithManager(k8sManager)).To(Succeed())

		Expect((&BIOSVersionReconciler{
			Client:           k8sManager.GetClient(),
			ManagerNamespace: ns.Name,
			Insecure:         true,
			Scheme:           k8sManager.GetScheme(),
			ResyncInterval:   10 * time.Millisecond,
			BMCOptions: bmc.Options{
				PowerPollingInterval: 50 * time.Millisecond,
				PowerPollingTimeout:  200 * time.Millisecond,
				BasicAuth:            true,
			},
		}).SetupWithManager(k8sManager)).To(Succeed())

		Expect((&BIOSVersionSetReconciler{
			Client: k8sManager.GetClient(),
			Scheme: k8sManager.GetScheme(),
		}).SetupWithManager(k8sManager)).To(Succeed())

		Expect((&BMCSettingsReconciler{
			Client:           k8sManager.GetClient(),
			ManagerNamespace: ns.Name,
			Insecure:         true,
			Scheme:           k8sManager.GetScheme(),
			ResyncInterval:   10 * time.Millisecond,
			BMCOptions: bmc.Options{
				PowerPollingInterval: 50 * time.Millisecond,
				PowerPollingTimeout:  200 * time.Millisecond,
				BasicAuth:            true,
			},
		}).SetupWithManager(k8sManager)).To(Succeed())

		Expect((&BMCVersionReconciler{
			Client:           k8sManager.GetClient(),
			ManagerNamespace: ns.Name,
			Insecure:         true,
			Scheme:           k8sManager.GetScheme(),
			ResyncInterval:   10 * time.Millisecond,
			BMCOptions: bmc.Options{
				PowerPollingInterval: 50 * time.Millisecond,
				PowerPollingTimeout:  200 * time.Millisecond,
				BasicAuth:            true,
			},
		}).SetupWithManager(k8sManager)).To(Succeed())

		Expect((&BMCVersionSetReconciler{
			Client: k8sManager.GetClient(),
			Scheme: k8sManager.GetScheme(),
		}).SetupWithManager(k8sManager)).To(Succeed())

		Expect((&BIOSSettingsSetReconciler{
			Client: k8sManager.GetClient(),
			Scheme: k8sManager.GetScheme(),
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
			for _, serverAddr := range redfishMockServers {
				By(fmt.Sprintf("Starting the mock Redfish servers %v", serverAddr))
				Expect(k8sManager.Add(manager.RunnableFunc(func(ctx context.Context) error {
					mockServer := server.NewMockServer(GinkgoLogr, serverAddr.String())
					if err := mockServer.Start(ctx); err != nil {
						return fmt.Errorf("failed to start mock Redfish server %v", serverAddr)
					}
					<-ctx.Done()
					return nil
				}))).Should(Succeed())
			}
		} else {
			By("Starting the default mock Redfish server")
			Expect(k8sManager.Add(manager.RunnableFunc(func(ctx context.Context) error {
				mockServer := server.NewMockServer(GinkgoLogr, ":8000")
				if err := mockServer.Start(ctx); err != nil {
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

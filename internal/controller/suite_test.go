// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
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
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	//+kubebuilder:scaffold:imports
)

const (
	pollingInterval      = 50 * time.Millisecond
	eventuallyTimeout    = 3 * time.Second
	consistentlyDuration = 1 * time.Second
)

var (
	cfg         *rest.Config
	k8sClient   client.Client
	testEnv     *envtest.Environment
	registryURL = "http://localhost:30000"
)

func TestControllers(t *testing.T) {
	SetDefaultConsistentlyPollingInterval(pollingInterval)
	SetDefaultEventuallyPollingInterval(pollingInterval)
	SetDefaultEventuallyTimeout(eventuallyTimeout)
	SetDefaultConsistentlyDuration(consistentlyDuration)
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

func DeleteAllMetalResources(ctx context.Context, namespace string) {
	var serverClaim metalv1alpha1.ServerClaim
	Expect(k8sClient.DeleteAllOf(ctx, &serverClaim, client.InNamespace(namespace))).To(Succeed())
	var serverClaimList metalv1alpha1.ServerClaimList
	Eventually(ObjectList(&serverClaimList)).Should(HaveField("Items", BeEmpty()))

	var endpoint metalv1alpha1.Endpoint
	Expect(k8sClient.DeleteAllOf(ctx, &endpoint)).To(Succeed())
	var endpointList metalv1alpha1.EndpointList
	Eventually(ObjectList(&endpointList)).Should(HaveField("Items", BeEmpty()))

	var bmc metalv1alpha1.BMC
	Expect(k8sClient.DeleteAllOf(ctx, &bmc)).To(Succeed())
	var bmcList metalv1alpha1.BMCList
	Eventually(ObjectList(&bmcList)).Should(HaveField("Items", BeEmpty()))

	var serverBootConfiguration metalv1alpha1.ServerBootConfiguration
	Expect(k8sClient.DeleteAllOf(ctx, &serverBootConfiguration, client.InNamespace(namespace))).To(Succeed())
	var serverBootConfigurationList metalv1alpha1.ServerBootConfigurationList
	Eventually(ObjectList(&serverBootConfigurationList)).Should(HaveField("Items", BeEmpty()))

	var server metalv1alpha1.Server
	Expect(k8sClient.DeleteAllOf(ctx, &server)).To(Succeed())
	var serverList metalv1alpha1.ServerList
	Eventually(ObjectList(&serverList)).Should(HaveField("Items", BeEmpty()))

	var bmcSecret metalv1alpha1.BMCSecret
	Expect(k8sClient.DeleteAllOf(ctx, &bmcSecret)).To(Succeed())
	var bmcSecretList metalv1alpha1.BMCSecretList
	Eventually(ObjectList(&bmcSecretList)).Should(HaveField("Items", BeEmpty()))
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
			fmt.Sprintf("1.31.0-%s-%s", runtime.GOOS, runtime.GOARCH)),
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	DeferCleanup(testEnv.Stop)

	Expect(metalv1alpha1.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())

	err = metalv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// set komega client
	SetClient(k8sClient)

	By("Starting the registry server")
	var mgrCtx context.Context
	mgrCtx, cancel := context.WithCancel(context.Background())
	DeferCleanup(cancel)
	registryServer := registry.NewServer(":30000")
	go func() {
		defer GinkgoRecover()
		Expect(registryServer.Start(mgrCtx)).To(Succeed(), "failed to start registry server")
	}()
})

func SetupTest() *corev1.Namespace {
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
			Client:   k8sManager.GetClient(),
			Scheme:   k8sManager.GetScheme(),
			Insecure: true,
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
			BMCOptions: bmc.BMCOptions{
				PowerPollingInterval: 50 * time.Millisecond,
				PowerPollingTimeout:  200 * time.Millisecond,
				BasicAuth:            true,
			},
			DiscoveryTimeout: 500 * time.Millisecond, // Force timeout to be quick for tests
		}).SetupWithManager(k8sManager)).To(Succeed())

		Expect((&ServerClaimReconciler{
			Client: k8sManager.GetClient(),
			Scheme: k8sManager.GetScheme(),
		}).SetupWithManager(k8sManager)).To(Succeed())

		Expect((&ServerBootConfigurationReconciler{
			Client: k8sManager.GetClient(),
			Scheme: k8sManager.GetScheme(),
		}).SetupWithManager(k8sManager)).To(Succeed())

		go func() {
			defer GinkgoRecover()
			Expect(k8sManager.Start(mgrCtx)).To(Succeed(), "failed to start manager")
		}()
	})

	return ns
}

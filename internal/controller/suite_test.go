/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/afritzler/metal-operator/internal/registry"

	"github.com/afritzler/metal-operator/internal/api/macdb"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	metalv1alpha1 "github.com/afritzler/metal-operator/api/v1alpha1"
	//+kubebuilder:scaffold:imports
)

const (
	pollingInterval      = 50 * time.Millisecond
	eventuallyTimeout    = 5 * time.Second
	consistentlyDuration = 1 * time.Second
)

var (
	cfg         *rest.Config
	k8sClient   client.Client
	testEnv     *envtest.Environment
	registryURL = "http://localhost:12345"
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
			fmt.Sprintf("1.29.0-%s-%s", runtime.GOOS, runtime.GOARCH)),
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
	registryServer := registry.NewServer(":12345")
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
			Client:           k8sManager.GetClient(),
			Scheme:           k8sManager.GetScheme(),
			Insecure:         true,
			ManagerNamespace: ns.Name,
			ProbeImage:       "foo:latest",
			ProbeOSImage:     "fooOS:latest",
			RegistryURL:      registryURL,
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

// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/ironcore-dev/metal-operator/internal/registry"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sSchema "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	//+kubebuilder:scaffold:imports
)

const (
	pollingInterval      = 50 * time.Millisecond
	eventuallyTimeout    = 3 * time.Second
	consistentlyDuration = 1 * time.Second
)

var (
	cfg     *rest.Config
	clients Clients
)

func TestMetalctl(t *testing.T) {
	SetDefaultConsistentlyPollingInterval(pollingInterval)
	SetDefaultEventuallyPollingInterval(pollingInterval)
	SetDefaultEventuallyTimeout(eventuallyTimeout)
	SetDefaultConsistentlyDuration(consistentlyDuration)
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	// Source client with CRDs
	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,

		// The BinaryAssetsDirectory is only required if you want to run the tests directly
		// without call the makefile target test. If not informed it will look for the
		// default path defined in controller-runtime which is /usr/local/kubebuilder/.
		// Note that you must have the required binaries setup under the bin directory to perform
		// the tests directly. When we run make test it will be setup and used automatically.
		BinaryAssetsDirectory: filepath.Join("..", "..", "..", "bin", "k8s",
			fmt.Sprintf("1.31.0-%s-%s", runtime.GOOS, runtime.GOARCH)),
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	DeferCleanup(testEnv.Stop)

	Expect(metalv1alpha1.AddToScheme(k8sSchema.Scheme)).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	clients.source, err = client.New(cfg, client.Options{Scheme: k8sSchema.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(clients.source).NotTo(BeNil())

	// Target client without CRDs
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "config", "crd", "bases", "metal.ironcore.dev_bmcs.yaml")},
		// The BinaryAssetsDirectory is only required if you want to run the tests directly
		// without call the makefile target test. If not informed it will look for the
		// default path defined in controller-runtime which is /usr/local/kubebuilder/.
		// Note that you must have the required binaries setup under the bin directory to perform
		// the tests directly. When we run make test it will be setup and used automatically.
		BinaryAssetsDirectory: filepath.Join("..", "..", "..", "bin", "k8s",
			fmt.Sprintf("1.31.0-%s-%s", runtime.GOOS, runtime.GOARCH)),
	}

	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	DeferCleanup(testEnv.Stop)

	clients.target, err = client.New(cfg, client.Options{Scheme: k8sSchema.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(clients.target).NotTo(BeNil())

	By("Starting the registry server")
	var mgrCtx context.Context
	mgrCtx, cancel := context.WithCancel(context.Background())
	DeferCleanup(cancel)
	registryServer := registry.NewServer("localhost:30000")
	go func() {
		defer GinkgoRecover()
		Expect(registryServer.Start(mgrCtx)).To(Succeed(), "failed to start registry server")
	}()

})

func SetupTest() *corev1.Namespace {
	ns := &corev1.Namespace{}

	BeforeEach(func(ctx SpecContext) {
		// var mgrCtx context.Context
		// mgrCtx, cancel := context.WithCancel(context.Background())
		// DeferCleanup(cancel)

		*ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
		}
		targetNs := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
		}
		Expect(clients.source.Create(ctx, ns)).To(Succeed(), "failed to create test namespace")
		Expect(clients.target.Create(ctx, &targetNs)).To(Succeed(), "failed to create test namespace")
		DeferCleanup(clients.source.Delete, ns)
		DeferCleanup(clients.target.Delete, &targetNs)

		// k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		// 	Scheme: k8sSchema.Scheme,
		// 	Controller: config.Controller{
		// 		// need to skip unique controller name validation
		// 		// since all tests need a dedicated controller
		// 		SkipNameValidation: ptr.To(true),
		// 	},
		// })
		// Expect(err).ToNot(HaveOccurred())

		// go func() {
		// 	defer GinkgoRecover()
		// 	Expect(k8sManager.Start(mgrCtx)).To(Succeed(), "failed to start manager")
		// }()
	})

	return ns
}

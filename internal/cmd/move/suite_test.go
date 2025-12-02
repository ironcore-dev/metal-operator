// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package move

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sSchema "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	//+kubebuilder:scaffold:imports
)

const (
	pollingInterval      = 50 * time.Millisecond
	eventuallyTimeout    = 3 * time.Second
	consistentlyDuration = 1 * time.Second
)

var (
	clients Clients
)

func TestMetalctl(t *testing.T) {
	SetDefaultConsistentlyPollingInterval(pollingInterval)
	SetDefaultEventuallyPollingInterval(pollingInterval)
	SetDefaultEventuallyTimeout(eventuallyTimeout)
	SetDefaultConsistentlyDuration(consistentlyDuration)
	RegisterFailHandler(Fail)

	RunSpecs(t, "cmdutils Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	sourceEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,

		// The BinaryAssetsDirectory is only required if you want to run the tests directly
		// without call the makefile target test. If not informed it will look for the
		// default path defined in controller-runtime which is /usr/local/kubebuilder/.
		// Note that you must have the required binaries setup under the bin directory to perform
		// the tests directly. When we run make test it will be setup and used automatically.
		BinaryAssetsDirectory: filepath.Join("..", "..", "..", "bin", "k8s",
			fmt.Sprintf("1.34.0-%s-%s", runtime.GOOS, runtime.GOARCH)),
	}

	sourceCfg, err := sourceEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(sourceCfg).NotTo(BeNil())

	DeferCleanup(sourceEnv.Stop)

	Expect(metalv1alpha1.AddToScheme(k8sSchema.Scheme)).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	clients.Source, err = client.New(sourceCfg, client.Options{Scheme: k8sSchema.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(clients.Source).NotTo(BeNil())

	targetEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,

		// The BinaryAssetsDirectory is only required if you want to run the tests directly
		// without call the makefile target test. If not informed it will look for the
		// default path defined in controller-runtime which is /usr/local/kubebuilder/.
		// Note that you must have the required binaries setup under the bin directory to perform
		// the tests directly. When we run make test it will be setup and used automatically.
		BinaryAssetsDirectory: filepath.Join("..", "..", "..", "bin", "k8s",
			fmt.Sprintf("1.34.0-%s-%s", runtime.GOOS, runtime.GOARCH)),
	}

	// cfg is defined in this file globally.
	targetCfg, err := targetEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(targetCfg).NotTo(BeNil())

	DeferCleanup(targetEnv.Stop)

	clients.Target, err = client.New(targetCfg, client.Options{Scheme: k8sSchema.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(clients.Target).NotTo(BeNil())
})

func SetupTest() *corev1.Namespace {
	ns := &corev1.Namespace{}

	BeforeEach(func(ctx SpecContext) {
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
		Expect(clients.Source.Create(ctx, ns)).To(Succeed(), "failed to create test namespace")
		Expect(clients.Target.Create(ctx, &targetNs)).To(Succeed(), "failed to create test namespace")
		DeferCleanup(clients.Source.Delete, ns)
		DeferCleanup(clients.Target.Delete, &targetNs)
	})

	return ns
}

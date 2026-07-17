// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package metaldata_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/metaldata"
)

const (
	staticKey = "static-key"
	staticVal = "static-val"
)

var (
	idx           *metaldata.Index
	reader        *mutableReader
	scheme        = runtime.NewScheme()
	testServerURL string
)

// mutableReader lets tests swap the backing client.Reader between cases while
// keeping the metaldata.Server constructed once at suite start.
type mutableReader struct {
	inner client.Reader
}

func (m *mutableReader) reset(objs ...client.Object) {
	m.inner = fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func (m *mutableReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return m.inner.Get(ctx, key, obj, opts...)
}

func (m *mutableReader) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return m.inner.List(ctx, list, opts...)
}

func TestMetaldata(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Metaldata Suite")
}

var _ = BeforeSuite(func() {
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(metalv1alpha1.AddToScheme(scheme))

	ctx, cancel := context.WithCancel(context.Background())
	DeferCleanup(cancel)

	idx = metaldata.NewIndex(GinkgoLogr, map[string]string{staticKey: staticVal})
	reader = &mutableReader{}
	reader.reset()

	srv := metaldata.NewServer(GinkgoLogr, idx, reader, "127.0.0.1:0")
	go func() {
		defer GinkgoRecover()
		Expect(srv.Start(ctx)).To(Succeed(), "failed to start metaldata server")
	}()

	Eventually(srv.Ready).Should(BeTrue())

	testServerURL = fmt.Sprintf("http://%s", srv.Addr())

	Eventually(func() error {
		_, err := http.Get(testServerURL)
		return err
	}).Should(Succeed())
})

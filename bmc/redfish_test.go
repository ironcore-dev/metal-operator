// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stmcginnis/gofish/schemas"
)

// fakeBMCClient is a minimal stub implementing BMC for unit testing DiscoverManager.
type fakeBMCClient struct {
	BMC
	manager    *schemas.Manager
	managerErr error
}

func (f *fakeBMCClient) DiscoverManager(_ context.Context) (*schemas.Manager, error) {
	return f.manager, f.managerErr
}

var _ = Describe("DiscoverManager", func() {
	It("should return a manager with GraphicalConsole.MaxConcurrentSessions set", func(ctx SpecContext) {
		fake := &fakeBMCClient{
			manager: &schemas.Manager{
				GraphicalConsole: schemas.GraphicalConsole{
					MaxConcurrentSessions: 2,
				},
			},
		}
		manager, err := fake.DiscoverManager(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(manager).NotTo(BeNil())
		Expect(manager.GraphicalConsole.MaxConcurrentSessions).To(Equal(uint(2)))
	})

	It("should return a manager with GraphicalConsole.ConnectTypesSupported set", func(ctx SpecContext) {
		fake := &fakeBMCClient{
			manager: &schemas.Manager{
				GraphicalConsole: schemas.GraphicalConsole{
					ConnectTypesSupported: []schemas.GraphicalConnectTypesSupported{"KVMIP"},
				},
			},
		}
		manager, err := fake.DiscoverManager(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(manager).NotTo(BeNil())
		Expect(manager.GraphicalConsole.ConnectTypesSupported).To(HaveLen(1))
	})

	It("should return an error when no suitable manager is found", func(ctx SpecContext) {
		fake := &fakeBMCClient{
			managerErr: fmt.Errorf("no manager found with graphical console capabilities"),
		}
		manager, err := fake.DiscoverManager(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no manager found"))
		Expect(manager).To(BeNil())
	})

	It("should return an error when the BMC connection fails", func(ctx SpecContext) {
		fake := &fakeBMCClient{
			managerErr: fmt.Errorf("connection refused"),
		}
		manager, err := fake.DiscoverManager(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("connection refused"))
		Expect(manager).To(BeNil())
	})
})

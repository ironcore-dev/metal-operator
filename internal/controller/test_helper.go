// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	. "github.com/onsi/ginkgo/v2"                         // nolint: staticcheck
	. "github.com/onsi/gomega"                            // nolint: staticcheck
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega" // nolint: staticcheck

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

// EnsureCleanState ensures that all ServerClaims and cluster scoped objects are removed from the API server.
func EnsureCleanState() {
	GinkgoHelper()

	Eventually(func(g Gomega) error {
		bmcList := &metalv1alpha1.BMCList{}
		g.Eventually(ObjectList(bmcList)).Should(HaveField("Items", HaveLen(0)))

		bmcSecretList := &metalv1alpha1.BMCSecretList{}
		g.Eventually(ObjectList(bmcSecretList)).Should(HaveField("Items", HaveLen(0)))

		claimList := &metalv1alpha1.ServerClaimList{}
		g.Eventually(ObjectList(claimList)).Should(HaveField("Items", HaveLen(0)))

		bmcSettingsList := &metalv1alpha1.BMCSettingsList{}
		g.Eventually(ObjectList(bmcSettingsList)).Should(HaveField("Items", HaveLen(0)))

		bmcVersionSetList := &metalv1alpha1.BMCVersionSetList{}
		g.Eventually(ObjectList(bmcVersionSetList)).Should(HaveField("Items", HaveLen(0)))

		bmcVersionList := &metalv1alpha1.BMCVersionList{}
		g.Eventually(ObjectList(bmcVersionList)).Should(HaveField("Items", HaveLen(0)))

		biosSettingsSetList := &metalv1alpha1.BIOSSettingsSetList{}
		g.Eventually(ObjectList(biosSettingsSetList)).Should(HaveField("Items", HaveLen(0)))

		biosSettings := &metalv1alpha1.BIOSSettingsList{}
		g.Eventually(ObjectList(biosSettings)).Should(HaveField("Items", HaveLen(0)))

		maintenanceList := &metalv1alpha1.ServerMaintenanceList{}
		g.Eventually(ObjectList(maintenanceList)).Should(HaveField("Items", HaveLen(0)))

		serverList := &metalv1alpha1.ServerList{}
		g.Eventually(ObjectList(serverList)).Should(HaveField("Items", HaveLen(0)))

		return nil
	}).Should(Succeed())
}

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
		endpoints := &metalv1alpha1.EndpointList{}
		g.Eventually(ObjectList(endpoints)).Should(HaveField("Items", HaveLen(0)))

		bmcs := &metalv1alpha1.BMCList{}
		g.Eventually(ObjectList(bmcs)).Should(HaveField("Items", HaveLen(0)))

		bmcSecrets := &metalv1alpha1.BMCSecretList{}
		g.Eventually(ObjectList(bmcSecrets)).Should(HaveField("Items", HaveLen(0)))

		claims := &metalv1alpha1.ServerClaimList{}
		g.Eventually(ObjectList(claims)).Should(HaveField("Items", HaveLen(0)))

		bmcSettingsList := &metalv1alpha1.BMCSettingsList{}
		g.Eventually(ObjectList(bmcSettingsList)).Should(HaveField("Items", HaveLen(0)))

		bmcVersionSets := &metalv1alpha1.BMCVersionSetList{}
		g.Eventually(ObjectList(bmcVersionSets)).Should(HaveField("Items", HaveLen(0)))

		bmcVersions := &metalv1alpha1.BMCVersionList{}
		g.Eventually(ObjectList(bmcVersions)).Should(HaveField("Items", HaveLen(0)))

		biosVersions := &metalv1alpha1.BIOSVersionList{}
		g.Eventually(ObjectList(biosVersions)).Should(HaveField("Items", HaveLen(0)))

		biosSettingsSets := &metalv1alpha1.BIOSSettingsSetList{}
		g.Eventually(ObjectList(biosSettingsSets)).Should(HaveField("Items", HaveLen(0)))

		biosSettingsList := &metalv1alpha1.BIOSSettingsList{}
		g.Eventually(ObjectList(biosSettingsList)).Should(HaveField("Items", HaveLen(0)))

		maintenances := &metalv1alpha1.ServerMaintenanceList{}
		g.Eventually(ObjectList(maintenances)).Should(HaveField("Items", HaveLen(0)))

		servers := &metalv1alpha1.ServerList{}
		g.Eventually(ObjectList(servers)).Should(HaveField("Items", HaveLen(0)))

		return nil
	}).Should(Succeed())
}

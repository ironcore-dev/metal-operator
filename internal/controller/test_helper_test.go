// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2" // nolint: staticcheck
	. "github.com/onsi/gomega"    // nolint: staticcheck
	"sigs.k8s.io/controller-runtime/pkg/client"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

// EnsureCleanState ensures that all ServerClaims and cluster scoped objects are removed from the API server.
// This function properly deletes resources in dependency order and waits for finalizers to complete.
func EnsureCleanState() {
	GinkgoHelper()

	ctx := context.Background()

	// Step 1: Delete all dependent resources first (in reverse dependency order)
	// This ensures proper cleanup without orphaned resources

	// Delete ServerClaims first (they reference Servers)
	claimList := &metalv1alpha1.ServerClaimList{}
	Expect(k8sClient.List(ctx, claimList)).To(Succeed())
	for i := range claimList.Items {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &claimList.Items[i]))).To(Succeed())
	}

	// Delete BIOS settings BEFORE maintenance (they may create maintenance)
	biosSettingsList := &metalv1alpha1.BIOSSettingsList{}
	Expect(k8sClient.List(ctx, biosSettingsList)).To(Succeed())
	for i := range biosSettingsList.Items {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &biosSettingsList.Items[i]))).To(Succeed())
	}

	biosSettingsSetList := &metalv1alpha1.BIOSSettingsSetList{}
	Expect(k8sClient.List(ctx, biosSettingsSetList)).To(Succeed())
	for i := range biosSettingsSetList.Items {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &biosSettingsSetList.Items[i]))).To(Succeed())
	}

	// Wait for BIOS settings to be deleted before proceeding
	Eventually(func() int {
		biosList := &metalv1alpha1.BIOSSettingsList{}
		_ = k8sClient.List(ctx, biosList)
		return len(biosList.Items)
	}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Equal(0))

	// Delete maintenance resources (they block Server deletion via finalizers)
	maintenanceList := &metalv1alpha1.ServerMaintenanceList{}
	Expect(k8sClient.List(ctx, maintenanceList)).To(Succeed())
	for i := range maintenanceList.Items {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &maintenanceList.Items[i]))).To(Succeed())
	}

	// Wait for maintenance to be fully deleted before deleting Servers
	Eventually(func() int {
		mList := &metalv1alpha1.ServerMaintenanceList{}
		_ = k8sClient.List(ctx, mList)
		return len(mList.Items)
	}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Equal(0))

	// Delete BMC version resources
	bmcVersionList := &metalv1alpha1.BMCVersionList{}
	Expect(k8sClient.List(ctx, bmcVersionList)).To(Succeed())
	for i := range bmcVersionList.Items {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &bmcVersionList.Items[i]))).To(Succeed())
	}

	bmcVersionSetList := &metalv1alpha1.BMCVersionSetList{}
	Expect(k8sClient.List(ctx, bmcVersionSetList)).To(Succeed())
	for i := range bmcVersionSetList.Items {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &bmcVersionSetList.Items[i]))).To(Succeed())
	}

	// Delete BMC settings
	bmcSettingsList := &metalv1alpha1.BMCSettingsList{}
	Expect(k8sClient.List(ctx, bmcSettingsList)).To(Succeed())
	for i := range bmcSettingsList.Items {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &bmcSettingsList.Items[i]))).To(Succeed())
	}

	// Delete Servers (they reference BMCs and BMCSecrets)
	serverList := &metalv1alpha1.ServerList{}
	Expect(k8sClient.List(ctx, serverList)).To(Succeed())
	for i := range serverList.Items {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &serverList.Items[i]))).To(Succeed())
	}

	// Delete BMCs
	bmcList := &metalv1alpha1.BMCList{}
	Expect(k8sClient.List(ctx, bmcList)).To(Succeed())
	for i := range bmcList.Items {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &bmcList.Items[i]))).To(Succeed())
	}

	// Delete BMCSecrets
	bmcSecretList := &metalv1alpha1.BMCSecretList{}
	Expect(k8sClient.List(ctx, bmcSecretList)).To(Succeed())
	for i := range bmcSecretList.Items {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &bmcSecretList.Items[i]))).To(Succeed())
	}

	// Step 2: Wait for all resources to be fully deleted (finalizers completed)
	// Use a single Eventually with proper assertions (no nested Eventually)

	Eventually(func(g Gomega) {
		// Verify all ServerClaims are gone
		claims := &metalv1alpha1.ServerClaimList{}
		g.Expect(k8sClient.List(ctx, claims)).To(Succeed())
		g.Expect(claims.Items).To(BeEmpty(), "ServerClaims should be deleted")

		// Verify all maintenance resources are gone
		maintenance := &metalv1alpha1.ServerMaintenanceList{}
		g.Expect(k8sClient.List(ctx, maintenance)).To(Succeed())
		g.Expect(maintenance.Items).To(BeEmpty(), "ServerMaintenance should be deleted")

		// Verify all BIOS settings are gone
		biosSettings := &metalv1alpha1.BIOSSettingsList{}
		g.Expect(k8sClient.List(ctx, biosSettings)).To(Succeed())
		g.Expect(biosSettings.Items).To(BeEmpty(), "BIOSSettings should be deleted")

		biosSettingsSets := &metalv1alpha1.BIOSSettingsSetList{}
		g.Expect(k8sClient.List(ctx, biosSettingsSets)).To(Succeed())
		g.Expect(biosSettingsSets.Items).To(BeEmpty(), "BIOSSettingsSets should be deleted")

		// Verify all BMC versions are gone
		bmcVersions := &metalv1alpha1.BMCVersionList{}
		g.Expect(k8sClient.List(ctx, bmcVersions)).To(Succeed())
		g.Expect(bmcVersions.Items).To(BeEmpty(), "BMCVersions should be deleted")

		bmcVersionSets := &metalv1alpha1.BMCVersionSetList{}
		g.Expect(k8sClient.List(ctx, bmcVersionSets)).To(Succeed())
		g.Expect(bmcVersionSets.Items).To(BeEmpty(), "BMCVersionSets should be deleted")

		// Verify all BMC settings are gone
		bmcSettings := &metalv1alpha1.BMCSettingsList{}
		g.Expect(k8sClient.List(ctx, bmcSettings)).To(Succeed())
		g.Expect(bmcSettings.Items).To(BeEmpty(), "BMCSettings should be deleted")

		// Verify all Servers are gone - with better error message
		servers := &metalv1alpha1.ServerList{}
		g.Expect(k8sClient.List(ctx, servers)).To(Succeed())
		if len(servers.Items) > 0 {
			// Provide detailed info about stuck servers
			for _, srv := range servers.Items {
				GinkgoWriter.Printf("Server %s stuck: DeletionTimestamp=%v, Finalizers=%v, MaintenanceRef=%v\n",
					srv.Name, srv.DeletionTimestamp, srv.Finalizers, srv.Spec.ServerMaintenanceRef)
			}
		}
		g.Expect(servers.Items).To(BeEmpty(), "Servers should be deleted")

		// Verify all BMCs are gone
		bmcs := &metalv1alpha1.BMCList{}
		g.Expect(k8sClient.List(ctx, bmcs)).To(Succeed())
		g.Expect(bmcs.Items).To(BeEmpty(), "BMCs should be deleted")

		// Verify all BMCSecrets are gone
		bmcSecrets := &metalv1alpha1.BMCSecretList{}
		g.Expect(k8sClient.List(ctx, bmcSecrets)).To(Succeed())
		g.Expect(bmcSecrets.Items).To(BeEmpty(), "BMCSecrets should be deleted")
	}).WithTimeout(3 * time.Minute).WithPolling(1 * time.Second).Should(Succeed())
}

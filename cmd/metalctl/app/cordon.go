// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"

	cmdclient "github.com/ironcore-dev/metal-operator/internal/cmd/client"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

var (
	cordonKubeconfig  string
	cordonKubeContext string
	cordonDryRun      bool
)

// NewCordonCommand returns the `metalctl cordon` command that marks a resource as
// unschedulable, preventing new ServerClaims from binding to it.
func NewCordonCommand() *cobra.Command {
	cordonCmd := &cobra.Command{
		Use:   "cordon",
		Short: "Mark a resource as unschedulable",
		Args:  cobra.NoArgs,
	}
	cordonCmd.AddCommand(newCordonServerCommand(true))

	cordonCmd.PersistentFlags().StringVar(&cordonKubeconfig, "kubeconfig", "", "Path to a kubeconfig.")
	cordonCmd.PersistentFlags().StringVar(&cordonKubeContext, "context", "", "Name of the kubeconfig context to use.")
	cordonCmd.PersistentFlags().BoolVar(&cordonDryRun, "dry-run", false,
		"Only print the object that would be sent, without patching it.")
	return cordonCmd
}

// NewUncordonCommand returns the `metalctl uncordon` command that marks a resource as
// schedulable again, allowing new ServerClaims to bind to it.
func NewUncordonCommand() *cobra.Command {
	uncordonCmd := &cobra.Command{
		Use:   "uncordon",
		Short: "Mark a resource as schedulable",
		Args:  cobra.NoArgs,
	}
	uncordonCmd.AddCommand(newCordonServerCommand(false))

	uncordonCmd.PersistentFlags().StringVar(&cordonKubeconfig, "kubeconfig", "", "Path to a kubeconfig.")
	uncordonCmd.PersistentFlags().StringVar(&cordonKubeContext, "context", "", "Name of the kubeconfig context to use.")
	uncordonCmd.PersistentFlags().BoolVar(&cordonDryRun, "dry-run", false,
		"Only print the object that would be sent, without patching it.")
	return uncordonCmd
}

func newCordonServerCommand(unschedulable bool) *cobra.Command {
	short := "Mark a Server as schedulable, allowing new ServerClaims to bind to it"
	if unschedulable {
		short = "Mark a Server as unschedulable, preventing new ServerClaims from binding to it"
	}
	return &cobra.Command{
		Use:   "server SERVER_NAME",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCordonServer(cmd.Context(), args[0], unschedulable)
		},
	}
}

func runCordonServer(ctx context.Context, serverName string, unschedulable bool) error {
	k8sClient, err := cmdclient.CreateClient(cordonKubeconfig, cordonKubeContext, scheme)
	if err != nil {
		return err
	}

	server := &metalv1alpha1.Server{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: serverName}, server); err != nil {
		return fmt.Errorf("failed to get Server %s: %w", serverName, err)
	}

	if server.Spec.Unschedulable == unschedulable {
		// Already in the desired state; nothing to do.
		return nil
	}

	base := server.DeepCopy()
	server.Spec.Unschedulable = unschedulable

	patchOpts := []client.PatchOption{}
	if cordonDryRun {
		patchOpts = append(patchOpts, client.DryRunAll)
	}
	if err := k8sClient.Patch(ctx, server, client.MergeFrom(base), patchOpts...); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("server %s not found", serverName)
		}
		return fmt.Errorf("failed to patch Server %s: %w", serverName, err)
	}
	return nil
}

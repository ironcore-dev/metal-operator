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

// NewCordonCommand returns the `metalctl cordon` command that marks a Server as
// unclaimable, preventing new ServerClaims from binding to it.
func NewCordonCommand() *cobra.Command {
	return newCordonServerCommand(true)
}

// NewUncordonCommand returns the `metalctl uncordon` command that marks a Server as
// claimable again, allowing new ServerClaims to bind to it.
func NewUncordonCommand() *cobra.Command {
	return newCordonServerCommand(false)
}

func newCordonServerCommand(unclaimable bool) *cobra.Command {
	use := "uncordon SERVER_NAME"
	short := "Mark a Server as claimable, allowing new ServerClaims to bind to it"
	if unclaimable {
		use = "cordon SERVER_NAME"
		short = "Mark a Server as unclaimable, preventing new ServerClaims from binding to it"
	}
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCordonServer(cmd.Context(), args[0], unclaimable)
		},
	}
	cmd.PersistentFlags().StringVar(&cordonKubeconfig, "kubeconfig", "", "Path to a kubeconfig.")
	cmd.PersistentFlags().StringVar(&cordonKubeContext, "context", "", "Name of the kubeconfig context to use.")
	cmd.PersistentFlags().BoolVar(&cordonDryRun, "dry-run", false,
		"Only print the object that would be sent, without patching it.")
	return cmd
}

func runCordonServer(ctx context.Context, serverName string, unclaimable bool) error {
	k8sClient, err := cmdclient.CreateClient(cordonKubeconfig, cordonKubeContext, scheme)
	if err != nil {
		return err
	}

	server := &metalv1alpha1.Server{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: serverName}, server); err != nil {
		return fmt.Errorf("failed to get Server %s: %w", serverName, err)
	}

	if server.Spec.Unclaimable == unclaimable {
		// Already in the desired state; nothing to do.
		return nil
	}

	base := server.DeepCopy()
	server.Spec.Unclaimable = unclaimable

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

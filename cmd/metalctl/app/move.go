// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	utils "github.com/ironcore-dev/metal-operator/cmdutils"
)

var (
	sourceKubeconfig string
	targetKubeconfig string
	namespace        string
	dryRun           bool
	verbose          bool
)

func NewMoveCommand() *cobra.Command {
	move := &cobra.Command{
		Use:   "move",
		Short: "Move metal-operator CRs from one cluster to another",
		RunE:  runMove,
	}
	move.Flags().StringVar(&sourceKubeconfig, "source-kubeconfig", "", "Kubeconfig pointing to the source cluster")
	move.Flags().StringVar(&targetKubeconfig, "target-kubeconfig", "", "Kubeconfig pointing to the target cluster")
	move.Flags().StringVar(&namespace, "namespace", "", "namespace to filter CRs to migrate. Defaults to all namespaces if not specified")
	move.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be moved without executing the migration")
	move.Flags().BoolVar(&verbose, "verbose", false, "enable verbose logging for detailed output during migration")
	move.MarkFlagRequired("source-kubeconfig")
	move.MarkFlagRequired("target-kubeconfig")

	if verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}
	return move
}

func makeClient(kubeconfig string) (client.Client, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to load cluster kubeconfig: %w", err)
	}
	return client.New(cfg, client.Options{Scheme: scheme})
}

func makeClients() (utils.Clients, error) {
	var clients utils.Clients
	var err error

	clients.Source, err = makeClient(sourceKubeconfig)
	if err != nil {
		return clients, fmt.Errorf("failed to construct a source cluster client: %w", err)
	}
	clients.Target, err = makeClient(targetKubeconfig)
	if err != nil {
		return clients, fmt.Errorf("failed to construct a target cluster client: %w", err)
	}
	return clients, nil
}

func runMove(cmd *cobra.Command, args []string) error {
	clients, err := makeClients()
	if err != nil {
		return err
	}
	ctx := cmd.Context()

	crdList := &apiextensionsv1.CustomResourceDefinitionList{}
	if err := clients.Source.List(ctx, crdList); err != nil {
		return err
	}
	crsSchema := []schema.GroupVersionKind{}
	for _, crd := range crdList.Items {
		if crd.GroupVersionKind().GroupVersion() == metalv1alpha1.GroupVersion {
			crsSchema = append(crsSchema, crd.GroupVersionKind())
		}
	}
	return utils.Move(ctx, clients, crsSchema, namespace, dryRun)
}

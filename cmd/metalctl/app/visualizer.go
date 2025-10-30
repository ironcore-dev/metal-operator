// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"log"

	cmdclient "github.com/ironcore-dev/metal-operator/internal/cmd/client"
	"github.com/ironcore-dev/metal-operator/internal/cmd/visualizer"
	"github.com/spf13/cobra"
)

var (
	port int
)

func NewVisualizationCommand() *cobra.Command {
	visualizerCmd := &cobra.Command{
		Use:   "visualizer",
		Short: "Visualize server topology",
		RunE:  runVisualizer,
		Aliases: []string{
			"viz",
		},
	}

	visualizerCmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig.")
	visualizerCmd.Flags().IntVar(&port, "port", 8080, "Port to run the web server on")

	return visualizerCmd
}

func runVisualizer(_ *cobra.Command, _ []string) error {
	log.Println("A 3D visualizer for server resources")

	c, err := cmdclient.CreateClient(kubeconfig, scheme)
	if err != nil {
		log.Fatalf("Error creating kubernetes client: %v", err)
	}

	vis := visualizer.NewVisualizer(c, port)

	return vis.StartAndServe()
}

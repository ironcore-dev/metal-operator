// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func CreateClient(kubeconfig string, scheme *runtime.Scheme) (client.Client, error) {
	if len(kubeconfig) == 0 {
		kubeconfig = os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			fmt.Println("Error: --kubeconfig flag or KUBECONFIG environment variable must be set")
			os.Exit(1)
		}
	}

	clientConfig, err := config.GetConfigWithContext("")
	if err != nil {
		return nil, fmt.Errorf("failed getting client config: %w", err)
	}

	k8sClient, err := client.New(clientConfig, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed creating client: %w", err)
	}
	return k8sClient, nil
}

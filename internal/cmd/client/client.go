// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateClient(kubeconfig, context string, scheme *runtime.Scheme) (client.Client, error) {
	if len(kubeconfig) == 0 {
		kubeconfig = os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			fmt.Println("Error: --kubeconfig flag or KUBECONFIG environment variable must be set")
			os.Exit(1)
		}
	}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	paths := filepath.SplitList(kubeconfig)
	if len(paths) == 1 {
		loadingRules.ExplicitPath = kubeconfig
	} else {
		loadingRules.Precedence = paths
	}
	overrides := &clientcmd.ConfigOverrides{}
	if context != "" {
		overrides.CurrentContext = context
	}
	clientConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed getting client config: %w", err)
	}

	k8sClient, err := client.New(clientConfig, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed creating client: %w", err)
	}
	return k8sClient, nil
}

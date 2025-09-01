package cmds

import (
	"fmt"
	"os"
	"time"

	"log"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var (
	kubeconfig     string
	kubeconfigPath string
	timeout        time.Duration
)

func NewResetCommand() *cobra.Command {
	resetCmd := &cobra.Command{
		Use:   "reset",
		Short: "Resets a BMC",
		RunE:  runReset,
	}

	resetCmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig.")
	resetCmd.Flags().DurationVar(&timeout, "timeout", 2*time.Minute, "Timeout for the reset operation.")

	return resetCmd
}

func runReset(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("bmc name is required")
	}
	if len(args) > 1 {
		return fmt.Errorf("too many arguments")
	}

	var bmcName = args[0]

	k8sClient, err := createClient()
	if err != nil {
		return err
	}
	bmcObj := &metalv1alpha1.BMC{}
	if err := k8sClient.Get(cmd.Context(), client.ObjectKey{Name: bmcName}, bmcObj); err != nil {
		log.Printf("failed to get bmc %q: %v", bmcName, err)
		os.Exit(1)
	}
	if err := bmcutils.ResetBMC(cmd.Context(), k8sClient, bmcObj, timeout); err != nil {
		log.Printf("failed to reset bmc %q: %v", bmcName, err)
		os.Exit(1)
	}
	return nil
}

func createClient() (client.Client, error) {
	if kubeconfig != "" {
		kubeconfigPath = kubeconfig
	} else {
		kubeconfigPath = os.Getenv("KUBECONFIG")
		if kubeconfigPath == "" {
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
		return nil, fmt.Errorf("failed creating controller-runtime client: %w", err)
	}
	return k8sClient, nil
}

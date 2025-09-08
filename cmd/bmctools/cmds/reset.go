package cmds

import (
	"fmt"
	"os"
	"time"

	"log"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	"github.com/spf13/cobra"
)

var (
	bmcAddress      string
	bmcManufacturer string
	bmcModel        string
	timeout         time.Duration
)

func NewResetCommand() *cobra.Command {
	resetCmd := &cobra.Command{
		Use:   "reset",
		Short: "Resets a BMC",
		RunE:  runReset,
	}
	resetCmd.Flags().DurationVar(&timeout, "timeout", 2*time.Minute, "Timeout for the reset operation.")
	resetCmd.Flags().StringVar(&bmcAddress, "bmc_address", "", "BMC address to connect to.")
	resetCmd.Flags().StringVar(&bmcManufacturer, "bmc_manufacturer", "", "BMC manufacturer name")
	resetCmd.Flags().StringVar(&bmcModel,
		"bmc_model", "", "BMC model. If not set, it will use the default model for the manufacturer.",
	)
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

	password, ok := os.LookupEnv(metalv1alpha1.BMCSecretUsernameKeyName)
	if !ok {
		return fmt.Errorf("password environment variable must be set")
	}
	username, ok := os.LookupEnv(metalv1alpha1.BMCSecretPasswordKeyName)
	if !ok {
		return fmt.Errorf("username environment variable must be set")
	}

	if err := bmcutils.SSHResetBMC(cmd.Context(), bmcAddress, username, bmcManufacturer, password, timeout); err != nil {
		log.Printf("failed to reset bmc %q: %v", bmcName, err)
		return err
	}
	log.Printf("successfully reset bmc %q", bmcName)
	return nil
}

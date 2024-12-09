// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"

	"github.com/ironcore-dev/metal-operator/internal/console"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var (
	kubeconfigPath      string
	kubeconfig          string
	serialConsoleNumber int
)

func NewConsoleCommand() *cobra.Command {
	consoleCmd := &cobra.Command{
		Use:   "console",
		Short: "Access the serial console of a Server",
		RunE:  runConsole,
	}

	consoleCmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig.")
	consoleCmd.Flags().IntVar(&serialConsoleNumber, "serial-console-number", 1, "Serial console number.")

	return consoleCmd
}

func runConsole(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("server name is required")
	}
	var serverName string
	if len(args) > 1 {
		return fmt.Errorf("too many arguments")
	}
	serverName = args[0]

	k8sClient, err := createClient()
	if err != nil {
		return err
	}

	if err := openConsoleStream(cmd.Context(), k8sClient, serverName); err != nil {
		return err
	}

	return nil
}

func openConsoleStream(ctx context.Context, k8sClient client.Client, serverName string) error {
	consoleConfig, err := console.GetConfigForServerName(ctx, k8sClient, serverName)
	if err != nil {
		return fmt.Errorf("failed to get console config: %w", err)
	}
	if consoleConfig == nil {
		return fmt.Errorf("console config is nil")
	}

	// Create SSH client configuration
	sshConfig := &ssh.ClientConfig{
		User: consoleConfig.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(consoleConfig.Password),
		},
		// TODO: use proper key verification
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	// Connect to the BMC
	bmcAddress := net.JoinHostPort(consoleConfig.BMCAddress, "22")
	conn, err := ssh.Dial("tcp", bmcAddress, sshConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to BMC: %w", err)
	}
	defer func(conn *ssh.Client) {
		if err = conn.Close(); err != nil {
			log.Printf("failed to close SSH connection: %v", err)
		}
	}(conn)

	// Start a session
	session, err := conn.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer func(session *ssh.Session) {
		if err = session.Close(); err != nil {
			log.Printf("failed to close SSH session: %v", err)
		}
	}(session)

	// Request a pseudo-terminal for interactive sessions
	if err = session.RequestPty("xterm", 80, 40, ssh.TerminalModes{
		ssh.ECHO:          0,     // Disable echo
		ssh.TTY_OP_ISPEED: 14400, // Input speed
		ssh.TTY_OP_OSPEED: 14400, // Output speed
	}); err != nil {
		return fmt.Errorf("failed to request pseudo-terminal failed: %w", err)
	}

	// Start the SOL session
	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("could not get stdin pipe: %w", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("could not get stdout pipe: %w", err)
	}

	go func() {
		_, err = io.Copy(os.Stdout, stdout)
		if err != nil {
			log.Printf("failed to copy stdout: %s", err)
		}
	}() // Stream the SOL output to the terminal

	if err = session.Start(fmt.Sprintf("console %d", serialConsoleNumber)); err != nil {
		return fmt.Errorf("failed to start SOL command: %w", err)
	}

	log.Println("Serial-over-LAN session active. Press Ctrl+C to exit.")
	go func() {
		// Allow sending input to the session
		_, err = io.Copy(stdin, os.Stdin)
		if err != nil {
			log.Printf("failed to copy stdin: %s", err)
		}
	}()

	// Wait for the session to end
	if err := session.Wait(); err != nil {
		return fmt.Errorf("error during SOL session: %v", err)
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

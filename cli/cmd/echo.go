package cmd

import (
	"context"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"strconv"
	"strings"
)

func init() {
	rootCmd.AddCommand(echoCmd)
}

var echoCmd = &cobra.Command{
	Use:   "echo <host>:<port> msg",
	Short: "Echos a message through the server",
	Long:  `Echos a message through the server`,
	Run:   echo,
	Args:  cobra.ExactArgs(2),
}

func echo(cmd *cobra.Command, args []string) {
	// ensure our server is passed
	if len(args) != 2 {
		pterm.Error.Printf("invalid arguments passed - should be 2\n")
		return
	}

	// split the target
	split := strings.Split(args[0], ":")
	if len(split) != 2 {
		pterm.Error.Printf("invalid server - should be <host>:<port>\n")
		return
	}

	port, err := strconv.ParseInt(split[1], 10, 32)
	if err != nil {
		pterm.Error.Printf("invalid port for server\n")
		return
	}

	client, err := NewWorkspaceClient(WorkspaceClientOptions{
		Host: split[0],
		Port: int(port),
	})
	if err != nil {
		pterm.Error.Printf("failed to create client: %v\n", err)
		return
	}

	echoRes, err := client.Echo(context.TODO(), args[1])
	if err != nil {
		pterm.Error.Printf("ECHO FAILED\n%v\n", err)
		return
	}

	pterm.Info.Printf("ECHO COMPLETED\nECHO: %s\n", echoRes)
}

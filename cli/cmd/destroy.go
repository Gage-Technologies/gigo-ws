package cmd

import (
	"context"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"strconv"
	"strings"
)

func init() {
	rootCmd.AddCommand(destroyCmd)
}

var destroyCmd = &cobra.Command{
	Use:   "destroy <host>:<port> workspace_id",
	Short: "Destroys an existing workspace",
	Long:  `Destroys an existing workspace`,
	Run:   destroyWorkspace,
	Args:  cobra.ExactArgs(2),
}

func destroyWorkspace(cmd *cobra.Command, args []string) {
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

	wsId, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		pterm.Error.Printf("invalid workspace id\n")
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

	pterm.Debug.Printf("Destroy Workspace Request: %+v\n", wsId)

	spinner, err := pterm.DefaultSpinner.Start("Destroying Workspace")
	if err != nil {
		pterm.Error.Printf("failed to start spinner: %v\n", err)
		return
	}

	err = client.DestroyWorkspace(context.TODO(), wsId)
	if err != nil {
		_ = spinner.Stop()
		pterm.Error.Printf("WORKSPACE DESTROY FAILED\n%v\n", err)
		return
	}

	_ = spinner.Stop()

	pterm.Info.Printf("WORKSPACE DESTROYED\n")
}

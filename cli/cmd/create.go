package cmd

import (
	"context"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"os"
	"strconv"
	"strings"
)

func init() {
	rootCmd.AddCommand(createCmd)

	// optional config
	createCmd.Flags().StringP("config", "c", "", "config for the workspace")

	// optional individual params
	createCmd.Flags().Int64P("workspace_id", "w", -1, "workspace id")
	createCmd.Flags().Int64P("owner_id", "o", -1, "owner id")
	createCmd.Flags().StringP("owner_email", "e", "", "owner email")
	createCmd.Flags().StringP("owner_name", "n", "", "owner name")
	createCmd.Flags().IntP("disk", "d", 0, "disk")
	createCmd.Flags().IntP("cpu", "q", 0, "cpu")
	createCmd.Flags().IntP("memory", "m", 0, "memory")
	createCmd.Flags().StringP("container", "b", "", "container")
	createCmd.Flags().StringP("access_url", "u", "", "access url")

}

var createCmd = &cobra.Command{
	Use:   "create <host>:<port> [options]",
	Short: "Creates a new workspace",
	Long:  `Creates a new workspace`,
	Run:   createWorkspace,
	Args:  cobra.ExactArgs(1),
}

func createWorkspace(cmd *cobra.Command, args []string) {
	// ensure our server is passed
	if len(args) != 1 {
		pterm.Error.Printf("no server passed\n")
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

	cfgPath, err := cmd.Flags().GetString("config")
	if err != nil {
		pterm.Error.Printf("failed to retrieve config path: %v\n", err)
		return
	}

	var opts CreateWorkspaceOptions

	if cfgPath != "" {
		buf, err := os.ReadFile(cfgPath)
		if err != nil {
			pterm.Error.Printf("failed to read file: %v\n", err)
			return
		}

		err = yaml.Unmarshal(buf, &opts)
		if err != nil {
			pterm.Error.Printf("failed to unmarshall config - is it yaml?\n")
			return
		}
	} else {
		panic("not implemented")
	}

	pterm.Debug.Printf("Create Workspace Request: %+v\n", opts)

	spinner, err := pterm.DefaultSpinner.Start("Creating Workspace")
	if err != nil {
		pterm.Error.Printf("failed to start spinner: %v\n", err)
		return
	}

	agent, err := client.CreateWorkspace(context.TODO(), opts)
	if err != nil {
		_ = spinner.Stop()
		pterm.Error.Printf("WORKSPACE CREATION FAILED\n%v\n", err)
		return
	}

	_ = spinner.Stop()

	pterm.Info.Printf("WORKSPACE CREATED\nAGENT ID: %d\nTOKEN   : %s\n", agent.ID, agent.Token)
}

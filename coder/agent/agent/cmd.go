package agent

import (
	"context"
	"fmt"
	"gigo-ws/coder/agent/agent/usershell"
	"gigo-ws/utils"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
	"time"

	"github.com/gage-technologies/gigo-lib/coder/agentsdk"
	"github.com/go-cmd/cmd"
	"golang.org/x/xerrors"
)

// executeCommandEnv
//
//	 Wrapper for utils.ExecuteCommand that sets up the default
//		user shell environment.
//
//	 NOTE: DO NOT PASS A SHELL COMMAND LIKE `bash -c 'echo example'`
//	 THIS FUNCTION IS A SHELL WRAPPER - YOU WILL RECEIVE SUCH
//	 FUNCTIONALITY BY DEFAULT
func (a *agent) executeCommandEnv(ctx context.Context, env []string, dir string, rawCommand string) (*utils.CommandResult, error) {
	currentUser, err := user.Current()
	if err != nil {
		return nil, xerrors.Errorf("get current user: %w", err)
	}
	username := currentUser.Username

	shell, err := usershell.Get(username)
	if err != nil {
		return nil, xerrors.Errorf("get user shell: %w", err)
	}

	// OpenSSH executes all commands with the users current shell.
	// We replicate that behavior for IDE support.
	caller := "-c"
	if runtime.GOOS == "windows" {
		caller = "/c"
	}
	args := []string{caller, rawCommand}

	// gliderlabs/ssh returns a command slice of zero
	// when a shell is requested.
	if len(rawCommand) == 0 {
		args = []string{}
		if runtime.GOOS != "windows" {
			// On Linux and macOS, we should start a login
			// shell to consume juicy environment variables!
			args = append(args, "-l")
		}
	}

	rawMetadata := a.metadata.Load()
	if rawMetadata == nil {
		return nil, xerrors.Errorf("no metadata was provided: %w", err)
	}
	metadata, valid := rawMetadata.(agentsdk.WorkspaceAgentMetadata)
	if !valid {
		return nil, xerrors.Errorf("metadata is the wrong type: %T", metadata)
	}

	// create a new command
	c := cmd.NewCmd(shell, args...)

	// home is always gigo unless otherwise specified
	c.Dir = "/home/gigo"
	if len(dir) > 0 {
		c.Dir = dir
	}
	c.Env = append(os.Environ(), env...)
	executablePath, err := os.Executable()
	if err != nil {
		return nil, xerrors.Errorf("getting os executable: %w", err)
	}
	// Set environment variables reliable detection of being inside a
	// Coder workspace.
	c.Env = append(c.Env, "CODER=true")
	c.Env = append(c.Env, fmt.Sprintf("USER=%s", username))
	// Git on Windows resolves with UNIX-style paths.
	// If using backslashes, it's unable to find the executable.
	unixExecutablePath := strings.ReplaceAll(executablePath, "\\", "/")
	c.Env = append(c.Env, fmt.Sprintf(`GIT_SSH_COMMAND=%s gitssh --`, unixExecutablePath))

	// specific Gigo Agent subcommands require the agent token exposed
	auth := a.client.SessionAuth()
	c.Env = append(c.Env, fmt.Sprintf("GIGO_AGENT_TOKEN=%s", auth.Token))
	c.Env = append(c.Env, fmt.Sprintf("GIGO_WORKSPACE_ID=%d", auth.WorkspaceID))

	// Set SSH connection environment variables (these are also set by OpenSSH
	// and thus expected to be present by SSH clients). Since the agent does
	// networking in-memory, trying to provide accurate values here would be
	// nonsensical. For now, we hard code these values so that they're present.
	srcAddr, srcPort := "0.0.0.0", "0"
	dstAddr, dstPort := "0.0.0.0", "0"
	c.Env = append(c.Env, fmt.Sprintf("SSH_CLIENT=%s %s %s", srcAddr, srcPort, dstPort))
	c.Env = append(c.Env, fmt.Sprintf("SSH_CONNECTION=%s %s %s %s", srcAddr, srcPort, dstAddr, dstPort))

	// This adds the ports dialog to code-server that enables
	// proxying a port dynamically.
	c.Env = append(c.Env, fmt.Sprintf("VSCODE_PROXY_URI=%s", metadata.VSCodePortProxyURI))

	// Hide Coder message on code-server's "Getting Started" page
	c.Env = append(c.Env, "CS_DISABLE_GETTING_STARTED_OVERRIDE=true")

	// Load environment variables passed via the agent.
	// These should override all variables we manually specify.
	for envKey, value := range metadata.GigoConfig.Environment {
		// Expanding environment variables allows for customization
		// of the $PATH, among other variables. Customers can prepend
		// or append to the $PATH, so allowing expand is required!
		c.Env = append(c.Env, fmt.Sprintf("%s=%s", envKey, os.ExpandEnv(value)))
	}

	// Agent-level environment variables should take over all!
	// This is used for setting agent-specific variables like "GIGO_AGENT_TOKEN".
	for envKey, value := range a.envVars {
		c.Env = append(c.Env, fmt.Sprintf("%s=%s", envKey, value))
	}

	// start command
	statusChan := c.Start()

	// wait for command or context
	select {
	case <-ctx.Done():
		// stop command since we are exiting early
		err := c.Stop()
		return nil, fmt.Errorf("context closed - %v", err)
	case status := <-statusChan:
		// load data from status by retrieving the last
		// line of output - go-cmd is a bit weird on how
		// it handle output. the last string in the slice
		// is the final output
		stdOut := ""
		stdErr := ""
		if len(status.Stdout) > 0 {
			stdOut = strings.Join(status.Stdout, "\n")
		}
		if len(status.Stderr) > 0 {
			stdErr = strings.Join(status.Stderr, "\n")
		}

		// format the start and end time from the timestamps
		start := time.Unix(0, status.StartTs)
		end := time.Unix(0, status.StopTs)

		return &utils.CommandResult{
			Command:  rawCommand,
			Stdout:   stdOut,
			Stderr:   stdErr,
			ExitCode: status.Exit,
			Start:    start,
			End:      end,
			Cost:     end.Sub(start),
		}, nil
	}
}

// createCommand processes raw command input with OpenSSH-like behavior.
// If the rawCommand provided is empty, it will default to the users shell.
// This injects environment variables specified by the user at launch too.
func (a *agent) createCommand(ctx context.Context, rawCommand string, dir string, env []string) (*exec.Cmd, error) {
	currentUser, err := user.Current()
	if err != nil {
		return nil, xerrors.Errorf("get current user: %w", err)
	}
	username := currentUser.Username

	shell, err := usershell.Get(username)
	if err != nil {
		return nil, xerrors.Errorf("get user shell: %w", err)
	}

	rawMetadata := a.metadata.Load()
	if rawMetadata == nil {
		return nil, xerrors.Errorf("no metadata was provided: %w", err)
	}
	metadata, valid := rawMetadata.(agentsdk.WorkspaceAgentMetadata)
	if !valid {
		return nil, xerrors.Errorf("metadata is the wrong type: %T", metadata)
	}

	// OpenSSH executes all commands with the users current shell.
	// We replicate that behavior for IDE support.
	caller := "-c"
	if runtime.GOOS == "windows" {
		caller = "/c"
	}
	args := []string{caller, rawCommand}

	// gliderlabs/ssh returns a command slice of zero
	// when a shell is requested.
	if len(rawCommand) == 0 {
		args = []string{}
		if runtime.GOOS != "windows" {
			// On Linux and macOS, we should start a login
			// shell to consume juicy environment variables!
			args = append(args, "-l")
		}
	}

	command := exec.CommandContext(ctx, shell, args...)
	// home is always gigo unless otherwise specified
	command.Dir = "/home/gigo"
	if len(dir) > 0 {
		command.Dir = dir
	}
	command.Env = append(os.Environ(), env...)
	executablePath, err := os.Executable()
	if err != nil {
		return nil, xerrors.Errorf("getting os executable: %w", err)
	}
	// Set environment variables reliable detection of being inside a
	// Coder workspace.
	command.Env = append(command.Env, "CODER=true")
	command.Env = append(command.Env, fmt.Sprintf("USER=%s", username))
	// Git on Windows resolves with UNIX-style paths.
	// If using backslashes, it's unable to find the executable.
	unixExecutablePath := strings.ReplaceAll(executablePath, "\\", "/")
	command.Env = append(command.Env, fmt.Sprintf(`GIT_SSH_COMMAND=%s gitssh --`, unixExecutablePath))

	// specific Gigo Agent subcommands require the agent token exposed
	auth := a.client.SessionAuth()
	command.Env = append(command.Env, fmt.Sprintf("GIGO_AGENT_TOKEN=%s", auth.Token))
	command.Env = append(command.Env, fmt.Sprintf("GIGO_WORKSPACE_ID=%d", auth.WorkspaceID))

	// Set SSH connection environment variables (these are also set by OpenSSH
	// and thus expected to be present by SSH clients). Since the agent does
	// networking in-memory, trying to provide accurate values here would be
	// nonsensical. For now, we hard code these values so that they're present.
	srcAddr, srcPort := "0.0.0.0", "0"
	dstAddr, dstPort := "0.0.0.0", "0"
	command.Env = append(command.Env, fmt.Sprintf("SSH_CLIENT=%s %s %s", srcAddr, srcPort, dstPort))
	command.Env = append(command.Env, fmt.Sprintf("SSH_CONNECTION=%s %s %s %s", srcAddr, srcPort, dstAddr, dstPort))

	// This adds the ports dialog to code-server that enables
	// proxying a port dynamically.
	command.Env = append(command.Env, fmt.Sprintf("VSCODE_PROXY_URI=%s", metadata.VSCodePortProxyURI))

	// Hide Coder message on code-server's "Getting Started" page
	command.Env = append(command.Env, "CS_DISABLE_GETTING_STARTED_OVERRIDE=true")

	// Load environment variables passed via the agent.
	// These should override all variables we manually specify.
	for envKey, value := range metadata.GigoConfig.Environment {
		// Expanding environment variables allows for customization
		// of the $PATH, among other variables. Customers can prepend
		// or append to the $PATH, so allowing expand is required!
		command.Env = append(command.Env, fmt.Sprintf("%s=%s", envKey, os.ExpandEnv(value)))
	}

	// Agent-level environment variables should take over all!
	// This is used for setting agent-specific variables like "GIGO_AGENT_TOKEN".
	for envKey, value := range a.envVars {
		command.Env = append(command.Env, fmt.Sprintf("%s=%s", envKey, value))
	}

	return command, nil
}

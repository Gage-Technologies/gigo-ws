package agent

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/buger/jsonparser"
	"github.com/sourcegraph/conc/pool"

	"gigo-ws/utils"

	"cdr.dev/slog"
	"github.com/coder/retry"
	"github.com/gage-technologies/gigo-lib/coder/agentsdk"
	"github.com/gage-technologies/gigo-lib/db/models"
	utils2 "github.com/gage-technologies/gigo-lib/utils"
	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"
)

// this file is dedicated to functions and routines
// that boostrap a gigo workspace for the user
//
// bootstrapping steps:
//
// 1. initialize the workspace
// 2. (if not exists) write git configuration
// 3. write workspace configuration
// 4. (if not exists) clone repository
// 6. (if not exists) write container compose
// 7. compose up
// 8. execute user defined commands
// 9. (if vscode) install code-server
// 10. (if vscode) install code-server extensions
// 11. (if vscode) launch code-server
// 12. (if not vscode) wait forever

// embed the bootstrap_scripts directory
// into the agent binary

//go:embed bootstrap_scripts
var bootstrapScripts embed.FS

const gitConfigTemplate = `
[user]
	email = %s
	name = %s
[url "%s"]
	insteadOf = %s
`

type WorkspaceConfig struct {
	Secret            string                    `json:"secret"`
	WorkspaceID       int64                     `json:"workspace_id"`
	WorkspaceIDString string                    `json:"workspace_id_string"`
	Repo              string                    `json:"repo"`
	Commit            string                    `json:"commit"`
	Expiration        int64                     `json:"expiration"`
	OwnerID           int64                     `json:"owner_id"`
	OwnerIDString     string                    `json:"owner_id_string"`
	ChallengeType     models.ChallengeType      `json:"challenge_type"`
	WorkspaceSettings *models.WorkspaceSettings `json:"workspace_settings"`
}

type CodeServerHealth struct {
	Status        string `json:"status"`
	LastHeartbeat int64  `json:"lastHeartbeat"`
}

type StateFromFile struct {
	LastInitState int `json:"last_init_state"`
}

// formatConfigEnv
//
// Formats the gigo config environment for ExecCommand
func formatConfigEnv(metadata agentsdk.WorkspaceAgentMetadata) []string {
	// format environment into string format and forward our $PATH to the environment
	env := []string{fmt.Sprintf("PATH=%s", os.Getenv("PATH"))}
	for k, v := range metadata.GigoConfig.Environment {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}

// hasBeenInitialized
//
//	Determines whether we have previously initialized
//	the workspace by locating the workspace config on
//	the filesystem
func hasBeenInitialized() (bool, error) {
	// check if the workspace config exists on the filesystem
	exists, err := utils2.PathExists("/home/gigo/.gigo/ws-config.json")
	if err != nil {
		return false, fmt.Errorf("failed to check for workspace config: %v", err)
	}
	return exists, nil
}

// writeGitConfig
//
//	Writes the workspace git config to `gigo` user's
//	home .gitconfig file. If a git config already exists
//	it will be overwritten silently since each workspace
//	session has a new git token.
func writeGitConfig(ctx context.Context, metadata agentsdk.WorkspaceAgentMetadata) error {
	// exit silently if we don't have a token because
	// it already exists
	if metadata.GitToken == "exists" {
		return nil
	}

	// parse repo url so that we can get
	// the url parts
	repoUrl, err := url.Parse(metadata.Repo)
	if err != nil {
		return fmt.Errorf("failed to parse repo url: %v", err)
	}

	// form the original git server url
	// and the new authenticated url
	gitServerUrl := fmt.Sprintf("%s://%s", repoUrl.Scheme, repoUrl.Host)
	authenticatedUrl := fmt.Sprintf("%s://%s:%s@%s", repoUrl.Scheme, metadata.GitName, metadata.GitToken, repoUrl.Host)

	// format git configuration
	gitConfig := fmt.Sprintf(gitConfigTemplate, metadata.GitEmail, metadata.GitName, authenticatedUrl, gitServerUrl)

	// write git configuration to file
	// we want to overwrite here because each
	// workspace session has a new git token
	err = os.WriteFile("/home/gigo/.gitconfig", []byte(gitConfig), 0600)
	if err != nil {
		return fmt.Errorf("failed to write git configuration: %v", err)
	}

	return nil
}

// writeWorkspaceConfig
//
//	Writes the workspace config to `gigo` user's home
//	gigo configuration directory. If a config already
//	exists, it will be overwritten silently since each
//	workspace session has a new workspace secret.
func writeWorkspaceConfig(ctx context.Context, metadata agentsdk.WorkspaceAgentMetadata, secret string) error {
	// format workspace config from metadata
	cfg := WorkspaceConfig{
		Secret:            secret,
		WorkspaceID:       metadata.WorkspaceID,
		WorkspaceIDString: metadata.WorkspaceIDString,
		Repo:              metadata.Repo,
		Commit:            metadata.Commit,
		Expiration:        metadata.Expiration,
		OwnerID:           metadata.OwnerID,
		ChallengeType:     metadata.ChallengeType,
		OwnerIDString:     metadata.OwnerIDString,
		WorkspaceSettings: metadata.WorkspaceSettings,
	}

	// create gigo directory if it doesn't exist
	err := os.MkdirAll("/home/gigo/.gigo", 0700)
	if err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	// marshall workspace config into a json
	jsonCfg, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal workspace config: %v", err)
	}

	// write workspace config to file
	// we want to overwrite here because each
	// workspace session has a new expiration
	err = os.WriteFile("/home/gigo/.gigo/ws-config.json", jsonCfg, 0600)
	if err != nil {
		return fmt.Errorf("failed to write workspace config: %v", err)
	}

	return nil
}

// cloneRepo
//
//	Clones the workspace repository to the
//	configured working directory using the
//	configured credentials and commit
func cloneRepo(ctx context.Context, metadata agentsdk.WorkspaceAgentMetadata) (*utils.CommandResult, error) {
	// clone repository to the working directory
	res, err := utils.ExecuteCommand(
		ctx, nil, "",
		"git", "clone", "--recursive", metadata.Repo, metadata.GigoConfig.WorkingDirectory,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to clone repository: %v", err)
	}

	// ensure good status code
	if res.ExitCode != 0 {
		return res, fmt.Errorf("failed to clone repository")
	}

	// checkout repo
	res, err = utils.ExecuteCommand(
		ctx, nil, "",
		"bash", "-c", fmt.Sprintf("cd %s && git checkout %s", metadata.GigoConfig.WorkingDirectory, metadata.Commit),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to checkout repository: %v", err)
	}

	// ensure good status code
	if res.ExitCode != 0 {
		return res, fmt.Errorf("failed to checkout repository")
	}

	return res, nil
}

// writeContainerCompose
//
//	Writes a docker-compose format container spec
//	to the user gigo configuration directory
func writeContainerCompose(ctx context.Context, metadata agentsdk.WorkspaceAgentMetadata) error {
	// create container directory if it doesn't exist
	err := os.MkdirAll("/home/gigo/.gigo/containers", 0700)
	if err != nil {
		return fmt.Errorf("failed to create container directory: %v", err)
	}

	// serialize container compose to yaml bytes
	yamlBytes, err := yaml.Marshal(metadata.GigoConfig.Containers)
	if err != nil {
		return fmt.Errorf("failed to marshal container spec: %v", err)
	}

	// create docker-compose file
	err = os.WriteFile("/home/gigo/.gigo/containers/docker-compose.yml", yamlBytes, 0600)
	if err != nil {
		return fmt.Errorf("failed to create docker-compose file: %v", err)
	}

	return nil
}

// containerComposeUp
//
//	Executes a compose up command on the container
//	compose configuration
func containerComposeUp(ctx context.Context, metadata agentsdk.WorkspaceAgentMetadata) (*utils.CommandResult, error) {
	// execute compose up command but use both the old
	// and the new compose formats
	res, err := utils.ExecuteCommand(
		ctx, formatConfigEnv(metadata), "/home/gigo/.gigo/containers",
		"bash", "-c", "(sudo -E docker compose up -d || sudo -E docker-compose up -d)",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to compose up containers: %v", err)
	}

	// ensure good status code
	if res.ExitCode != 0 {
		return res, fmt.Errorf("failed to compose up containers")
	}

	return res, nil
}

// handleUserExecutions
//
//	Iterates over the user executions performing
//	the commands in the user defined environment
func handleUserExecutions(ctx context.Context, metadata agentsdk.WorkspaceAgentMetadata, lastInitState models.WorkspaceInitState) (*utils.CommandResult, error) {
	// iterate over the user executions
	for _, exec := range metadata.GigoConfig.Exec {
		// skip init only execs if this is not
		// our first bootstrap in the workspace
		if exec.Init && lastInitState > models.WorkspaceInitShellExecutions {
			continue
		}

		// execute command
		res, err := utils.ExecuteCommand(
			ctx, formatConfigEnv(metadata), metadata.GigoConfig.WorkingDirectory,
			"bash", "-c", exec.Command,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to execute command: %v", err)
		}

		// ensure good status code
		if res.ExitCode != 0 {
			return res, fmt.Errorf("failed to execute command")
		}
	}

	return nil, nil
}

// installCodeServer
//
//	Installs code-server
func (a *agent) installCodeServer(ctx context.Context, metadata agentsdk.WorkspaceAgentMetadata) (*utils.CommandResult, error) {
	// check if code-server is already installed
	_, err := os.Stat("/home/gigo/.local/bin/code-server")
	// only install code-server if it doesn't exist
	if err != nil {
		// load the install_codeserver script from the bootstrap_scripts directory
		installScript, err := bootstrapScripts.ReadFile("bootstrap_scripts/install_codeserver")
		if err != nil {
			return nil, fmt.Errorf("failed to read install_codeserver script: %v", err)
		}

		// write the install_codeserver script to a file
		err = os.WriteFile("/tmp/install_codeserver", installScript, 0700)
		if err != nil {
			return nil, fmt.Errorf("failed to write install_codeserver script: %v", err)
		}

		// create an environment for the install script containing the agent token and workspace id
		auth := a.client.SessionAuth()
		env := []string{
			"HOME=/home/gigo",
			fmt.Sprintf("GIGO_AGENT_TOKEN=%s", auth.Token),
			fmt.Sprintf("GIGO_WORKSPACE_ID=%d", auth.WorkspaceID),
			fmt.Sprintf("GIGO_API_URL=%s", strings.TrimSuffix(a.accessUrl.String(), "/")),
		}

		// install code-server
		res, err := utils.ExecuteCommand(
			ctx, env, "",
			"sh", "/tmp/install_codeserver", "--prefix=/home/gigo/.local",
		)
		if err != nil {
			return nil, fmt.Errorf("failed to install code-server: %v", err)
		}

		// ensure good status code
		if res.ExitCode != 0 {
			return res, fmt.Errorf("failed to install code-server")
		}
	}

	// check to make sure that the code-server binary exists
	_, err = os.Stat("/home/gigo/.local/bin/code-server")
	if err != nil {
		return nil, fmt.Errorf("failed to install code-server - binary missing")
	}

	// link the code-server binary to the /usr/local/bin directory if it doesn't exist
	_, err = os.Stat("/usr/local/bin/code-server")
	if err != nil {
		// link the code-server binary
		_, err := utils.ExecuteCommand(
			ctx, nil, "",
			"bash", "-c", "sudo ln -s /home/gigo/.local/bin/code-server /usr/local/bin/code-server",
		)
		if err != nil {
			return nil, fmt.Errorf("failed to link code-server binary: %v", err)
		}
	}

	return nil, nil
}

// installCodeServerExtensions
//
//	Installs mandatory and user-defined code-server extensions
//	including the download and install of the gigo extension
func (a *agent) installCodeServerExtensions(ctx context.Context, metadata agentsdk.WorkspaceAgentMetadata) (*utils.CommandResult, error) {
	// get the vscode version from code-server
	res, err := utils.ExecuteCommand(
		ctx, nil, "",
		"code-server", "--version", "--json",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get code-server version: %v", err)
	}

	// ensure good status code
	if res.ExitCode != 0 {
		return res, fmt.Errorf("failed to get code-server version")
	}

	// extract the vscode key from the version output
	vscVersion, err := jsonparser.GetString([]byte(res.Stdout), "vscode")
	if err != nil {
		return nil, fmt.Errorf("failed to parse code-server version: %v", err)
	}

	// create wait group to execute extension installations in parallel
	wg := pool.NewWithResults[*utils.CommandResult]().WithErrors().WithMaxGoroutines(8)

	// we can download in parallel but installing should be done one at a time
	// otherwise we run into weird issues with extensions not installing correctly
	installMu := &sync.Mutex{}

	wg.Go(func() (*utils.CommandResult, error) {
		// cleanup extension on exit
		defer os.Remove("/tmp/gigo-developer.vsix")

		a.logger.Info(ctx, "installing gigo vscode extension")

		// download the gigo extension
		dlCtx, cancel := context.WithTimeout(ctx, time.Minute*2)
		defer cancel()
		err := a.client.WorkspaceGetExtension(dlCtx, "/tmp/gigo-developer.vsix")
		if err != nil {
			return nil, fmt.Errorf("failed to download gigo vscode extension: %v", err)
		}

		installMu.Lock()
		// execute install of gigo extension
		res, err := utils.ExecuteCommand(
			ctx, nil, "",
			"code-server", "--install-extension", "/tmp/gigo-developer.vsix",
		)
		installMu.Unlock()
		if err != nil {
			return nil, fmt.Errorf("failed to install gigo extension: %v", err)
		}

		// ensure good status code
		if res.ExitCode != 0 {
			errStr := strings.TrimSpace(res.Stderr)
			if errStr == "" {
				errStr = strings.TrimSpace(res.Stdout)
			}
			return res, fmt.Errorf("failed to install gigo extension: %v", errStr)
		}

		a.logger.Info(ctx, "gigo vscode extension installed")

		return nil, nil
	})

	wg.Go(func() (*utils.CommandResult, error) {
		// cleanup extension on exit
		defer os.Remove("/tmp/code-teacher.vsix")

		a.logger.Info(ctx, "installing code teacher vscode extension")

		// download the code teacher extension
		dlCtx, cancel := context.WithTimeout(ctx, time.Minute*2)
		defer cancel()
		err := a.client.WorkspaceGetCtExtension(dlCtx, "/tmp/code-teacher.vsix")
		if err != nil {
			return nil, fmt.Errorf("failed to download code teacher vscode extension: %v", err)
		}

		installMu.Lock()
		// execute install of gigo extension
		res, err := utils.ExecuteCommand(
			ctx, nil, "",
			"code-server", "--install-extension", "/tmp/code-teacher.vsix",
		)
		installMu.Unlock()
		if err != nil {
			return nil, fmt.Errorf("failed to install code teacher extension: %v", err)
		}

		// ensure good status code
		if res.ExitCode != 0 {
			errStr := strings.TrimSpace(res.Stderr)
			if errStr == "" {
				errStr = strings.TrimSpace(res.Stdout)
			}
			return res, fmt.Errorf("failed to install code teacher extension: %s", errStr)
		}

		a.logger.Info(ctx, "code teacher vscode extension installed")

		return nil, nil
	})

	// only install the gigo theme for premium users
	if metadata.UserStatus == models.UserStatusPremium {
		wg.Go(func() (*utils.CommandResult, error) {
			// cleanup extension on exit
			defer os.Remove("/tmp/gigo-theme.vsix")

			a.logger.Info(ctx, "installing gigo pro theme extension")

			// download the code teacher extension
			dlCtx, cancel := context.WithTimeout(ctx, time.Minute*2)
			defer cancel()
			err := a.client.WorkspaceGetThemeExtension(dlCtx, "/tmp/gigo-theme.vsix")
			if err != nil {
				return nil, fmt.Errorf("failed to download gigo pro theme extension: %v", err)
			}

			installMu.Lock()
			// execute install of gigo extension
			res, err := utils.ExecuteCommand(
				ctx, nil, "",
				"code-server", "--install-extension", "/tmp/gigo-theme.vsix",
			)
			installMu.Unlock()
			if err != nil {
				return nil, fmt.Errorf("failed to install gigo pro theme extension: %v", err)
			}

			// ensure good status code
			if res.ExitCode != 0 {
				errStr := strings.TrimSpace(res.Stderr)
				if errStr == "" {
					errStr = strings.TrimSpace(res.Stdout)
				}
				return res, fmt.Errorf("failed to install gigo pro theme extension: %s", errStr)
			}

			a.logger.Info(ctx, "gigo pro theme extension installed")

			return nil, nil
		})
	} else {
		a.logger.Info(ctx, "skipping gigo pro theme extension installation")
	}

	if metadata.HolidaySeason > 0 && metadata.UserHolidayTheme {
		wg.Go(func() (*utils.CommandResult, error) {
			// cleanup extension on exit
			defer os.Remove("/tmp/gigo-holiday-theme.vsix")

			a.logger.Info(ctx, "installing gigo pro theme extension")

			// download the code teacher extension
			dlCtx, cancel := context.WithTimeout(ctx, time.Minute*2)
			defer cancel()
			err := a.client.WorkspaceGetHolidayThemeExtension(dlCtx, "/tmp/gigo-holiday-theme.vsix", int(metadata.HolidaySeason))
			if err != nil {
				return nil, fmt.Errorf("failed to download gigo holiday theme extension: %v", err)
			}

			installMu.Lock()
			// execute install of gigo extension
			res, err := utils.ExecuteCommand(
				ctx, nil, "",
				"code-server", "--install-extension", "/tmp/gigo-holiday-theme.vsix",
			)
			installMu.Unlock()
			if err != nil {
				return nil, fmt.Errorf("failed to install gigo holiday theme extension: %v", err)
			}

			// ensure good status code
			if res.ExitCode != 0 {
				errStr := strings.TrimSpace(res.Stderr)
				if errStr == "" {
					errStr = strings.TrimSpace(res.Stdout)
				}
				return res, fmt.Errorf("failed to install gigo holiday theme extension: %s", errStr)
			}

			a.logger.Info(ctx, "gigo holiday theme extension installed")

			return nil, nil
		})
	} else {
		a.logger.Info(ctx, "skipping gigo holiday theme extension installation")
	}

	// append code tour extension to the user defined extensions since it is mandatory in Gigo
	metadata.GigoConfig.VSCode.Extensions = append(metadata.GigoConfig.VSCode.Extensions, "vsls-contrib.codetour")

	// iterate user defined extensions performing install to code-server
	installedExt := make(map[string]bool)
	for _, ext := range metadata.GigoConfig.VSCode.Extensions {
		// copy the extension to prevent Go's stupid pointer behavior in loops
		ext := ext

		// skip if we have already installed this extension
		if installedExt[ext] {
			continue
		}
		installedExt[ext] = true

		wg.Go(func() (*utils.CommandResult, error) {
			a.logger.Info(ctx, "installing user defined extension", slog.F("extension", ext))
			originalExt := ext

			// check if the extension is a url that we should download
			if strings.HasPrefix(ext, "http") {
				// download extension and install
				a.logger.Info(ctx, "downloading extension from url", slog.F("url", ext))

				// create request and clearly identify ourselves
				req, err := http.NewRequestWithContext(ctx, "GET", ext, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to create http request: %v", err)
				}
				req.Header.Set("User-Agent", "gigo-agent - contact: dev@gigo.dev")

				// perform request
				res, err := http.DefaultClient.Do(req)
				if err != nil {
					return nil, fmt.Errorf("failed to perform http request: %v", err)
				}

				// ensure good status code
				if res.StatusCode != 200 {
					return nil, fmt.Errorf("failed to download extension")
				}

				// create temp file to write extension to
				tmpFile, err := os.CreateTemp("", "gigo-extension-*.vsix")
				if err != nil {
					return nil, fmt.Errorf("failed to create temp file: %v", err)
				}
				defer tmpFile.Close()

				// cleanup temp file on exit
				defer os.Remove(tmpFile.Name())

				// write extension to temp file
				_, err = io.Copy(tmpFile, res.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to write extension to temp file: %v", err)
				}

				// close temp file
				err = tmpFile.Close()
				if err != nil {
					return nil, fmt.Errorf("failed to close temp file: %v", err)
				}

				// assign extension path to extension variable
				extUrl := ext
				ext = tmpFile.Name()

				a.logger.Info(ctx, "extension downloaded and saved to temp file", slog.F("url", extUrl), slog.F("path", ext))
			} else {
				// create temp file to write extension to
				tmpFile, err := os.CreateTemp("", "gigo-extension-*.vsix")
				if err != nil {
					return nil, fmt.Errorf("failed to create temp file: %v", err)
				}
				_ = tmpFile.Close()

				// cleanup temp file on exit
				defer os.Remove(tmpFile.Name())

				// download the extension via the local pull through cache of the gigo system
				err = a.client.WorkspaceGetOpenVsxExtension(ctx, ext, "", vscVersion, tmpFile.Name())
				if err != nil {
					return nil, fmt.Errorf("failed to download extension: %v", err)
				}

				// assign extension path to extension variable
				extUrl := ext
				ext = tmpFile.Name()

				a.logger.Info(ctx, "extension downloaded and saved to temp file", slog.F("url", extUrl), slog.F("path", ext))
			}

			installMu.Lock()
			res, err := utils.ExecuteCommand(
				ctx, nil, "",
				"code-server", "--install-extension", ext,
			)
			installMu.Unlock()
			if err != nil {
				return nil, fmt.Errorf("failed to install extension %s: %v", ext, err)
			}

			// ensure good status code
			if res.ExitCode != 0 {
				errStr := strings.TrimSpace(res.Stderr)
				if errStr == "" {
					errStr = strings.TrimSpace(res.Stdout)
				}
				return res, fmt.Errorf("failed to install extension %s: %s", ext, errStr)
			}

			a.logger.Info(ctx, "extension installed", slog.F("extension", originalExt), slog.F("stdout", res.Stdout), slog.F("stderr", res.Stderr))

			return nil, nil
		})
	}

	// wait for all extension installs to complete
	allRes, err := wg.Wait()

	// check if any of the extension install commands failed
	for _, res := range allRes {
		if res != nil && res.ExitCode != 0 {
			return res, fmt.Errorf("failed to install extension")
		}
	}

	// handle any errors that may have occurred
	if err != nil {
		fmt.Println("extension install failed: ", err)
		return nil, err
	}

	return nil, nil
}

// runCodeServer
//
//	Launches a code-server instance on a controlled
//	retry within the agent's environment
func (a *agent) runCodeServer(ctx context.Context) (*utils.CommandResult, error) {
	// create variable to track failure count so we
	// can track time between fails to determine if
	// we are in a death loop
	failedCount := 0

	// run code-server on a retry loop
	for r := retry.New(time.Millisecond*250, time.Second*5); r.Wait(ctx); {
		a.logger.Info(ctx, "attempting code-server launch", slog.F("attempt", failedCount+1))

		// launch code-server inside agent environments
		res, err := a.executeCommandEnv(ctx, nil, "", "code-server --auth none --port 13337 -an 'Gigo Workspace' -w 'Welcome to Gigo!' --disable-telemetry --disable-workspace-trust")

		// TODO: update agent state

		// TODO: check whether the buffered logging is going to
		// be an issue with memory since this is a long running
		// process. if it is look into a streaming command and
		// a bounded buffer

		// this error will only fire if there is an
		// error on the go side of launching a command
		// so we just count the failures without regard
		// for runtime
		if err != nil {
			if failedCount >= 5 {
				return nil, fmt.Errorf("failed to launch code-server: %v", err)
			}
			failedCount++
			a.logger.Warn(ctx, "code-server failed", slog.Error(err), slog.F("fail-count", failedCount))
		}

		// technically we should never get here unless there
		// is a problem because code-server should not die
		// so we should go ahead and determine if this is a
		// fail loop
		if res.ExitCode != 0 {
			// if we have failed more than 5 times within 15 seconds
			// of starting code-server we are in a death loop and need
			// to bail
			if res.Cost <= 15*time.Second {
				failedCount++
			} else {
				// reset failed count if this is a longer
				// failure - since software isn't perfect
				// we can assume there will be the occasional
				// bug that causes code-server to die
				failedCount = 0
			}

			// determine if we are logging or exiting
			exiting := false
			if failedCount >= 5 {
				exiting = true
			}

			if exiting {
				return res, fmt.Errorf("failed to launch code-server")
			}
			a.logger.Warn(ctx, "code-server failed", slog.Error(err),
				slog.F("code", res.ExitCode), slog.F("stdout", res.Stdout),
				slog.F("stderr", res.Stderr), slog.F("fail-count", failedCount))
		}
	}

	// if we get here a real problem has occurred
	return nil, fmt.Errorf("unknown exit from code-server")
}

// initializeCodeServerSettings
//
// Creates the settings.json file based off of the current workspace agent metadata
func (a *agent) initializeCodeServerSettings(ctx context.Context, metadata agentsdk.WorkspaceAgentMetadata) {
	// select the theme dependent on if the user is pro or not
	theme := "Default Dark Modern"
	if metadata.UserStatus == models.UserStatusPremium {
		theme = "Gigo Pro Theme"
	}

	a.logger.Info(ctx, "holiday status", slog.F("theme_name: ", metadata.HolidaySeason.String()), slog.F("user_setting: ", metadata.UserHolidayTheme))
	if metadata.HolidaySeason > 0 && metadata.UserHolidayTheme {
		a.logger.Info(ctx, "setting holiday theme", slog.F("theme_name: ", metadata.HolidaySeason.String()))
		theme = metadata.HolidaySeason.String()
	}

	a.logger.Info(ctx, "theme selected", slog.F("theme_name: ", theme))

	// create the base config we intend to merge
	cfg := map[string]interface{}{
		"workbench.colorTheme": theme,
	}

	// detect if the vnc file exists to determine if we need to set the display
	_, err := os.Stat("/gigo/vnc")
	if err == nil {
		// hard set the display to use the vnc display
		metadata.GigoConfig.Environment["DISPLAY"] = ":90.0"
	}

	// set the display in the linux environment
	cfg["terminal.integrated.env.linux"] = metadata.GigoConfig.Environment

	// set the default launch theme to modern dark if it's not already set
	_, err = os.Stat("/home/gigo/.local/share/code-server/User/settings.json")
	// handle an existing file by loading it and merging our changes into it
	if err == nil {
		// read the existing file from the disk
		buf, err := os.ReadFile("/home/gigo/.local/share/code-server/User/settings.json")

		// unmarshal the existing data
		var settings map[string]interface{}
		err = json.Unmarshal([]byte(buf), &settings)
		if err != nil {
			a.logger.Warn(ctx, "failed to unmarshal settings.json", slog.Error(err))
		}

		// override the existing settings with our changes
		for k, v := range cfg {
			settings[k] = v
		}

		// remarshall the merged data
		b, err := json.MarshalIndent(&settings, "", "\t")
		if err != nil {
			a.logger.Warn(ctx, "failed to marshal settings.json", slog.Error(err))
		} else {
			// overwrite the existing file
			err := os.WriteFile(
				"/home/gigo/.local/share/code-server/User/settings.json", b, 0665,
			)
			if err != nil {
				a.logger.Warn(
					ctx, "failed to update settings.json file",
					slog.Error(err),
				)
			}
		}
	} else {
		// marshall the json
		b, err := json.MarshalIndent(&cfg, "", "\t")
		if err != nil {
			a.logger.Warn(ctx, "failed to marshal settings.json", slog.Error(err))
		} else {
			// write a new file to the settings
			err := os.WriteFile(
				"/home/gigo/.local/share/code-server/User/settings.json", b, 0665,
			)
			if err != nil {
				a.logger.Warn(
					ctx, "failed to create settings.json file",
					slog.Error(err),
				)
			}
		}
	}
}

// waitHealthyCodeServer
//
//	Waits on a retry loop pinging code-server for a healthy state
func (a *agent) waitHealthyCodeServer(ctx context.Context, liveChan chan bool, metadata agentsdk.WorkspaceAgentMetadata) {
	for r := retry.New(time.Millisecond*250, time.Second*5); r.Wait(ctx); {
		// check if we're still alive
		select {
		case <-ctx.Done():
			return
		default:
		}

		// make call to code-server url
		res, err := http.Get("http://localhost:13337/healthz")
		if err != nil {
			a.logger.Warn(ctx, "failed to check code-server health", slog.Error(err))
			continue
		}

		// check status
		if res.StatusCode != 200 {
			buf, _ := io.ReadAll(res.Body)
			a.logger.Warn(
				ctx, "code-server health check failed",
				slog.Error(err), slog.F("body", string(buf)),
			)
			continue
		}

		// initialize the code server settings for the gigo workspace
		a.initializeCodeServerSettings(ctx, metadata)

		// write to chan to trigger release of start wait
		liveChan <- true
		return
	}
}

// reportStateFailure
//
//	Helper function to handle formatting a failed
//	initialization state and relaying to the Gigo
//	core system
func (a *agent) reportStateFailure(ctx context.Context, state models.WorkspaceInitState, err error, cmdRes *utils.CommandResult) {
	// configure base failure request
	failure := agentsdk.PostWorkspaceInitFailure{
		State:  state,
		Status: -1,
	}

	// handle go based error
	if err != nil {
		failure.Stderr = err.Error()
	}

	// handle command error
	if cmdRes != nil {
		failure.Command = cmdRes.Command
		failure.Stdout = cmdRes.Stdout
		if cmdRes.Stderr != "" {
			failure.Stderr = cmdRes.Stderr
		}
		failure.Status = cmdRes.ExitCode
	}

	a.logger.Error(
		ctx, "failed to initialize state",
		slog.F("commandResponse", cmdRes), slog.Error(err),
		slog.F("state", state), slog.F("command", failure.Command),
		slog.F("status", failure.Status), slog.F("stderr", failure.Stderr),
	)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		a.logger.Error(ctx, "failed to get home dir for last init", slog.Error(err))
		// handle the error if getting the home directory fails
		return
	}

	data, err := json.Marshal(StateFromFile{LastInitState: int(state)})
	if err != nil {
		a.logger.Error(ctx, "failed to marshal state into json", slog.Error(err))
		// handle the error if getting the home directory fails
		return
	}

	err = os.WriteFile(filepath.Join(homeDir, ".last_init_state.json"), data, 0644)
	if err != nil {
		a.logger.Error(ctx, "failed to write last init to file", slog.Error(err))
		failure.Stderr = err.Error()
	}

	// execute api call to gigo system on a retry
	// with up to 3 attempts
	attempts := 0
	for r := retry.New(time.Second, time.Second*5); r.Wait(ctx); {
		err := a.client.WorkspaceInitializationFailure(ctx, failure)
		if err != nil {
			// exit if we have failed 3 or more times
			if attempts >= 2 {
				return
			}
			attempts++
			a.logger.Error(ctx, "failed to report init failure", slog.Error(err))
		}
		return
	}
}

// reportStateCompleted
//
//	Helper function for reporting init state completion
//	to the Gigo core system. It's basically just a wrapper
//	around retry logic to keep the runBootstrap function
//	clean
func (a *agent) reportStateCompleted(ctx context.Context, state models.WorkspaceInitState) {
	a.logger.Debug(ctx, "completed workspace init state", slog.F("state", state))

	homeDir, err := os.UserHomeDir()
	if err != nil {
		a.logger.Error(ctx, "failed to get home dir for last init", slog.Error(err))
		// handle the error if getting the home directory fails
		return
	}

	data, err := json.Marshal(StateFromFile{LastInitState: int(state)})
	if err != nil {
		a.logger.Error(ctx, "failed to marshal state into json", slog.Error(err))
		// handle the error if getting the home directory fails
		return
	}

	err = os.WriteFile(filepath.Join(homeDir, ".last_init_state.json"), data, 0644)
	if err != nil {
		a.logger.Error(ctx, "failed to write last init to file", slog.Error(err))
		return
	}
	a.logger.Debug(ctx, "wrote state to last state file", slog.F("state", state))

	// execute api call to gigo system on a retry
	// with up to 3 attempts
	attempts := 0
	for r := retry.New(time.Second, time.Second*5); r.Wait(ctx); {
		err := a.client.WorkspaceInitializationStepCompleted(ctx, state)
		if err != nil {
			// exit if we have failed 3 or more times
			if attempts >= 2 {
				return
			}
			attempts++
			a.logger.Error(ctx, "failed to report init state completed", slog.Error(err))
		}
		return
	}
}

// runBootstrap
//
//	Main routine for the agent that will bootstrap
//	the Gigo environment and either run code-server
//	or wait forever
func (a *agent) runBootstrap(ctx context.Context) error {
	// TODO: check if this is okay given that the
	// agent may restart in the middle of the bootstrap

	// get metadata for the function execution
	rawMetadata := a.metadata.Load()
	if rawMetadata == nil {
		return xerrors.Errorf("no metadata was provided")
	}
	metadata, valid := rawMetadata.(agentsdk.WorkspaceAgentMetadata)
	if !valid {
		return xerrors.Errorf("metadata is the wrong type: %T", metadata)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		a.logger.Error(ctx, "failed to get home dir for last init", slog.Error(err))
		a.reportStateFailure(ctx, models.WorkspaceInitRemoteInitialization, nil, nil)
		// handle the error if getting the home directory fails
		return err
	}

	_, err = os.Stat(filepath.Join(homeDir, ".last_init_state.json"))

	var file *os.File

	if err != nil {
		a.logger.Debug(ctx, "no last init state file found, creating one")
		file, err = os.Create(filepath.Join(homeDir, ".last_init_state.json"))
		if err != nil {
			a.logger.Error(ctx, "failed to create last init state file", slog.Error(err))
			a.reportStateFailure(ctx, models.WorkspaceInitRemoteInitialization, nil, nil)
			return err
		}

		defer file.Close()

		metadata.LastInitState = -1
	} else {

		file, err = os.Open(filepath.Join(homeDir, ".last_init_state.json"))
		if err != nil {
			a.logger.Error(ctx, "failed to open last init state file", slog.Error(err))
			a.reportStateFailure(ctx, models.WorkspaceInitRemoteInitialization, nil, nil)
			return err
		}

		defer file.Close()

		a.logger.Debug(ctx, "init state from memory: ", slog.F("state", metadata.LastInitState))
		var data StateFromFile

		decoder := json.NewDecoder(file)
		err = decoder.Decode(&data)
		if err != nil {
			a.logger.Error(ctx, "failed to json marshal last init state file", slog.Error(err))
			a.reportStateFailure(ctx, models.WorkspaceInitRemoteInitialization, nil, nil)
			return err
		}

		a.logger.Debug(ctx, "last init state from file: ", slog.F("", data.LastInitState))

		metadata.LastInitState = models.WorkspaceInitState(data.LastInitState)
		// }

	}

	a.logger.Debug(ctx, "last init state read from file: ", slog.F("state", metadata.LastInitState))

	a.reportStateCompleted(ctx, models.WorkspaceInitRemoteInitialization)

	// perform workspace initialization if this
	// is the workspace's first execution
	if metadata.LastInitState <= models.WorkspaceInitWriteGitConfig && metadata.ChallengeType != models.BytesChallenge {
		err := writeGitConfig(ctx, metadata)
		if err != nil {
			a.reportStateFailure(ctx, models.WorkspaceInitWriteGitConfig, err, nil)
			return err
		}
		a.reportStateCompleted(ctx, models.WorkspaceInitWriteGitConfig)
	}

	err = writeWorkspaceConfig(ctx, metadata, a.client.SessionAuth().Token)
	if err != nil {
		a.reportStateFailure(ctx, models.WorkspaceInitWriteWorkspaceConfig, err, nil)
		return err
	}
	a.reportStateCompleted(ctx, models.WorkspaceInitWriteWorkspaceConfig)

	if metadata.LastInitState <= models.WorkspaceInitGitClone && metadata.ChallengeType != models.BytesChallenge {
		res, err := cloneRepo(ctx, metadata)
		if err != nil || (res != nil && res.ExitCode != 0) {
			a.reportStateFailure(ctx, models.WorkspaceInitGitClone, err, res)
			return err
		}
		a.reportStateCompleted(ctx, models.WorkspaceInitGitClone)
	}

	// perform bootstrap steps that occur every start
	if len(metadata.GigoConfig.Containers) > 0 {
		err = writeContainerCompose(ctx, metadata)
		if err != nil {
			a.reportStateFailure(ctx, models.WorkspaceInitWriteContainerCompose, err, nil)
			return err
		}
		a.reportStateCompleted(ctx, models.WorkspaceInitWriteContainerCompose)

		res, err := containerComposeUp(ctx, metadata)
		if err != nil || (res != nil && res.ExitCode != 0) {
			a.reportStateFailure(ctx, models.WorkspaceInitContainerComposeUp, err, res)
			return err
		}
		a.reportStateCompleted(ctx, models.WorkspaceInitContainerComposeUp)
	}

	if metadata.GigoConfig.VSCode.Enabled {
		res, err := a.installCodeServer(ctx, metadata)
		if err != nil || (res != nil && res.ExitCode != 0) {
			a.reportStateFailure(ctx, models.WorkspaceInitVSCodeInstall, err, res)
			return err
		}
		a.reportStateCompleted(ctx, models.WorkspaceInitVSCodeInstall)

		res, err = a.installCodeServerExtensions(ctx, metadata)
		if err != nil || (res != nil && res.ExitCode != 0) {
			a.reportStateFailure(ctx, models.WorkspaceInitVSCodeExtensionInstall, err, res)
			return err
		}
		a.reportStateCompleted(ctx, models.WorkspaceInitVSCodeExtensionInstall)
	}

	res, err := handleUserExecutions(ctx, metadata, metadata.LastInitState)
	if err != nil || (res != nil && res.ExitCode != 0) {
		a.reportStateFailure(ctx, models.WorkspaceInitShellExecutions, err, res)
		return err
	}
	a.reportStateCompleted(ctx, models.WorkspaceInitShellExecutions)

	file.Seek(0, 0)

	return nil
}

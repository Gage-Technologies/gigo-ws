package utils

// Copied from Coder @ https://github.com/coder/coder
// Modified by Gage Technologies @ https://github.com/gage-technologies

import (
	_ "embed"
	"fmt"
	"strings"
)

var (
	// These used to be hard-coded, but after growing significantly more complex
	// it made sense to put them in their own files (e.g. for linting).
	//go:embed resources/scripts/bootstrap_windows.ps1
	windowsScript string
	//go:embed resources/scripts/bootstrap_linux.sh
	linuxScript string
	//go:embed resources/scripts/bootstrap_darwin.sh
	darwinScript string

	// A mapping of operating-system ($GOOS) to architecture ($GOARCH)
	// to agent install and run script. ${DOWNLOAD_URL} is replaced
	// with strings.ReplaceAll() when being consumed. ${ARCH} is replaced
	// with the architecture when being provided.
	agentScripts = map[string]map[string]string{
		"windows": {
			"amd64": windowsScript,
			"arm64": windowsScript,
		},
		"linux": {
			"amd64": linuxScript,
			"arm64": linuxScript,
			"armv7": linuxScript,
		},
		"darwin": {
			"amd64": darwinScript,
			"arm64": darwinScript,
		},
	}
)

// AgentScriptEnv returns a key-pair of scripts that are consumed
// by the gigo Terraform Provider. See:
// https://github.com/Gage-Technologies/gigo-terraform-provider/blob/18af01b7a0cfdd1df1242b331f8b29ffef7051ff/provider/agent.go#L100
func AgentScriptEnv() []string {
	env := make([]string, 0)
	for operatingSystem, scripts := range agentScripts {
		for architecture, script := range scripts {
			script := strings.ReplaceAll(script, "${ARCH}", architecture)
			env = append(env, fmt.Sprintf("GIGO_AGENT_SCRIPT_%s_%s=%s", operatingSystem, architecture, script))
		}
	}
	return env
}

package lsp

import (
	"context"
	"fmt"
	"gigo-ws/utils"
	"github.com/gage-technologies/gigo-lib/coder/agentsdk"
	"github.com/gage-technologies/gigo-lib/db/models"
	utils2 "github.com/gage-technologies/gigo-lib/utils"
	"os"
	"strings"
)

var (
	ErrLangNotSupported = fmt.Errorf("language not supported")
)

const pythonLspScript = `#!/bin/bash
eval "$(/opt/conda/miniconda/bin/conda shell.bash hook)" &> /dev/null
conda activate /opt/python-bytes/default &> /dev/null
python -u -m pylsp --ws --port $PORT
`

func launchPythonLsp(ctx context.Context, stdout, stderr chan string) (*utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/tmp/pyrun"); !ok {
		err := os.MkdirAll("/tmp/pyrun", 0755)
		if err != nil {
			return nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	return utils.ExecuteCommandStream(ctx, nil, "/tmp/pyrun", stdout, stderr,
		"bash", "-c", strings.ReplaceAll(pythonLspScript, "$PORT", fmt.Sprint(agentsdk.ZitiAgentLspWsPort)))
}

func launchGolangLsp(ctx context.Context, stdout, stderr chan string) (*utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/tmp/gorun"); !ok {
		err := os.MkdirAll("/tmp/gorun", 0755)
		if err != nil {
			return nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	return utils.ExecuteCommandStream(ctx, nil, "/tmp/gorun", stdout, stderr,
		"lsp-ws-proxy", "--listen", fmt.Sprintf("%d", agentsdk.ZitiAgentLspWsPort), "--", "gopls")
}

func launchLsp(lang models.ProgrammingLanguage, ctx context.Context, stdout, stderr chan string) (*utils.CommandResult, error) {
	switch lang {
	case models.Python:
		return launchPythonLsp(ctx, stdout, stderr)
	case models.Go:
		return launchGolangLsp(ctx, stdout, stderr)
	default:
		return nil, ErrLangNotSupported
	}
}

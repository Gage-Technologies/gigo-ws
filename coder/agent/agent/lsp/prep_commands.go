package lsp

import (
	"context"
	"fmt"
	"gigo-ws/utils"
	"github.com/gage-technologies/gigo-lib/db/models"
	utils2 "github.com/gage-technologies/gigo-lib/utils"
	"os"
)

const pythonLspPrepScript = `#!/bin/bash
eval "$(/opt/conda/miniconda/bin/conda shell.bash hook)" &> /dev/null
conda activate /opt/python-bytes/default &> /dev/null
pipreqs --force . &> /dev/null
pip install -r requirements.txt &> /dev/null
`

func prepPythonLsp(ctx context.Context, fileContent string) error {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/tmp/pyrun"); !ok {
		err := os.MkdirAll("/tmp/pyrun", 0755)
		if err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// write the python file
	err := os.WriteFile("/tmp/pyrun/main.py", []byte(fileContent), 0755)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	_, err = utils.ExecuteCommand(ctx, nil, "/tmp/pyrun", "bash", "-c", pythonLspPrepScript)
	return err
}

func prepGolangLsp(ctx context.Context, fileContent string) error {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/tmp/gorun"); !ok {
		err := os.MkdirAll("/tmp/gorun", 0755)
		if err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// write the python file
	err := os.WriteFile("/tmp/gorun/main.go", []byte(fileContent), 0755)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	// conditionally initialize the go module
	if ok, _ := utils2.PathExists("/tmp/gorun/go.mod"); !ok {
		_, _ = utils.ExecuteCommand(ctx, nil, "/tmp/gorun", "go", "mod", "init", "gigo-byte")
	}

	// install deps
	_, err = utils.ExecuteCommand(ctx, nil, "/tmp/gorun", "go", "mod", "tidy")
	return err
}

func PrepLsp(lang models.ProgrammingLanguage, ctx context.Context, fileContent string) error {
	switch lang {
	case models.Python:
		return prepPythonLsp(ctx, fileContent)
	case models.Go:
		return prepGolangLsp(ctx, fileContent)
	default:
		return ErrLangNotSupported
	}
}

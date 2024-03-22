package lsp

import (
	"context"
	"fmt"
	"gigo-ws/utils"
	"github.com/gage-technologies/gigo-lib/db/models"
	utils2 "github.com/gage-technologies/gigo-lib/utils"
	"os"
	"path/filepath"
)

const pythonLspPrepScript = `#!/bin/bash
eval "$(/opt/conda/miniconda/bin/conda shell.bash hook)" &> /dev/null
conda activate /opt/python-bytes/default &> /dev/null
pipreqs --force . &> /dev/null
pip install -r requirements.txt &> /dev/null
`

func prepFile(basepath string, fileContent string, fileName string) error {
	// form the full path
	fullPath := filepath.Join(basepath, fileName)

	// ensure the parent directory exists
	if ok, _ := utils2.PathExists(filepath.Dir(fullPath)); !ok {
		os.MkdirAll(filepath.Dir(fullPath), 0755)
	}

	// write the python file
	err := os.WriteFile(fullPath, []byte(fileContent), 0755)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func prepPythonLsp(ctx context.Context, fileContent string, fileName string) error {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/home/gigo/.gigo/agent-exec/pyrun"); !ok {
		err := os.MkdirAll("/home/gigo/.gigo/agent-exec/pyrun", 0755)
		if err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// write the python file
	if fileName == "" {
		fileName = "main.py"
	}
	err := prepFile("/home/gigo/.gigo/agent-exec/pyrun", fileContent, fileName)

	_, err = utils.ExecuteCommand(ctx, nil, "/home/gigo/.gigo/agent-exec/pyrun", "bash", "-c", pythonLspPrepScript)
	return err
}

func prepGolangLsp(ctx context.Context, fileContent string, fileName string) error {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/home/gigo/.gigo/agent-exec/gorun"); !ok {
		err := os.MkdirAll("/home/gigo/.gigo/agent-exec/gorun", 0755)
		if err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// write the go file
	if fileName == "" {
		fileName = "main.go"
	}
	err := prepFile("/home/gigo/.gigo/agent-exec/gorun", fileContent, fileName)
	if err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// conditionally initialize the go module
	if ok, _ := utils2.PathExists("/home/gigo/.gigo/agent-exec/gorun/go.mod"); !ok {
		_, _ = utils.ExecuteCommand(ctx, nil, "/home/gigo/.gigo/agent-exec/gorun", "go", "mod", "init", "gigo-byte")
	}

	// install deps
	_, err = utils.ExecuteCommand(ctx, nil, "/home/gigo/.gigo/agent-exec/gorun", "go", "mod", "tidy")
	return err
}

func PrepLsp(lang models.ProgrammingLanguage, ctx context.Context, fileContent string, fileName string) error {
	switch lang {
	case models.Python:
		return prepPythonLsp(ctx, fileContent, fileName)
	case models.Go:
		return prepGolangLsp(ctx, fileContent, fileName)
	default:
		return ErrLangNotSupported
	}
}

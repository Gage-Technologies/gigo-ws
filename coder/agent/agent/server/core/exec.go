package core

import (
	"context"
	"fmt"
	"gigo-ws/coder/agent/agent/server/payload"
	"gigo-ws/coder/agent/agent/server/types"
	"gigo-ws/utils"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cdr.dev/slog"
	"github.com/buger/jsonparser"
	"github.com/gage-technologies/gigo-lib/db/models"
	utils2 "github.com/gage-technologies/gigo-lib/utils"
)

const pythonPrepScript = `#!/bin/bash
eval "$(/opt/conda/miniconda/bin/conda shell.bash hook)" &> /dev/null
conda activate /opt/python-bytes/default &> /dev/null
pipreqs --force . &> /dev/null
pip install -r requirements.txt &> /dev/null
`

const jsPrepScript = `#!/bin/bash
npm init -y
`

const cppPrepScript = `#!/bin/bash
g++ -o main main.cpp
chmod +x main
`

const tsPrepScript = `#!/bin/bash
npm init -y
tsc main.ts
`

const rustPrepScript = `#!/bin/bash
cargo build
`

const csPrePrepScript = `#!/bin/bash
dotnet new console --name csrun
mv csrun/Program.cs csrun/main.cs
`

type ActiveCommand struct {
	Ctx          context.Context
	Cancel       context.CancelFunc
	Stdin        io.Writer
	ResponseChan chan payload.ExecResponsePayload
}

func writeFiles(basepath string, files []types.ExecFiles) (*types.ExecFiles, error) {
	executeFile := new(types.ExecFiles)

	for _, k := range files {
		// copy the k value to fix the stupid go iteration bug
		k := k

		// form the full path
		fullPath := filepath.Join(basepath, k.FileName)

		// ensure the parent directory exists
		if ok, _ := utils2.PathExists(filepath.Dir(fullPath)); !ok {
			os.MkdirAll(filepath.Dir(fullPath), 0755)
		}

		// write the python file
		err := os.WriteFile(fullPath, []byte(k.Code), 0755)
		if err != nil {
			return nil, fmt.Errorf("failed to write file: %w", err)
		}

		if k.Execute {
			executeFile = &k
		}
	}

	if executeFile == nil {
		return &types.ExecFiles{}, nil
	}

	return executeFile, nil
}

func prepExecCommand(execCommand string, execFileName string) (string, []string) {
	// replace the filename placeholder
	preppedCommand := strings.ReplaceAll(execCommand, "{{filename}}", execFileName)

	// split the command by spaces and return it as an array
	parts := strings.Split(preppedCommand, " ")
	if len(parts) == 1 {
		return parts[0], []string{}
	}
	return parts[0], parts[1:]
}

func execPythonFiles(ctx context.Context, stdout chan string, stderr chan string, files []types.ExecFiles, execCommand string, logger slog.Logger) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/home/gigo/.gigo/agent-exec/pyrun"); ok {
		err := os.RemoveAll("/home/gigo/.gigo/agent-exec/pyrun")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to remove directory: %w", err)
		}
	}

	err := os.MkdirAll("/home/gigo/.gigo/agent-exec/pyrun", 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create directory: %w", err)
	}

	executeFile, err := writeFiles("/home/gigo/.gigo/agent-exec/pyrun", files)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	// execute python code
	_, _ = utils.ExecuteCommand(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/pyrun", "bash", "-c", pythonPrepScript)

	if execCommand != "" {
		binary, args := prepExecCommand(execCommand, executeFile.FileName)
		logger.Info(ctx, "executing remote command", slog.F("binary", binary), slog.F("args", args))
		return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/pyrun", stdout,
			stderr, true, binary, args...)
	}
	logger.Info(ctx, "executing default run command", slog.F("binary", "/opt/python-bytes/default/bin/python"), slog.F("args", []string{"-u", fmt.Sprintf("%v", executeFile.FileName)}))
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/pyrun", stdout,
		stderr, true, "/opt/python-bytes/default/bin/python", "-u", fmt.Sprintf("%v", executeFile.FileName))
}

func execJavascriptFiles(ctx context.Context, stdout chan string, stderr chan string, files []types.ExecFiles, execCommand string, logger slog.Logger) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/home/gigo/.gigo/agent-exec/jsrun"); ok {
		err := os.RemoveAll("/home/gigo/.gigo/agent-exec/jsrun")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to remove directory: %w", err)
		}
	}

	err := os.MkdirAll("/home/gigo/.gigo/agent-exec/jsrun", 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create directory: %w", err)
	}

	executeFile, err := writeFiles("/home/gigo/.gigo/agent-exec/jsrun", files)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	// execute js code
	if ok, _ := utils2.PathExists("/home/gigo/.gigo/agent-exec/jsrun/package.json"); !ok {
		_, _ = utils.ExecuteCommand(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/jsrun", "bash", "-c", jsPrepScript)
	}

	_, _ = utils.ExecuteCommandStream(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/jsrun", stdout, stderr, true, "yarn", "install")

	if execCommand != "" {
		binary, args := prepExecCommand(execCommand, executeFile.FileName)
		logger.Info(ctx, "executing remote command", slog.F("binary", binary), slog.F("args", args))
		return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/jsrun", stdout,
			stderr, true, binary, args...)
	}

	// parse the package.json to determine if we have a start command
	packageBuf, err := os.ReadFile("/home/gigo/.gigo/agent-exec/jsrun/package.json")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read file: %w", err)
	}
	startCmd, _ := jsonparser.GetString(packageBuf, "scripts", "start")
	if startCmd != "" {
		logger.Info(ctx, "executing default run command", slog.F("binary", "yarn"), slog.F("args", []string{"start"}))
		// utilize the start script if we have it
		return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/jsrun", stdout,
			stderr, true, "yarn", "start")
	} else {
		logger.Info(ctx, "executing default run command", slog.F("binary", "node"), slog.F("args", []string{fmt.Sprintf("%v", executeFile.FileName)}))
		// run the current file if we don't have a start script
		return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/jsrun", stdout,
			stderr, true, "node", fmt.Sprintf("%v", executeFile.FileName))
	}
}

func execCppFiles(ctx context.Context, stdout chan string, stderr chan string, files []types.ExecFiles, execCommand string, logger slog.Logger) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/home/gigo/.gigo/agent-exec/cpprun"); ok {
		err := os.RemoveAll("/home/gigo/.gigo/agent-exec/cpprun")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to remove directory: %w", err)
		}
	}

	err := os.MkdirAll("/home/gigo/.gigo/agent-exec/cpprun", 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create directory: %w", err)
	}

	executeFile, err := writeFiles("/home/gigo/.gigo/agent-exec/cpprun", files)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	if execCommand != "" {
		binary, args := prepExecCommand(execCommand, executeFile.FileName)
		logger.Info(ctx, "executing remote command", slog.F("binary", binary), slog.F("args", args))
		return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/cpprun", stdout,
			stderr, true, binary, args...)
	}

	// execute js code
	_, _ = utils.ExecuteCommandStream(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/cpprun", stdout, stderr, true,
		"bash", "-c", fmt.Sprintf(`#!/bin/bash
		g++ -o main %v
		chmod +x main
		`, executeFile.FileName),
	)
	logger.Info(ctx, "executing default run command", slog.F("binary", "./main"), slog.F("args", []string{}))
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/cpprun", stdout,
		stderr, true, "./main")
}

func execTypescriptFiles(ctx context.Context, stdout chan string, stderr chan string, files []types.ExecFiles, execCommand string, logger slog.Logger) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/home/gigo/.gigo/agent-exec/tsrun"); ok {
		err := os.RemoveAll("/home/gigo/.gigo/agent-exec/tsrun")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to remove directory: %w", err)
		}
	}

	err := os.MkdirAll("/home/gigo/.gigo/agent-exec/tsrun", 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create directory: %w", err)
	}

	executeFile, err := writeFiles("/home/gigo/.gigo/agent-exec/tsrun", files)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	if execCommand != "" {
		binary, args := prepExecCommand(execCommand, executeFile.FileName)
		logger.Info(ctx, "executing remote command", slog.F("binary", binary), slog.F("args", args))
		return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/tsrun", stdout,
			stderr, true, binary, args...)
	}

	// execute js code
	_, _ = utils.ExecuteCommand(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/tsrun", "bash", "-c", fmt.Sprintf(`#!/bin/bash
		npm init -y
		tsc %v
		`, executeFile.FileName),
	)
	extension := filepath.Ext(executeFile.FileName)
	parsedName := strings.TrimSuffix(executeFile.FileName, extension)
	logger.Info(ctx, "executing default run command", slog.F("binary", "node"), slog.F("args", []string{fmt.Sprintf("%v.js", parsedName)}))
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/tsrun", stdout,
		stderr, true, "node", fmt.Sprintf("%v.js", parsedName))
}

func execRustFiles(ctx context.Context, stdout chan string, stderr chan string, files []types.ExecFiles, execCommand string, logger slog.Logger) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/home/gigo/.gigo/agent-exec/rsrun"); ok {
		err := os.RemoveAll("/home/gigo/.gigo/agent-exec/rsrun")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to remove directory: %w", err)
		}
	}

	err := os.MkdirAll("/home/gigo/.gigo/agent-exec", 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create directory: %w", err)
	}

	if ok, _ := utils2.PathExists("/home/gigo/.gigo/agent-exec/rsrun/Cargo.toml"); !ok {
		_, err := utils.ExecuteCommand(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec", "bash", "-c", `#!/bin/bash
			cargo new rsrun
			rm /home/gigo/.gigo/agent-exec/rsrun/src/main.rs
		`,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	executeFile, err := writeFiles("/home/gigo/.gigo/agent-exec/rsrun", files)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	if execCommand != "" {
		binary, args := prepExecCommand(execCommand, executeFile.FileName)
		logger.Info(ctx, "executing remote command", slog.F("binary", binary), slog.F("args", args))
		return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/rsrun", stdout,
			stderr, true, binary, args...)
	}

	logger.Info(ctx, "executing default run command", slog.F("binary", "cargo"), slog.F("args", []string{"run"}))
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/rsrun", stdout,
		stderr, true, "cargo", "run")
}

func execCSharpFiles(ctx context.Context, stdout chan string, stderr chan string, files []types.ExecFiles, execCommand string, logger slog.Logger) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/home/gigo/.gigo/agent-exec/csrun"); ok {
		err := os.RemoveAll("/home/gigo/.gigo/agent-exec/csrun")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to remove directory: %w", err)
		}
	}

	err := os.MkdirAll("/home/gigo/.gigo/agent-exec", 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create directory: %w", err)
	}

	filesHasProj := false
	for _, file := range files {
		if strings.HasSuffix(file.FileName, ".csproj") {
			filesHasProj = true
			break
		}
	}

	_, err = utils.ExecuteCommand(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec", "bash", "-c", `#!/bin/bash
		dotnet new console --name csrun
		rm /home/gigo/.gigo/agent-exec/csrun/Program.cs
	`,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to init directory: %w", err)
	}

	if filesHasProj {
		os.Remove("/home/gigo/.gigo/agent-exec/csrun/csrun.csproj")
	}

	executeFile, err := writeFiles("/home/gigo/.gigo/agent-exec/csrun", files)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	if execCommand != "" {
		binary, args := prepExecCommand(execCommand, executeFile.FileName)
		logger.Info(ctx, "executing remote command", slog.F("binary", binary), slog.F("args", args))
		return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/csrun", stdout,
			stderr, true, binary, args...)
	}

	logger.Info(ctx, "executing default run command", slog.F("binary", "dotnet"), slog.F("args", []string{"run"}))
	// execute js code
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/csrun", stdout,
		stderr, true, "dotnet", "run")
}

func execJavaFiles(ctx context.Context, stdout chan string, stderr chan string, files []types.ExecFiles, execCommand string, logger slog.Logger) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/home/gigo/.gigo/agent-exec/jrun"); ok {
		err := os.RemoveAll("/home/gigo/.gigo/agent-exec/jrun")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to remove directory: %w", err)
		}
	}

	err := os.MkdirAll("/home/gigo/.gigo/agent-exec/jrun", 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create directory: %w", err)
	}

	executeFile, err := writeFiles("/home/gigo/.gigo/agent-exec/jrun", files)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	if execCommand != "" {
		binary, args := prepExecCommand(execCommand, executeFile.FileName)
		logger.Info(ctx, "executing remote command", slog.F("binary", binary), slog.F("args", args))
		return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/jrun", stdout,
			stderr, true, binary, args...)
	}

	_, _ = utils.ExecuteCommand(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/jrun", "bash", "-c", fmt.Sprintf(`#!/bin/bash
		javac %v.java
	`, executeFile.FileName))

	extension := filepath.Ext(executeFile.FileName)
	parsedName := strings.TrimSuffix(executeFile.FileName, extension)

	logger.Info(ctx, "executing default run command", slog.F("binary", "java"), slog.F("args", []string{fmt.Sprintf("%v", parsedName)}))
	// execute js code
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/jrun", stdout,
		stderr, true, "java", fmt.Sprintf("%v", parsedName))
}

func execGolangFiles(ctx context.Context, stdout chan string, stderr chan string, files []types.ExecFiles, execCommand string, logger slog.Logger) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/home/gigo/.gigo/agent-exec/gorun"); ok {
		err := os.RemoveAll("/home/gigo/.gigo/agent-exec/gorun")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to remove directory: %w", err)
		}
	}

	err := os.MkdirAll("/home/gigo/.gigo/agent-exec/gorun", 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create directory: %w", err)
	}

	executeFile, err := writeFiles("/home/gigo/.gigo/agent-exec/gorun", files)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	// conditionally initialize the go module
	if ok, _ := utils2.PathExists("/home/gigo/.gigo/agent-exec/gorun/go.mod"); !ok {
		_, _ = utils.ExecuteCommand(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/gorun", "go", "mod", "init", "gigo-byte")
	}

	if execCommand != "" {
		binary, args := prepExecCommand(execCommand, executeFile.FileName)
		logger.Info(ctx, "executing remote command", slog.F("binary", binary), slog.F("args", args))
		return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/gorun", stdout,
			stderr, true, binary, args...)
	}

	// execute go code
	_, _ = utils.ExecuteCommandStream(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/gorun", stdout, stderr, true, "go", "mod", "tidy")
	logger.Info(ctx, "executing default run command", slog.F("binary", "go"), slog.F("args", []string{"run", executeFile.FileName}))
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/gorun", stdout,
		stderr, true, "go", "run", executeFile.FileName)
}

// ///////////////////////////////////

func execPython(ctx context.Context, code string, stdout chan string, stderr chan string) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/home/gigo/.gigo/agent-exec/pyrun"); !ok {
		err := os.MkdirAll("/home/gigo/.gigo/agent-exec/pyrun", 0755)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// write the python file
	err := os.WriteFile("/home/gigo/.gigo/agent-exec/pyrun/main.py", []byte(code), 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	// execute python code
	_, _ = utils.ExecuteCommand(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/pyrun", "bash", "-c", pythonPrepScript)
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/pyrun", stdout,
		stderr, true, "/opt/python-bytes/default/bin/python", "-u", "main.py")
}

func execJavascript(ctx context.Context, code string, stdout chan string, stderr chan string) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/home/gigo/.gigo/agent-exec/jsrun"); !ok {
		err := os.MkdirAll("/home/gigo/.gigo/agent-exec/jsrun", 0755)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// write the js file
	err := os.WriteFile("/home/gigo/.gigo/agent-exec/jsrun/index.js", []byte(code), 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	// execute js code
	_, _ = utils.ExecuteCommand(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/jsrun", "bash", "-c", jsPrepScript)
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/jsrun", stdout,
		stderr, true, "node", "index.js")
}

func execCpp(ctx context.Context, code string, stdout chan string, stderr chan string) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/home/gigo/.gigo/agent-exec/cpprun"); !ok {
		err := os.MkdirAll("/home/gigo/.gigo/agent-exec/cpprun", 0755)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// write the js file
	err := os.WriteFile("/home/gigo/.gigo/agent-exec/cpprun/main.cpp", []byte(code), 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	// execute js code
	_, _ = utils.ExecuteCommandStream(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/cpprun", stdout, stderr, true, "bash", "-c", cppPrepScript)
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/cpprun", stdout,
		stderr, true, "./main")
}

func execTypescript(ctx context.Context, code string, stdout chan string, stderr chan string) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/home/gigo/.gigo/agent-exec/tsrun"); !ok {
		err := os.MkdirAll("/home/gigo/.gigo/agent-exec/tsrun", 0755)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// write the js file
	err := os.WriteFile("/home/gigo/.gigo/agent-exec/tsrun/main.ts", []byte(code), 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	// execute js code
	_, _ = utils.ExecuteCommand(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/tsrun", "bash", "-c", tsPrepScript)
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/tsrun", stdout,
		stderr, true, "node", "main.js")
}

func execRust(ctx context.Context, code string, stdout chan string, stderr chan string) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/home/gigo/.gigo/agent-exec/rsrun"); !ok {
		_, err := utils.ExecuteCommand(ctx, os.Environ(), "/tmp", "cargo", "new", "rsrun")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// write the js file
	err := os.WriteFile("/home/gigo/.gigo/agent-exec/rsrun/src/main.rs", []byte(code), 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	// execute js code
	// _, _ = utils.ExecuteCommand(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/rsrun", "bash", "-c", rustPrepScript)
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/rsrun", stdout,
		stderr, true, "cargo", "run")
}

func execCSharp(ctx context.Context, code string, stdout chan string, stderr chan string) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/home/gigo/.gigo/agent-exec/csrun"); !ok {
		_, err := utils.ExecuteCommand(ctx, os.Environ(), "/tmp", "bash", "-c", csPrePrepScript)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}
	// write the js file
	err := os.WriteFile("/home/gigo/.gigo/agent-exec/csrun/main.cs", []byte(code), 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	// execute js code
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/csrun", stdout,
		stderr, true, "dotnet", "run")
}

func execJava(ctx context.Context, code string, stdout chan string, stderr chan string, filename *string) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/home/gigo/.gigo/agent-exec/jrun"); !ok {
		err := os.MkdirAll("/home/gigo/.gigo/agent-exec/jrun", 0755)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	var nameStr string = "main"

	if filename != nil {
		nameStr = *filename
	}
	// write the js file
	err := os.WriteFile(fmt.Sprintf("/home/gigo/.gigo/agent-exec/jrun/%v.java", nameStr), []byte(code), 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	_, _ = utils.ExecuteCommand(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/jrun", "bash", "-c", fmt.Sprintf(`#!/bin/bash
		javac %v.java
	`, nameStr))

	// execute js code
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/jrun", stdout,
		stderr, true, "java", fmt.Sprintf("%v", nameStr))
}

func execGolang(ctx context.Context, code string, stdout chan string, stderr chan string) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/home/gigo/.gigo/agent-exec/gorun"); !ok {
		err := os.MkdirAll("/home/gigo/.gigo/agent-exec/gorun", 0755)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// write the python file
	err := os.WriteFile("/home/gigo/.gigo/agent-exec/gorun/main.go", []byte(code), 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	// conditionally initialize the go module
	if ok, _ := utils2.PathExists("/home/gigo/.gigo/agent-exec/gorun/go.mod"); !ok {
		_, _ = utils.ExecuteCommand(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/gorun", "go", "mod", "init", "gigo-byte")
	}

	// execute go code
	_, _ = utils.ExecuteCommand(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/gorun", "go", "mod", "tidy")
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/home/gigo/.gigo/agent-exec/gorun", stdout,
		stderr, true, "go", "run", "main.go")
}

func execBash(ctx context.Context, code string, stdout chan string, stderr chan string, workingDir *string) (io.WriteCloser, <-chan *utils.CommandResult, func(), error) {
	var dir string

	var cleanupFunc func()
	if workingDir == nil {
		// Create a temporary directory
		tempDir, err := ioutil.TempDir("/tmp", "bash")
		if err != nil {
			fmt.Println("Failed to create temporary directory:", err)
			return nil, nil, nil, fmt.Errorf("failed to create temp directory at: %v, err: %w", "/tmp/bash", err)
		}

		dir = tempDir

		// Defer the removal of the temporary directory
		cleanupFunc = func() {
			os.RemoveAll(tempDir)
		}
	} else {
		dir = *workingDir
	}

	stdin, resultsChan, err := utils.ExecuteCommandStreamStdin(ctx, os.Environ(), dir, stdout,
		stderr, true, "bash", "-c", code)
	return stdin, resultsChan, cleanupFunc, err
}

// updateOutput updates the output slice with new data.
// It either appends a new line or updates the last partial line.
func updateOutput(output *[]payload.OutputRow, lastLineIndex **int, newData string) {
	if newData == "" {
		// No new data to process
		return
	}

	// Function to append new data as a new line
	appendNewLine := func(data string) {
		*output = append(*output, payload.OutputRow{
			Content:   data,
			Timestamp: time.Now().UnixNano(),
		})
		lastIndex := len(*output) - 1
		*lastLineIndex = &lastIndex
	}

	// Process each character in newData
	for i := 0; i < len(newData); i++ {
		switch newData[i] {
		case '\r':
			// Carriage return - reset the current line or start a new line
			if *lastLineIndex != nil {
				(*output)[**lastLineIndex].Content = ""
			} else {
				appendNewLine("")
			}

			// Check if the next character is a newline
			if i+1 < len(newData) && newData[i+1] == '\n' {
				i++                  // Skip the newline character
				*lastLineIndex = nil // Start a new line after the newline
			}
		case '\n':
			// Newline character - start a new line
			*lastLineIndex = nil
		default:
			// Regular character - add to the current line or start a new one
			if *lastLineIndex != nil {
				// Append to the existing line
				(*output)[**lastLineIndex].Content += string(newData[i])
			} else {
				// Start a new line
				appendNewLine(string(newData[i]))
			}
		}
	}
}

func ExecCode(ctx context.Context, codeString string, language models.ProgrammingLanguage, execCommand string,
	fileName *string, files []types.ExecFiles, logger slog.Logger) (*ActiveCommand, error) {
	payloadChan := make(chan payload.ExecResponsePayload, 100)
	stdOut := make(chan string, 1024*1024*10 /* 10MB */)
	stdErr := make(chan string, 1024*1024*10 /* 10MB */)

	// default payload
	res := payload.ExecResponsePayload{
		StdOut:     make([]payload.OutputRow, 0),
		StdErr:     make([]payload.OutputRow, 0),
		StatusCode: -1,
		Done:       false,
	}

	// create a new command context with a cancel
	commandCtx, commandCancel := context.WithCancel(context.Background())

	var stdin io.WriteCloser
	var completionChan <-chan *utils.CommandResult
	var cleanupFunc func()
	var err error

	// execute cleanup function on exit if it is set
	defer func() {
		if completionChan == nil {
			commandCancel()
			if cleanupFunc != nil {
				cleanupFunc()
			}
		}
	}()

	if codeString == "" {
		// ignore empty filenames
		cleanedFiles := make([]types.ExecFiles, 0, len(files))
		if len(files) > 0 {
			for _, f := range files {
				if f.FileName == "" {
					continue
				}
				cleanedFiles = append(cleanedFiles, f)
			}
		}
		files = cleanedFiles

		switch language {
		case models.Python:
			stdin, completionChan, err = execPythonFiles(commandCtx, stdOut, stdErr, files, execCommand, logger)
			if err != nil {
				return nil, fmt.Errorf("failed to exec python files: %v", err)
			}
		case models.Go:
			stdin, completionChan, err = execGolangFiles(commandCtx, stdOut, stdErr, files, execCommand, logger)
			if err != nil {
				return nil, fmt.Errorf("failed to exec golang files: %v", err)
			}
		case models.Cpp:
			stdin, completionChan, err = execCppFiles(commandCtx, stdOut, stdErr, files, execCommand, logger)
			if err != nil {
				return nil, fmt.Errorf("failed to exec cpp files: %v", err)
			}
		case models.Csharp:
			stdin, completionChan, err = execCSharpFiles(commandCtx, stdOut, stdErr, files, execCommand, logger)
			if err != nil {
				return nil, fmt.Errorf("failed to exec C# files: %v", err)
			}
		case models.JavaScript:
			stdin, completionChan, err = execJavascriptFiles(commandCtx, stdOut, stdErr, files, execCommand, logger)
			if err != nil {
				return nil, fmt.Errorf("failed to exec javascript files: %v", err)
			}

		case models.Java:
			stdin, completionChan, err = execJavaFiles(commandCtx, stdOut, stdErr, files, execCommand, logger)
			if err != nil {
				return nil, fmt.Errorf("failed to exec java: %v", err)
			}

		case models.TypeScript:
			stdin, completionChan, err = execTypescriptFiles(commandCtx, stdOut, stdErr, files, execCommand, logger)
			if err != nil {
				return nil, fmt.Errorf("failed to exec typescript: %v", err)
			}
		case models.Rust:
			stdin, completionChan, err = execRustFiles(commandCtx, stdOut, stdErr, files, execCommand, logger)
			if err != nil {
				return nil, fmt.Errorf("failed to exec rust: %v", err)
			}
		default:
			return nil, fmt.Errorf("unsupported programming language: %s", language.String())
		}
	} else {
		switch language {
		case models.Python:
			stdin, completionChan, err = execPython(commandCtx, codeString, stdOut, stdErr)
			if err != nil {
				return nil, fmt.Errorf("failed to exec python: %v", err)
			}
		case models.Go:
			stdin, completionChan, err = execGolang(commandCtx, codeString, stdOut, stdErr)
			if err != nil {
				return nil, fmt.Errorf("failed to exec golang: %v", err)
			}
		case models.Cpp:
			stdin, completionChan, err = execCpp(commandCtx, codeString, stdOut, stdErr)
			if err != nil {
				return nil, fmt.Errorf("failed to exec cpp: %v", err)
			}
		case models.Csharp:
			stdin, completionChan, err = execCSharp(commandCtx, codeString, stdOut, stdErr)
			if err != nil {
				return nil, fmt.Errorf("failed to exec C#: %v", err)
			}
		case models.JavaScript:
			stdin, completionChan, err = execJavascript(commandCtx, codeString, stdOut, stdErr)
			if err != nil {
				return nil, fmt.Errorf("failed to exec javascript: %v", err)
			}

		case models.Java:
			stdin, completionChan, err = execJava(commandCtx, codeString, stdOut, stdErr, fileName)
			if err != nil {
				return nil, fmt.Errorf("failed to exec java: %v", err)
			}

		case models.TypeScript:
			stdin, completionChan, err = execTypescript(commandCtx, codeString, stdOut, stdErr)
			if err != nil {
				return nil, fmt.Errorf("failed to exec typescript: %v", err)
			}
		case models.Rust:
			stdin, completionChan, err = execRust(commandCtx, codeString, stdOut, stdErr)
			if err != nil {
				return nil, fmt.Errorf("failed to exec rust: %v", err)
			}
		case models.Bash:
			stdin, completionChan, cleanupFunc, err = execBash(commandCtx, codeString, stdOut, stdErr, fileName)
			if err != nil {
				return nil, fmt.Errorf("failed to exec bash: %v", err)
			}
		default:
			return nil, fmt.Errorf("unsupported programming language: %s", language.String())
		}
	}

	// wrap stdin in a multiwriter that will also forward to the stdout channel
	stdinMultiWriter := io.MultiWriter(stdin, NewStdoutForwardWriter(stdOut))

	// execute loop in go routine to read from the stdout and stderr channels
	// and pipe the content back to the payload channel
	go func() {
		var lastStdOutLineIndex, lastStdErrLineIndex *int

		defer func() {
			if cleanupFunc != nil {
				cleanupFunc()
			}
		}()

		for {
			select {
			case s := <-stdOut:
				updateOutput(&res.StdOut, &lastStdOutLineIndex, s)
			case s := <-stdErr:
				updateOutput(&res.StdErr, &lastStdErrLineIndex, s)
			case commandRes := <-completionChan:
				if commandRes == nil {
					return // no more results available
				}
				res.StatusCode = commandRes.ExitCode
				res.Done = true
				payloadChan <- res
				logger.Info(ctx, "completed execution", slog.F("result", commandRes))
				return
			}
			payloadChan <- res
		}
	}()

	return &ActiveCommand{
		Ctx:          commandCtx,
		Cancel:       commandCancel,
		Stdin:        stdinMultiWriter,
		ResponseChan: payloadChan,
	}, nil
}

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"gigo-ws/coder/agent/agent/server/payload"
	"gigo-ws/utils"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cdr.dev/slog"
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

func execPythonFiles(ctx context.Context, stdout chan string, stderr chan string, files []ExecFiles) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/tmp/pyrun"); ok {
		err := os.RemoveAll("/tmp/pyrun")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to remove directory: %w", err)
		}
	}

	err := os.MkdirAll("/tmp/pyrun", 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create directory: %w", err)
	}

	executeFile := new(ExecFiles)

	for _, k := range files {
		// write the python file
		err := os.WriteFile(fmt.Sprintf("/tmp/pyrun/%v", k.FileName), []byte(k.Code), 0755)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to write file: %w", err)
		}

		if k.Execute {
			executeFile = &k
		}
	}

	// execute python code
	_, _ = utils.ExecuteCommand(ctx, os.Environ(), "/tmp/pyrun", "bash", "-c", pythonPrepScript)
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/tmp/pyrun", stdout,
		stderr, true, "/opt/python-bytes/default/bin/python", "-u", fmt.Sprintf("%v", executeFile.FileName))
}

func execJavascriptFiles(ctx context.Context, stdout chan string, stderr chan string, files []ExecFiles) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/tmp/jsrun"); ok {
		err := os.RemoveAll("/tmp/jsrun")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to remove directory: %w", err)
		}
	}

	err := os.MkdirAll("/tmp/jsrun", 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create directory: %w", err)
	}

	executeFile := new(ExecFiles)

	for _, k := range files {
		// write the python file
		err := os.WriteFile(fmt.Sprintf("/tmp/jsrun/%v", k.FileName), []byte(k.Code), 0755)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to write file: %w", err)
		}

		if k.Execute {
			executeFile = &k
		}
	}

	// execute js code
	_, _ = utils.ExecuteCommand(ctx, os.Environ(), "/tmp/jsrun", "bash", "-c", jsPrepScript)
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/tmp/jsrun", stdout,
		stderr, true, "node", fmt.Sprintf("%v", executeFile.FileName))
}

func execCppFiles(ctx context.Context, stdout chan string, stderr chan string, files []ExecFiles) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/tmp/cpprun"); ok {
		err := os.RemoveAll("/tmp/cpprun")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to remove directory: %w", err)
		}
	}

	err := os.MkdirAll("/tmp/cpprun", 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create directory: %w", err)
	}

	executeFile := new(ExecFiles)

	for _, k := range files {
		// write the python file
		err := os.WriteFile(fmt.Sprintf("/tmp/cpprun/%v", k.FileName), []byte(k.Code), 0755)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to write file: %w", err)
		}

		if k.Execute {
			executeFile = &k
		}
	}
	// execute js code
	_, err = utils.ExecuteCommand(ctx, os.Environ(), "/tmp/cpprun", "bash", "-c", fmt.Sprintf(`#!/bin/bash
		g++ -o main %v
		chmod +x main
		`, executeFile.FileName),
	)

	if err != nil {
		stdout <- err.Error()
	}
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/tmp/cpprun", stdout,
		stderr, true, "./main")
}

func execTypescriptFiles(ctx context.Context, stdout chan string, stderr chan string, files []ExecFiles) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/tmp/tsrun"); ok {
		err := os.RemoveAll("/tmp/tsrun")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to remove directory: %w", err)
		}
	}

	err := os.MkdirAll("/tmp/tsrun", 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create directory: %w", err)
	}

	executeFile := new(ExecFiles)

	for _, k := range files {
		// write the python file
		err := os.WriteFile(fmt.Sprintf("/tmp/tsrun/%v", k.FileName), []byte(k.Code), 0755)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to write file: %w", err)
		}

		if k.Execute {
			executeFile = &k
		}
	}

	// execute js code
	_, _ = utils.ExecuteCommand(ctx, os.Environ(), "/tmp/tsrun", "bash", "-c", fmt.Sprintf(`#!/bin/bash
		npm init -y
		tsc %v
		`, executeFile.FileName),
	)
	extension := filepath.Ext(executeFile.FileName)
	parsedName := strings.TrimSuffix(executeFile.FileName, extension)
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/tmp/tsrun", stdout,
		stderr, true, "node", fmt.Sprintf("%v.js", parsedName))
}

func execRustFiles(ctx context.Context, stdout chan string, stderr chan string, files []ExecFiles) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/tmp/rsrun"); ok {
		err := os.RemoveAll("/tmp/rsrun")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to remove directory: %w", err)
		}
	}

	c, err := utils.ExecuteCommand(ctx, os.Environ(), "/tmp", "cargo", "new", "rsrun")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create directory: %w", err)
	}
	b, _ := json.Marshal(c)
	fmt.Println("prep: ", string(b))

	for _, k := range files {
		// write the python file
		err := os.WriteFile(fmt.Sprintf("/tmp/rsrun/src/%v", k.FileName), []byte(k.Code), 0755)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to write file: %w", err)
		}
	}

	// execute js code
	// _, _ = utils.ExecuteCommand(ctx, os.Environ(), "/tmp/rsrun", "bash", "-c", rustPrepScript)
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/tmp/rsrun", stdout,
		stderr, true, "cargo", "run")
}

func execCSharpFiles(ctx context.Context, stdout chan string, stderr chan string, files []ExecFiles) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/tmp/csrun"); ok {
		err := os.RemoveAll("/tmp/csrun")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to remove directory: %w", err)
		}
	}

	_, err := utils.ExecuteCommand(ctx, os.Environ(), "/tmp", "bash", "-c", `#!/bin/bash
			dotnet new console --name csrun
			`,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create directory: %w", err)
	}

	for _, k := range files {
		// write the python file
		err := os.WriteFile(fmt.Sprintf("/tmp/csrun/%v", k.FileName), []byte(k.Code), 0755)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to write file: %w", err)
		}
	}

	// execute js code
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/tmp/csrun", stdout,
		stderr, true, "dotnet", "run")
}

func execJavaFiles(ctx context.Context, stdout chan string, stderr chan string, files []ExecFiles) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/tmp/jrun"); ok {
		err := os.RemoveAll("/tmp/jrun")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to remove directory: %w", err)
		}
	}

	err := os.MkdirAll("/tmp/jrun", 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create directory: %w", err)
	}

	executeFile := new(ExecFiles)

	for _, k := range files {
		// write the python file
		err := os.WriteFile(fmt.Sprintf("/tmp/jrun/%v", k.FileName), []byte(k.Code), 0755)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to write file: %w", err)
		}

		if k.Execute {
			executeFile = &k
		}
	}

	_, _ = utils.ExecuteCommand(ctx, os.Environ(), "/tmp/jrun", "bash", "-c", fmt.Sprintf(`#!/bin/bash
		javac %v.java
	`, executeFile.FileName))

	extension := filepath.Ext(executeFile.FileName)
	parsedName := strings.TrimSuffix(executeFile.FileName, extension)

	// execute js code
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/tmp/jrun", stdout,
		stderr, true, "java", fmt.Sprintf("%v", parsedName))
}

func execGolangFiles(ctx context.Context, stdout chan string, stderr chan string, files []ExecFiles) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/tmp/gorun"); ok {
		err := os.RemoveAll("/tmp/gorun")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to remove directory: %w", err)
		}
	}

	err := os.MkdirAll("/tmp/gorun", 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create directory: %w", err)
	}

	executeFile := new(ExecFiles)

	for _, k := range files {
		// write the python file
		err := os.WriteFile(fmt.Sprintf("/tmp/gorun/%v", k.FileName), []byte(k.Code), 0755)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to write file: %w", err)
		}

		if k.Execute {
			executeFile = &k
		}
	}

	// conditionally initialize the go module
	if ok, _ := utils2.PathExists("/tmp/gorun/go.mod"); !ok {
		_, _ = utils.ExecuteCommand(ctx, os.Environ(), "/tmp/gorun", "go", "mod", "init", "gigo-byte")
	}

	// execute go code
	_, _ = utils.ExecuteCommand(ctx, os.Environ(), "/tmp/gorun", "go", "mod", "tidy")
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/tmp/gorun", stdout,
		stderr, true, "go", "run", executeFile.FileName)
}

// ///////////////////////////////////

func execPython(ctx context.Context, code string, stdout chan string, stderr chan string) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/tmp/pyrun"); !ok {
		err := os.MkdirAll("/tmp/pyrun", 0755)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// write the python file
	err := os.WriteFile("/tmp/pyrun/main.py", []byte(code), 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	// execute python code
	_, _ = utils.ExecuteCommand(ctx, os.Environ(), "/tmp/pyrun", "bash", "-c", pythonPrepScript)
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/tmp/pyrun", stdout,
		stderr, true, "/opt/python-bytes/default/bin/python", "-u", "main.py")
}

func execJavascript(ctx context.Context, code string, stdout chan string, stderr chan string) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/tmp/jsrun"); !ok {
		err := os.MkdirAll("/tmp/jsrun", 0755)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// write the js file
	err := os.WriteFile("/tmp/jsrun/index.js", []byte(code), 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	// execute js code
	_, _ = utils.ExecuteCommand(ctx, os.Environ(), "/tmp/jsrun", "bash", "-c", jsPrepScript)
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/tmp/jsrun", stdout,
		stderr, true, "node", "index.js")
}

func execCpp(ctx context.Context, code string, stdout chan string, stderr chan string) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/tmp/cpprun"); !ok {
		err := os.MkdirAll("/tmp/cpprun", 0755)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// write the js file
	err := os.WriteFile("/tmp/cpprun/main.cpp", []byte(code), 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	// execute js code
	_, _ = utils.ExecuteCommand(ctx, os.Environ(), "/tmp/cpprun", "bash", "-c", cppPrepScript)
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/tmp/cpprun", stdout,
		stderr, true, "./main")
}

func execTypescript(ctx context.Context, code string, stdout chan string, stderr chan string) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/tmp/tsrun"); !ok {
		err := os.MkdirAll("/tmp/tsrun", 0755)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// write the js file
	err := os.WriteFile("/tmp/tsrun/main.ts", []byte(code), 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	// execute js code
	_, _ = utils.ExecuteCommand(ctx, os.Environ(), "/tmp/tsrun", "bash", "-c", tsPrepScript)
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/tmp/tsrun", stdout,
		stderr, true, "node", "main.js")
}

func execRust(ctx context.Context, code string, stdout chan string, stderr chan string) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/tmp/rsrun"); !ok {
		_, err := utils.ExecuteCommand(ctx, os.Environ(), "/tmp", "cargo", "new", "rsrun")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// write the js file
	err := os.WriteFile("/tmp/rsrun/src/main.rs", []byte(code), 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	// execute js code
	// _, _ = utils.ExecuteCommand(ctx, os.Environ(), "/tmp/rsrun", "bash", "-c", rustPrepScript)
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/tmp/rsrun", stdout,
		stderr, true, "cargo", "run")
}

func execCSharp(ctx context.Context, code string, stdout chan string, stderr chan string) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/tmp/csrun"); !ok {
		_, err := utils.ExecuteCommand(ctx, os.Environ(), "/tmp", "bash", "-c", csPrePrepScript)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}
	// write the js file
	err := os.WriteFile("/tmp/csrun/main.cs", []byte(code), 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	// execute js code
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/tmp/csrun", stdout,
		stderr, true, "dotnet", "run")
}

func execJava(ctx context.Context, code string, stdout chan string, stderr chan string, filename *string) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/tmp/jrun"); !ok {
		err := os.MkdirAll("/tmp/jrun", 0755)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	var nameStr string = "main"

	if filename != nil {
		nameStr = *filename
	}
	// write the js file
	err := os.WriteFile(fmt.Sprintf("/tmp/jrun/%v.java", nameStr), []byte(code), 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	_, _ = utils.ExecuteCommand(ctx, os.Environ(), "/tmp/jrun", "bash", "-c", fmt.Sprintf(`#!/bin/bash
		javac %v.java
	`, nameStr))

	// execute js code
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/tmp/jrun", stdout,
		stderr, true, "java", fmt.Sprintf("%v", nameStr))
}

func execGolang(ctx context.Context, code string, stdout chan string, stderr chan string) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/tmp/gorun"); !ok {
		err := os.MkdirAll("/tmp/gorun", 0755)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// write the python file
	err := os.WriteFile("/tmp/gorun/main.go", []byte(code), 0755)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	// conditionally initialize the go module
	if ok, _ := utils2.PathExists("/tmp/gorun/go.mod"); !ok {
		_, _ = utils.ExecuteCommand(ctx, os.Environ(), "/tmp/gorun", "go", "mod", "init", "gigo-byte")
	}

	// execute go code
	_, _ = utils.ExecuteCommand(ctx, os.Environ(), "/tmp/gorun", "go", "mod", "tidy")
	return utils.ExecuteCommandStreamStdin(ctx, os.Environ(), "/tmp/gorun", stdout,
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

func ExecCode(ctx context.Context, codeString string, language models.ProgrammingLanguage, fileName *string, files []ExecFiles, logger slog.Logger) (*ActiveCommand, error) {
	payloadChan := make(chan payload.ExecResponsePayload, 100)
	stdOut := make(chan string)
	stdErr := make(chan string)

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

		switch language {
		case models.Python:
			stdin, completionChan, err = execPythonFiles(commandCtx, stdOut, stdErr, files)
			if err != nil {
				return nil, fmt.Errorf("failed to exec python files: %v", err)
			}
		case models.Go:
			stdin, completionChan, err = execGolangFiles(commandCtx, stdOut, stdErr, files)
			if err != nil {
				return nil, fmt.Errorf("failed to exec golang files: %v", err)
			}
		case models.Cpp:
			stdin, completionChan, err = execCppFiles(commandCtx, stdOut, stdErr, files)
			if err != nil {
				return nil, fmt.Errorf("failed to exec cpp files: %v", err)
			}
		case models.Csharp:
			stdin, completionChan, err = execCSharpFiles(commandCtx, stdOut, stdErr, files)
			if err != nil {
				return nil, fmt.Errorf("failed to exec C# files: %v", err)
			}
		case models.JavaScript:
			stdin, completionChan, err = execJavascriptFiles(commandCtx, stdOut, stdErr, files)
			if err != nil {
				return nil, fmt.Errorf("failed to exec javascript files: %v", err)
			}

		case models.Java:
			stdin, completionChan, err = execJavaFiles(commandCtx, stdOut, stdErr, files)
			if err != nil {
				return nil, fmt.Errorf("failed to exec java: %v", err)
			}

		case models.TypeScript:
			stdin, completionChan, err = execTypescriptFiles(commandCtx, stdOut, stdErr, files)
			if err != nil {
				return nil, fmt.Errorf("failed to exec typescript: %v", err)
			}
		case models.Rust:
			stdin, completionChan, err = execRustFiles(commandCtx, stdOut, stdErr, files)
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

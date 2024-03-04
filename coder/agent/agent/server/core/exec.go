package core

import (
	"context"
	"fmt"
	"gigo-ws/coder/agent/agent/server/payload"
	"gigo-ws/utils"
	"io"
	"os"
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

const rustPrePrepScript = `#!/bin/bash
cargo new rsrun
cd rsrun/src
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
	_, _ = utils.ExecuteCommand(ctx, nil, "/tmp/pyrun", "bash", "-c", pythonPrepScript)
	return utils.ExecuteCommandStreamStdin(ctx, nil, "/tmp/pyrun", stdout,
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
	_, _ = utils.ExecuteCommand(ctx, nil, "/tmp/jsrun", "bash", "-c", jsPrepScript)
	return utils.ExecuteCommandStreamStdin(ctx, nil, "/tmp/jsrun", stdout,
		stderr, true, "node", "", "index.js")
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
	_, _ = utils.ExecuteCommand(ctx, nil, "/tmp/jsrun", "bash", "-c", cppPrepScript)
	return utils.ExecuteCommandStreamStdin(ctx, nil, "/tmp/cpprun", stdout,
		stderr, true, "", "", "./main")
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
	_, _ = utils.ExecuteCommand(ctx, nil, "/tmp/tsrun", "bash", "-c", tsPrepScript)
	return utils.ExecuteCommandStreamStdin(ctx, nil, "/tmp/tsrun", stdout,
		stderr, true, "node", "", "main.js")
}

func execRust(ctx context.Context, code string, stdout chan string, stderr chan string) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/tmp/rsrun"); !ok {
		_, err := utils.ExecuteCommand(ctx, nil, "/tmp/rsrun", "bash", "-c", rustPrePrepScript)
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
	_, _ = utils.ExecuteCommand(ctx, nil, "/tmp/tsrun", "bash", "-c", rustPrepScript)
	return utils.ExecuteCommandStreamStdin(ctx, nil, "/tmp/tsrun", stdout,
		stderr, true, "cargo", "run", "")
}

func execCSharp(ctx context.Context, code string, stdout chan string, stderr chan string) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// ensure the parent directory exists
	if ok, _ := utils2.PathExists("/tmp/csrun"); !ok {
		_, err := utils.ExecuteCommand(ctx, nil, "/tmp", "bash", "-c", csPrePrepScript)
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
	return utils.ExecuteCommandStreamStdin(ctx, nil, "/tmp/csrun", stdout,
		stderr, true, "dotnet", "run", "")
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

	_, _ = utils.ExecuteCommand(ctx, nil, "/tmp/jrun", "bash", "-c", fmt.Sprintf(`#!/bin/bash
		javac %v.java
	`, nameStr))

	// execute js code
	return utils.ExecuteCommandStreamStdin(ctx, nil, "/tmp/csrun", stdout,
		stderr, true, "java", "", fmt.Sprintf("%v", nameStr))
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
		_, _ = utils.ExecuteCommand(ctx, nil, "/tmp/gorun", "go", "mod", "init", "gigo-byte")
	}

	// execute go code
	_, _ = utils.ExecuteCommand(ctx, nil, "/tmp/gorun", "go", "mod", "tidy")
	return utils.ExecuteCommandStreamStdin(ctx, nil, "/tmp/gorun", stdout,
		stderr, true, "go", "run", "main.go")
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

func ExecCode(ctx context.Context, codeString string, language models.ProgrammingLanguage, fileName *string, logger slog.Logger) (*ActiveCommand, error) {
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
	var err error
	switch language {
	case models.Python:
		stdin, completionChan, err = execPython(commandCtx, codeString, stdOut, stdErr)
		if err != nil {
			commandCancel()
			return nil, fmt.Errorf("failed to exec python: %v", err)
		}
	case models.Go:
		stdin, completionChan, err = execGolang(commandCtx, codeString, stdOut, stdErr)
		if err != nil {
			commandCancel()
			return nil, fmt.Errorf("failed to exec golang: %v", err)
		}
	case models.Cpp:
		stdin, completionChan, err = execCpp(commandCtx, codeString, stdOut, stdErr)
		if err != nil {
			commandCancel()
			return nil, fmt.Errorf("failed to exec cpp: %v", err)
		}
	case models.Csharp:
		stdin, completionChan, err = execCSharp(commandCtx, codeString, stdOut, stdErr)
		if err != nil {
			commandCancel()
			return nil, fmt.Errorf("failed to exec C#: %v", err)
		}
	case models.JavaScript:
		stdin, completionChan, err = execJavascript(commandCtx, codeString, stdOut, stdErr)
		if err != nil {
			commandCancel()
			return nil, fmt.Errorf("failed to exec javascript: %v", err)
		}

	case models.Java:
		stdin, completionChan, err = execJava(commandCtx, codeString, stdOut, stdErr, fileName)
		if err != nil {
			commandCancel()
			return nil, fmt.Errorf("failed to exec golang: %v", err)
		}

	case models.TypeScript:
		stdin, completionChan, err = execTypescript(commandCtx, codeString, stdOut, stdErr)
		if err != nil {
			commandCancel()
			return nil, fmt.Errorf("failed to exec golang: %v", err)
		}
	case models.Rust:
		stdin, completionChan, err = execRust(commandCtx, codeString, stdOut, stdErr)
		if err != nil {
			commandCancel()
			return nil, fmt.Errorf("failed to exec golang: %v", err)
		}
	default:
		commandCancel()
		return nil, fmt.Errorf("unsupported programming language: %s", language.String())
	}

	// wrap stdin in a multiwriter that will also forward to the stdout channel
	stdinMultiWriter := io.MultiWriter(stdin, NewStdoutForwardWriter(stdOut))

	// execute loop in go routine to read from the stdout and stderr channels
	// and pipe the content back to the payload channel
	go func() {
		var lastStdOutLineIndex, lastStdErrLineIndex *int

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
				logger.Info(ctx, "comepleted execution", slog.F("result", commandRes))
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

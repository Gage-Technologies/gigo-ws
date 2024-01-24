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

type ActiveCommand struct {
	Ctx          context.Context
	Cancel       context.CancelFunc
	Stdin        io.WriteCloser
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
                i++ // Skip the newline character
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

func ExecCode(ctx context.Context, codeString string, language models.ProgrammingLanguage, logger slog.Logger) (*ActiveCommand, error) {
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

	// create a new command context derived from the parent context with a cancel
	commandCtx, commandCancel := context.WithCancel(ctx)

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
	default:
		commandCancel()
		return nil, fmt.Errorf("unsupported programming language: %s", language.String())
	}

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
		Stdin:        stdin,
		ResponseChan: payloadChan,
	}, nil
}

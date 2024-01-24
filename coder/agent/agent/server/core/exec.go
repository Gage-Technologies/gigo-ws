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
eval "$(conda shell.bash hook)" &> /dev/null
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
		stderr, "/opt/python-bytes/default/bin/python", "-u", "main.py")
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
		stderr, "go", "run", "main.go")
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
		for {
			select {
			case s := <-stdOut:
				res.StdOut = append(
					res.StdOut,
					payload.OutputRow{
						Content:   s,
						Timestamp: time.Now().UnixNano(),
					},
				)
			case s := <-stdErr:
				res.StdErr = append(
					res.StdErr,
					payload.OutputRow{
						Content:   s,
						Timestamp: time.Now().UnixNano(),
					},
				)
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

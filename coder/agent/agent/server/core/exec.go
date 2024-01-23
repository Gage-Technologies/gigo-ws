package core

import (
	"context"
	"fmt"
	"gigo-ws/coder/agent/agent/server/payload"
	"gigo-ws/utils"
	"io"
	"time"

	"cdr.dev/slog"
	"github.com/gage-technologies/gigo-lib/db/models"
)

const pythonExecScript = `#!/bin/bash
eval "$(/opt/conda/miniconda/bin/conda shell.bash hook)" &> /dev/null
/opt/conda/miniconda/bin/conda activate /opt/python-bytes/default &> /dev/null
pipreqs --force . &> /dev/null
pip install -r requirements.txt &> /dev/null
python <<EOF
%s
EOF
`

const golangExecScript = `#!/bin/bash
mkdir -p /tmp/gorun
cat <<EOF > /tmp/gorun/main.go
%s
EOF
go run /tmp/gorun/main.go
`

type ActiveCommand struct {
	Ctx          context.Context
	Cancel       context.CancelFunc
	Stdin        io.WriteCloser
	ResponseChan chan payload.ExecResponsePayload
}

func execPython(ctx context.Context, code string, stdout chan string, stderr chan string) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// execute python code
	return utils.ExecuteCommandStreamStdin(ctx, nil, stdout,
		stderr, "bash", "-c", fmt.Sprintf(pythonExecScript, code))
}

func execGolang(ctx context.Context, code string, stdout chan string, stderr chan string) (io.WriteCloser, <-chan *utils.CommandResult, error) {
	// execute python code
	return utils.ExecuteCommandStreamStdin(ctx, nil, stdout,
		stderr, "bash", "-c", fmt.Sprintf(golangExecScript, code))
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

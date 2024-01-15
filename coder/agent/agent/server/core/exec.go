package core

import (
	"context"
	"fmt"
	"gigo-ws/coder/agent/agent/server/payload"
	"gigo-ws/utils"
	"time"

	"cdr.dev/slog"
	"github.com/gage-technologies/gigo-lib/db/models"
)

const pythonExecScript = `#!/bin/bash
eval "$(/opt/conda/miniconda/bin/conda shell.bash hook)" &> /dev/null
/opt/conda/miniconda/bin/conda activate /opt/python-bytes/default &> /dev/null
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

func execPython(ctx context.Context, code string, stdout chan string, stderr chan string, payloadChan chan payload.ExecResponsePayload, logger slog.Logger) error {
	// default payload
	res := payload.ExecResponsePayload{
		StdOut:     make([]payload.OutputRow, 0),
		StdErr:     make([]payload.OutputRow, 0),
		StatusCode: -1,
		Done:       false,
	}

	// execute loop in go routine to read from the stdout and stderr channels
	// and pipe the content back to the payload channel
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case s := <-stdout:
				res.StdOut = append(
					res.StdOut,
					payload.OutputRow{
						Content:   s,
						Timestamp: time.Now().UnixNano(),
					},
				)
			case s := <-stderr:
				res.StdErr = append(
					res.StdErr,
					payload.OutputRow{
						Content:   s,
						Timestamp: time.Now().UnixNano(),
					},
				)
			}
			payloadChan <- res
		}
	}()

	// execute python code
	commandRes, err := utils.ExecuteCommandStream(ctx, nil, stdout,
		stderr, "bash", "-c", fmt.Sprintf(pythonExecScript, code))
	if err != nil {
		logger.Error(ctx, "failed to execute python code: %s", slog.Error(err))
		return err
	}

	// update the response payload and return to payload channel
	res.StatusCode = commandRes.ExitCode
	res.Done = true
	payloadChan <- res
	logger.Info(ctx, "executed python code with status code: %d", slog.F("status_code", res.StatusCode))
	return nil
}

func execGolang(ctx context.Context, code string, stdout chan string, stderr chan string, payloadChan chan payload.ExecResponsePayload, logger slog.Logger) error {
	// default payload
	res := payload.ExecResponsePayload{
		StdOut:     make([]payload.OutputRow, 0),
		StdErr:     make([]payload.OutputRow, 0),
		StatusCode: -1,
		Done:       false,
	}

	// execute loop in go routine to read from the stdout and stderr channels
	// and pipe the content back to the payload channel
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case s := <-stdout:
				res.StdOut = append(
					res.StdOut,
					payload.OutputRow{
						Content:   s,
						Timestamp: time.Now().UnixNano(),
					},
				)
			case s := <-stderr:
				res.StdErr = append(
					res.StdErr,
					payload.OutputRow{
						Content:   s,
						Timestamp: time.Now().UnixNano(),
					},
				)
			}
			payloadChan <- res
		}
	}()

	// execute python code
	commandRes, err := utils.ExecuteCommandStream(ctx, nil, stdout,
		stderr, "bash", "-c", fmt.Sprintf(golangExecScript, code))
	if err != nil {
		logger.Error(ctx, "failed to execute golang code: %s", slog.Error(err))
		return err
	}

	// update the response payload and return to payload channel
	res.StatusCode = commandRes.ExitCode
	res.Done = true
	payloadChan <- res
	logger.Info(ctx, "executed golang code with status code: %d", slog.F("status_code", res.StatusCode))
	return nil
}

func ExecCode(ctx context.Context, codeString string, language models.ProgrammingLanguage, logger slog.Logger) (chan payload.ExecResponsePayload, error) {

	payloadChan := make(chan payload.ExecResponsePayload, 100)

	stdOut := make(chan string)
	stdErr := make(chan string)

	switch language {

	case models.Python:
		err := execPython(ctx, codeString, stdOut, stdErr, payloadChan, logger)
		if err != nil {
			return nil, err
		}
		return payloadChan, nil
	case models.Go:
		err := execGolang(ctx, codeString, stdOut, stdErr, payloadChan, logger)
		if err != nil {
			return nil, err
		}
		return payloadChan, nil
	default:
		return nil, fmt.Errorf("unsupported programming language: %s", language.String())
	}

}

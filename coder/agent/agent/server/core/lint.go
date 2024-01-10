package core

import (
	"cdr.dev/slog"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gigo-ws/coder/agent/agent/server/payload"
	"gigo-ws/utils"
)

const pythonLintScript = `#!/bin/bash
eval "$(conda shell.bash hook)" &> /dev/null
conda activate /opt/python-bytes/default &> /dev/null
mkdir -p /tmp/pyrun
cat <<EOF > /tmp/pyrun/main.py
%s
EOF
pylint --output-format=json /tmp/pyrun/main.py
`

const golangLintScript = `#!/bin/bash
mkdir -p /tmp/gorun
cat <<EOF > /tmp/gorun/main.go
%s
EOF
golangci-lint run --out-format=json /tmp/gorun/main.go
`

func lintPython(ctx context.Context, code string, logger slog.Logger) (*payload.LintResponsePayload, error) {
	// default payload
	res := payload.LintResponsePayload{
		StdOut:     make([]payload.OutputRow, 0),
		StdErr:     make([]payload.OutputRow, 0),
		StatusCode: -1,
		LintRes:    payload.LintResult{},
		Done:       false,
	}

	// execute python code
	commandRes, err := utils.ExecuteCommand(ctx, nil, "",
		"bash", "-c", fmt.Sprintf(pythonLintScript, code))
	if err != nil {
		logger.Error(ctx, "failed to lint python code: %s", slog.Error(err))
		return nil, err
	}

	if commandRes.Stderr != "" {
		logger.Error(ctx, "failed to lint python code: %s", slog.Error(err))
		return nil, errors.New(fmt.Sprintf("failed to lint python code: %s", commandRes.Stderr))
	}

	var pyLintRes []payload.PythonLintResultInstance

	err = json.Unmarshal([]byte(commandRes.Stdout), &pyLintRes)
	if err != nil {
		logger.Error(ctx, "failed to unmarshal lint python response: %s", slog.Error(err))
		return nil, err
	}

	res.LintRes.PyFullLint = payload.PythonLintResult{
		Results: pyLintRes,
	}
	for _, pyLintRes := range res.LintRes.PyFullLint.Results {
		res.LintRes.Results = append(res.LintRes.Results, payload.GenericLintResult{
			Column:  pyLintRes.Column,
			Line:    pyLintRes.Line,
			Message: pyLintRes.Message,
		})
	}

	// update the response payload and return to payload channel
	res.StatusCode = commandRes.ExitCode
	res.Done = true

	logger.Info(ctx, "linted python code with status code: %d", slog.F("status_code", res.StatusCode))
	return &res, nil
}

func lintGolang(ctx context.Context, code string, logger slog.Logger) (*payload.LintResponsePayload, error) {
	// default payload
	res := payload.LintResponsePayload{
		StdOut:     make([]payload.OutputRow, 0),
		StdErr:     make([]payload.OutputRow, 0),
		StatusCode: -1,
		LintRes:    payload.LintResult{},
		Done:       false,
	}

	// execute python code
	commandRes, err := utils.ExecuteCommand(ctx, nil, "",
		"bash", "-c", fmt.Sprintf(golangLintScript, code))
	if err != nil {
		logger.Error(ctx, "failed to lint python code: %s", slog.Error(err))
		return nil, err
	}

	if commandRes.Stderr != "" {
		logger.Error(ctx, "failed to lint python code: %s", slog.Error(err))
		return nil, errors.New(fmt.Sprintf("failed to lint python code: %s", commandRes.Stderr))
	}

	var goLintRes payload.GoLintResult

	err = json.Unmarshal([]byte(commandRes.Stdout), &goLintRes)
	if err != nil {
		logger.Error(ctx, "failed to unmarshal lint python response: %s", slog.Error(err))
		return nil, err
	}

	res.LintRes.GoFullLint = goLintRes

	if len(goLintRes.Issues) > 0 {
		for _, issue := range goLintRes.Issues {
			res.LintRes.Results = append(res.LintRes.Results, payload.GenericLintResult{
				Column:  issue.Position.Column,
				Line:    issue.Position.Line,
				Message: issue.Text,
			})
		}
	}

	// update the response payload and return to payload channel
	res.StatusCode = commandRes.ExitCode
	res.Done = true

	logger.Info(ctx, "linted python code with status code: %d", slog.F("status_code", res.StatusCode))
	return &res, nil
}

func LintCode(ctx context.Context, codeString string, language ProgrammingLanguage, logger slog.Logger) (*payload.LintResponsePayload, error) {

	switch language {

	case Python:
		lintRes, err := lintPython(ctx, codeString, logger)
		if err != nil {
			return nil, err
		}
		return lintRes, nil
	case Golang:
		lintRes, err := lintGolang(ctx, codeString, logger)
		if err != nil {
			return nil, err
		}
		return lintRes, nil
	default:
		return nil, fmt.Errorf("unsupported programming language: %s", language.String())
	}

}

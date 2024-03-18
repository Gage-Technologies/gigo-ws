package utils

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestExecuteCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		binary  string
		args    []string
		out     string
		err     string
		code    int
		timeout time.Duration
	}{
		{
			name:    "success",
			binary:  "echo",
			args:    []string{"foo"},
			out:     "foo",
			err:     "",
			code:    0,
			timeout: time.Hour,
		},
		{
			name:    "error",
			binary:  "bash",
			args:    []string{"-c", "echo foo 1>&2; exit 69"},
			out:     "",
			err:     "foo",
			code:    69,
			timeout: time.Hour,
		},
		{
			name:    "timeout",
			binary:  "sleep",
			args:    []string{"1s"},
			out:     "",
			err:     "context closed",
			code:    -1,
			timeout: time.Millisecond * 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.TODO(), tt.timeout)
			defer cancel()
			out, err := ExecuteCommand(ctx, nil, "", tt.binary, tt.args...)
			if err != nil {
				if tt.err == "" || (tt.err != "" && !strings.HasPrefix(err.Error(), tt.err)) {
					t.Fatal(err)
				}
				return
			}

			if tt.code > -1 && out.ExitCode != tt.code {
				t.Errorf("expected %v, got %v", tt.code, out.ExitCode)
			}

			if tt.out != out.Stdout {
				t.Errorf("expected %q, got %q", tt.out, out.Stdout)
			}

			if tt.err != out.Stderr {
				t.Errorf("expected %q, got %q", tt.err, out.Stderr)
			}
		})
	}
}

func TestExecuteCommandStream(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		binary  string
		args    []string
		out     string
		err     string
		code    int
		timeout time.Duration
	}{
		{
			name:    "success",
			binary:  "echo",
			args:    []string{"foo"},
			out:     "foo",
			err:     "",
			code:    0,
			timeout: time.Hour,
		},
		{
			name:    "error",
			binary:  "bash",
			args:    []string{"-c", "echo foo 1>&2; exit 69"},
			out:     "",
			err:     "foo",
			code:    69,
			timeout: time.Hour,
		},
		{
			name:    "timeout",
			binary:  "sleep",
			args:    []string{"1s"},
			out:     "",
			err:     "context closed",
			code:    -1,
			timeout: time.Millisecond * 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.TODO(), tt.timeout)
			defer cancel()

			// make channel buffers large enough that we don't have to
			// run a concurrent routine to read the input in a stream fashion
			outChan := make(chan string, 10)
			errChan := make(chan string, 10)

			out, err := ExecuteCommandStream(ctx, nil, "", outChan, errChan, false, tt.binary, tt.args...)
			if err != nil {
				if tt.err == "" || (tt.err != "" && !strings.HasPrefix(err.Error(), tt.err)) {
					t.Fatal(err)
				}
				return
			}

			if tt.code > -1 && out.ExitCode != tt.code {
				t.Errorf("expected %v, got %v", tt.code, out.ExitCode)
			}
			stdOut := ""
			for {
				exit := false
				select {
				case o := <-outChan:
					if len(stdOut) > 0 {
						stdOut += "\n"
					}
					stdOut += o
					continue
				default:
					exit = true
				}
				if exit {
					break
				}
			}
			if tt.out != stdOut {
				t.Errorf("expected %q, got %q", tt.out, stdOut)
			}
			stdErr := ""
			for {
				exit := false
				select {
				case o := <-errChan:
					if len(stdErr) > 0 {
						stdOut += "\n"
					}
					stdErr += o
					continue
				default:
					exit = true
				}
				if exit {
					break
				}
			}
			if tt.err != stdErr {
				t.Errorf("expected %q, got %q", tt.err, stdErr)
			}
		})
	}
}

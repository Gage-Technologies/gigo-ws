package utils

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/go-cmd/cmd"
)

type CommandResult struct {
	Command  string
	Stdout   string
	Stderr   string
	ExitCode int
	Start    time.Time
	End      time.Time
	Cost     time.Duration
}

// ExecuteCommand
//
//	Helper function to execute commands safely via the
//	github.com/go-cmd/cmd library
func ExecuteCommand(ctx context.Context, env []string, dir string, binary string, args ...string) (*CommandResult, error) {
	// create a new command
	c := cmd.NewCmd(binary, args...)
	c.Env = env

	// conditionally set the working directory
	if len(dir) > 0 {
		c.Dir = dir
	}

	// start command
	statusChan := c.Start()

	// wait for command or context
	select {
	case <-ctx.Done():
		// stop command since we are exiting early
		err := c.Stop()
		return nil, fmt.Errorf("context closed - %v", err)
	case status := <-statusChan:
		// load data from status by retrieving the last
		// line of output - go-cmd is a bit weird on how
		// it handle output. the last string in the slice
		// is the final output
		stdOut := ""
		stdErr := ""
		if len(status.Stdout) > 0 {
			stdOut = strings.Join(status.Stdout, "\n")
		}
		if len(status.Stderr) > 0 {
			stdErr = strings.Join(status.Stderr, "\n")
		}

		// format the start and end time from the timestamps
		start := time.Unix(0, status.StartTs)
		end := time.Unix(0, status.StopTs)

		return &CommandResult{
			Command:  strings.Join(append([]string{binary}, args...), " "),
			Stdout:   stdOut,
			Stderr:   stdErr,
			ExitCode: status.Exit,
			Start:    start,
			End:      end,
			Cost:     end.Sub(start),
		}, nil
	}
}

// ExecuteCommandStream
//
//		Helper function to execute commands safely via the
//		github.com/go-cmd/cmd library utilizing the streaming
//	 functionality to return data line-by-line via channels
func ExecuteCommandStream(ctx context.Context, env []string, dir string, stdOut chan string, stdErr chan string, directStreaming bool,
	binary string, args ...string) (*CommandResult, error) {
	// create a new command using streaming API
	c := cmd.NewCmdOptions(cmd.Options{
		Buffered:        false,
		Streaming:       true,
		DirectStreaming: directStreaming,
	}, binary, args...)
	c.Env = env

	// conditionally set the working directory
	if len(dir) > 0 {
		c.Dir = dir
	}

	// create channel to track done
	done := make(chan struct{})

	// launch go func to handle streaming
	go func() {
		// defer closure to mark completion of streaming
		defer close(done)

		// Done when both channels have been closed
		// https://dave.cheney.net/2013/04/30/curious-channels
		for c.Stdout != nil || c.Stderr != nil {
			// pipe output to channels
			select {
			case line, ok := <-c.Stdout:
				// set stdout channel nil when we close
				if !ok {
					c.Stdout = nil
					continue
				}
				stdOut <- line
			case line, ok := <-c.Stderr:
				// set stderr channel nil when we close
				if !ok {
					c.Stderr = nil
					continue
				}
				stdErr <- line
			}
		}
	}()

	// start command
	statusChan := c.Start()

	// wait for command or context
	select {
	case <-ctx.Done():
		// stop command since we are exiting early
		err := c.Stop()
		// wait for streams to close
		<-done
		return nil, fmt.Errorf("context closed - %v", err)
	case status := <-statusChan:
		// wait for streams to close
		<-done

		// format the start and end time from the timestamps
		start := time.Unix(0, status.StartTs)
		end := time.Unix(0, status.StopTs)

		// return command result without output
		return &CommandResult{
			Command:  strings.Join(append([]string{binary}, args...), " "),
			ExitCode: status.Exit,
			Start:    start,
			End:      end,
			Cost:     end.Sub(start),
		}, nil
	}
}

// ExecuteCommandStreamStdin
//
//		Helper function to execute commands safely via the
//		github.com/go-cmd/cmd library utilizing the streaming
//	 functionality to return data line-by-line via channels
func ExecuteCommandStreamStdin(ctx context.Context, env []string, dir string, stdOut chan string, stdErr chan string,
	directStreaming bool, binary string, args ...string) (io.WriteCloser, <-chan *CommandResult, error) {
	// create a new command using streaming API
	c := cmd.NewCmdOptions(cmd.Options{
		Buffered:        false,
		Streaming:       true,
		DirectStreaming: directStreaming,
	}, binary, args...)
	c.Env = env

	// conditionally set the working directory
	if len(dir) > 0 {
		c.Dir = dir
	}

	// create channel to track done
	done := make(chan struct{})

	// launch go func to handle streaming
	go func() {
		// defer closure to mark completion of streaming
		defer close(done)

		// Done when both channels have been closed
		// https://dave.cheney.net/2013/04/30/curious-channels
		for c.Stdout != nil || c.Stderr != nil {
			// pipe output to channels
			select {
			case line, ok := <-c.Stdout:
				// set stdout channel nil when we close
				if !ok {
					c.Stdout = nil
					continue
				}
				stdOut <- line
			case line, ok := <-c.Stderr:
				// set stderr channel nil when we close
				if !ok {
					c.Stderr = nil
					continue
				}
				stdErr <- line
			}
		}
	}()

	// create the output channel
	outputChan := make(chan *CommandResult)

	// start command
	internalStartTime := time.Now()
	statusChan, stdin := c.StartWithStdin()

	// wait for command or context
	go func() {
		select {
		case <-ctx.Done():
			// stop command since we are exiting early
			_ = c.Stop()
			// wait for streams to close
			<-done

			// try to retrieve the status
			select {
			case status := <-statusChan:
				// format the start and end time from the timestamps
				start := time.Unix(0, status.StartTs)
				end := time.Unix(0, status.StopTs)
				// write the response to the channel
				outputChan <- &CommandResult{
					Command:  strings.Join(append([]string{binary}, args...), " "),
					ExitCode: status.Exit,
					Start:    start,
					End:      end,
					Cost:     end.Sub(start),
				}
			case <-time.After(time.Second):
				// write the response to the channel
				outputChan <- &CommandResult{
					Command:  strings.Join(append([]string{binary}, args...), " "),
					ExitCode: -1,
					Start:    internalStartTime,
					End:      time.Now(),
					Cost:     time.Now().Sub(internalStartTime),
				}
			}
		case status := <-statusChan:
			// wait for streams to close
			<-done

			// format the start and end time from the timestamps
			start := time.Unix(0, status.StartTs)
			end := time.Unix(0, status.StopTs)

			// return command result without output
			outputChan <- &CommandResult{
				Command:  strings.Join(append([]string{binary}, args...), " "),
				ExitCode: status.Exit,
				Start:    start,
				End:      end,
				Cost:     end.Sub(start),
			}
		}
	}()

	return stdin, outputChan, nil
}

package core

import (
	"context"
	"fmt"
	"gigo-ws/coder/agent/agent/server/payload"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/gage-technologies/gigo-lib/db/models"
)

func Test_ExecCode(t *testing.T) {
	payloadRes, err := ExecCode(context.Background(), "print('hello world')", models.Python, nil, slog.Make(sloghuman.Sink(os.Stdout)))
	if err != nil {
		t.Error(err)
		return
	}

	for {
		res := <-payloadRes.ResponseChan
		if res.Done {
			if res.StatusCode != 0 {
				t.Error(res.StatusCode)
				return
			}

			if res.StdOut == nil || len(res.StdOut) == 0 {
				t.Error("failed to receive stdout")
				return
			}

			if res.StdOut[0].Content != "hello world" {
				t.Error(res.StdOut[0].Content)
				return
			}

			t.Log("python executed successfully: ", res.StdOut[0].Content)
			break
		}
	}

	payloadRes, err = ExecCode(context.Background(), "package main\nimport \"fmt\"\n\nfunc main(){\n\tfmt.Println(\"hello world\")\n}", models.Go, nil, slog.Make(sloghuman.Sink(os.Stdout)))
	if err != nil {
		t.Error(err)
		return
	}

	for {
		res := <-payloadRes.ResponseChan
		if res.Done {
			if res.StatusCode != 0 {
				t.Error(res.StatusCode)
				t.Error(res.StdErr)
				return
			}

			if res.StdOut == nil || len(res.StdOut) == 0 {
				t.Error("failed to receive stdout")
				return
			}

			if strings.TrimSpace(res.StdOut[0].Content) != "hello world" {
				t.Error(res.StdOut[0].Content)
				return
			}

			t.Log("golang executed successfully: ", res.StdOut[0].Content)
			break
		}
	}

	payloadRes, err = ExecCode(context.Background(), "import time\nprint('started')\ntime.sleep(15)\nprint('completed')", models.Python, nil, slog.Make(sloghuman.Sink(os.Stdout)))
	if err != nil {
		t.Error(err)
		return
	}

	done := make(chan struct{})
	go func() {
		for {
			res := <-payloadRes.ResponseChan
			fmt.Println("\n\nnew result received:\n", res)
			if res.Done {
				if res.StatusCode != -1 {
					t.Error(res.StatusCode)
					break
				}

				if res.StdOut == nil || len(res.StdOut) == 0 {
					t.Error("failed to receive stdout")
					break
				}

				if res.StdOut[0].Content != "started" {
					t.Error(res.StdOut[0].Content)
					break
				}

				t.Log("python executed successfully: ", res.StdOut[0].Content)
				break
			}
		}
		done <- struct{}{}
	}()

	time.Sleep(time.Second * 5)
	payloadRes.Cancel()

	<-done
}

func TestUpdateOutput(t *testing.T) {
	tests := []struct {
		name            string
		existingOutput  []payload.OutputRow
		lastLineIndex   *int
		newData         string
		expectedOutput  []payload.OutputRow
		expectedLastIdx *int
	}{
		{
			name:            "Append new data without newline",
			existingOutput:  []payload.OutputRow{},
			lastLineIndex:   nil,
			newData:         "Test data",
			expectedOutput:  []payload.OutputRow{{Content: "Test data", Timestamp: 0}},
			expectedLastIdx: func() *int { i := 0; return &i }(),
		},
		{
			name:           "Append new line",
			existingOutput: []payload.OutputRow{{Content: "Test data", Timestamp: 0}},
			lastLineIndex:  func() *int { i := 0; return &i }(),
			newData:        "\nNew line",
			expectedOutput: []payload.OutputRow{
				{Content: "Test data", Timestamp: 0},
				{Content: "New line", Timestamp: 0},
			},
			expectedLastIdx: func() *int { i := 1; return &i }(),
		},
		{
			name:            "Carriage return - overwrite line",
			existingOutput:  []payload.OutputRow{{Content: "Old data", Timestamp: 0}},
			lastLineIndex:   func() *int { i := 0; return &i }(),
			newData:         "\rNew data",
			expectedOutput:  []payload.OutputRow{{Content: "New data", Timestamp: 0}},
			expectedLastIdx: func() *int { i := 0; return &i }(),
		},
		{
			name:           "Multiple newlines in data",
			existingOutput: []payload.OutputRow{{Content: "Line1", Timestamp: 0}},
			lastLineIndex:  func() *int { i := 0; return &i }(),
			newData:        "Line2\nLine3\nLine4",
			expectedOutput: []payload.OutputRow{
				{Content: "Line1Line2", Timestamp: 0},
				{Content: "Line3", Timestamp: 0},
				{Content: "Line4", Timestamp: 0},
			},
			expectedLastIdx: func() *int { i := 2; return &i }(),
		},
		{
			name:           "Mixed newlines and carriage returns",
			existingOutput: []payload.OutputRow{{Content: "Line1", Timestamp: 0}},
			lastLineIndex:  func() *int { i := 0; return &i }(),
			newData:        "\rLine2\nLine3\r\nLine4",
			expectedOutput: []payload.OutputRow{
				{Content: "Line2", Timestamp: 0},
				{Content: "", Timestamp: 0},
				{Content: "Line4", Timestamp: 0},
			},
			expectedLastIdx: func() *int { i := 2; return &i }(),
		},
		{
			name:           "Consecutive carriage returns",
			existingOutput: []payload.OutputRow{{Content: "Line1", Timestamp: 0}},
			lastLineIndex:  func() *int { i := 0; return &i }(),
			newData:        "\rLine2\rLine3",
			expectedOutput: []payload.OutputRow{
				{Content: "Line3", Timestamp: 0},
			},
			expectedLastIdx: func() *int { i := 0; return &i }(),
		},
		{
			name:           "Empty new data",
			existingOutput: []payload.OutputRow{{Content: "Line1", Timestamp: 0}},
			lastLineIndex:  func() *int { i := 0; return &i }(),
			newData:        "",
			expectedOutput: []payload.OutputRow{
				{Content: "Line1", Timestamp: 0},
			},
			expectedLastIdx: func() *int { i := 0; return &i }(),
		},
		{
			name:           "Newline at start of data",
			existingOutput: []payload.OutputRow{{Content: "Line1", Timestamp: 0}},
			lastLineIndex:  func() *int { i := 0; return &i }(),
			newData:        "\nLine2",
			expectedOutput: []payload.OutputRow{
				{Content: "Line1", Timestamp: 0},
				{Content: "Line2", Timestamp: 0},
			},
			expectedLastIdx: func() *int { i := 1; return &i }(),
		},
		{
			name:           "Carriage return at start of data",
			existingOutput: []payload.OutputRow{{Content: "Line1", Timestamp: 0}},
			lastLineIndex:  func() *int { i := 0; return &i }(),
			newData:        "\rLine2",
			expectedOutput: []payload.OutputRow{
				{Content: "Line2", Timestamp: 0},
			},
			expectedLastIdx: func() *int { i := 0; return &i }(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock the time
			// mockTime := time.Now().UnixNano()
			// timeNow = func() int64 { return mockTime }

			updateOutput(&tt.existingOutput, &tt.lastLineIndex, tt.newData)

			// Check if output matches expected
			for i, row := range tt.expectedOutput {
				row.Timestamp = 0
				tt.expectedOutput[i] = row
			}
			for i, row := range tt.existingOutput {
				row.Timestamp = 0
				tt.existingOutput[i] = row
			}

			if !reflect.DeepEqual(tt.existingOutput, tt.expectedOutput) {
				t.Errorf("updateOutput() got = %v, want %v", tt.existingOutput, tt.expectedOutput)
			}

			// Check last index
			if (tt.lastLineIndex == nil) != (tt.expectedLastIdx == nil) ||
				(tt.lastLineIndex != nil && *tt.lastLineIndex != *tt.expectedLastIdx) {
				t.Errorf("updateOutput() last index got = %v, want %v", tt.lastLineIndex, tt.expectedLastIdx)
			}
		})
	}
}

// Mock function to get current time
var timeNow = func() int64 {
	return time.Now().UnixNano()
}

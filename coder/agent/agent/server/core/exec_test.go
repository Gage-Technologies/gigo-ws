package core

import (
	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"context"
	"os"
	"testing"
)

func Test_ExecCode(t *testing.T) {
	payloadRes, err := ExecCode(context.Background(), "print('hello world')", Python, slog.Make(sloghuman.Sink(os.Stdout)))
	if err != nil {
		t.Error(err)
		return
	}

	for {
		res := <-payloadRes
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

	payloadRes, err = ExecCode(context.Background(), "package main\nimport \"fmt\"\n\nfunc main(){\n\tfmt.Println(\"hello world\")\n}", Golang, slog.Make(sloghuman.Sink(os.Stdout)))
	if err != nil {
		t.Error(err)
		return
	}

	for {
		res := <-payloadRes
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

			if res.StdOut[0].Content != "hello world\n" {
				t.Error(res.StdOut[0].Content)
				return
			}

			t.Log("golang executed successfully: ", res.StdOut[0].Content)
			break
		}
	}
}

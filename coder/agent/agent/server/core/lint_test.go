package core

import (
	"context"
	"fmt"
	"os"
	"testing"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/gage-technologies/gigo-lib/db/models"
)

func Test_LintCode(t *testing.T) {
	ctx := context.Background()

	res, err := LintCode(ctx, "print('hello world')", models.Python, slog.Make(sloghuman.Sink(os.Stdout)))
	if err != nil {
		t.Error(err)
		return
	}

	//if res.StatusCode != 0 {
	//	t.Error(fmt.Printf("process failed with exit code: %v", res.StatusCode))
	//	return
	//
	//}

	if res.Done == false {
		t.Error(fmt.Printf("process did not finish"))
		return
	}

	if res.LintRes.PyFullLint.Results == nil {
		t.Error(fmt.Printf("pylint result is nil"))
		return
	}

	if len(res.LintRes.PyFullLint.Results) == 0 {
		t.Error(fmt.Printf("pylint result is empty"))
		return
	}

	if res.LintRes.PyFullLint.Results[0].Line != 1 {
		t.Error(fmt.Printf("pylint result line is not correct"))
		return
	}

	if res.LintRes.PyFullLint.Results[0].Column != 0 {
		t.Error(fmt.Printf("pylint result column is not correct"))
		return
	}

	if res.LintRes.PyFullLint.Results[0].Message != "Missing module docstring" {
		t.Error(fmt.Printf("pylint result message is not correct"))
		return
	}

	t.Log(res.LintRes)

	res, err = LintCode(ctx, "fmt.Println('hello world')", models.Go, slog.Make(sloghuman.Sink(os.Stdout)))
	if err != nil {
		t.Error(err)
		return
	}

	//if res.StatusCode != 0 {
	//	t.Error(fmt.Printf("process failed with exit code: %v", res.StatusCode))
	//	return
	//
	//}

	if res.Done == false {
		t.Error(fmt.Printf("process did not finish"))
		return
	}

	if res.LintRes.GoFullLint.Issues == nil {
		t.Error(fmt.Printf("golang result is nil"))
		return
	}

	if len(res.LintRes.GoFullLint.Issues) == 0 {
		t.Error(fmt.Printf("golang result is empty"))
		return
	}

	if res.LintRes.GoFullLint.Issues[0].Position.Column != 1 {
		t.Error(fmt.Printf("golang result column is not correct"))
		return
	}

	if res.LintRes.GoFullLint.Issues[0].Position.Column != 1 {
		t.Error(fmt.Printf("golang result line is not correct"))
		return
	}

	if res.LintRes.GoFullLint.Issues[0].Position.Offset != 0 {
		t.Error(fmt.Printf("golang result offset is not correct"))
		return
	}

	if res.LintRes.GoFullLint.Issues[0].Text != "expected 'package', found fmt" {
		t.Error(fmt.Printf("golang result message is not correct"))
		return
	}

	t.Log(res.LintRes)

}

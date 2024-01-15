package payload

import "github.com/gage-technologies/gigo-lib/db/models"

type LintRequestPayload struct {
	Lang models.ProgrammingLanguage `json:"lang"`
	Code string                     `json:"code"`
}

type LintResult struct {
	Results    []GenericLintResult `json:"results"`
	GoFullLint GoLintResult        `json:"go_full_lint"`
	PyFullLint PythonLintResult    `json:"py_full_lint"`
}

type GenericLintResult struct {
	Column  int    `json:"column"`
	Line    int    `json:"line"`
	Message string `json:"message"`
}

type PythonLintResult struct {
	Results []PythonLintResultInstance `json:"results"`
}

type PythonLintResultInstance struct {
	Type      string `json:"type"`
	Module    string `json:"module"`
	Obj       string `json:"obj"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	EndLine   *int   `json:"end_line"`
	EndColumn *int   `json:"end_column"`
	Path      string `json:"path"`
	Symbol    string `json:"symbol"`
	Message   string `json:"message"`
	MessageId string `json:"message-id"`
}

type GoLintResult struct {
	Issues []goLintIssues `json:"Issues"`
	Report goLintReport   `json:"Report"`
}

type goLintIssues struct {
	FromLinter           string     `json:"FromLinter"`
	Text                 string     `json:"Text"`
	Severity             string     `json:"Severity"`
	SourceLines          []string   `json:"SourceLines"`
	Replacement          []string   `json:"Replacement"`
	Position             goPosition `json:"Pos"`
	ExpectNoLint         bool       `json:"ExpectNoLint"`
	ExpectedNoLintLinter string     `json:"ExpectedNoLintLinter"`
}

type goPosition struct {
	Filename string `json:"Filename"`
	Offset   int    `json:"Offset"`
	Line     int    `json:"Line"`
	Column   int    `json:"Column"`
}

type goLinter struct {
	Name             string `json:"Name"`
	Enabled          bool   `json:"Enabled"`
	EnabledByDefault bool   `json:"EnabledByDefault"`
}

type goLintReport struct {
	Linters []goLinter `json:"Linters"`
}

type LintResponsePayload struct {
	StdOut     []OutputRow `json:"stdout"`
	StdErr     []OutputRow `json:"stderr"`
	StatusCode int         `json:"status_code"`
	LintRes    LintResult  `json:"lint_res"`
	Done       bool        `json:"done"`
}

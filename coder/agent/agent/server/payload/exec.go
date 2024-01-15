package payload

import "github.com/gage-technologies/gigo-lib/db/models"

type ExecRequestPayload struct {
	Lang models.ProgrammingLanguage `json:"lang"`
	Code string                     `json:"code"`
}

type ExecResponsePayload struct {
	StdOut     []OutputRow `json:"stdout"`
	StdErr     []OutputRow `json:"stderr"`
	StatusCode int         `json:"status_code"`
	Done       bool        `json:"done"`
}

type OutputRow struct {
	Timestamp int64  `json:"timestamp"`
	Content   string `json:"content"`
}

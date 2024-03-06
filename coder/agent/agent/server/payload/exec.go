package payload

import "github.com/gage-technologies/gigo-lib/db/models"

type CancelExecRequestPayload struct {
	CommandID string `json:"command_id" validate:"number"`
}

type CancelExecResponsePayload struct {
	CommandID string `json:"command_id" validate:"number"`
}

type StdinExecRequestPayload struct {
	CommandID string `json:"command_id" validate:"number"`
	Input     string `json:"input"`
}

type StdinExecResponsePayload struct {
	CommandID string `json:"command_id" validate:"number"`
}

type ExecRequestPayload struct {
	Lang     models.ProgrammingLanguage `json:"lang"`
	Code     string                     `json:"code"`
	FileName *string                    `json:"file_name"`
	Files    string                     `json:"exec_files"`
}

type ExecResponsePayload struct {
	CommandID       int64       `json:"command_id"`
	CommandIDString string      `json:"command_id_string"`
	StdOut          []OutputRow `json:"stdout"`
	StdErr          []OutputRow `json:"stderr"`
	StatusCode      int         `json:"status_code"`
	Done            bool        `json:"done"`
}

type OutputRow struct {
	Timestamp int64  `json:"timestamp"`
	Content   string `json:"content"`
}

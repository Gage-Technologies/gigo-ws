package payload

import "github.com/gage-technologies/gigo-lib/db/models"

type LaunchLspRequestPayload struct {
	Lang    models.ProgrammingLanguage `json:"lang" validate:"oneof=5 6"`
	Content string                     `json:"content"`
}

type LaunchLspResponsePayload struct {
	Success bool `json:"success"`
}
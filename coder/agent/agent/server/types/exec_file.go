package types

type ExecFiles struct {
	FileName string `json:"file_name"`
	Execute  bool   `json:"execute"`
	Code     string `json:"code"`
}

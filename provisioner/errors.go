package provisioner

type ResultParseError struct {
	s string
}

func NewResultParseError(s string) error {
	return &ResultParseError{s: s}
}

func (e *ResultParseError) Error() string {
	return e.s
}

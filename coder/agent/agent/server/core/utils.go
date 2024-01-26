package core

// StdoutForwardWriter used to forward the stdin content to stdout
type StdoutForwardWriter struct {
    C chan string
}

// NewStdoutForwardWriter creates a new instance of StdoutForwardWriter
func NewStdoutForwardWriter(stdout chan string) *StdoutForwardWriter {
	return &StdoutForwardWriter{stdout}
}

// Write implements the io.Writer interface for StdoutForwardWriter.
// It sends the data to the stdout channel and returns the number of bytes written.
func (cw *StdoutForwardWriter) Write(p []byte) (n int, err error) {
    // Send a copy of p to avoid external modification
    data := make([]byte, len(p))
    copy(data, p)

    // Send data to the channel
    cw.C <- string(data)

    // Return the number of bytes written and nil error
    return len(p), nil
}
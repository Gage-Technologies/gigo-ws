package utils

import (
	"io"
	"sync"
)

// ChannelReader is a custom io.Reader implementation that reads from a channel.
type ChannelReader struct {
	ch      chan []byte
	buf     []byte
	mu      sync.Mutex // protects buffer
	closed  bool
	closeCh chan struct{}
}

// NewChannelReader creates a new ChannelReader with the provided buffer size.
func NewChannelReader(bufferSize int) *ChannelReader {
	return &ChannelReader{
		ch:      make(chan []byte, bufferSize),
		closeCh: make(chan struct{}),
	}
}

// Read implements the io.Reader interface.
// It reads data from the channel into p.
func (r *ChannelReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed && len(r.buf) == 0 {
		return 0, io.EOF
	}

	if len(r.buf) == 0 {
		var ok bool
		select {
		case r.buf, ok = <-r.ch:
			if !ok {
				r.closed = true
				return 0, io.EOF
			}
		case <-r.closeCh:
			return 0, io.EOF
		}
	}

	n := copy(p, r.buf)
	r.buf = r.buf[n:]
	return n, nil
}

// WriteToChannel allows writing data to the channel.
func (r *ChannelReader) WriteToChannel(data []byte) {
	r.ch <- data
}

// Close closes the channel, no more data can be written after closing.
func (r *ChannelReader) Close() error {
	close(r.closeCh)
	close(r.ch)
	return nil
}
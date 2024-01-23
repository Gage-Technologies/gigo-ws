package utils

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestChannelReader_Read(t *testing.T) {
	tests := []struct {
		name      string
		writeData []string
		expected  string
	}{
		{
			name:      "single write",
			writeData: []string{"Hello, world!"},
			expected:  "Hello, world!",
		},
		{
			name:      "multiple writes",
			writeData: []string{"Hello, ", "world!"},
			expected:  "Hello, world!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := NewChannelReader(10)

			go func() {
				for _, data := range tt.writeData {
					reader.WriteToChannel([]byte(data))
				}
				reader.Close()
			}()

			var result strings.Builder
			buffer := make([]byte, 5) // smaller buffer to force multiple reads
			for {
				n, err := reader.Read(buffer)
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Errorf("Read() error = %v", err)
					return
				}
				result.Write(buffer[:n])
			}

			if got := result.String(); got != tt.expected {
				t.Errorf("Read() got = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestChannelReader_Close(t *testing.T) {
	reader := NewChannelReader(10)
	dataToWrite := "Hello"
	go func() {
		reader.WriteToChannel([]byte(dataToWrite))
		reader.Close()
	}()

	// Allow some time for the goroutine to run
	time.Sleep(100 * time.Millisecond)

	buffer := make([]byte, len(dataToWrite))
	n, err := reader.Read(buffer)
	if err != nil {
		t.Errorf("Read() error = %v", err)
		return
	}
	if got := string(buffer[:n]); got != dataToWrite {
		t.Errorf("Read() got = %v, want %v", got, dataToWrite)
	}

	// Attempting another read should result in io.EOF
	_, err = reader.Read(buffer)
	if err != io.EOF {
		t.Errorf("Expected io.EOF after close, but got %v", err)
	}
}

// Package output holds OutputSink adapters.
package output

import (
	"os"
	"strings"
)

// BufferSink collects the rendered prompt in memory (used by tests/harness).
type BufferSink struct {
	B strings.Builder
}

// Write implements domain.OutputSink.
func (s *BufferSink) Write(p string) error {
	s.B.WriteString(p)
	return nil
}

// String returns the collected output.
func (s *BufferSink) String() string { return s.B.String() }

// StdoutSink writes the rendered prompt to standard output.
type StdoutSink struct{}

// Write implements domain.OutputSink.
func (StdoutSink) Write(p string) error {
	_, err := os.Stdout.WriteString(p)
	return err
}

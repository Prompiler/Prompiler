// file.go adds a FileSink: an OutputSink that writes the rendered prompt to a
// file (create-or-truncate, mode 0644).
package output

import "os"

// FileSink writes the rendered prompt to a file.
type FileSink struct {
	path string
}

// NewFileSink returns a FileSink writing to path.
func NewFileSink(path string) *FileSink {
	return &FileSink{path: path}
}

// Write implements domain.OutputSink by writing p to the configured path.
func (s *FileSink) Write(p string) error {
	return os.WriteFile(s.path, []byte(p), 0o644)
}

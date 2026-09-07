// clipboard.go adds a ClipboardSink: an OutputSink that copies the rendered
// prompt to the OS clipboard through an injected ClipboardWriter seam so the
// real clipboard is never touched by tests.
package output

import "github.com/atotto/clipboard"

// ClipboardWriter is the seam the clipboard sink writes through.
type ClipboardWriter interface {
	WriteAll(text string) error
}

// RealClipboard writes to the system clipboard via atotto/clipboard.
type RealClipboard struct{}

// WriteAll implements ClipboardWriter.
func (RealClipboard) WriteAll(text string) error { return clipboard.WriteAll(text) }

// ClipboardSink writes the rendered prompt to the clipboard.
type ClipboardSink struct {
	w ClipboardWriter
}

// NewClipboardSink returns a ClipboardSink writing through w.
func NewClipboardSink(w ClipboardWriter) *ClipboardSink {
	return &ClipboardSink{w: w}
}

// Write implements domain.OutputSink.
func (s *ClipboardSink) Write(p string) error { return s.w.WriteAll(p) }

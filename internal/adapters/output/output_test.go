package output_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Jh123x/prompiler/internal/adapters/output"
)

func TestFileSink(t *testing.T) {
	t.Run("writes the prompt to a file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "out.txt")
		sink := output.NewFileSink(path)
		require.NoError(t, sink.Write("hello, world"))
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "hello, world", string(data))
	})

	t.Run("replaces existing file contents", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "out.txt")
		require.NoError(t, os.WriteFile(path, []byte("old"), 0o644))
		sink := output.NewFileSink(path)
		require.NoError(t, sink.Write("new"))
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "new", string(data))
	})

	t.Run("errors when the path is a directory", func(t *testing.T) {
		dir := t.TempDir()
		sink := output.NewFileSink(dir)
		require.Error(t, sink.Write("boom"))
	})
}

// fakeClipboard records the text written and returns a canned error.
type fakeClipboard struct {
	got   string
	calls int
	err   error
}

func (f *fakeClipboard) WriteAll(text string) error {
	f.calls++
	f.got = text
	return f.err
}

func TestClipboardSink(t *testing.T) {
	t.Run("forwards the prompt string", func(t *testing.T) {
		fake := &fakeClipboard{}
		sink := output.NewClipboardSink(fake)
		require.NoError(t, sink.Write("copy me"))
		assert.Equal(t, 1, fake.calls)
		assert.Equal(t, "copy me", fake.got)
	})

	t.Run("propagates a write error", func(t *testing.T) {
		boom := &fakeClipboard{err: os.ErrPermission}
		sink := output.NewClipboardSink(boom)
		err := sink.Write("copy me")
		require.Error(t, err)
		assert.ErrorIs(t, err, os.ErrPermission)
	})
}

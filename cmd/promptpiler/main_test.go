package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTUI records the roots the TUI is asked to start on.
type fakeTUI struct {
	roots []string
	err   error
}

func (f *fakeTUI) start(root string) error {
	f.roots = append(f.roots, root)
	return f.err
}

// writeFixture materializes files under a fresh temp dir and returns its path.
func writeFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	return dir
}

const greetSource = `
template Greet {
  variables {
    name: string
  }
  prompt {Hello, {{ name }}}
}
`

const typeMismatchSource = `
template Broken {
  variables {
    x: int
    y: string
  }
  prompt {{{ x + y }}}
}
`

func TestDispatch(t *testing.T) {
	t.Run("no args starts the TUI on the current directory", func(t *testing.T) {
		tui := &fakeTUI{}
		var stdout, stderr bytes.Buffer

		code := dispatch(nil, &stdout, &stderr, tui.start)

		assert.Equal(t, 0, code)
		assert.Equal(t, []string{"."}, tui.roots, "TUI must be started once on \".\"")
		assert.Empty(t, stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("no args propagates a TUI failure as exit 1", func(t *testing.T) {
		tui := &fakeTUI{err: os.ErrPermission}
		var stdout, stderr bytes.Buffer

		code := dispatch(nil, &stdout, &stderr, tui.start)

		assert.Equal(t, 1, code)
		assert.Contains(t, stderr.String(), "tui:")
	})

	t.Run("list prints the template names to stdout", func(t *testing.T) {
		dir := writeFixture(t, map[string]string{
			"Greet.ppl":      greetSource,
			"variables.json": `{"name": "Ada"}`,
		})
		var stdout, stderr bytes.Buffer

		code := dispatch([]string{"list", "-root", dir}, &stdout, &stderr, nil)

		assert.Equal(t, 0, code)
		assert.Equal(t, "Greet\n", stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("check reports a type error to stderr and exits 1", func(t *testing.T) {
		dir := writeFixture(t, map[string]string{"Broken.ppl": typeMismatchSource})
		var stdout, stderr bytes.Buffer

		code := dispatch([]string{"check", "-root", dir}, &stdout, &stderr, nil)

		assert.Equal(t, 1, code)
		assert.Empty(t, stdout.String())
		assert.Contains(t, stderr.String(), "type_mismatch")
	})

	t.Run("check passes on a valid root", func(t *testing.T) {
		dir := writeFixture(t, map[string]string{"Greet.ppl": greetSource})
		var stdout, stderr bytes.Buffer

		code := dispatch([]string{"check", "-root", dir}, &stdout, &stderr, nil)

		assert.Equal(t, 0, code)
		assert.Empty(t, stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("run renders the template to stdout", func(t *testing.T) {
		dir := writeFixture(t, map[string]string{
			"Greet.ppl":      greetSource,
			"variables.json": `{"name": "Ada"}`,
		})
		var stdout, stderr bytes.Buffer

		code := dispatch([]string{"run", "-root", dir, "Greet"}, &stdout, &stderr, nil)

		assert.Equal(t, 0, code)
		assert.Equal(t, "Hello, Ada\n", stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("run without a template name prints usage and exits 2", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		code := dispatch([]string{"run"}, &stdout, &stderr, nil)

		assert.Equal(t, 2, code)
		assert.Empty(t, stdout.String())
		assert.True(t, strings.Contains(stderr.String(), "usage"))
	})

	t.Run("unknown subcommand prints usage and exits 2", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		code := dispatch([]string{"frobnicate"}, &stdout, &stderr, nil)

		assert.Equal(t, 2, code)
		assert.Empty(t, stdout.String())
		assert.True(t, strings.Contains(stderr.String(), "usage"))
	})
}

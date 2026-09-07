// Package source holds SourceProvider adapters (in-memory and filesystem).
package source

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Jh123x/prompiler/internal/domain"
)

// MemSource serves modules from an in-memory map (path → source text).
type MemSource struct {
	Files map[string]string
}

// ListModules implements domain.SourceProvider.
func (m *MemSource) ListModules(root string) ([]string, error) {
	var paths []string
	for p := range m.Files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths, nil
}

// Read implements domain.SourceProvider.
func (m *MemSource) Read(root, path string) ([]byte, error) {
	if content, ok := m.Files[path]; ok {
		return []byte(content), nil
	}
	return nil, fmt.Errorf("module %q not found", path)
}

// NewFSSource is a Wire provider returning the filesystem source as a port.
func NewFSSource() domain.SourceProvider { return FSSource{} }

// FSSource serves modules from the filesystem under a project root.
type FSSource struct{}

// ListModules returns every .ppl path under root, relative to root.
func (FSSource) ListModules(root string) ([]string, error) {
	var paths []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".ppl") {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			paths = append(paths, rel)
		}
		return nil
	})
	sort.Strings(paths)
	return paths, err
}

// Read implements domain.SourceProvider.
func (FSSource) Read(root, path string) ([]byte, error) {
	return os.ReadFile(filepath.Join(root, path))
}

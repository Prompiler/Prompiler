package types_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Jh123x/prompiler/internal/ast"
	"github.com/Jh123x/prompiler/internal/builtin"
	"github.com/Jh123x/prompiler/internal/lexer"
	"github.com/Jh123x/prompiler/internal/parser"
	"github.com/Jh123x/prompiler/internal/resolver"
	"github.com/Jh123x/prompiler/internal/token"
	"github.com/Jh123x/prompiler/internal/types"
)

// typecheckFiles runs lex → parse → resolve → check and returns type-check
// diagnostics.
func typecheckFiles(t *testing.T, files map[string]string) []token.Diagnostic {
	t.Helper()
	parsed := map[string]*ast.File{}
	for path, src := range files {
		toks, _ := lexer.New(src).Lex()
		file, _ := parser.New(toks).ParseFile()
		parsed[path] = file
	}
	syms, rdiags := resolver.New(parsed).Resolve()
	require.Empty(t, rdiags)
	_, cdiags := types.NewChecker(builtin.NewRegistry()).Check(parsed, syms)
	return cdiags
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func TestFeatureAndScenarioTypecheckClean(t *testing.T) {
	t.Run("feature and scenario fixtures typecheck with zero diagnostics", func(t *testing.T) {
		for _, sub := range []string{"feature", "scenario"} {
			dirs := map[string][]string{}
			err := filepath.Walk("../../examples/"+sub, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !info.IsDir() && strings.HasSuffix(path, ".ppl") {
					dirs[filepath.Dir(path)] = append(dirs[filepath.Dir(path)], path)
				}
				return nil
			})
			require.NoError(t, err)

			for dir, paths := range dirs {
				files := map[string]string{}
				for _, p := range paths {
					files[filepath.Base(p)] = readFile(t, p)
				}
				assert.Empty(t, typecheckFiles(t, files), "%s: typecheck diagnostics", dir)
			}
		}
	})
}

type errorEntry struct {
	Stage    string `json:"stage"`
	Category string `json:"category"`
}

type solution struct {
	ExpectedErrors []errorEntry `json:"expected_errors"`
}

func TestTypecheckErrorFixtures(t *testing.T) {
	dirs, err := os.ReadDir("../../examples/errors")
	require.NoError(t, err)

	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		t.Run(d.Name(), func(t *testing.T) {
			dir := filepath.Join("../../examples/errors", d.Name())
			var sol solution
			require.NoError(t, json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "solution.json"))), &sol))

			var want []string
			for _, e := range sol.ExpectedErrors {
				if e.Stage == "typecheck" {
					want = append(want, e.Category)
				}
			}
			if len(want) == 0 {
				t.Skip("runtime fixture; checked in the evaluator tests")
			}

			ppl := readFile(t, filepath.Join(dir, d.Name()+".ppl"))
			diags := typecheckFiles(t, map[string]string{d.Name() + ".ppl": ppl})
			var got []string
			for _, diag := range diags {
				got = append(got, string(diag.Category))
			}
			sort.Strings(want)
			sort.Strings(got)
			assert.Equal(t, want, got)
		})
	}
}

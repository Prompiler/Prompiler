package types_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/bytedance/sonic"
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
			require.NoError(t, sonic.Unmarshal([]byte(readFile(t, filepath.Join(dir, "solution.json"))), &sol))

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

func TestGenericWrongArity(t *testing.T) {
	t.Run("reports wrong arity instead of panicking", func(t *testing.T) {
		src := `func id<T>(x: T): T { return x }
template T { variables { n: int } prompt { {{ id(n, 2) }} } }`
		diags := typecheckFiles(t, map[string]string{"t.ppl": src})
		require.NotEmpty(t, diags)
	})
}

func TestIncludeValidation(t *testing.T) {
	t.Run("rejects an unknown template", func(t *testing.T) {
		src := `template T { variables { a: string } prompt { {{ include Missing(a: a) }} } }`
		diags := typecheckFiles(t, map[string]string{"t.ppl": src})
		require.NotEmpty(t, diags)
	})

	t.Run("rejects a missing required child variable", func(t *testing.T) {
		files := map[string]string{
			"main.ppl": `import "child.ppl"
template Main { variables { c: string } prompt { {{ include Child(x: c) }} } }`,
			"child.ppl": `template Child {
  variables {
    x: string
    y: string
  }
  prompt { {{ x }} {{ y }} }
}`,
		}
		diags := typecheckFiles(t, files)
		require.NotEmpty(t, diags)
	})
}

func TestLengthOnPrimitive(t *testing.T) {
	t.Run("rejects .length on a non-string primitive", func(t *testing.T) {
		src := `template T { variables { n: int } prompt { {{ n.length }} } }`
		diags := typecheckFiles(t, map[string]string{"t.ppl": src})
		require.NotEmpty(t, diags)
	})
}

func TestFieldAssignment(t *testing.T) {
	t.Run("rejects field mutation", func(t *testing.T) {
		src := `class Point {
  x: int
  func bump(): int {
    this.x = this.x + 1
    return this.x
  }
}
template T { variables { p: Point } prompt { {{ p.bump() }} } }`
		diags := typecheckFiles(t, map[string]string{"t.ppl": src})
		require.NotEmpty(t, diags)
	})
}

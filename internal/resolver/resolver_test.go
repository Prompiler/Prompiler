package resolver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Jh123x/prompiler/internal/ast"
	"github.com/Jh123x/prompiler/internal/lexer"
	"github.com/Jh123x/prompiler/internal/parser"
	"github.com/Jh123x/prompiler/internal/token"
)

func parse(t *testing.T, src string) *ast.File {
	t.Helper()
	toks, _ := lexer.New(src).Lex()
	file, diags := parser.New(toks).ParseFile()
	require.Empty(t, diags)
	return file
}

func resolveSrcs(t *testing.T, files map[string]string) []token.Diagnostic {
	t.Helper()
	parsed := map[string]*ast.File{}
	for path, src := range files {
		parsed[path] = parse(t, src)
	}
	_, diags := New(parsed).Resolve()
	return diags
}

func TestScopeShadowing(t *testing.T) {
	t.Run("looks up through the parent scope", func(t *testing.T) {
		s := newScope(nil)
		s.define("x", &Symbol{Name: "x", Kind: KindLocal})
		got := newScope(s).lookup("x")
		require.NotNil(t, got)
		assert.Equal(t, KindLocal, got.Kind)
	})

	t.Run("allows shadowing an outer name", func(t *testing.T) {
		s := newScope(nil)
		s.define("x", &Symbol{Name: "x", Kind: KindLocal})
		assert.True(t, newScope(s).define("x", &Symbol{Name: "x", Kind: KindLocal}))
	})

	t.Run("rejects a same-scope duplicate", func(t *testing.T) {
		s := newScope(nil)
		s.define("x", &Symbol{Name: "x", Kind: KindLocal})
		assert.False(t, s.define("x", &Symbol{Name: "x", Kind: KindLocal}))
	})
}

func TestVarSelfReferenceResolvesOuter(t *testing.T) {
	t.Run("var x = x + 1 reads the outer binding", func(t *testing.T) {
		src := `func demo(n: int): int {
  var x = n
  if (x > 0) {
    var x = x + 1
    x = x * 2
  }
  return x
}`
		diags := resolveSrcs(t, map[string]string{"demo.ppl": src})
		assert.Empty(t, diags)
	})
}

func TestDuplicateParam(t *testing.T) {
	t.Run("duplicate parameter is a duplicate_name error", func(t *testing.T) {
		src := `func f(x: int, x: int): int { return x }`
		diags := resolveSrcs(t, map[string]string{"f.ppl": src})
		require.Len(t, diags, 1)
		assert.Equal(t, token.CatDuplicateName, diags[0].Category)
	})
}

func TestDuplicateGlobal(t *testing.T) {
	t.Run("duplicate top-level name is a duplicate_name error", func(t *testing.T) {
		src := "class A { x: int }\nclass A { y: int }\n"
		diags := resolveSrcs(t, map[string]string{"a.ppl": src})
		require.Len(t, diags, 1)
		assert.Equal(t, token.CatDuplicateName, diags[0].Category)
		assert.Contains(t, diags[0].Message, "a.ppl")
	})

	t.Run("duplicate across files lists both paths", func(t *testing.T) {
		diags := resolveSrcs(t, map[string]string{
			"b/named.ppl": "class Named { x: string }\n",
			"a/named.ppl": "class Named { y: string }\n",
		})
		require.Len(t, diags, 1)
		assert.Equal(t, token.CatDuplicateName, diags[0].Category)
		assert.Contains(t, diags[0].Message, "a/named.ppl")
		assert.Contains(t, diags[0].Message, "b/named.ppl")
	})
}

func TestMissingImport(t *testing.T) {
	t.Run("missing import is reported", func(t *testing.T) {
		src := `import "nonexistent.ppl"
template T { variables { x: int } prompt { {{ x }} } }`
		diags := resolveSrcs(t, map[string]string{"t.ppl": src})
		require.Len(t, diags, 1)
		assert.Equal(t, token.Category("missing_import"), diags[0].Category)
	})
}

func TestOnboardingEmailResolves(t *testing.T) {
	t.Run("multi-file scenario resolves its imports", func(t *testing.T) {
		dir := "../../examples/scenario/onboarding-email"
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)

		files := map[string]string{}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".ppl") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			require.NoError(t, err)
			files[e.Name()] = string(data)
		}
		diags := resolveSrcs(t, files)
		assert.Empty(t, diags)
	})
}

func TestAllExamplesResolveCleanly(t *testing.T) {
	t.Run("every example resolves with zero diagnostics", func(t *testing.T) {
		dirs := map[string][]string{}
		err := filepath.Walk("../../examples", func(path string, info os.FileInfo, err error) error {
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
			files := map[string]*ast.File{}
			for _, p := range paths {
				data, err := os.ReadFile(p)
				require.NoError(t, err)
				toks, _ := lexer.New(string(data)).Lex()
				file, pdiags := parser.New(toks).ParseFile()
				if !assert.Empty(t, pdiags, "%s: parse diagnostics", p) {
					continue
				}
				files[filepath.Base(p)] = file
			}
			_, diags := New(files).Resolve()
			assert.Empty(t, diags, "%s: resolve diagnostics", dir)
		}
	})
}

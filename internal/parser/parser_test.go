package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Jh123x/prompiler/internal/ast"
	"github.com/Jh123x/prompiler/internal/lexer"
	"github.com/Jh123x/prompiler/internal/token"
)

func parseSource(t *testing.T, src string) (*ast.File, []token.Diagnostic) {
	t.Helper()
	toks, ldiags := lexer.New(src).Lex()
	p := New(toks)
	file, pdiags := p.ParseFile()
	return file, append(ldiags, pdiags...)
}

func TestParseDeclarations(t *testing.T) {
	t.Run("parses every declaration form", func(t *testing.T) {
		src := `enum Difficulty { Easy, Medium, Hard }
class ReviewConfig {
  strictness: Difficulty
  files: string[]
  max_issues: int
  func is_strict(): bool { return this.strictness == Difficulty.Hard }
}
interface Named {
  name: string
  func label(): string
}
func double(x: int): int { return x * 2 }
template Greet {
  description: "hi"
  variables {
    name: string
    count: int = 1
  }
  prompt { Hello {{ name }} }
}`
		file, diags := parseSource(t, src)
		require.Empty(t, diags)
		require.Len(t, file.Decls, 5)

		assert.IsType(t, &ast.EnumDecl{}, file.Decls[0])
		assert.IsType(t, &ast.ClassDecl{}, file.Decls[1])
		assert.IsType(t, &ast.InterfaceDecl{}, file.Decls[2])
		assert.IsType(t, &ast.FuncDecl{}, file.Decls[3])

		tmpl, ok := file.Decls[4].(*ast.TemplateDecl)
		require.True(t, ok)
		assert.Equal(t, "hi", tmpl.Description)
		require.Len(t, tmpl.Variables, 2)
		assert.True(t, tmpl.Variables[1].HasDefault)
		assert.NotNil(t, tmpl.Prompt)
	})
}

func TestParseExpressions(t *testing.T) {
	t.Run("parses operator precedence", func(t *testing.T) {
		src := `func f(a: int, b: int, c: bool): int {
  return a + b * 2 - 3 / 1 % 2 == 1 && !c || a > b
}`
		_, diags := parseSource(t, src)
		require.Empty(t, diags)
	})
}

func TestParseStatements(t *testing.T) {
	t.Run("parses var, assignment, element assignment, if, for, and return", func(t *testing.T) {
		src := `func demo(n: int): int {
  var x = n
  x = x + 1
  arr[0] = x
  m["k"] = x
  if (x > 0) { x = x - 1 } else { return x }
  for (i in range(n)) { x = x + i }
  return x
}`
		_, diags := parseSource(t, src)
		require.Empty(t, diags)
	})
}

func TestParsePromptConstructs(t *testing.T) {
	t.Run("nests for, if, include, and interpolation segments", func(t *testing.T) {
		src := `template T {
  variables {
    items: string[]
    show: bool
  }
  prompt {
    Items:
    {% for item in items %}
    - {{ item }}
    {% end %}
    {% if show %}
    shown
    {% else %}
    hidden
    {% end %}
    {{ include Header(company: "acme") }}
  }
}`
		file, diags := parseSource(t, src)
		require.Empty(t, diags)
		tmpl := file.Decls[0].(*ast.TemplateDecl)
		require.NotNil(t, tmpl.Prompt)
		require.NotEmpty(t, tmpl.Prompt.Segments)

		var kinds []string
		var collect func([]ast.PromptSegment)
		collect = func(segs []ast.PromptSegment) {
			for _, seg := range segs {
				switch s := seg.(type) {
				case *ast.TextSegment:
					kinds = append(kinds, "text")
				case *ast.ForSegment:
					kinds = append(kinds, "for")
					collect(s.Body)
				case *ast.IfSegment:
					kinds = append(kinds, "if")
					collect(s.Then)
					collect(s.Else)
				case *ast.IncludeSegment:
					kinds = append(kinds, "include")
				case *ast.InterpSegment:
					kinds = append(kinds, "interp")
				}
			}
		}
		collect(tmpl.Prompt.Segments)
		joined := strings.Join(kinds, ",")
		for _, want := range []string{"for", "if", "include", "interp"} {
			assert.Contains(t, joined, want)
		}
	})
}

func TestParseEscaping(t *testing.T) {
	t.Run("resolves prompt escapes in text segments", func(t *testing.T) {
		src := `template E {
  variables { name: string }
  prompt {
    Literal: \{\{ name \}\}
    Backslash: C:\\path
    Brace: \}
  }
}`
		file, diags := parseSource(t, src)
		require.Empty(t, diags)
		seg := file.Decls[0].(*ast.TemplateDecl).Prompt.Segments[0].(*ast.TextSegment)
		assert.Contains(t, seg.Text, "{{ name }}")
	})
}

func TestParseErrorRecovery(t *testing.T) {
	t.Run("collects syntax errors without aborting", func(t *testing.T) {
		src := `template A { variables { x: } prompt { {{ } } }
enum Broken { A, , B }
func f(: int): int { return }
template B { prompt { {% for x in items %} }} }`
		_, diags := parseSource(t, src)
		assert.NotEmpty(t, diags, "expected syntax diagnostics")
	})
}

func TestAllExamplesParseCleanly(t *testing.T) {
	t.Run("every example parses with zero diagnostics", func(t *testing.T) {
		var files []string
		err := filepath.Walk("../../examples", func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && strings.HasSuffix(path, ".ppl") {
				files = append(files, path)
			}
			return nil
		})
		require.NoError(t, err)
		require.NotEmpty(t, files)

		for _, f := range files {
			data, err := os.ReadFile(f)
			require.NoError(t, err)
			toks, _ := lexer.New(string(data)).Lex()
			_, diags := New(toks).ParseFile()
			assert.Empty(t, diags, "%s: parse diagnostics", f)
		}
	})
}

package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(file.Decls) != 5 {
		t.Fatalf("expected 5 declarations, got %d", len(file.Decls))
	}
	if _, ok := file.Decls[0].(*ast.EnumDecl); !ok {
		t.Errorf("decl 0 not enum: %T", file.Decls[0])
	}
	if _, ok := file.Decls[1].(*ast.ClassDecl); !ok {
		t.Errorf("decl 1 not class: %T", file.Decls[1])
	}
	if _, ok := file.Decls[2].(*ast.InterfaceDecl); !ok {
		t.Errorf("decl 2 not interface: %T", file.Decls[2])
	}
	if _, ok := file.Decls[3].(*ast.FuncDecl); !ok {
		t.Errorf("decl 3 not func: %T", file.Decls[3])
	}
	tmpl, ok := file.Decls[4].(*ast.TemplateDecl)
	if !ok {
		t.Fatalf("decl 4 not template: %T", file.Decls[4])
	}
	if tmpl.Description != "hi" {
		t.Errorf("description = %q", tmpl.Description)
	}
	if len(tmpl.Variables) != 2 || !tmpl.Variables[1].HasDefault {
		t.Errorf("variables parsed wrong: %+v", tmpl.Variables)
	}
	if tmpl.Prompt == nil {
		t.Error("prompt not parsed")
	}
}

func TestParseExpressionsPrecedence(t *testing.T) {
	// Parse a func body with a chained expression; assert no diagnostics.
	src := `func f(a: int, b: int, c: bool): int {
  return a + b * 2 - 3 / 1 % 2 == 1 && !c || a > b
}`
	_, diags := parseSource(t, src)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestParseStatements(t *testing.T) {
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
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestParsePromptConstructs(t *testing.T) {
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
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	tmpl := file.Decls[0].(*ast.TemplateDecl)
	if tmpl.Prompt == nil || len(tmpl.Prompt.Segments) == 0 {
		t.Fatalf("prompt segments not parsed: %+v", tmpl.Prompt)
	}
	// Expect (recursively): Text, For, Interp, If, Include.
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
		if !strings.Contains(joined, want) {
			t.Errorf("prompt missing %q segment: %v", want, joined)
		}
	}
}

func TestParseEscaping(t *testing.T) {
	src := `template E {
  variables { name: string }
  prompt {
    Literal: \{\{ name \}\}
    Backslash: C:\\path
    Brace: \}
  }
}`
	file, diags := parseSource(t, src)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	seg := file.Decls[0].(*ast.TemplateDecl).Prompt.Segments[0].(*ast.TextSegment)
	if !strings.Contains(seg.Text, "{{ name }}") {
		t.Errorf("escaped braces not resolved: %q", seg.Text)
	}
}

func TestParseErrorRecovery(t *testing.T) {
	src := `template A { variables { x: } prompt { {{ } } }
enum Broken { A, , B }
func f(: int): int { return }
template B { prompt { {% for x in items %} }} }`
	_, diags := parseSource(t, src)
	if len(diags) == 0 {
		t.Fatal("expected syntax diagnostics, got none")
	}
}

func TestAllExamplesParseCleanly(t *testing.T) {
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
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no examples found")
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		toks, _ := lexer.New(string(data)).Lex()
		_, diags := New(toks).ParseFile()
		if len(diags) != 0 {
			t.Errorf("%s: %d parse diagnostics: %v", f, len(diags), diags)
		}
	}
}

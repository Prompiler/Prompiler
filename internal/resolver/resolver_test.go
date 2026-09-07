package resolver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jh123x/prompiler/internal/ast"
	"github.com/Jh123x/prompiler/internal/lexer"
	"github.com/Jh123x/prompiler/internal/parser"
	"github.com/Jh123x/prompiler/internal/token"
)

func parse(t *testing.T, src string) *ast.File {
	t.Helper()
	toks, _ := lexer.New(src).Lex()
	file, diags := parser.New(toks).ParseFile()
	if len(diags) != 0 {
		t.Fatalf("parse diagnostics: %v", diags)
	}
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
	s := newScope(nil)
	s.define("x", &Symbol{Name: "x", Kind: KindLocal})
	inner := newScope(s)
	if got := inner.lookup("x"); got == nil || got.Kind != KindLocal {
		t.Fatalf("lookup through parent failed: %v", got)
	}
	if !inner.define("x", &Symbol{Name: "x", Kind: KindLocal}) {
		t.Fatal("shadowing an outer name should be allowed")
	}
	if inner.define("x", &Symbol{Name: "x", Kind: KindLocal}) {
		t.Fatal("same-scope duplicate should be rejected")
	}
}

func TestVarSelfReferenceResolvesOuter(t *testing.T) {
	// `var x = x + 1` inside a block must resolve the RHS `x` to the outer scope
	// and must NOT be a duplicate-name error.
	src := `func demo(n: int): int {
  var x = n
  if (x > 0) {
    var x = x + 1
    x = x * 2
  }
  return x
}`
	diags := resolveSrcs(t, map[string]string{"demo.ppl": src})
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestDuplicateParam(t *testing.T) {
	src := `func f(x: int, x: int): int { return x }`
	diags := resolveSrcs(t, map[string]string{"f.ppl": src})
	if len(diags) != 1 || diags[0].Category != token.CatDuplicateName {
		t.Fatalf("expected one duplicate_name diagnostic, got %v", diags)
	}
}

func TestDuplicateGlobal(t *testing.T) {
	src := "class A { x: int }\nclass A { y: int }\n"
	diags := resolveSrcs(t, map[string]string{"a.ppl": src})
	if len(diags) != 1 || diags[0].Category != token.CatDuplicateName {
		t.Fatalf("expected one duplicate_name diagnostic, got %v", diags)
	}
}

func TestMissingImport(t *testing.T) {
	src := `import "nonexistent.ppl"
template T { variables { x: int } prompt { {{ x }} } }`
	diags := resolveSrcs(t, map[string]string{"t.ppl": src})
	if len(diags) != 1 || diags[0].Category != token.Category("missing_import") {
		t.Fatalf("expected one missing_import diagnostic, got %v", diags)
	}
}

func TestOnboardingEmailResolves(t *testing.T) {
	// Parse the multi-file scenario and resolve as one program.
	dir := "../../examples/scenario/onboarding-email"
	files := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".ppl") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		files[e.Name()] = string(data)
	}
	diags := resolveSrcs(t, files)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestAllExamplesResolveCleanly(t *testing.T) {
	// Group .ppl files by directory; each directory is one program.
	dirs := map[string][]string{}
	err := filepath.Walk("../../examples", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".ppl") {
			d := filepath.Dir(path)
			dirs[d] = append(dirs[d], path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	for dir, paths := range dirs {
		files := map[string]*ast.File{}
		for _, p := range paths {
			data, err := os.ReadFile(p)
			if err != nil {
				t.Fatalf("read %s: %v", p, err)
			}
			toks, _ := lexer.New(string(data)).Lex()
			file, pdiags := parser.New(toks).ParseFile()
			if len(pdiags) != 0 {
				t.Errorf("%s: parse diagnostics %v", p, pdiags)
				continue
			}
			files[filepath.Base(p)] = file
		}
		_, diags := New(files).Resolve()
		if len(diags) != 0 {
			t.Errorf("%s: %d resolve diagnostics: %v", dir, len(diags), diags)
		}
	}
}

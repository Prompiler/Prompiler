package types_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Jh123x/prompiler/internal/ast"
	"github.com/Jh123x/prompiler/internal/builtin"
	"github.com/Jh123x/prompiler/internal/lexer"
	"github.com/Jh123x/prompiler/internal/parser"
	"github.com/Jh123x/prompiler/internal/resolver"
	"github.com/Jh123x/prompiler/internal/token"
	"github.com/Jh123x/prompiler/internal/types"
)

// typecheckFiles runs lex → parse → resolve → check over a set of sources and
// returns only the type-check diagnostics.
func typecheckFiles(t *testing.T, files map[string]string) []token.Diagnostic {
	t.Helper()
	parsed := map[string]*ast.File{}
	for path, src := range files {
		toks, _ := lexer.New(src).Lex()
		file, _ := parser.New(toks).ParseFile()
		parsed[path] = file
	}
	syms, rdiags := resolver.New(parsed).Resolve()
	if len(rdiags) != 0 {
		t.Fatalf("resolve diagnostics: %v", rdiags)
	}
	_, cdiags := types.NewChecker(builtin.NewRegistry()).Check(parsed, syms)
	return cdiags
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestFeatureAndScenarioTypecheckClean(t *testing.T) {
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
		if err != nil {
			t.Fatalf("walk %s: %v", sub, err)
		}
		for dir, paths := range dirs {
			files := map[string]string{}
			for _, p := range paths {
				files[filepath.Base(p)] = readFile(t, p)
			}
			diags := typecheckFiles(t, files)
			if len(diags) != 0 {
				t.Errorf("%s: %d typecheck diagnostics: %v", dir, len(diags), diags)
			}
		}
	}
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
	if err != nil {
		t.Fatalf("read errors dir: %v", err)
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		dir := filepath.Join("../../examples/errors", d.Name())
		solData := readFile(t, filepath.Join(dir, "solution.json"))
		var sol solution
		if err := json.Unmarshal([]byte(solData), &sol); err != nil {
			t.Fatalf("%s: bad solution.json: %v", d.Name(), err)
		}
		var want []string
		for _, e := range sol.ExpectedErrors {
			if e.Stage == "typecheck" {
				want = append(want, e.Category)
			}
		}
		if len(want) == 0 {
			continue // runtime fixture; checked in M5
		}
		ppl := readFile(t, filepath.Join(dir, d.Name()+".ppl"))
		diags := typecheckFiles(t, map[string]string{d.Name() + ".ppl": ppl})
		var got []string
		for _, diag := range diags {
			got = append(got, string(diag.Category))
		}
		sort.Strings(want)
		sort.Strings(got)
		if strings.Join(want, ",") != strings.Join(got, ",") {
			t.Errorf("%s: expected typecheck categories %v, got %v", d.Name(), want, got)
		}
	}
}

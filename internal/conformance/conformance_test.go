// Package conformance is the executable fixture suite: it runs every
// examples/** fixture through the full pipeline (analyze → collect → render)
// and compares against solution.json.
package conformance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Jh123x/prompiler/internal/adapters/jsonvalue"
	"github.com/Jh123x/prompiler/internal/builtin"
	"github.com/Jh123x/prompiler/internal/domain"
	"github.com/Jh123x/prompiler/internal/eval"
	"github.com/Jh123x/prompiler/internal/token"
)

type errorEntry struct {
	Stage    string `json:"stage"`
	Category string `json:"category"`
}

type solution struct {
	Template       string       `json:"template"`
	ExpectedPrompt *string      `json:"expected_prompt"`
	ExpectedErrors []errorEntry `json:"expected_errors"`
}

func TestConformance(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test (fixture conformance) skipped in -short mode")
	}
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
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	for dir, paths := range dirs {
		solData, err := os.ReadFile(filepath.Join(dir, "solution.json"))
		if err != nil {
			continue
		}
		var sol solution
		if err := json.Unmarshal(solData, &sol); err != nil {
			t.Errorf("%s: bad solution.json: %v", dir, err)
			continue
		}

		src := domain.SourceSet{}
		for _, p := range paths {
			data, err := os.ReadFile(p)
			if err != nil {
				t.Errorf("%s: read %s: %v", dir, p, err)
				continue
			}
			src[filepath.Base(p)] = string(data)
		}
		prog, diags := domain.Analyze(dir, src, builtin.NewRegistry())

		if sol.ExpectedPrompt != nil {
			checkValid(t, dir, sol.Template, *sol.ExpectedPrompt, prog, diags)
		} else {
			checkErrors(t, dir, sol.Template, sol.ExpectedErrors, prog, diags)
		}
	}
}

func checkValid(t *testing.T, dir, name, want string, prog *domain.Program, diags []token.Diagnostic) {
	t.Helper()
	if len(diags) > 0 {
		t.Errorf("%s: expected clean analysis, got %d diagnostics", dir, len(diags))
		return
	}
	if prog == nil {
		t.Errorf("%s: no program", dir)
		return
	}
	inputs, err := collectInputs(dir, prog, name)
	if err != nil {
		t.Errorf("%s: %v", dir, err)
		return
	}
	out, rerr := eval.NewComposer(prog.Files, prog.Sem).Render(name, inputs)
	if rerr != nil {
		t.Errorf("%s: render error %s", dir, rerr.Category)
		return
	}
	if out != want {
		t.Errorf("%s:\n got %q\nwant %q", dir, out, want)
	}
}

func checkErrors(t *testing.T, dir, name string, expected []errorEntry, prog *domain.Program, diags []token.Diagnostic) {
	t.Helper()
	var want []string
	for _, e := range expected {
		want = append(want, e.Stage+":"+e.Category)
	}
	sort.Strings(want)

	var got []string
	if len(diags) > 0 {
		for _, d := range diags {
			got = append(got, string(d.Stage)+":"+string(d.Category))
		}
	} else if prog != nil {
		inputs, err := collectInputs(dir, prog, name)
		if err == nil {
			if _, rerr := eval.NewComposer(prog.Files, prog.Sem).Render(name, inputs); rerr != nil {
				got = append(got, "runtime:"+string(rerr.Category))
			}
		}
	}
	sort.Strings(got)
	if strings.Join(want, ",") != strings.Join(got, ",") {
		t.Errorf("%s: expected errors %v, got %v", dir, want, got)
	}
}

func collectInputs(dir string, prog *domain.Program, name string) (eval.InputValues, error) {
	varData, err := os.ReadFile(filepath.Join(dir, "variables.json"))
	if err != nil {
		return eval.InputValues{}, err
	}
	vs, err := jsonvalue.New(varData, prog.Sem.Env)
	if err != nil {
		return nil, err
	}
	var vars []domain.Variable
	for n, tt := range prog.Sem.TemplateVars[name] {
		vars = append(vars, domain.Variable{Name: n, Type: tt})
	}
	sort.Slice(vars, func(i, j int) bool { return vars[i].Name < vars[j].Name })
	return vs.Provide(vars)
}

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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		solData, err := os.ReadFile(filepath.Join(dir, "solution.json"))
		if err != nil {
			continue
		}
		var sol solution
		require.NoError(t, json.Unmarshal(solData, &sol))

		t.Run(filepath.Base(dir), func(t *testing.T) {
			src := domain.SourceSet{}
			for _, p := range paths {
				data, err := os.ReadFile(p)
				require.NoError(t, err)
				src[filepath.Base(p)] = string(data)
			}
			prog, diags := domain.Analyze(dir, src, builtin.NewRegistry())

			if sol.ExpectedPrompt != nil {
				checkValid(t, dir, sol.Template, *sol.ExpectedPrompt, prog, diags)
			} else {
				checkErrors(t, dir, sol.Template, sol.ExpectedErrors, prog, diags)
			}
		})
	}
}

func checkValid(t *testing.T, dir, name, want string, prog *domain.Program, diags []token.Diagnostic) {
	t.Helper()
	require.Empty(t, diags, "%s: expected clean analysis", dir)
	require.NotNil(t, prog, "%s: no program", dir)

	inputs, err := collectInputs(dir, prog, name)
	require.NoError(t, err, "%s: collect inputs", dir)

	out, rerr := eval.NewComposer(prog.Files, prog.Sem).Render(name, inputs)
	require.Nil(t, rerr, "%s: render error %s", dir, rerr)
	assert.Equal(t, want, out, "%s: rendered output", dir)
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
		if inputs, err := collectInputs(dir, prog, name); err == nil {
			if _, rerr := eval.NewComposer(prog.Files, prog.Sem).Render(name, inputs); rerr != nil {
				got = append(got, "runtime:"+string(rerr.Category))
			}
		}
	}
	sort.Strings(got)
	assert.Equal(t, want, got, "%s: expected errors", dir)
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

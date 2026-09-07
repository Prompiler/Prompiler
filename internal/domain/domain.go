// Package domain holds the orchestration: the Program aggregate, the Analyze
// factory, the ports (SourceProvider/ValueSource/OutputSink), and the
// application services. It is the composition root where stages and ports are
// wired via dependency injection.
package domain

import (
	"fmt"
	"sort"

	"github.com/Jh123x/prompiler/internal/ast"
	"github.com/Jh123x/prompiler/internal/eval"
	"github.com/Jh123x/prompiler/internal/lexer"
	"github.com/Jh123x/prompiler/internal/parser"
	"github.com/Jh123x/prompiler/internal/resolver"
	"github.com/Jh123x/prompiler/internal/token"
	"github.com/Jh123x/prompiler/internal/types"
)

// SourceSet is a set of in-memory source texts keyed by module path.
type SourceSet map[string]string

// Program is the immutable result of analysis: the parsed files, symbol table,
// and semantic model. Produced only by Analyze.
type Program struct {
	Root  string
	Files map[string]*ast.File
	Syms  *resolver.SymbolTable
	Sem   *types.SemanticModel
}

// Analyze runs lexer → parser → resolver → checker over a source set, returning
// either a Program or the first-stage diagnostics (never a half-checked
// program). The builtin surface is injected.
func Analyze(root string, src SourceSet, builtins types.Builtins) (*Program, []token.Diagnostic) {
	files := map[string]*ast.File{}
	var diags []token.Diagnostic
	for path, text := range src {
		toks, ldiags := lexer.New(text).Lex()
		diags = append(diags, ldiags...)
		file, pdiags := parser.New(toks).ParseFile()
		diags = append(diags, pdiags...)
		files[path] = file
	}
	if len(diags) > 0 {
		return nil, diags
	}
	syms, rdiags := resolver.New(files).Resolve()
	if len(rdiags) > 0 {
		return nil, rdiags
	}
	sem, cdiags := types.NewChecker(builtins).Check(files, syms)
	if len(cdiags) > 0 {
		return nil, cdiags
	}
	return &Program{Root: root, Files: files, Syms: syms, Sem: sem}, nil
}

// --- ports (owned by the core, implemented by adapters) ---

// SourceProvider resolves and reads module source text.
type SourceProvider interface {
	ListModules(root string) ([]string, error)
	Read(root, path string) ([]byte, error)
}

// ValueSource collects typed input values for a template's variables.
type ValueSource interface {
	Provide(vars []Variable) (eval.InputValues, error)
}

// OutputSink writes a rendered prompt.
type OutputSink interface {
	Write(prompt string) error
}

// Variable is a template input slot (name + type).
type Variable struct {
	Name string
	Type types.Type
}

// Application orchestrates the pure contexts against the real world. It is the
// only component that touches the ports; dependencies are injected.
type Application struct {
	Source   SourceProvider
	Builtins types.Builtins
}

// NewApplication wires the application (DI).
func NewApplication(source SourceProvider, builtins types.Builtins) *Application {
	return &Application{Source: source, Builtins: builtins}
}

// AnalyzeRoot loads all modules under root and analyzes them.
func (a *Application) AnalyzeRoot(root string) (*Program, []token.Diagnostic) {
	paths, err := a.Source.ListModules(root)
	if err != nil {
		return nil, []token.Diagnostic{{Stage: token.StageParse, Category: "io_error", Message: err.Error()}}
	}
	src := SourceSet{}
	for _, p := range paths {
		data, err := a.Source.Read(root, p)
		if err != nil {
			return nil, []token.Diagnostic{{Stage: token.StageParse, Category: "io_error", Message: err.Error()}}
		}
		src[p] = string(data)
	}
	return Analyze(root, src, a.Builtins)
}

// TemplateInfo is a summary of a template for listing.
type TemplateInfo struct {
	Path        string // module path (relative to root) that declares the template
	Name        string
	Description string
}

// ListTemplates enumerates the templates reachable from root, ordered by source
// path (then name, for files that declare multiple templates).
func (a *Application) ListTemplates(root string) ([]TemplateInfo, error) {
	prog, diags := a.AnalyzeRoot(root)
	if len(diags) > 0 {
		return nil, diagError(diags)
	}
	var out []TemplateInfo
	for path, f := range prog.Files {
		for _, d := range f.Decls {
			if td, ok := d.(*ast.TemplateDecl); ok {
				out = append(out, TemplateInfo{Path: path, Name: td.Name, Description: td.Description})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// CheckTemplates type-checks all templates under root.
func (a *Application) CheckTemplates(root string) ([]token.Diagnostic, error) {
	_, diags := a.AnalyzeRoot(root)
	return diags, nil
}

func diagError(diags []token.Diagnostic) error {
	if len(diags) == 0 {
		return fmt.Errorf("analysis failed")
	}
	d := diags[0]
	return fmt.Errorf("%s (%s): %s", d.Category, d.Stage, d.Message)
}

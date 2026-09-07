// Package domain holds the orchestration: the Program aggregate, the Analyze
// factory, the ports (SourceProvider/ValueSource/OutputSink), and the
// application services. It is the composition root where stages and ports are
// wired via dependency injection.
package domain

import (
	"fmt"
	"path"
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
	Name        string // empty when the entry is a whole-file syntax error
	Description string
	Diagnostics []token.Diagnostic // parse diagnostics for the containing file (nil when clean)
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

// DiscoverTemplates lists every template reachable from root, analyzing each
// containing directory as an independent program so a broken directory does not
// hide or poison the others. Templates whose directory fails to analyze (syntax
// or type error) are still listed, with Diagnostics populated so the TUI can
// flag them as errors.
func (a *Application) DiscoverTemplates(root string) ([]TemplateInfo, error) {
	paths, err := a.Source.ListModules(root)
	if err != nil {
		return nil, err
	}
	byDir := map[string][]string{}
	for _, p := range paths {
		d := path.Dir(p)
		byDir[d] = append(byDir[d], p)
	}
	var out []TemplateInfo
	for _, files := range byDir {
		sort.Strings(files)
		src := SourceSet{}
		for _, p := range files {
			data, err := a.Source.Read(root, p)
			if err != nil {
				return nil, err
			}
			src[p] = string(data)
		}
		prog, diags := Analyze("", src, a.Builtins)
		if len(diags) == 0 {
			for filePath, f := range prog.Files {
				for _, d := range f.Decls {
					if td, ok := d.(*ast.TemplateDecl); ok {
						out = append(out, TemplateInfo{Path: filePath, Name: td.Name, Description: td.Description})
					}
				}
			}
			continue
		}
		// Directory has errors: recover its templates via parse-only and attach
		// the diagnostics so the TUI can flag them.
		for _, p := range files {
			data, _ := a.Source.Read(root, p)
			file, pdiags := parseModule(string(data))
			decls := templateDecls(file)
			if len(decls) == 0 {
				if len(pdiags) > 0 {
					out = append(out, TemplateInfo{Path: p, Diagnostics: diags})
				}
				continue
			}
			for _, td := range decls {
				out = append(out, TemplateInfo{Path: p, Name: td.Name, Description: td.Description, Diagnostics: diags})
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

// parseModule lexes and parses a single module without running the resolver or
// checker.
func parseModule(src string) (*ast.File, []token.Diagnostic) {
	toks, ldiags := lexer.New(src).Lex()
	if len(ldiags) > 0 {
		return nil, ldiags
	}
	return parser.New(toks).ParseFile()
}

// templateDecls returns the template declarations in f, or nil when f is nil.
func templateDecls(f *ast.File) []*ast.TemplateDecl {
	if f == nil {
		return nil
	}
	var out []*ast.TemplateDecl
	for _, d := range f.Decls {
		if td, ok := d.(*ast.TemplateDecl); ok {
			out = append(out, td)
		}
	}
	return out
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

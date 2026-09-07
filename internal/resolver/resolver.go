// Package resolver builds the import graph, the top-level symbol table, and the
// lexical bindings of every identifier. It resolves name references (shadowing
// included) but does not type-check; the checker consumes its output.
package resolver

import (
	"fmt"
	"maps"
	"path"

	"github.com/Jh123x/prompiler/internal/ast"
	"github.com/Jh123x/prompiler/internal/token"
)

// SymbolKind classifies a binding.
type SymbolKind int

const (
	KindType        SymbolKind = iota // enum / class / interface
	KindFunc                          // free function
	KindTemplate                      // template
	KindParam                         // function/method parameter
	KindLocal                         // `var` local
	KindLoopVar                       // `for` loop variable
	KindTemplateVar                   // template variable
)

// Symbol is a named binding. Decl is set for top-level declarations.
type Symbol struct {
	Name string
	Kind SymbolKind
	Decl ast.Decl
}

// SymbolTable is the resolver's output: global top-level symbols and the
// per-identifier binding map consumed by the checker.
type SymbolTable struct {
	Globals  map[string]*Symbol
	Bindings map[*ast.Ident]*Symbol
}

// scope is a lexical scope in the shadowing chain.
type scope struct {
	parent *scope
	syms   map[string]*Symbol
}

func newScope(parent *scope) *scope {
	return &scope{parent: parent, syms: map[string]*Symbol{}}
}

// define adds a name to this scope, returning false on a same-scope duplicate.
func (s *scope) define(name string, sym *Symbol) bool {
	if _, ok := s.syms[name]; ok {
		return false
	}
	s.syms[name] = sym
	return true
}

// lookup walks outward for the nearest binding of name.
func (s *scope) lookup(name string) *Symbol {
	for sc := s; sc != nil; sc = sc.parent {
		if sym, ok := sc.syms[name]; ok {
			return sym
		}
	}
	return nil
}

// Resolver resolves a set of already-parsed files.
type Resolver struct {
	files    map[string]*ast.File
	globals  map[string]*Symbol
	bindings map[*ast.Ident]*Symbol
	diags    []token.Diagnostic
}

// New returns a Resolver over the given parsed files (path → file).
func New(files map[string]*ast.File) *Resolver {
	return &Resolver{
		files:    files,
		globals:  map[string]*Symbol{},
		bindings: map[*ast.Ident]*Symbol{},
	}
}

// Resolve runs import resolution, global collection, and scope resolution.
func (r *Resolver) Resolve() (*SymbolTable, []token.Diagnostic) {
	r.resolveImports()
	r.collectGlobals()
	r.resolveBodies()
	return &SymbolTable{Globals: r.globals, Bindings: r.bindings}, r.diags
}

func (r *Resolver) err(cat token.Category, span token.Span, msg string) {
	r.diags = append(r.diags, token.Diagnostic{
		Stage:    token.StageTypecheck,
		Category: cat,
		Span:     span,
		Message:  msg,
	})
}

// cleanImport resolves an import path relative to the importing file's directory.
func cleanImport(importingPath, importPath string) string {
	return path.Clean(path.Join(path.Dir(importingPath), importPath))
}

func (r *Resolver) resolveImports() {
	// Missing-import detection.
	for filePath, file := range r.files {
		for _, d := range file.Decls {
			imp, ok := d.(*ast.ImportDecl)
			if !ok {
				continue
			}
			target := cleanImport(filePath, imp.Path)
			if _, ok := r.files[target]; !ok {
				r.err(token.Category("missing_import"), imp.Span(), fmt.Sprintf("import %q not found", imp.Path))
			}
		}
	}

	// Cyclic-import detection (DFS over the import graph). Not fixture-asserted
	// but required by spec §2.1/§11.
	graph := make(map[string][]string, len(r.files))
	for filePath, file := range r.files {
		for _, d := range file.Decls {
			if imp, ok := d.(*ast.ImportDecl); ok {
				target := cleanImport(filePath, imp.Path)
				if _, ok := r.files[target]; ok {
					graph[filePath] = append(graph[filePath], target)
				}
			}
		}
	}
	state := map[string]int{} // 0=unvisited,1=in-progress,2=done
	var stack []string
	var dfs func(string)
	dfs = func(node string) {
		state[node] = 1
		stack = append(stack, node)
		for _, next := range graph[node] {
			switch state[next] {
			case 0:
				dfs(next)
			case 1:
				r.err(token.CatCyclicImport, r.files[node].Decls[0].Span(), fmt.Sprintf("cyclic import involving %q", next))
			}
		}
		stack = stack[:len(stack)-1]
		state[node] = 2
	}
	for filePath := range r.files {
		if state[filePath] == 0 {
			dfs(filePath)
		}
	}
}

func (r *Resolver) collectGlobals() {
	first := map[string]string{} // name → file path of the first declaration
	for filePath, file := range r.files {
		for _, d := range file.Decls {
			var name string
			var kind SymbolKind
			switch decl := d.(type) {
			case *ast.EnumDecl:
				name, kind = decl.Name, KindType
			case *ast.ClassDecl:
				name, kind = decl.Name, KindType
			case *ast.InterfaceDecl:
				name, kind = decl.Name, KindType
			case *ast.FuncDecl:
				name, kind = decl.Name, KindFunc
			case *ast.TemplateDecl:
				name, kind = decl.Name, KindTemplate
			default:
				continue
			}
			if _, dup := r.globals[name]; dup {
				r.err(token.CatDuplicateName, d.Span(), duplicateTopLevelMsg(name, first[name], filePath))
				continue
			}
			r.globals[name] = &Symbol{Name: name, Kind: kind, Decl: d}
			first[name] = filePath
		}
	}
}

// duplicateTopLevelMsg formats a duplicate top-level name diagnostic, naming
// the source file paths of both declarations.
func duplicateTopLevelMsg(name, a, b string) string {
	if a == b {
		return fmt.Sprintf("duplicate top-level name %q (declared in %q)", name, a)
	}
	if a > b {
		a, b = b, a
	}
	return fmt.Sprintf("duplicate top-level name %q (declared in %q and %q)", name, a, b)
}

func (r *Resolver) resolveBodies() {
	global := newScope(nil)
	maps.Copy(global.syms, r.globals)
	for _, file := range r.files {
		for _, d := range file.Decls {
			switch decl := d.(type) {
			case *ast.FuncDecl:
				r.resolveFunc(decl, global)
			case *ast.ClassDecl:
				for i := range decl.Methods {
					r.resolveMethod(&decl.Methods[i], global)
				}
			case *ast.TemplateDecl:
				r.resolveTemplate(decl, global)
			}
		}
	}
}

func (r *Resolver) resolveFunc(fn *ast.FuncDecl, global *scope) {
	sc := newScope(global)
	for i := range fn.Params {
		p := &fn.Params[i]
		if !sc.define(p.Name, &Symbol{Name: p.Name, Kind: KindParam}) {
			r.err(token.CatDuplicateName, fn.Span(), fmt.Sprintf("duplicate parameter %q", p.Name))
		}
	}
	r.resolveStmts(fn.Body, sc)
}

func (r *Resolver) resolveMethod(m *ast.MethodDecl, global *scope) {
	sc := newScope(global)
	for i := range m.Params {
		p := &m.Params[i]
		if !sc.define(p.Name, &Symbol{Name: p.Name, Kind: KindParam}) {
			r.err(token.CatDuplicateName, m.Span(), fmt.Sprintf("duplicate parameter %q", p.Name))
		}
	}
	r.resolveStmts(m.Body, sc)
}

func (r *Resolver) resolveTemplate(t *ast.TemplateDecl, global *scope) {
	sc := newScope(global)
	for i := range t.Variables {
		v := &t.Variables[i]
		if !sc.define(v.Name, &Symbol{Name: v.Name, Kind: KindTemplateVar}) {
			r.err(token.CatDuplicateName, v.Span(), fmt.Sprintf("duplicate variable %q", v.Name))
		}
	}
	if t.Prompt != nil {
		r.resolvePromptSegments(t.Prompt.Segments, sc)
	}
}

func (r *Resolver) resolveStmts(stmts []ast.Stmt, sc *scope) {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.VarStmt:
			// Resolve the initializer BEFORE defining the new binding, so
			// `var x = x + 1` reads the outer `x` (features.md P0 #3).
			r.resolveExpr(s.Init, sc)
			if !sc.define(s.Name, &Symbol{Name: s.Name, Kind: KindLocal}) {
				r.err(token.CatDuplicateName, s.Span(), fmt.Sprintf("duplicate name %q", s.Name))
			}
		case *ast.AssignStmt:
			r.resolveExpr(s.Target, sc)
			r.resolveExpr(s.Value, sc)
		case *ast.IfStmt:
			r.resolveExpr(s.Cond, sc)
			r.resolveStmts(s.Then, newScope(sc))
			r.resolveStmts(s.Else, newScope(sc))
		case *ast.ForStmt:
			r.resolveExpr(s.Iter, sc)
			loop := newScope(sc)
			loop.define(s.Var, &Symbol{Name: s.Var, Kind: KindLoopVar})
			r.resolveStmts(s.Body, loop)
		case *ast.ReturnStmt:
			r.resolveExpr(s.Value, sc)
		}
	}
}

func (r *Resolver) resolvePromptSegments(segs []ast.PromptSegment, sc *scope) {
	for _, seg := range segs {
		switch s := seg.(type) {
		case *ast.InterpSegment:
			r.resolveExpr(s.Expr, sc)
		case *ast.ForSegment:
			r.resolveExpr(s.Iter, sc)
			loop := newScope(sc)
			loop.define(s.Var, &Symbol{Name: s.Var, Kind: KindLoopVar})
			r.resolvePromptSegments(s.Body, loop)
		case *ast.IfSegment:
			r.resolveExpr(s.Cond, sc)
			r.resolvePromptSegments(s.Then, sc)
			r.resolvePromptSegments(s.Else, sc)
		case *ast.IncludeSegment:
			for i := range s.Args {
				r.resolveExpr(s.Args[i].Value, sc)
			}
		}
	}
}

func (r *Resolver) resolveExpr(e ast.Expr, sc *scope) {
	switch x := e.(type) {
	case *ast.Ident:
		if x.Name == "this" {
			return // validated by the checker (this_outside_method)
		}
		if sym := sc.lookup(x.Name); sym != nil {
			r.bindings[x] = sym
		}
		// Unknown names are left unbound; the checker reports them (with
		// knowledge of builtin functions/types).
	case *ast.FieldAccess:
		r.resolveExpr(x.Recv, sc) // the field name is not a binding
	case *ast.Index:
		r.resolveExpr(x.Recv, sc)
		r.resolveExpr(x.Idx, sc)
	case *ast.Call:
		r.resolveExpr(x.Callee, sc)
		for _, a := range x.Args {
			r.resolveExpr(a, sc)
		}
	case *ast.MethodCall:
		r.resolveExpr(x.Recv, sc)
		for _, a := range x.Args {
			r.resolveExpr(a, sc)
		}
	case *ast.Unary:
		r.resolveExpr(x.Operand, sc)
	case *ast.Binary:
		r.resolveExpr(x.L, sc)
		r.resolveExpr(x.R, sc)
	case *ast.ArrayLit:
		for _, el := range x.Elems {
			r.resolveExpr(el, sc)
		}
	case *ast.MapLit:
		for _, en := range x.Entries {
			r.resolveExpr(en.Key, sc)
			r.resolveExpr(en.Value, sc)
		}
	case *ast.ClassLit:
		for _, f := range x.Fields {
			r.resolveExpr(f.Value, sc)
		}
	}
}

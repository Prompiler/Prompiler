package types

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Jh123x/prompiler/internal/ast"
	"github.com/Jh123x/prompiler/internal/resolver"
	"github.com/Jh123x/prompiler/internal/token"
)

// SemanticModel is the checker's query surface: the type environment, the type
// of every expression, and the resolved callee of every call.
type SemanticModel struct {
	Env   *Env
	Types map[ast.Expr]Type
	Calls map[*ast.Call]*FuncSig

	// TemplateVars maps each template to its variables' types (used by the
	// renderer to evaluate enum-member and `none` defaults).
	TemplateVars map[string]map[string]Type
}

// TypeChecker performs full static analysis over resolved files.
type TypeChecker struct {
	builtins Builtins
	env      *Env
	syms     *resolver.SymbolTable
	sm       *SemanticModel
	diags    []token.Diagnostic

	thisType   Type
	returnType Type
	inMethod   bool

	typeParams      map[string]*TypeVar
	savedTypeParams []map[string]*TypeVar
	templates       map[string]*ast.TemplateDecl
}

// pushTypeParams extends the current type-parameter scope with params (for a
// generic class/interface/function/method), resolving bounds after registration.
func (tc *TypeChecker) pushTypeParams(params []ast.TypeParam, out *[]*TypeVar) {
	newScope := map[string]*TypeVar{}
	for k, v := range tc.typeParams {
		newScope[k] = v
	}
	tc.savedTypeParams = append(tc.savedTypeParams, tc.typeParams)
	tc.typeParams = newScope
	for _, tp := range params {
		tv := &TypeVar{Name: tp.Name}
		tc.typeParams[tp.Name] = tv
		*out = append(*out, tv)
	}
	for i, tp := range params {
		if tp.Bound != nil {
			(*out)[i].Bound = tc.resolveTypeRef(tp.Bound)
		}
	}
}

func (tc *TypeChecker) popTypeParams() {
	tc.typeParams = tc.savedTypeParams[len(tc.savedTypeParams)-1]
	tc.savedTypeParams = tc.savedTypeParams[:len(tc.savedTypeParams)-1]
}

// NewChecker returns a checker wired to the injected builtin surface (DI).
func NewChecker(b Builtins) *TypeChecker {
	return &TypeChecker{builtins: b}
}

// Check type-checks all files and returns the semantic model plus diagnostics.
func (tc *TypeChecker) Check(files map[string]*ast.File, syms *resolver.SymbolTable) (*SemanticModel, []token.Diagnostic) {
	tc.syms = syms
	tc.env = &Env{Types: map[string]Type{}, Funcs: map[string]*FuncSig{}, Methods: tc.builtins}
	tc.sm = &SemanticModel{Env: tc.env, Types: map[ast.Expr]Type{}, Calls: map[*ast.Call]*FuncSig{}, TemplateVars: map[string]map[string]Type{}}

	tc.collectTypes(files)
	tc.collectFuncs(files)
	tc.collectTemplateVars(files)
	tc.checkDecls(files)

	return tc.sm, tc.diags
}

func (tc *TypeChecker) err(cat token.Category, span token.Span, msg string) {
	tc.diags = append(tc.diags, token.Diagnostic{Stage: token.StageTypecheck, Category: cat, Span: span, Message: msg})
}

var invalidT = Invalid{}

// --- type/function collection (two-pass to allow forward + self references) ---

func (tc *TypeChecker) collectTypes(files map[string]*ast.File) {
	for _, file := range files {
		for _, d := range file.Decls {
			switch decl := d.(type) {
			case *ast.EnumDecl:
				tc.env.Types[decl.Name] = &Enum{Name: decl.Name, Members: decl.Members}
			case *ast.ClassDecl:
				tc.env.Types[decl.Name] = &Class{Name: decl.Name}
			case *ast.InterfaceDecl:
				tc.env.Types[decl.Name] = &Interface{Name: decl.Name}
			}
		}
	}
	for _, file := range files {
		for _, d := range file.Decls {
			switch decl := d.(type) {
			case *ast.ClassDecl:
				c := tc.env.Types[decl.Name].(*Class)
				tc.pushTypeParams(decl.TypeParams, &c.TypeParams)
				for _, f := range decl.Fields {
					ft := tc.resolveTypeRef(f.Type)
					c.Fields = append(c.Fields, FieldInfo{Name: f.Name, Type: ft})
				}
				for i := range decl.Methods {
					c.Methods = append(c.Methods, tc.resolveMethodSig(&decl.Methods[i]))
				}
				tc.popTypeParams()
				tc.checkRecursiveFields(c, decl)
			case *ast.InterfaceDecl:
				i := tc.env.Types[decl.Name].(*Interface)
				tc.pushTypeParams(decl.TypeParams, &i.TypeParams)
				for _, f := range decl.Fields {
					i.Fields = append(i.Fields, FieldInfo{Name: f.Name, Type: tc.resolveTypeRef(f.Type)})
				}
				for j := range decl.Methods {
					i.Methods = append(i.Methods, tc.resolveMethodSig(&decl.Methods[j]))
				}
				tc.popTypeParams()
			}
		}
	}
}

func (tc *TypeChecker) checkRecursiveFields(c *Class, decl *ast.ClassDecl) {
	for _, f := range c.Fields {
		if Equal(f.Type, c) {
			tc.err(token.CatTypeMismatch, decl.Span(), fmt.Sprintf("directly recursive field %q (must be behind a collection or Optional)", f.Name))
		}
	}
}

func (tc *TypeChecker) resolveMethodSig(m *ast.MethodDecl) *MethodSig {
	sig := &MethodSig{Name: m.Name}
	tc.pushTypeParams(m.TypeParams, &sig.TypeParams)
	for _, p := range m.Params {
		sig.Params = append(sig.Params, tc.resolveTypeRef(p.Type))
	}
	sig.Return = tc.resolveTypeRef(m.Return)
	tc.popTypeParams()
	return sig
}

func (tc *TypeChecker) collectFuncs(files map[string]*ast.File) {
	for _, file := range files {
		for _, d := range file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok {
				continue
			}
			sig := &FuncSig{Name: fn.Name}
			tc.pushTypeParams(fn.TypeParams, &sig.TypeParams)
			for _, p := range fn.Params {
				sig.Params = append(sig.Params, tc.resolveTypeRef(p.Type))
			}
			sig.Return = tc.resolveTypeRef(fn.Return)
			tc.popTypeParams()
			tc.env.Funcs[fn.Name] = sig
		}
	}
}

func (tc *TypeChecker) resolveTypeRef(tr ast.TypeRef) Type {
	switch t := tr.(type) {
	case *ast.NamedType:
		switch t.Name {
		case "string":
			return TypeString
		case "int":
			return TypeInt
		case "float":
			return TypeFloat
		case "bool":
			return TypeBool
		}
		if tv, ok := tc.typeParams[t.Name]; ok {
			return tv
		}
		if typ, ok := tc.env.Types[t.Name]; ok {
			if len(t.TypeArgs) > 0 {
				return tc.instantiate(typ, t.TypeArgs, tr.Span())
			}
			return typ
		}
		tc.err(token.CatUnknownType, tr.Span(), fmt.Sprintf("unknown type %q", t.Name))
		return invalidT
	case *ast.ArrayType:
		return &Array{Elem: tc.resolveTypeRef(t.Elem)}
	case *ast.MapType:
		return &Map{Value: tc.resolveTypeRef(t.Value)}
	case *ast.OptionalType:
		return &Optional{Elem: tc.resolveTypeRef(t.Elem)}
	}
	return invalidT
}

// --- declarations ---

func (tc *TypeChecker) checkDecls(files map[string]*ast.File) {
	for _, file := range files {
		for _, d := range file.Decls {
			switch decl := d.(type) {
			case *ast.EnumDecl:
				tc.checkEnum(decl)
			case *ast.ClassDecl:
				tc.checkClassMembers(decl)
				cls := tc.env.Types[decl.Name].(*Class)
				for i := range decl.Methods {
					tc.checkMethod(&decl.Methods[i], cls.Methods[i], cls)
				}
			case *ast.InterfaceDecl:
				tc.checkInterfaceMembers(decl)
			case *ast.FuncDecl:
				tc.checkFunc(decl)
			case *ast.TemplateDecl:
				tc.checkTemplate(decl)
			}
		}
	}
	tc.checkIncludeDAG(files)
}

func (tc *TypeChecker) checkEnum(e *ast.EnumDecl) {
	seen := map[string]bool{}
	for _, m := range e.Members {
		if seen[m] {
			tc.err(token.CatDuplicateName, e.Span(), fmt.Sprintf("duplicate enum member %q", m))
		}
		seen[m] = true
	}
}

func (tc *TypeChecker) checkClassMembers(c *ast.ClassDecl) {
	seen := map[string]bool{}
	for _, f := range c.Fields {
		if seen[f.Name] {
			tc.err(token.CatDuplicateMember, c.Span(), fmt.Sprintf("duplicate member %q", f.Name))
		}
		seen[f.Name] = true
	}
	for _, m := range c.Methods {
		if seen[m.Name] {
			tc.err(token.CatDuplicateMember, c.Span(), fmt.Sprintf("duplicate member %q", m.Name))
		}
		seen[m.Name] = true
	}
}

func (tc *TypeChecker) checkInterfaceMembers(i *ast.InterfaceDecl) {
	seen := map[string]bool{}
	for _, f := range i.Fields {
		if seen[f.Name] {
			tc.err(token.CatDuplicateMember, i.Span(), fmt.Sprintf("duplicate member %q", f.Name))
		}
		seen[f.Name] = true
	}
	for _, m := range i.Methods {
		if seen[m.Name] {
			tc.err(token.CatDuplicateMember, i.Span(), fmt.Sprintf("duplicate member %q", m.Name))
		}
		seen[m.Name] = true
	}
}

func (tc *TypeChecker) checkMethod(m *ast.MethodDecl, sig *MethodSig, classType Type) {
	tc.thisType = classType
	tc.inMethod = true
	tc.returnType = sig.Return
	sc := newTypeScope(nil)
	for i, p := range m.Params {
		sc.define(p.Name, sig.Params[i])
	}
	tc.checkStmts(m.Body, sc)
	tc.inMethod = false
	tc.thisType = nil
}

func (tc *TypeChecker) checkFunc(fn *ast.FuncDecl) {
	sig := tc.env.Funcs[fn.Name]
	tc.returnType = sig.Return
	tc.inMethod = false
	sc := newTypeScope(nil)
	for i, p := range fn.Params {
		sc.define(p.Name, sig.Params[i])
	}
	tc.checkStmts(fn.Body, sc)
	tc.returnType = nil
}

// collectTemplateVars registers every template and its variables' types before
// bodies are checked, so include validation can see the whole template set.
func (tc *TypeChecker) collectTemplateVars(files map[string]*ast.File) {
	tc.templates = map[string]*ast.TemplateDecl{}
	for _, file := range files {
		for _, d := range file.Decls {
			if td, ok := d.(*ast.TemplateDecl); ok {
				tc.templates[td.Name] = td
				varTypes := map[string]Type{}
				for _, v := range td.Variables {
					varTypes[v.Name] = tc.resolveTypeRef(v.Type)
				}
				tc.sm.TemplateVars[td.Name] = varTypes
			}
		}
	}
}

func (tc *TypeChecker) checkTemplate(t *ast.TemplateDecl) {
	sc := newTypeScope(nil)
	varTypes := tc.sm.TemplateVars[t.Name]
	for i := range t.Variables {
		v := &t.Variables[i]
		vt := varTypes[v.Name]
		sc.define(v.Name, vt)
		if v.HasDefault {
			tc.checkDefault(v, vt)
		}
	}
	if t.Prompt != nil {
		tc.checkPromptSegments(t.Prompt.Segments, sc)
	}
}

func (tc *TypeChecker) checkDefault(v *ast.VarDecl, target Type) {
	switch e := v.Default.(type) {
	case *ast.NoneLit:
		if _, ok := target.(*Optional); !ok {
			tc.err(token.CatDefaultNotAssignable, v.Span(), fmt.Sprintf("none is not assignable to %s", target))
		}
		return
	case *ast.Ident:
		if enum, ok := target.(*Enum); ok {
			if !enumHasMember(enum, e.Name) {
				tc.err(token.CatEnumDefaultNotMember, v.Span(), fmt.Sprintf("%s is not a member of %s", e.Name, enum.Name))
			}
			return
		}
	}
	dt := tc.checkExpr(v.Default, newTypeScope(nil))
	if !Assignable(dt, target) {
		tc.err(token.CatDefaultNotAssignable, v.Span(), fmt.Sprintf("default value of type %s is not assignable to %s", dt, target))
	}
}

func enumHasMember(e *Enum, name string) bool {
	return slices.Contains(e.Members, name)
}

// --- statements ---

type typeScope struct {
	parent *typeScope
	types  map[string]Type
}

func newTypeScope(parent *typeScope) *typeScope {
	return &typeScope{parent: parent, types: map[string]Type{}}
}
func (s *typeScope) define(name string, t Type) { s.types[name] = t }
func (s *typeScope) lookup(name string) (Type, bool) {
	for sc := s; sc != nil; sc = sc.parent {
		if t, ok := sc.types[name]; ok {
			return t, true
		}
	}
	return nil, false
}

func (tc *TypeChecker) checkStmts(stmts []ast.Stmt, sc *typeScope) {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.VarStmt:
			t := tc.checkExpr(s.Init, sc)
			sc.define(s.Name, t)
		case *ast.AssignStmt:
			tc.checkExpr(s.Target, sc)
			vt := tc.checkExpr(s.Value, sc)
			// element assignment target must be array/map; the value must be assignable to the element type.
			tc.checkAssignTarget(s.Target, vt, sc)
		case *ast.IfStmt:
			tc.checkCond(s.Cond, sc)
			tc.checkStmts(s.Then, newTypeScope(sc))
			tc.checkStmts(s.Else, newTypeScope(sc))
		case *ast.ForStmt:
			it := tc.checkExpr(s.Iter, sc)
			if arr, ok := it.(*Array); ok {
				loop := newTypeScope(sc)
				loop.define(s.Var, arr.Elem)
				tc.checkStmts(s.Body, loop)
			} else if !isInvalid(it) {
				tc.err(token.CatIterateNonArray, s.Span(), fmt.Sprintf("cannot iterate over %s", it))
			}
		case *ast.ReturnStmt:
			t := tc.checkExpr(s.Value, sc)
			if tc.returnType != nil {
				tc.requireAssignable(t, tc.returnType, s.Span())
			}
		}
	}
}

func (tc *TypeChecker) checkAssignTarget(target ast.Expr, vt Type, sc *typeScope) {
	idx, ok := target.(*ast.Index)
	if !ok {
		// plain assignment: the target must be an Ident; reassignment type is checked against its binding.
		if id, ok := target.(*ast.Ident); ok {
			if bound, ok := sc.lookup(id.Name); ok {
				tc.requireAssignable(vt, bound, target.Span())
			}
			return
		}
		tc.err(token.CatTypeMismatch, target.Span(), "assignment target must be a variable or index")
		return
	}
	recv := tc.checkExpr(idx.Recv, sc)
	switch r := recv.(type) {
	case *Array:
		tc.requireAssignable(vt, r.Elem, target.Span())
	case *Map:
		tc.requireAssignable(vt, r.Value, target.Span())
	default:
		if !isInvalid(recv) {
			tc.err(token.CatTypeMismatch, target.Span(), fmt.Sprintf("element assignment requires an array or map, got %s", recv))
		}
	}
}

func (tc *TypeChecker) checkCond(cond ast.Expr, sc *typeScope) {
	t := tc.checkExpr(cond, sc)
	if !Equal(t, TypeBool) && !isInvalid(t) {
		tc.err(token.CatTypeMismatch, cond.Span(), fmt.Sprintf("condition must be bool, got %s", t))
	}
}

func (tc *TypeChecker) checkPromptSegments(segs []ast.PromptSegment, sc *typeScope) {
	for _, seg := range segs {
		switch s := seg.(type) {
		case *ast.InterpSegment:
			t := tc.checkExpr(s.Expr, sc)
			if !isStringable(t) && !isInvalid(t) {
				tc.err(token.CatTypeMismatch, s.Span(), fmt.Sprintf("cannot interpolate %s", t))
			}
		case *ast.ForSegment:
			it := tc.checkExpr(s.Iter, sc)
			if arr, ok := it.(*Array); ok {
				loop := newTypeScope(sc)
				loop.define(s.Var, arr.Elem)
				tc.checkPromptSegments(s.Body, loop)
			} else if !isInvalid(it) {
				tc.err(token.CatIterateNonArray, s.Span(), fmt.Sprintf("cannot iterate over %s", it))
			}
		case *ast.IfSegment:
			tc.checkCond(s.Cond, sc)
			tc.checkPromptSegments(s.Then, sc)
			tc.checkPromptSegments(s.Else, sc)
		case *ast.IncludeSegment:
			child := tc.templates[s.Template]
			if child == nil {
				tc.err(token.CatUnknownName, s.Span(), fmt.Sprintf("unknown template %q", s.Template))
				break
			}
			childVarTypes := tc.sm.TemplateVars[s.Template]
			provided := map[string]bool{}
			for i := range s.Args {
				arg := &s.Args[i]
				vt, ok := childVarTypes[arg.Name]
				if !ok {
					tc.err(token.CatTypeMismatch, s.Span(), fmt.Sprintf("template %s has no variable %q", s.Template, arg.Name))
					continue
				}
				at := tc.checkExpr(arg.Value, sc)
				tc.requireAssignable(at, vt, arg.Value.Span())
				provided[arg.Name] = true
			}
			for _, v := range child.Variables {
				if !v.HasDefault && !provided[v.Name] {
					tc.err(token.CatTypeMismatch, s.Span(), fmt.Sprintf("missing required variable %q for template %s", v.Name, s.Template))
				}
			}
		}
	}
}

// isStringable reports whether a type may be interpolated (§8.1).
func isStringable(t Type) bool {
	switch t.(type) {
	case Primitive, *Enum:
		return true
	}
	return false
}

func isInvalid(t Type) bool {
	_, ok := t.(Invalid)
	return ok
}

// --- expressions ---

func (tc *TypeChecker) checkExpr(e ast.Expr, sc *typeScope) Type {
	t := tc.checkExprInner(e, sc)
	tc.sm.Types[e] = t
	return t
}

func (tc *TypeChecker) checkExprInner(e ast.Expr, sc *typeScope) Type {
	switch x := e.(type) {
	case *ast.IntLit:
		return TypeInt
	case *ast.FloatLit:
		return TypeFloat
	case *ast.StringLit:
		return TypeString
	case *ast.BoolLit:
		return TypeBool
	case *ast.NoneLit:
		return &Optional{Elem: nil}
	case *ast.ArrayLit:
		var elem Type
		for i, el := range x.Elems {
			t := tc.checkExpr(el, sc)
			if isInvalid(t) {
				continue
			}
			if elem == nil {
				elem = t
			} else if !Equal(elem, t) {
				tc.err(token.CatTypeMismatch, el.Span(), fmt.Sprintf("array elements must share a type (got %s and %s)", elem, t))
			}
			_ = i
		}
		if elem == nil {
			elem = invalidT
		}
		return &Array{Elem: elem}
	case *ast.MapLit:
		var vt Type
		for _, en := range x.Entries {
			kt := tc.checkExpr(en.Key, sc)
			if !Equal(kt, TypeString) && !isInvalid(kt) {
				tc.err(token.CatTypeMismatch, en.Key.Span(), fmt.Sprintf("map keys must be strings, got %s", kt))
			}
			vt2 := tc.checkExpr(en.Value, sc)
			if isInvalid(vt2) {
				continue
			}
			if vt == nil {
				vt = vt2
			} else if !Equal(vt, vt2) {
				tc.err(token.CatTypeMismatch, en.Value.Span(), fmt.Sprintf("map values must share a type (got %s and %s)", vt, vt2))
			}
		}
		if vt == nil {
			vt = invalidT
		}
		return &Map{Value: vt}
	case *ast.ClassLit:
		typ, ok := tc.env.Types[x.TypeName]
		if !ok {
			tc.err(token.CatUnknownType, x.Span(), fmt.Sprintf("unknown type %q", x.TypeName))
			return invalidT
		}
		cls, ok := typ.(*Class)
		if !ok {
			tc.err(token.CatTypeMismatch, x.Span(), fmt.Sprintf("%s is not a class", x.TypeName))
			return invalidT
		}
		for _, f := range x.Fields {
			ft, ok := cls.FieldType(f.Name)
			if !ok {
				tc.err(token.CatTypeMismatch, x.Span(), fmt.Sprintf("class %s has no field %q", x.TypeName, f.Name))
				continue
			}
			fv := tc.checkExpr(f.Value, sc)
			tc.requireAssignable(fv, ft, f.Value.Span())
		}
		return cls
	case *ast.Ident:
		if x.Name == "this" {
			if !tc.inMethod {
				tc.err(token.CatThisOutsideMethod, x.Span(), "`this` used outside a method body")
				return invalidT
			}
			return tc.thisType
		}
		if t, ok := sc.lookup(x.Name); ok {
			return t
		}
		tc.err(token.CatUnknownName, x.Span(), fmt.Sprintf("unknown name %q", x.Name))
		return invalidT
	case *ast.FieldAccess:
		return tc.checkFieldAccess(x, sc)
	case *ast.Index:
		return tc.checkIndex(x, sc)
	case *ast.Call:
		return tc.checkCall(x, sc)
	case *ast.MethodCall:
		return tc.checkMethodCall(x, sc)
	case *ast.Unary:
		return tc.checkUnary(x, sc)
	case *ast.Binary:
		return tc.checkBinary(x, sc)
	}
	return invalidT
}

func (tc *TypeChecker) checkFieldAccess(x *ast.FieldAccess, sc *typeScope) Type {
	// Enum-member access: `Type.Member` where Type is an enum.
	if id, ok := x.Recv.(*ast.Ident); ok {
		if t, ok := tc.env.Types[id.Name]; ok {
			if enum, ok := t.(*Enum); ok {
				if !enumHasMember(enum, x.Field) {
					tc.err(token.CatUnknownMethod, x.Span(), fmt.Sprintf("%s has no member %q", enum.Name, x.Field))
				}
				return enum
			}
			tc.err(token.CatUnknownMethod, x.Span(), fmt.Sprintf("cannot access member %q of type %s", x.Field, t))
			return invalidT
		}
	}
	recv := tc.checkExpr(x.Recv, sc)
	if isInvalid(recv) {
		return invalidT
	}
	// `length` property on string/array/map (but not numeric/bool primitives).
	if x.Field == "length" {
		switch r := recv.(type) {
		case Primitive:
			if r == TypeString {
				return TypeInt
			}
		case *Array, *Map:
			return TypeInt
		}
	}
	switch r := recv.(type) {
	case *Class:
		if ft, ok := r.FieldType(x.Field); ok {
			return ft
		}
	case *Interface:
		if ft, ok := r.FieldType(x.Field); ok {
			return ft
		}
	}
	tc.err(token.CatUnknownMethod, x.Span(), fmt.Sprintf("no field %q on %s", x.Field, recv))
	return invalidT
}

func (tc *TypeChecker) checkIndex(x *ast.Index, sc *typeScope) Type {
	recv := tc.checkExpr(x.Recv, sc)
	idx := tc.checkExpr(x.Idx, sc)
	if isInvalid(recv) || isInvalid(idx) {
		return invalidT
	}
	switch r := recv.(type) {
	case *Array:
		if !Equal(idx, TypeInt) {
			tc.err(token.CatTypeMismatch, x.Idx.Span(), fmt.Sprintf("array index must be int, got %s", idx))
		}
		return r.Elem
	case *Map:
		if !Equal(idx, TypeString) {
			tc.err(token.CatTypeMismatch, x.Idx.Span(), fmt.Sprintf("map index must be string, got %s", idx))
		}
		return r.Value
	}
	tc.err(token.CatTypeMismatch, x.Span(), fmt.Sprintf("cannot index %s", recv))
	return invalidT
}

func (tc *TypeChecker) checkCall(x *ast.Call, sc *typeScope) Type {
	id, ok := x.Callee.(*ast.Ident)
	if !ok {
		tc.err(token.CatTypeMismatch, x.Span(), "calling a non-function")
		return invalidT
	}
	switch id.Name {
	case "some":
		if len(x.Args) != 1 {
			tc.err(token.CatTypeMismatch, x.Span(), "some expects one argument")
			return invalidT
		}
		return &Optional{Elem: tc.checkExpr(x.Args[0], sc)}
	case "int", "float", "string", "bool":
		if len(x.Args) != 1 {
			tc.err(token.CatTypeMismatch, x.Span(), fmt.Sprintf("%s cast expects one argument", id.Name))
			return invalidT
		}
		argT := tc.checkExpr(x.Args[0], sc)
		tc.checkCast(id.Name, argT, x.Args[0].Span())
		return castResult(id.Name)
	}
	var sig *FuncSig
	if s, ok := tc.env.Funcs[id.Name]; ok {
		sig = s
	} else if s, ok := tc.builtins.LookupFunction(id.Name); ok {
		sig = &s
	} else {
		tc.err(token.CatUnknownName, x.Span(), fmt.Sprintf("unknown function %q", id.Name))
		return invalidT
	}
	if len(sig.TypeParams) > 0 {
		ret, inst := tc.checkGenericCallArgs(x.TypeArgs, x.Args, sig.TypeParams, sig.Params, sig.Return, x.Span(), sc)
		if inst != nil {
			tc.sm.Calls[x] = inst
		}
		return ret
	}
	tc.checkArgs(x.Args, sig, x.Span(), sc)
	tc.sm.Calls[x] = sig
	return sig.Return
}

// checkGenericCallArgs resolves or infers type arguments for a generic
// function/method, checks bounds, substitutes, and checks the arguments.
func (tc *TypeChecker) checkGenericCallArgs(typeArgs []ast.TypeRef, args []ast.Expr, typeParams []*TypeVar, params []Type, ret Type, span token.Span, sc *typeScope) (Type, *FuncSig) {
	if len(args) != len(params) {
		tc.err(token.CatTypeMismatch, span, fmt.Sprintf("expected %d arguments, got %d", len(params), len(args)))
		return invalidT, nil
	}
	argTypes := make([]Type, len(args))
	for i, a := range args {
		argTypes[i] = tc.checkExpr(a, sc)
	}
	subst := typeSubst{}
	if len(typeArgs) > 0 {
		if len(typeArgs) != len(typeParams) {
			tc.err(token.CatTypeMismatch, span, fmt.Sprintf("expected %d type arguments, got %d", len(typeParams), len(typeArgs)))
			return invalidT, nil
		}
		for i, a := range typeArgs {
			resolved := tc.resolveTypeRef(a)
			if typeParams[i].Bound != nil && !Assignable(resolved, typeParams[i].Bound) {
				tc.err(token.CatInterfaceUnsatisfied, span, fmt.Sprintf("%s does not satisfy bound %s", resolved, typeParams[i].Bound))
			}
			subst[typeParams[i].Name] = resolved
		}
	} else {
		for i, pt := range params {
			if !unify(pt, argTypes[i], subst) {
				tc.err(token.CatTypeMismatch, span, "cannot infer type arguments")
				return invalidT, nil
			}
		}
		for _, tp := range typeParams {
			if tp.Bound != nil {
				if concrete, ok := subst[tp.Name]; ok && !Assignable(concrete, tp.Bound) {
					tc.err(token.CatInterfaceUnsatisfied, span, fmt.Sprintf("%s does not satisfy bound %s", concrete, tp.Bound))
				}
			}
		}
	}
	inst := &FuncSig{Params: substituteTypes(params, subst), Return: substitute(ret, subst)}
	for i, a := range args {
		tc.requireAssignable(argTypes[i], inst.Params[i], a.Span())
	}
	return inst.Return, inst
}

func (tc *TypeChecker) checkMethodCall(x *ast.MethodCall, sc *typeScope) Type {
	recv := tc.checkExpr(x.Recv, sc)
	if isInvalid(recv) {
		return invalidT
	}
	var sig MethodSig
	switch r := recv.(type) {
	case *Class:
		if m, ok := r.Method(x.Method); ok {
			sig = *m
		} else {
			tc.err(token.CatUnknownMethod, x.Span(), fmt.Sprintf("no method %q on %s", x.Method, r.Name))
			return invalidT
		}
	case *Interface:
		if m, ok := r.Method(x.Method); ok {
			sig = *m
		} else {
			tc.err(token.CatUnknownMethod, x.Span(), fmt.Sprintf("no method %q on %s", x.Method, r.Name))
			return invalidT
		}
	default:
		if m, ok := tc.builtins.LookupMethod(recv, x.Method); ok {
			sig = m
		} else {
			tc.err(token.CatUnknownMethod, x.Span(), fmt.Sprintf("no method %q on %s", x.Method, recv))
			return invalidT
		}
	}
	if len(sig.TypeParams) > 0 {
		ret, _ := tc.checkGenericCallArgs(x.TypeArgs, x.Args, sig.TypeParams, sig.Params, sig.Return, x.Span(), sc)
		return ret
	}
	tc.checkArgs(x.Args, &FuncSig{Name: sig.Name, Params: sig.Params, Return: sig.Return}, x.Span(), sc)
	return sig.Return
}

func (tc *TypeChecker) checkArgs(args []ast.Expr, sig *FuncSig, span token.Span, sc *typeScope) {
	if len(args) != len(sig.Params) {
		tc.err(token.CatTypeMismatch, span, fmt.Sprintf("expected %d arguments, got %d", len(sig.Params), len(args)))
		return
	}
	for i, a := range args {
		at := tc.checkExpr(a, sc)
		tc.requireAssignable(at, sig.Params[i], a.Span())
	}
}

// requireAssignable reports an assignability failure, using the interface
// satisfaction category when a class fails to satisfy an interface target.
func (tc *TypeChecker) requireAssignable(src, dst Type, span token.Span) {
	if isInvalid(src) || isInvalid(dst) {
		return
	}
	if Assignable(src, dst) {
		return
	}
	if iface, ok := dst.(*Interface); ok {
		if c, ok := src.(*Class); ok {
			if reason, ok := Satisfies(c, iface); !ok {
				tc.err(token.CatInterfaceUnsatisfied, span, fmt.Sprintf("%s does not satisfy interface %s (%s)", src, iface.Name, reason))
				return
			}
		}
	}
	tc.err(token.CatTypeMismatch, span, fmt.Sprintf("cannot use %s as %s", src, dst))
}

func (tc *TypeChecker) checkCast(name string, argT Type, span token.Span) {
	if isInvalid(argT) {
		return
	}
	switch name {
	case "int":
		if !isOneOf(argT, TypeInt, TypeFloat, TypeString) {
			tc.err(token.CatTypeMismatch, span, fmt.Sprintf("cannot cast %s to int", argT))
		}
	case "float":
		if !isOneOf(argT, TypeInt, TypeFloat, TypeString) {
			tc.err(token.CatTypeMismatch, span, fmt.Sprintf("cannot cast %s to float", argT))
		}
	case "string":
		if !isStringable(argT) {
			tc.err(token.CatTypeMismatch, span, fmt.Sprintf("cannot cast %s to string", argT))
		}
	case "bool":
		if !isOneOf(argT, TypeBool, TypeString, TypeInt) {
			tc.err(token.CatTypeMismatch, span, fmt.Sprintf("cannot cast %s to bool", argT))
		}
	}
}

func isOneOf(t Type, ts ...Type) bool {
	for _, u := range ts {
		if Equal(t, u) {
			return true
		}
	}
	return false
}

func castResult(name string) Type {
	switch name {
	case "int":
		return TypeInt
	case "float":
		return TypeFloat
	case "string":
		return TypeString
	case "bool":
		return TypeBool
	}
	return invalidT
}

func (tc *TypeChecker) checkUnary(x *ast.Unary, sc *typeScope) Type {
	op := tc.checkExpr(x.Operand, sc)
	if isInvalid(op) {
		return invalidT
	}
	switch x.Op {
	case token.BANG:
		if !Equal(op, TypeBool) {
			tc.err(token.CatTypeMismatch, x.Span(), fmt.Sprintf("! requires bool, got %s", op))
			return invalidT
		}
		return TypeBool
	case token.MINUS:
		if !Equal(op, TypeInt) && !Equal(op, TypeFloat) {
			tc.err(token.CatTypeMismatch, x.Span(), fmt.Sprintf("unary - requires a number, got %s", op))
			return invalidT
		}
		return op
	}
	return invalidT
}

func (tc *TypeChecker) checkBinary(x *ast.Binary, sc *typeScope) Type {
	l := tc.checkExpr(x.L, sc)
	r := tc.checkExpr(x.R, sc)
	if isInvalid(l) || isInvalid(r) {
		return invalidT
	}
	switch x.Op {
	case token.OR_OR, token.AND_AND:
		if !Equal(l, TypeBool) || !Equal(r, TypeBool) {
			tc.err(token.CatTypeMismatch, x.Span(), fmt.Sprintf("logical operator requires bool operands, got %s and %s", l, r))
			return invalidT
		}
		return TypeBool
	case token.EQ_EQ, token.NOT_EQ:
		if !Equal(l, r) {
			tc.err(token.CatTypeMismatch, x.Span(), fmt.Sprintf("cannot compare %s and %s", l, r))
			return invalidT
		}
		return TypeBool
	case token.LT, token.LT_EQ, token.GT, token.GT_EQ:
		if !isNumeric(l) || !isNumeric(r) || !Equal(l, r) {
			tc.err(token.CatTypeMismatch, x.Span(), fmt.Sprintf("comparison requires two numbers of the same type, got %s and %s", l, r))
			return invalidT
		}
		return TypeBool
	case token.PLUS:
		switch {
		case Equal(l, TypeInt) && Equal(r, TypeInt):
			return TypeInt
		case Equal(l, TypeFloat) && Equal(r, TypeFloat):
			return TypeFloat
		case Equal(l, TypeString) && Equal(r, TypeString):
			return TypeString
		default:
			tc.err(token.CatTypeMismatch, x.Span(), fmt.Sprintf("operator + cannot be applied to %s and %s (no implicit coercion)", l, r))
			return invalidT
		}
	case token.MINUS, token.STAR:
		if !isNumeric(l) || !Equal(l, r) {
			tc.err(token.CatTypeMismatch, x.Span(), fmt.Sprintf("arithmetic requires two numbers of the same type, got %s and %s", l, r))
			return invalidT
		}
		return l
	case token.SLASH:
		if !isNumeric(l) || !Equal(l, r) {
			tc.err(token.CatTypeMismatch, x.Span(), fmt.Sprintf("division requires two numbers of the same type, got %s and %s", l, r))
			return invalidT
		}
		return TypeFloat // int/int yields float
	case token.PERCENT:
		if !Equal(l, TypeInt) || !Equal(r, TypeInt) {
			tc.err(token.CatTypeMismatch, x.Span(), fmt.Sprintf("%% requires int operands, got %s and %s", l, r))
			return invalidT
		}
		return TypeInt
	}
	return invalidT
}

func isNumeric(t Type) bool {
	return Equal(t, TypeInt) || Equal(t, TypeFloat)
}

// --- include DAG ---

func (tc *TypeChecker) checkIncludeDAG(files map[string]*ast.File) {
	graph := map[string][]string{}
	for _, file := range files {
		for _, d := range file.Decls {
			if t, ok := d.(*ast.TemplateDecl); ok && t.Prompt != nil {
				graph[t.Name] = collectIncludes(t.Prompt.Segments)
			}
		}
	}
	state := map[string]int{}
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
				cycle := append(append([]string{}, stack...), next)
				tc.err(token.CatCyclicInclude, token.Span{}, fmt.Sprintf("cyclic template reference: %s", joinPath(cycle)))
			}
		}
		stack = stack[:len(stack)-1]
		state[node] = 2
	}
	for name := range graph {
		if state[name] == 0 {
			dfs(name)
		}
	}
}

func collectIncludes(segs []ast.PromptSegment) []string {
	var out []string
	for _, seg := range segs {
		switch s := seg.(type) {
		case *ast.IncludeSegment:
			out = append(out, s.Template)
		case *ast.ForSegment:
			out = append(out, collectIncludes(s.Body)...)
		case *ast.IfSegment:
			out = append(out, collectIncludes(s.Then)...)
			out = append(out, collectIncludes(s.Else)...)
		}
	}
	return out
}

func joinPath(path []string) string {
	return strings.Join(path, " -> ")
}

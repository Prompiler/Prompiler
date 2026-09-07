package eval

import (
	"fmt"
	"strconv"

	"github.com/Jh123x/prompiler/internal/ast"
	"github.com/Jh123x/prompiler/internal/token"
	"github.com/Jh123x/prompiler/internal/types"
)

// Evaluator is a tree-walking interpreter over the type-checked AST.
type Evaluator struct {
	sem     *types.SemanticModel
	funcs   map[string]*ast.FuncDecl
	classes map[string]*ast.ClassDecl

	thisVal Value
	hasThis bool
}

// NewEvaluator wires the evaluator to the parsed files and semantic model (DI).
func NewEvaluator(files map[string]*ast.File, sem *types.SemanticModel) *Evaluator {
	e := &Evaluator{sem: sem, funcs: map[string]*ast.FuncDecl{}, classes: map[string]*ast.ClassDecl{}}
	for _, f := range files {
		for _, d := range f.Decls {
			switch decl := d.(type) {
			case *ast.FuncDecl:
				e.funcs[decl.Name] = decl
			case *ast.ClassDecl:
				e.classes[decl.Name] = decl
			}
		}
	}
	return e
}

// evalEnv is a lexical chain of value bindings.
type evalEnv struct {
	parent *evalEnv
	vals   map[string]Value
}

func newEvalEnv(parent *evalEnv) *evalEnv { return &evalEnv{parent: parent, vals: map[string]Value{}} }
func (e *evalEnv) define(name string, v Value) {
	e.vals[name] = v
}
func (e *evalEnv) lookup(name string) (Value, bool) {
	for env := e; env != nil; env = env.parent {
		if v, ok := env.vals[name]; ok {
			return v, true
		}
	}
	return Value{}, false
}
func (e *evalEnv) assign(name string, v Value) bool {
	for env := e; env != nil; env = env.parent {
		if _, ok := env.vals[name]; ok {
			env.vals[name] = v
			return true
		}
	}
	return false
}

// EvalExpr evaluates an expression to a value.
func (e *Evaluator) EvalExpr(expr ast.Expr, env *evalEnv) (Value, *RuntimeError) {
	switch x := expr.(type) {
	case *ast.IntLit:
		return IntVal(x.Value), nil
	case *ast.FloatLit:
		return FloatVal(x.Value), nil
	case *ast.StringLit:
		return StringVal(x.Value), nil
	case *ast.BoolLit:
		return BoolVal(x.Value), nil
	case *ast.NoneLit:
		if t, ok := e.sem.Types[expr].(*types.Optional); ok {
			return NoneVal(t.Elem), nil
		}
		return NoneVal(nil), nil
	case *ast.ArrayLit:
		elems := make([]Value, 0, len(x.Elems))
		for _, el := range x.Elems {
			v, err := e.EvalExpr(el, env)
			if err != nil {
				return Value{}, err
			}
			elems = append(elems, v)
		}
		var elem types.Type
		if len(elems) > 0 {
			elem = elems[0].typ
		} else if t, ok := e.sem.Types[expr].(*types.Array); ok {
			elem = t.Elem
		}
		return ArrayVal(elem, elems), nil
	case *ast.MapLit:
		keys := make([]string, 0, len(x.Entries))
		vals := map[string]Value{}
		for _, en := range x.Entries {
			kv, err := e.EvalExpr(en.Key, env)
			if err != nil {
				return Value{}, err
			}
			vv, err := e.EvalExpr(en.Value, env)
			if err != nil {
				return Value{}, err
			}
			keys = append(keys, kv.str)
			vals[kv.str] = vv
		}
		var vt types.Type
		if t, ok := e.sem.Types[expr].(*types.Map); ok {
			vt = t.Value
		}
		return MapVal(vt, keys, vals), nil
	case *ast.ClassLit:
		t, _ := e.sem.Types[expr].(*types.Class)
		fields := map[string]Value{}
		for _, f := range x.Fields {
			v, err := e.EvalExpr(f.Value, env)
			if err != nil {
				return Value{}, err
			}
			fields[f.Name] = v
		}
		return ObjectVal(t, fields), nil
	case *ast.Ident:
		if x.Name == "this" {
			return e.thisVal, nil
		}
		if v, ok := env.lookup(x.Name); ok {
			return v, nil
		}
		return Value{}, rterr(token.CatUnknownName, x.Span(), fmt.Sprintf("unknown name %q", x.Name))
	case *ast.FieldAccess:
		return e.evalFieldAccess(x, env)
	case *ast.Index:
		return e.evalIndex(x, env)
	case *ast.Call:
		return e.evalCall(x, env)
	case *ast.MethodCall:
		return e.evalMethodCall(x, env)
	case *ast.Unary:
		return e.evalUnary(x, env)
	case *ast.Binary:
		return e.evalBinary(x, env)
	}
	return Value{}, rterr(token.CatUnknownName, expr.Span(), "unhandled expression")
}

func (e *Evaluator) evalFieldAccess(x *ast.FieldAccess, env *evalEnv) (Value, *RuntimeError) {
	// Enum member access: `Type.Member`.
	if id, ok := x.Recv.(*ast.Ident); ok {
		if t, ok := e.sem.Env.Types[id.Name]; ok {
			if enum, ok := t.(*types.Enum); ok {
				return EnumVal(enum.Name, x.Field), nil
			}
		}
	}
	recv, err := e.EvalExpr(x.Recv, env)
	if err != nil {
		return Value{}, err
	}
	if x.Field == "length" {
		switch {
		case recv.isString():
			return IntVal(int64(len(recv.str))), nil
		case recv.arr != nil:
			return IntVal(int64(len(*recv.arr))), nil
		case recv.m != nil:
			return IntVal(int64(len(recv.m.keys))), nil
		}
	}
	if recv.obj != nil {
		if v, ok := recv.obj.fields[x.Field]; ok {
			return v, nil
		}
	}
	return Value{}, rterr(token.CatUnknownMethod, x.Span(), fmt.Sprintf("no field %q", x.Field))
}

func (e *Evaluator) evalIndex(x *ast.Index, env *evalEnv) (Value, *RuntimeError) {
	recv, err := e.EvalExpr(x.Recv, env)
	if err != nil {
		return Value{}, err
	}
	idx, err := e.EvalExpr(x.Idx, env)
	if err != nil {
		return Value{}, err
	}
	if recv.arr != nil {
		i := int(idx.i)
		arr := *recv.arr
		if i < 0 || i >= len(arr) {
			return Value{}, rterr(token.CatIndexOutOfBounds, x.Span(), fmt.Sprintf("index %d out of bounds", i))
		}
		return arr[i], nil
	}
	if recv.m != nil {
		if v, ok := recv.m.vals[idx.str]; ok {
			return v, nil
		}
		return Value{}, rterr(token.CatIndexOutOfBounds, x.Span(), fmt.Sprintf("map key %q not found", idx.str))
	}
	return Value{}, rterr(token.CatTypeMismatch, x.Span(), "cannot index")
}

func (e *Evaluator) evalCall(x *ast.Call, env *evalEnv) (Value, *RuntimeError) {
	id, ok := x.Callee.(*ast.Ident)
	if !ok {
		return Value{}, rterr(token.CatTypeMismatch, x.Span(), "calling a non-function")
	}
	args := make([]Value, 0, len(x.Args))
	for _, a := range x.Args {
		v, err := e.EvalExpr(a, env)
		if err != nil {
			return Value{}, err
		}
		args = append(args, v)
	}
	switch id.Name {
	case "some":
		return SomeVal(args[0]), nil
	case "int", "float", "string", "bool":
		return e.cast(id.Name, args[0], x.Span())
	}
	if fn := e.funcs[id.Name]; fn != nil {
		return e.callUserFunc(fn, args)
	}
	return e.callBuiltinFunc(id.Name, args, x.Span())
}

func (e *Evaluator) callUserFunc(fn *ast.FuncDecl, args []Value) (Value, *RuntimeError) {
	env := newEvalEnv(nil)
	for i, p := range fn.Params {
		env.define(p.Name, args[i])
	}
	ret, _, err := e.EvalStmts(fn.Body, env)
	return ret, err
}

func (e *Evaluator) evalMethodCall(x *ast.MethodCall, env *evalEnv) (Value, *RuntimeError) {
	recv, err := e.EvalExpr(x.Recv, env)
	if err != nil {
		return Value{}, err
	}
	args := make([]Value, 0, len(x.Args))
	for _, a := range x.Args {
		v, err := e.EvalExpr(a, env)
		if err != nil {
			return Value{}, err
		}
		args = append(args, v)
	}
	if c, ok := recv.typ.(*types.Class); ok {
		if cd := e.classes[c.Name]; cd != nil {
			for i := range cd.Methods {
				if cd.Methods[i].Name == x.Method {
					return e.callUserMethod(&cd.Methods[i], recv, args)
				}
			}
		}
	}
	return e.callBuiltinMethod(recv, x.Method, args, x.Span())
}

func (e *Evaluator) callUserMethod(m *ast.MethodDecl, recv Value, args []Value) (Value, *RuntimeError) {
	env := newEvalEnv(nil)
	env.define("this", recv)
	e.thisVal = recv
	e.hasThis = true
	for i, p := range m.Params {
		env.define(p.Name, args[i])
	}
	ret, _, err := e.EvalStmts(m.Body, env)
	e.hasThis = false
	return ret, err
}

func (e *Evaluator) evalUnary(x *ast.Unary, env *evalEnv) (Value, *RuntimeError) {
	op, err := e.EvalExpr(x.Operand, env)
	if err != nil {
		return Value{}, err
	}
	switch x.Op {
	case token.BANG:
		return BoolVal(!op.b), nil
	case token.MINUS:
		if op.isInt() {
			return IntVal(-op.i), nil
		}
		return FloatVal(-op.f), nil
	}
	return Value{}, rterr(token.CatTypeMismatch, x.Span(), "bad unary")
}

func (e *Evaluator) evalBinary(x *ast.Binary, env *evalEnv) (Value, *RuntimeError) {
	if x.Op == token.AND_AND {
		l, err := e.EvalExpr(x.L, env)
		if err != nil {
			return Value{}, err
		}
		if !l.b {
			return BoolVal(false), nil
		}
		r, err := e.EvalExpr(x.R, env)
		if err != nil {
			return Value{}, err
		}
		return BoolVal(r.b), nil
	}
	if x.Op == token.OR_OR {
		l, err := e.EvalExpr(x.L, env)
		if err != nil {
			return Value{}, err
		}
		if l.b {
			return BoolVal(true), nil
		}
		r, err := e.EvalExpr(x.R, env)
		if err != nil {
			return Value{}, err
		}
		return BoolVal(r.b), nil
	}

	l, err := e.EvalExpr(x.L, env)
	if err != nil {
		return Value{}, err
	}
	r, err := e.EvalExpr(x.R, env)
	if err != nil {
		return Value{}, err
	}
	switch x.Op {
	case token.PLUS:
		switch {
		case l.isInt():
			return IntVal(l.i + r.i), nil
		case l.isFloat():
			return FloatVal(l.f + r.f), nil
		case l.isString():
			return StringVal(l.str + r.str), nil
		}
	case token.MINUS:
		if l.isInt() {
			return IntVal(l.i - r.i), nil
		}
		return FloatVal(l.f - r.f), nil
	case token.STAR:
		if l.isInt() {
			return IntVal(l.i * r.i), nil
		}
		return FloatVal(l.f * r.f), nil
	case token.SLASH:
		if l.isInt() {
			if r.i == 0 {
				return Value{}, rterr(token.CatDivisionByZero, x.Span(), "division by zero")
			}
			return FloatVal(float64(l.i) / float64(r.i)), nil
		}
		if r.f == 0 {
			return Value{}, rterr(token.CatDivisionByZero, x.Span(), "division by zero")
		}
		return FloatVal(l.f / r.f), nil
	case token.PERCENT:
		if r.i == 0 {
			return Value{}, rterr(token.CatDivisionByZero, x.Span(), "modulo by zero")
		}
		return IntVal(l.i % r.i), nil
	case token.EQ_EQ:
		return BoolVal(valueEqual(l, r)), nil
	case token.NOT_EQ:
		return BoolVal(!valueEqual(l, r)), nil
	case token.LT:
		return BoolVal(valueCompare(l, r) < 0), nil
	case token.LT_EQ:
		return BoolVal(valueCompare(l, r) <= 0), nil
	case token.GT:
		return BoolVal(valueCompare(l, r) > 0), nil
	case token.GT_EQ:
		return BoolVal(valueCompare(l, r) >= 0), nil
	}
	return Value{}, rterr(token.CatTypeMismatch, x.Span(), "bad binary")
}

func valueCompare(l, r Value) int {
	if l.isInt() {
		switch {
		case l.i < r.i:
			return -1
		case l.i > r.i:
			return 1
		}
		return 0
	}
	switch {
	case l.f < r.f:
		return -1
	case l.f > r.f:
		return 1
	}
	return 0
}

func valueEqual(l, r Value) bool {
	if l.typ == nil || r.typ == nil {
		return l.typ == r.typ
	}
	if !types.Equal(l.typ, r.typ) {
		return false
	}
	switch {
	case l.isString():
		return l.str == r.str
	case l.isInt():
		return l.i == r.i
	case l.isFloat():
		return l.f == r.f
	case l.isBool():
		return l.b == r.b
	case l.enumName != "":
		return l.member == r.member
	case l.arr != nil:
		a, b := *l.arr, *r.arr
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if !valueEqual(a[i], b[i]) {
				return false
			}
		}
		return true
	case l.m != nil:
		if len(l.m.keys) != len(r.m.keys) {
			return false
		}
		for k, v := range l.m.vals {
			rv, ok := r.m.vals[k]
			if !ok || !valueEqual(v, rv) {
				return false
			}
		}
		return true
	case l.obj != nil:
		if len(l.obj.fields) != len(r.obj.fields) {
			return false
		}
		for k, v := range l.obj.fields {
			rv, ok := r.obj.fields[k]
			if !ok || !valueEqual(v, rv) {
				return false
			}
		}
		return true
	case l.opt != nil:
		if l.opt.present != r.opt.present {
			return false
		}
		if !l.opt.present {
			return true
		}
		return valueEqual(l.opt.value, r.opt.value)
	}
	return false
}

func (e *Evaluator) cast(name string, v Value, span token.Span) (Value, *RuntimeError) {
	switch name {
	case "int":
		switch {
		case v.isInt():
			return IntVal(v.i), nil
		case v.isFloat():
			return IntVal(int64(v.f)), nil
		case v.isString():
			n, err := strconv.ParseInt(v.str, 10, 64)
			if err != nil {
				return Value{}, rterr(token.CatFailedCast, span, fmt.Sprintf("cannot parse %q as int", v.str))
			}
			return IntVal(n), nil
		}
	case "float":
		switch {
		case v.isInt():
			return FloatVal(float64(v.i)), nil
		case v.isFloat():
			return FloatVal(v.f), nil
		case v.isString():
			f, err := strconv.ParseFloat(v.str, 64)
			if err != nil {
				return Value{}, rterr(token.CatFailedCast, span, fmt.Sprintf("cannot parse %q as float", v.str))
			}
			return FloatVal(f), nil
		}
	case "string":
		s, ok := Stringify(v)
		if !ok {
			return Value{}, rterr(token.CatFailedCast, span, "not stringable")
		}
		return StringVal(s), nil
	case "bool":
		switch {
		case v.isBool():
			return BoolVal(v.b), nil
		case v.isInt():
			switch v.i {
			case 1:
				return BoolVal(true), nil
			case 0:
				return BoolVal(false), nil
			}
			return Value{}, rterr(token.CatFailedCast, span, "bool(x) accepts int 1/0")
		case v.isString():
			switch v.str {
			case "true":
				return BoolVal(true), nil
			case "false":
				return BoolVal(false), nil
			}
			return Value{}, rterr(token.CatFailedCast, span, `bool(x) accepts "true"/"false"`)
		}
	}
	return Value{}, rterr(token.CatFailedCast, span, "failed cast")
}

// EvalStmts executes statements, returning (returnValue, returned, error).
func (e *Evaluator) EvalStmts(stmts []ast.Stmt, env *evalEnv) (Value, bool, *RuntimeError) {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.VarStmt:
			v, err := e.EvalExpr(s.Init, env)
			if err != nil {
				return Value{}, false, err
			}
			env.define(s.Name, v)
		case *ast.AssignStmt:
			v, err := e.EvalExpr(s.Value, env)
			if err != nil {
				return Value{}, false, err
			}
			if err := e.assign(s.Target, v, env); err != nil {
				return Value{}, false, err
			}
		case *ast.IfStmt:
			c, err := e.EvalExpr(s.Cond, env)
			if err != nil {
				return Value{}, false, err
			}
			branch := s.Else
			if c.b {
				branch = s.Then
			}
			if ret, returned, err := e.EvalStmts(branch, newEvalEnv(env)); err != nil || returned {
				return ret, returned, err
			}
		case *ast.ForStmt:
			it, err := e.EvalExpr(s.Iter, env)
			if err != nil {
				return Value{}, false, err
			}
			for _, el := range *it.arr {
				loop := newEvalEnv(env)
				loop.define(s.Var, el)
				if ret, returned, err := e.EvalStmts(s.Body, loop); err != nil {
					return Value{}, false, err
				} else if returned {
					return ret, true, nil
				}
			}
		case *ast.ReturnStmt:
			v, err := e.EvalExpr(s.Value, env)
			if err != nil {
				return Value{}, false, err
			}
			return v, true, nil
		}
	}
	return Value{}, false, nil
}

func (e *Evaluator) assign(target ast.Expr, v Value, env *evalEnv) *RuntimeError {
	switch t := target.(type) {
	case *ast.Ident:
		if !env.assign(t.Name, v) {
			return rterr(token.CatUnknownName, t.Span(), fmt.Sprintf("unknown name %q", t.Name))
		}
		return nil
	case *ast.Index:
		recv, err := e.EvalExpr(t.Recv, env)
		if err != nil {
			return err
		}
		idx, err := e.EvalExpr(t.Idx, env)
		if err != nil {
			return err
		}
		if recv.arr != nil {
			i := int(idx.i)
			arr := *recv.arr
			if i < 0 || i >= len(arr) {
				return rterr(token.CatIndexOutOfBounds, t.Span(), fmt.Sprintf("index %d out of bounds", i))
			}
			arr[i] = v
			return nil
		}
		if recv.m != nil {
			if _, ok := recv.m.vals[idx.str]; !ok {
				recv.m.keys = append(recv.m.keys, idx.str)
			}
			recv.m.vals[idx.str] = v
			return nil
		}
		return rterr(token.CatTypeMismatch, t.Span(), "element assignment requires an array or map")
	}
	return rterr(token.CatTypeMismatch, target.Span(), "invalid assignment target")
}

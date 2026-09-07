package types

import (
	"fmt"

	"github.com/Jh123x/prompiler/internal/ast"
	"github.com/Jh123x/prompiler/internal/token"
)

// typeSubst maps a type-parameter name to its concrete type.
type typeSubst map[string]Type

// substitute replaces type variables per the substitution map.
func substitute(t Type, s typeSubst) Type {
	switch tt := t.(type) {
	case *TypeVar:
		if v, ok := s[tt.Name]; ok {
			return v
		}
		return tt
	case *Array:
		return &Array{Elem: substitute(tt.Elem, s)}
	case *Map:
		return &Map{Value: substitute(tt.Value, s)}
	case *Optional:
		return &Optional{Elem: substitute(tt.Elem, s)}
	case *Class:
		if len(tt.TypeArgs) > 0 {
			args := make([]Type, len(tt.TypeArgs))
			for i, a := range tt.TypeArgs {
				args[i] = substitute(a, s)
			}
			return &Class{Name: tt.Name, TypeParams: tt.TypeParams, TypeArgs: args, Fields: tt.Fields, Methods: tt.Methods}
		}
		return tt
	case *Interface:
		if len(tt.TypeArgs) > 0 {
			args := make([]Type, len(tt.TypeArgs))
			for i, a := range tt.TypeArgs {
				args[i] = substitute(a, s)
			}
			return &Interface{Name: tt.Name, TypeParams: tt.TypeParams, TypeArgs: args, Fields: tt.Fields, Methods: tt.Methods}
		}
		return tt
	}
	return t
}

func substituteTypes(ts []Type, s typeSubst) []Type {
	out := make([]Type, len(ts))
	for i, t := range ts {
		out[i] = substitute(t, s)
	}
	return out
}

func substituteFields(fields []FieldInfo, s typeSubst) []FieldInfo {
	out := make([]FieldInfo, len(fields))
	for i, f := range fields {
		out[i] = FieldInfo{Name: f.Name, Type: substitute(f.Type, s)}
	}
	return out
}

func substituteMethods(methods []*MethodSig, s typeSubst) []*MethodSig {
	out := make([]*MethodSig, len(methods))
	for i, m := range methods {
		nm := &MethodSig{Name: m.Name, TypeParams: m.TypeParams}
		nm.Params = make([]Type, len(m.Params))
		for j, p := range m.Params {
			nm.Params[j] = substitute(p, s)
		}
		nm.Return = substitute(m.Return, s)
		out[i] = nm
	}
	return out
}

// unify matches a parameter type (possibly containing type variables) against a
// concrete argument type, filling the substitution map.
func unify(p, a Type, s typeSubst) bool {
	if tv, ok := p.(*TypeVar); ok {
		if existing, ok := s[tv.Name]; ok {
			return Equal(existing, a)
		}
		s[tv.Name] = a
		return true
	}
	switch pt := p.(type) {
	case *Array:
		at, ok := a.(*Array)
		return ok && unify(pt.Elem, at.Elem, s)
	case *Map:
		at, ok := a.(*Map)
		return ok && unify(pt.Value, at.Value, s)
	case *Optional:
		at, ok := a.(*Optional)
		return ok && unify(pt.Elem, at.Elem, s)
	}
	return Equal(p, a)
}

// instantiate resolves explicit type arguments and substitutes them into a
// generic class/interface declaration.
func (tc *TypeChecker) instantiate(base Type, args []ast.TypeRef, span token.Span) Type {
	resolved := make([]Type, 0, len(args))
	for _, a := range args {
		resolved = append(resolved, tc.resolveTypeRef(a))
	}
	var typeParams []*TypeVar
	switch b := base.(type) {
	case *Class:
		typeParams = b.TypeParams
	case *Interface:
		typeParams = b.TypeParams
	default:
		tc.err(token.CatTypeMismatch, span, fmt.Sprintf("%s is not generic", base))
		return base
	}
	if len(resolved) != len(typeParams) {
		tc.err(token.CatTypeMismatch, span, fmt.Sprintf("expected %d type arguments, got %d", len(typeParams), len(resolved)))
		return base
	}
	subst := typeSubst{}
	for i, tp := range typeParams {
		if tp.Bound != nil && !Assignable(resolved[i], tp.Bound) {
			tc.err(token.CatInterfaceUnsatisfied, span, fmt.Sprintf("%s does not satisfy bound %s", resolved[i], tp.Bound))
		}
		subst[tp.Name] = resolved[i]
	}
	switch b := base.(type) {
	case *Class:
		return &Class{Name: b.Name, TypeParams: b.TypeParams, TypeArgs: resolved, Fields: substituteFields(b.Fields, subst), Methods: substituteMethods(b.Methods, subst)}
	case *Interface:
		return &Interface{Name: b.Name, TypeParams: b.TypeParams, TypeArgs: resolved, Fields: substituteFields(b.Fields, subst), Methods: substituteMethods(b.Methods, subst)}
	}
	return base
}

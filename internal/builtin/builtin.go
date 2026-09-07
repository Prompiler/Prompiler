// Package builtin holds the replaceable registry of the fixed builtin method
// surface, the standard library, and casts. It exposes only signatures (for the
// type checker); the evaluator owns the runtime implementations. It implements
// types.MethodResolver and types.FunctionResolver so the checker can depend on
// the interface rather than this package.
package builtin

import "github.com/Jh123x/prompiler/internal/types"

// Registry is the builtin surface (features.md #13: replaceable).
type Registry struct{}

// Compile-time assertion that Registry satisfies the checker's seams.
var (
	_ types.MethodResolver   = (*Registry)(nil)
	_ types.FunctionResolver = (*Registry)(nil)
)

// NewRegistry returns a registry populated with the standard builtin surface.
func NewRegistry() *Registry { return &Registry{} }

// LookupMethod implements types.MethodResolver.
func (r *Registry) LookupMethod(t types.Type, name string) (types.MethodSig, bool) {
	switch tt := t.(type) {
	case types.Primitive:
		if tt == types.TypeString {
			return stringMethod(name)
		}
	case *types.Map:
		return mapMethod(name, tt.Value)
	case *types.Optional:
		return optionalMethod(name, tt.Elem)
	}
	return types.MethodSig{}, false
}

// LookupFunction implements types.FunctionResolver (stdlib functions).
func (r *Registry) LookupFunction(name string) (types.FuncSig, bool) {
	switch name {
	case "range":
		return types.FuncSig{Name: "range", Params: []types.Type{types.TypeInt}, Return: &types.Array{Elem: types.TypeInt}}, true
	case "range_from":
		return types.FuncSig{Name: "range_from", Params: []types.Type{types.TypeInt, types.TypeInt}, Return: &types.Array{Elem: types.TypeInt}}, true
	case "range_step":
		return types.FuncSig{Name: "range_step", Params: []types.Type{types.TypeInt, types.TypeInt, types.TypeInt}, Return: &types.Array{Elem: types.TypeInt}}, true
	case "join":
		return types.FuncSig{Name: "join", Params: []types.Type{&types.Array{Elem: types.TypeString}, types.TypeString}, Return: types.TypeString}, true
	}
	return types.FuncSig{}, false
}

func stringMethod(name string) (types.MethodSig, bool) {
	noArgs := func(ret types.Type) types.MethodSig {
		return types.MethodSig{Name: name, Return: ret}
	}
	switch name {
	case "upper", "lower", "trim":
		return noArgs(types.TypeString), true
	case "replace_all":
		return types.MethodSig{Name: name, Params: []types.Type{types.TypeString, types.TypeString}, Return: types.TypeString}, true
	case "replace":
		return types.MethodSig{Name: name, Params: []types.Type{types.TypeString, types.TypeString, types.TypeInt}, Return: types.TypeString}, true
	}
	return types.MethodSig{}, false
}

func mapMethod(name string, value types.Type) (types.MethodSig, bool) {
	switch name {
	case "keys":
		return types.MethodSig{Name: name, Return: &types.Array{Elem: types.TypeString}}, true
	case "values":
		return types.MethodSig{Name: name, Return: &types.Array{Elem: value}}, true
	}
	return types.MethodSig{}, false
}

func optionalMethod(name string, elem types.Type) (types.MethodSig, bool) {
	switch name {
	case "is_empty":
		return types.MethodSig{Name: name, Return: types.TypeBool}, true
	case "value":
		return types.MethodSig{Name: name, Return: elem}, true
	}
	return types.MethodSig{}, false
}

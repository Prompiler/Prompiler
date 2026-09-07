// Package eval is the deterministic runtime: the value model, the tree-walking
// evaluator, the prompt renderer, and the template composer.
package eval

import (
	"strconv"

	"github.com/Jh123x/prompiler/internal/token"
	"github.com/Jh123x/prompiler/internal/types"
)

// RuntimeError is an evaluation failure that passes type checking but not
// constant folding (§16.1).
type RuntimeError struct {
	Category token.Category
	Span     token.Span
	Message  string
}

func (e *RuntimeError) Error() string { return e.Message }

func rterr(cat token.Category, span token.Span, msg string) *RuntimeError {
	return &RuntimeError{Category: cat, Span: span, Message: msg}
}

// Value is a runtime value. Arrays and maps are reference types (aliased);
// everything else is a value.
type Value struct {
	typ types.Type

	str      string
	i        int64
	f        float64
	b        bool
	enumName string
	member   string

	arr *[]Value
	m   *orderedMap
	obj *classVal
	opt *optionalVal
}

type orderedMap struct {
	keys []string
	vals map[string]Value
}

type classVal struct {
	fields map[string]Value
}

type optionalVal struct {
	present bool
	value   Value
}

// Type returns the value's type.
func (v Value) Type() types.Type { return v.typ }

// --- constructors (used by the evaluator and the JSON ValueSource) ---

func StringVal(s string) Value { return Value{typ: types.TypeString, str: s} }
func IntVal(i int64) Value     { return Value{typ: types.TypeInt, i: i} }
func FloatVal(f float64) Value { return Value{typ: types.TypeFloat, f: f} }
func BoolVal(b bool) Value     { return Value{typ: types.TypeBool, b: b} }
func EnumVal(enumName, member string) Value {
	return Value{typ: &types.Enum{Name: enumName}, enumName: enumName, member: member}
}

// ArrayVal returns an array value of the given element type.
func ArrayVal(elem types.Type, elems []Value) Value {
	return Value{typ: &types.Array{Elem: elem}, arr: &elems}
}

// MapVal returns a map value preserving insertion order.
func MapVal(valueType types.Type, keys []string, vals map[string]Value) Value {
	return Value{typ: &types.Map{Value: valueType}, m: &orderedMap{keys: keys, vals: vals}}
}

// ObjectVal returns a class instance value.
func ObjectVal(class *types.Class, fields map[string]Value) Value {
	return Value{typ: class, obj: &classVal{fields: fields}}
}

func SomeVal(v Value) Value {
	return Value{typ: &types.Optional{Elem: v.typ}, opt: &optionalVal{present: true, value: v}}
}

func NoneVal(elem types.Type) Value {
	return Value{typ: &types.Optional{Elem: elem}, opt: &optionalVal{present: false}}
}

// --- internal accessors ---

func (v Value) isInt() bool    { return v.typ == types.TypeInt }
func (v Value) isFloat() bool  { return v.typ == types.TypeFloat }
func (v Value) isString() bool { return v.typ == types.TypeString }
func (v Value) isBool() bool   { return v.typ == types.TypeBool }

// Stringify renders a scalar or enum value to its string form (§8.1). It
// returns false for arrays, maps, classes, and Optionals (not stringable).
func Stringify(v Value) (string, bool) {
	switch v.typ.(type) {
	case types.Primitive:
		switch {
		case v.isString():
			return v.str, true
		case v.isInt():
			return strconv.FormatInt(v.i, 10), true
		case v.isFloat():
			return strconv.FormatFloat(v.f, 'g', -1, 64), true
		case v.isBool():
			return strconv.FormatBool(v.b), true
		}
	case *types.Enum:
		return v.member, true
	}
	return "", false
}

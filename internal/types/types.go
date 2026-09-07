// Package types defines the type-system value objects and the interfaces the
// type checker, builtin registry, and evaluator share. This file holds only the
// representation and the seams; the checker logic lives in check.go (added in a
// later milestone).
package types

import "strings"

// Type is a value object describing a Prompiler type. Named types compare
// nominally (by name); composite types compare structurally.
type Type interface {
	String() string
	isType()
}

// Primitive is one of the four scalar primitives.
type Primitive int

const (
	TypeString Primitive = iota
	TypeInt
	TypeFloat
	TypeBool
)

// Array is `T[]`.
type Array struct{ Elem Type }

// Map is `map<string, T>` (string keys only).
type Map struct{ Value Type }

// Optional is `Optional<T>`.
type Optional struct{ Elem Type }

// Enum is a named closed set of members.
type Enum struct {
	Name    string
	Members []string
}

// FieldInfo is a resolved field (name + type).
type FieldInfo struct {
	Name string
	Type Type
}

// MethodSig is a resolved method signature (or a builtin method signature).
type MethodSig struct {
	Name       string
	TypeParams []*TypeVar
	Params     []Type
	Return     Type
}

// TypeVar is a generic type parameter used as a type (`T`). Bound may be nil
// (unconstrained).
type TypeVar struct {
	Name  string
	Bound Type
}

// Class is a nominal record type. TypeParams lists the declaration's generic
// parameters; TypeArgs lists the type arguments of an instantiation (nil for a
// non-generic class or the generic declaration itself).
type Class struct {
	Name       string
	TypeParams []*TypeVar
	TypeArgs   []Type
	Fields     []FieldInfo
	Methods    []*MethodSig
}

// Interface is a structural contract (required fields + method signatures).
type Interface struct {
	Name       string
	TypeParams []*TypeVar
	TypeArgs   []Type
	Fields     []FieldInfo
	Methods    []*MethodSig
}

func (p Primitive) String() string {
	switch p {
	case TypeString:
		return "string"
	case TypeInt:
		return "int"
	case TypeFloat:
		return "float"
	case TypeBool:
		return "bool"
	}
	return "?"
}
func (a *Array) String() string    { return a.Elem.String() + "[]" }
func (m *Map) String() string      { return "map<string, " + m.Value.String() + ">" }
func (o *Optional) String() string { return "Optional<" + o.Elem.String() + ">" }
func (e *Enum) String() string     { return e.Name }
func (c *Class) String() string {
	if len(c.TypeArgs) == 0 {
		return c.Name
	}
	var b strings.Builder
	b.WriteString(c.Name)
	b.WriteByte('<')
	for i, a := range c.TypeArgs {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(a.String())
	}
	b.WriteByte('>')
	return b.String()
}
func (i *Interface) String() string { return i.Name }
func (v *TypeVar) String() string   { return v.Name }

func (Primitive) isType()  {}
func (*Array) isType()     {}
func (*Map) isType()       {}
func (*Optional) isType()  {}
func (*Enum) isType()      {}
func (*Class) isType()     {}
func (*Interface) isType() {}
func (*TypeVar) isType()   {}

// MethodResolver resolves a method signature for a receiver type. Implemented
// by the builtin registry (and consulted for user-defined classes). This seam
// keeps the types package free of a builtin dependency.
type MethodResolver interface {
	LookupMethod(t Type, name string) (MethodSig, bool)
}

// FuncSig describes a function signature (free function, stdlib, or cast).
type FuncSig struct {
	Name       string
	TypeParams []*TypeVar
	Params     []Type
	Return     Type
}

// Env is the shared type environment assembled by the checker.
type Env struct {
	Types   map[string]Type
	Funcs   map[string]*FuncSig
	Methods MethodResolver
}

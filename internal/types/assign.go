package types

// FieldType looks up a class field by name.
func (c *Class) FieldType(name string) (Type, bool) {
	for _, f := range c.Fields {
		if f.Name == name {
			return f.Type, true
		}
	}
	return nil, false
}

// Method looks up a class method by name.
func (c *Class) Method(name string) (*MethodSig, bool) {
	for _, m := range c.Methods {
		if m.Name == name {
			return m, true
		}
	}
	return nil, false
}

// FieldType looks up an interface field by name.
func (i *Interface) FieldType(name string) (Type, bool) {
	for _, f := range i.Fields {
		if f.Name == name {
			return f.Type, true
		}
	}
	return nil, false
}

// Method looks up an interface method signature by name.
func (i *Interface) Method(name string) (*MethodSig, bool) {
	for _, m := range i.Methods {
		if m.Name == name {
			return m, true
		}
	}
	return nil, false
}

// Equal reports whether two types are identical. Named types compare
// nominally (by name + type args); composites compare structurally.
func Equal(s, t Type) bool {
	if s == nil || t == nil {
		return s == t
	}
	switch a := s.(type) {
	case Primitive:
		b, ok := t.(Primitive)
		return ok && a == b
	case *Array:
		b, ok := t.(*Array)
		return ok && Equal(a.Elem, b.Elem)
	case *Map:
		b, ok := t.(*Map)
		return ok && Equal(a.Value, b.Value)
	case *Optional:
		b, ok := t.(*Optional)
		return ok && Equal(a.Elem, b.Elem)
	case *Enum:
		b, ok := t.(*Enum)
		return ok && a.Name == b.Name
	case *Class:
		b, ok := t.(*Class)
		return ok && a.Name == b.Name && equalTypeArgs(a.TypeArgs, b.TypeArgs)
	case *Interface:
		b, ok := t.(*Interface)
		return ok && a.Name == b.Name && equalTypeArgs(a.TypeArgs, b.TypeArgs)
	case *TypeVar:
		b, ok := t.(*TypeVar)
		return ok && a.Name == b.Name
	}
	return false
}

func equalTypeArgs(a, b []Type) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

// Assignable reports whether a value of type s may be used where t is expected:
// type equality, or s is a class structurally satisfying interface t.
func Assignable(s, t Type) bool {
	if Equal(s, t) {
		return true
	}
	if iface, ok := t.(*Interface); ok {
		if c, ok := s.(*Class); ok {
			_, ok := Satisfies(c, iface)
			return ok
		}
	}
	return false
}

// Satisfies reports whether class c structurally satisfies interface i. It
// returns a human-readable reason when it does not.
func Satisfies(c *Class, i *Interface) (string, bool) {
	for _, f := range i.Fields {
		ct, ok := c.FieldType(f.Name)
		if !ok {
			return "missing field '" + f.Name + "'", false
		}
		if !Equal(ct, f.Type) {
			return "field '" + f.Name + "' has an incompatible type", false
		}
	}
	for _, im := range i.Methods {
		cm, ok := c.Method(im.Name)
		if !ok {
			return "missing method '" + im.Name + "'", false
		}
		if len(cm.Params) != len(im.Params) {
			return "method '" + im.Name + "' has the wrong arity", false
		}
		for i := range im.Params {
			if !Equal(cm.Params[i], im.Params[i]) {
				return "method '" + im.Name + "' has incompatible parameter types", false
			}
		}
		if !Assignable(cm.Return, im.Return) {
			return "method '" + im.Name + "' has a non-covariant return type", false
		}
	}
	return "", true
}

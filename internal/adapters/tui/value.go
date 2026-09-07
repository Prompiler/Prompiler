// value.go coerces an editable form back into typed eval.Value inputs. Its
// per-field semantics mirror the JSON ValueSource (jsonvalue.coerce), but the
// values come from user text and are built via the exported eval constructors.
package tui

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/Jh123x/prompiler/internal/eval"
	"github.com/Jh123x/prompiler/internal/types"
)

// isBlank reports whether the field carries no user input, so that an optional
// variable should be omitted (and its default applied).
func (f *FormField) isBlank() bool {
	switch f.typ.(type) {
	case types.Primitive, *types.Enum:
		return f.text == ""
	case *types.Class:
		return allBlank(f.fields)
	case *types.Interface:
		if f.chosen < 0 {
			return true
		}
		return allBlank(f.fields)
	case *types.Array:
		return len(f.elems) == 0
	case *types.Map:
		return len(f.entries) == 0
	case *types.Optional:
		return !f.present
	}
	return true
}

func allBlank(fields []*FormField) bool {
	for _, f := range fields {
		if !f.isBlank() {
			return false
		}
	}
	return true
}

// Value coerces this field into a typed eval.Value, mirroring jsonvalue.coerce.
func (f *FormField) Value() (eval.Value, error) {
	switch tt := f.typ.(type) {
	case types.Primitive:
		switch tt {
		case types.TypeString:
			// the empty string is a valid string value
			return eval.StringVal(f.text), nil
		case types.TypeInt:
			i, err := strconv.ParseInt(f.text, 10, 64)
			if err != nil {
				return eval.Value{}, fmt.Errorf("%q is not a valid int", f.text)
			}
			return eval.IntVal(i), nil
		case types.TypeFloat:
			fl, err := strconv.ParseFloat(f.text, 64)
			if err != nil {
				return eval.Value{}, fmt.Errorf("%q is not a valid float", f.text)
			}
			return eval.FloatVal(fl), nil
		case types.TypeBool:
			b, err := strconv.ParseBool(f.text)
			if err != nil {
				return eval.Value{}, fmt.Errorf("%q is not a valid bool", f.text)
			}
			return eval.BoolVal(b), nil
		}
	case *types.Enum:
		if !slices.Contains(f.members, f.text) {
			return eval.Value{}, fmt.Errorf("%q is not a member of %s", f.text, tt.Name)
		}
		return eval.EnumVal(tt.Name, f.text), nil
	case *types.Array:
		elems := make([]eval.Value, 0, len(f.elems))
		for _, el := range f.elems {
			v, err := el.Value()
			if err != nil {
				return eval.Value{}, err
			}
			elems = append(elems, v)
		}
		return eval.ArrayVal(tt.Elem, elems), nil
	case *types.Map:
		keys := make([]string, 0, len(f.entries))
		vals := map[string]eval.Value{}
		for _, e := range f.entries {
			if e.key == "" {
				return eval.Value{}, fmt.Errorf("empty map key")
			}
			v, err := e.value.Value()
			if err != nil {
				return eval.Value{}, err
			}
			keys = append(keys, e.key)
			vals[e.key] = v
		}
		return eval.MapVal(tt.Value, keys, vals), nil
	case *types.Optional:
		if !f.present {
			return eval.NoneVal(tt.Elem), nil
		}
		v, err := f.child.Value()
		if err != nil {
			return eval.Value{}, err
		}
		return eval.SomeVal(v), nil
	case *types.Class:
		fields := map[string]eval.Value{}
		for _, sub := range f.fields {
			v, err := sub.Value()
			if err != nil {
				return eval.Value{}, err
			}
			fields[sub.label] = v
		}
		return eval.ObjectVal(tt, fields), nil
	case *types.Interface:
		if f.chosen < 0 {
			return eval.Value{}, fmt.Errorf("choose a class satisfying %s", tt.Name)
		}
		cls := f.choices[f.chosen]
		fields := map[string]eval.Value{}
		for _, sub := range f.fields {
			v, err := sub.Value()
			if err != nil {
				return eval.Value{}, err
			}
			fields[sub.label] = v
		}
		return eval.ObjectVal(cls, fields), nil
	case types.Invalid:
		return eval.Value{}, fmt.Errorf("cannot coerce %s", tt)
	case *types.TypeVar:
		return eval.Value{}, fmt.Errorf("cannot coerce %s", tt)
	}
	return eval.Value{}, fmt.Errorf("cannot coerce %s", f.typ)
}

// ToInputValues coerces every non-omitted top-level field. An optional field
// left blank is skipped so the template's default applies (mirrors jsonvalue's
// "omitted" behaviour).
func (f *InputForm) ToInputValues() (eval.InputValues, error) {
	inputs := eval.InputValues{}
	for _, fd := range f.Fields {
		if fd.optional && fd.isBlank() {
			continue
		}
		val, err := fd.Value()
		if err != nil {
			return nil, fmt.Errorf("variable %q: %w", fd.label, err)
		}
		inputs[fd.label] = val
	}
	return inputs, nil
}

// Validate checks the form, returning any per-field errors in path order.
func (f *InputForm) Validate() []fieldError {
	var out []fieldError
	for _, fd := range f.Fields {
		if fd.optional && fd.isBlank() {
			continue
		}
		out = append(out, validateField(fd.label, fd)...)
	}
	return out
}

// validateField recursively validates field, mirroring Value's errors with
// human-readable paths.
func validateField(path string, f *FormField) []fieldError {
	if f.err != "" {
		return []fieldError{{path: path, msg: f.err}}
	}
	switch tt := f.typ.(type) {
	case types.Primitive:
		switch tt {
		case types.TypeInt:
			if _, err := strconv.ParseInt(f.text, 10, 64); err != nil {
				return []fieldError{{path: path, msg: fmt.Sprintf("%q is not a valid int", f.text)}}
			}
		case types.TypeFloat:
			if _, err := strconv.ParseFloat(f.text, 64); err != nil {
				return []fieldError{{path: path, msg: fmt.Sprintf("%q is not a valid float", f.text)}}
			}
		case types.TypeBool:
			if _, err := strconv.ParseBool(f.text); err != nil {
				return []fieldError{{path: path, msg: fmt.Sprintf("%q is not a valid bool", f.text)}}
			}
		}
	case *types.Enum:
		if !slices.Contains(f.members, f.text) {
			return []fieldError{{path: path, msg: fmt.Sprintf("%q is not a member of %s", f.text, tt.Name)}}
		}
	case *types.Array:
		var out []fieldError
		for i, el := range f.elems {
			out = append(out, validateField(fmt.Sprintf("%s[%d]", path, i), el)...)
		}
		return out
	case *types.Map:
		var out []fieldError
		for i, e := range f.entries {
			p := fmt.Sprintf("%s[%d]", path, i)
			if e.key == "" {
				out = append(out, fieldError{path: p, msg: "empty map key"})
			}
			out = append(out, validateField(p, e.value)...)
		}
		return out
	case *types.Optional:
		if f.present && f.child != nil {
			return validateField(path, f.child)
		}
	case *types.Class:
		var out []fieldError
		for _, sub := range f.fields {
			out = append(out, validateField(path+"."+sub.label, sub)...)
		}
		return out
	case *types.Interface:
		if f.chosen < 0 {
			return []fieldError{{path: path, msg: fmt.Sprintf("select a class satisfying %s", tt.Name)}}
		}
		var out []fieldError
		for _, sub := range f.fields {
			out = append(out, validateField(path+"."+sub.label, sub)...)
		}
		return out
	}
	return nil
}

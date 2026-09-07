// form.go builds an editable form from a template's variables. The resolved
// types come from the semantic model (prog.Sem.TemplateVars); declaration
// order and defaults come from the AST. This file is pure: no bubbletea state.
package tui

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/Jh123x/prompiler/internal/ast"
	"github.com/Jh123x/prompiler/internal/types"
)

// InputForm is an editable form over a template's top-level variables, kept in
// declaration order.
type InputForm struct {
	Fields []*FormField
}

// FormField is one editable field. Compound types carry child structures that
// the interactive model drills into.
type FormField struct {
	label    string
	typ      types.Type
	optional bool // has a default: blank => omit so the default applies
	default_ string
	err      string // set when the type cannot be coerced (invalid / typevar)

	text    string   // scalar text, or the chosen enum member
	members []string // enum members

	fields  []*FormField   // class fields, or the chosen class's fields for an interface
	choices []*types.Class // classes satisfying the interface (sorted by Name)
	chosen  int            // -1 until a satisfying class is chosen
	elems   []*FormField   // array elements
	entries []mapEntry     // map entries
	present bool           // Optional: some = true / none = false
	child   *FormField     // Optional: the Some(value) child

	env *types.Env // retained to resolve choices of nested interfaces
}

// mapEntry is a single map<string, T> entry (an empty key is an error).
type mapEntry struct {
	key   string
	value *FormField
}

// fieldError is one validation failure, positioned at a field path.
type fieldError struct {
	path string
	msg  string
}

// BuildForm assembles the top-level variable fields of td in declaration order.
// Types are looked up in varTypes (from prog.Sem.TemplateVars); env resolves
// interface choices.
func BuildForm(td *ast.TemplateDecl, varTypes map[string]types.Type, env *types.Env) *InputForm {
	form := &InputForm{}
	if td == nil {
		return form
	}
	for _, v := range td.Variables {
		f := buildField(varTypes[v.Name], env)
		f.label = v.Name
		f.optional = v.HasDefault
		if v.HasDefault {
			f.default_ = defaultText(v.Default)
		}
		form.Fields = append(form.Fields, f)
	}
	return form
}

// BuildField drills a single field by type (with no type environment, so
// interface fields have no choices yet).
func BuildField(typ types.Type) *FormField {
	return buildField(typ, nil)
}

// buildField builds a field of type t, resolving nested interface choices
// against env (which may be nil).
func buildField(t types.Type, env *types.Env) *FormField {
	f := &FormField{chosen: -1, env: env}
	switch tt := t.(type) {
	case types.Primitive:
		// scalar text starts blank
	case *types.Enum:
		f.members = tt.Members
	case *types.Class:
		f.fields = buildFields(tt.Fields, env)
	case *types.Interface:
		f.choices = satisfyingClasses(tt, env)
	case *types.Array:
		// elements added interactively
	case *types.Map:
		// entries added interactively
	case *types.Optional:
		f.child = buildField(tt.Elem, env)
	case types.Invalid:
		f.err = fmt.Sprintf("cannot coerce %s", tt)
	case *types.TypeVar:
		f.err = fmt.Sprintf("cannot coerce %s", tt)
	default:
		f.err = fmt.Sprintf("cannot coerce %s", t)
	}
	f.typ = t
	return f
}

// buildFields maps resolved class/interface fields onto labelled child fields.
func buildFields(fis []types.FieldInfo, env *types.Env) []*FormField {
	out := make([]*FormField, 0, len(fis))
	for _, fi := range fis {
		f := buildField(fi.Type, env)
		f.label = fi.Name
		out = append(out, f)
	}
	return out
}

// chooseClass selects the i-th satisfying class, materializing its fields.
func (f *FormField) chooseClass(i int) {
	if i < 0 || i >= len(f.choices) {
		return
	}
	f.chosen = i
	f.fields = buildFields(f.choices[i].Fields, f.env)
}

// satisfyingClasses lists the classes in env that structurally satisfy iface,
// sorted by name.
func satisfyingClasses(iface *types.Interface, env *types.Env) []*types.Class {
	var out []*types.Class
	if env == nil {
		return out
	}
	for _, t := range env.Types {
		c, ok := t.(*types.Class)
		if !ok {
			continue
		}
		if _, ok := types.Satisfies(c, iface); ok {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// defaultText renders a variable's default expression for display only.
func defaultText(d ast.Expr) string {
	switch x := d.(type) {
	case *ast.IntLit:
		return strconv.FormatInt(x.Value, 10)
	case *ast.FloatLit:
		return strconv.FormatFloat(x.Value, 'g', -1, 64)
	case *ast.StringLit:
		return x.Value
	case *ast.BoolLit:
		return strconv.FormatBool(x.Value)
	case *ast.NoneLit:
		return "none"
	case *ast.Ident:
		return x.Name
	default:
		return "(default)"
	}
}

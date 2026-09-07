package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Jh123x/prompiler/internal/ast"
	"github.com/Jh123x/prompiler/internal/builtin"
	"github.com/Jh123x/prompiler/internal/domain"
	"github.com/Jh123x/prompiler/internal/eval"
	"github.com/Jh123x/prompiler/internal/types"
)

// valueProbeSource is a single analyzed program whose templates each expose one
// shape of input value. Forms are built from the semantic model exactly as the
// interactive model does, so these tests exercise the same coercion path.
const valueProbeSource = `
enum Level { Low, High }

class Pair {
  a: int
  b: string
}

interface Shape {
  size: int
}

class Circle {
  size: int
}

class Square {
  size: int
  name: string
}

template ArrayProbe {
  variables {
    xs: int[]
  }
  prompt {[{% for x in xs %}{{ x }},{% end %}]}
}

template MapProbe {
  variables {
    tags: map<string, int>
  }
  prompt {{% for k in tags.keys() %}{{ k }}={{ tags[k] }};{% end %}}
}

template OptionalProbe {
  variables {
    maybe: Optional<int>
  }
  prompt {{% if maybe.is_empty() %}empty{% else %}full{% end %}}
}

template DefaultProbe {
  variables {
    count: int = 3
    level: Level = Low
  }
  prompt {{{ count }}|{{ level }}}
}

template ClassProbe {
  variables {
    pair: Pair
  }
  prompt {{{ pair.a }}:{{ pair.b }}}
}

template ShapeProbe {
  variables {
    shape: Shape
  }
  prompt {{{ shape.size }}}
}
`

func valueProbe(t *testing.T) *domain.Program {
	t.Helper()
	prog, diags := domain.Analyze(".", domain.SourceSet{"probe.ppl": valueProbeSource}, builtin.NewRegistry())
	require.Empty(t, diags, "probe source must analyze cleanly")
	require.NotNil(t, prog)
	return prog
}

func templateDeclOf(t *testing.T, prog *domain.Program, name string) *ast.TemplateDecl {
	t.Helper()
	for _, f := range prog.Files {
		for _, d := range f.Decls {
			if td, ok := d.(*ast.TemplateDecl); ok && td.Name == name {
				return td
			}
		}
	}
	t.Fatalf("template %q not found in probe", name)
	return nil
}

func valueForm(t *testing.T, prog *domain.Program, name string) *InputForm {
	t.Helper()
	td := templateDeclOf(t, prog, name)
	return BuildForm(td, prog.Sem.TemplateVars[name], prog.Sem.Env)
}

// renderForm coerces the form and renders the template, returning the output.
func renderForm(t *testing.T, prog *domain.Program, name string, mutate func(*InputForm)) string {
	t.Helper()
	form := valueForm(t, prog, name)
	if mutate != nil {
		mutate(form)
	}
	inputs, err := form.ToInputValues()
	require.NoError(t, err, "ToInputValues for %q", name)
	out, rerr := renderTemplate(prog, name, inputs)
	require.Nil(t, rerr, "render of %q", name)
	return out
}

// subField finds a child field of a class/interface/optional compound.
func subField(t *testing.T, f *FormField, label string) *FormField {
	t.Helper()
	for _, sub := range f.fields {
		if sub.label == label {
			return sub
		}
	}
	t.Fatalf("compound %q has no child %q", f.label, label)
	return nil
}

func TestScalarValue(t *testing.T) {
	cases := []struct {
		name string
		typ  types.Type
		text string
		want string
	}{
		{name: "string", typ: types.TypeString, text: "hi", want: "hi"},
		{name: "empty string is a valid string", typ: types.TypeString, text: "", want: ""},
		{name: "int", typ: types.TypeInt, text: "42", want: "42"},
		{name: "negative int", typ: types.TypeInt, text: "-7", want: "-7"},
		{name: "float", typ: types.TypeFloat, text: "3.5", want: "3.5"},
		{name: "bool true", typ: types.TypeBool, text: "true", want: "true"},
		{name: "bool false", typ: types.TypeBool, text: "false", want: "false"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := BuildField(c.typ)
			f.text = c.text
			v, err := f.Value()
			require.NoError(t, err)
			s, ok := eval.Stringify(v)
			require.True(t, ok)
			assert.Equal(t, c.want, s)
		})
	}
}

func TestScalarValueErrors(t *testing.T) {
	t.Run("int", func(t *testing.T) {
		f := BuildField(types.TypeInt)
		f.text = "abc"
		_, err := f.Value()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a valid int")
	})
	t.Run("float", func(t *testing.T) {
		f := BuildField(types.TypeFloat)
		f.text = "3.5x"
		_, err := f.Value()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a valid float")
	})
	t.Run("bool", func(t *testing.T) {
		f := BuildField(types.TypeBool)
		f.text = "yes"
		_, err := f.Value()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a valid bool")
	})
}

func TestEnumValue(t *testing.T) {
	enum := &types.Enum{Name: "Level", Members: []string{"Low", "High"}}

	t.Run("valid member coerces to an enum value", func(t *testing.T) {
		f := BuildField(enum)
		f.text = "High"
		v, err := f.Value()
		require.NoError(t, err)
		s, ok := eval.Stringify(v)
		require.True(t, ok)
		assert.Equal(t, "High", s)
	})

	t.Run("invalid member is rejected", func(t *testing.T) {
		f := BuildField(enum)
		f.text = "Bogus"
		_, err := f.Value()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not a member of Level")
	})

	t.Run("an unset required enum member is rejected", func(t *testing.T) {
		f := BuildField(enum)
		_, err := f.Value()
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"" is not a member of Level`)
	})
}

func TestArrayValue(t *testing.T) {
	prog := valueProbe(t)

	t.Run("collects scalar elements in order", func(t *testing.T) {
		out := renderForm(t, prog, "ArrayProbe", func(form *InputForm) {
			xs := mustField(t, form, "xs")
			for _, s := range []string{"1", "2", "3"} {
				el := BuildField(xs.typ.(*types.Array).Elem)
				el.text = s
				xs.elems = append(xs.elems, el)
			}
		})
		assert.Equal(t, "[1,2,3,]\n", out)
	})

	t.Run("an empty array is a valid value", func(t *testing.T) {
		out := renderForm(t, prog, "ArrayProbe", nil)
		assert.Equal(t, "[]\n", out)
	})

	t.Run("an invalid element surfaces the coercion error", func(t *testing.T) {
		form := valueForm(t, prog, "ArrayProbe")
		xs := mustField(t, form, "xs")
		el := BuildField(xs.typ.(*types.Array).Elem)
		el.text = "not-a-number"
		xs.elems = append(xs.elems, el)
		_, err := form.ToInputValues()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a valid int")
	})
}

func TestMapValue(t *testing.T) {
	prog := valueProbe(t)

	t.Run("preserves entry order", func(t *testing.T) {
		out := renderForm(t, prog, "MapProbe", func(form *InputForm) {
			tags := mustField(t, form, "tags")
			value := func(s string) *FormField {
				v := BuildField(tags.typ.(*types.Map).Value)
				v.text = s
				return v
			}
			tags.entries = []mapEntry{
				{key: "b", value: value("2")},
				{key: "a", value: value("1")},
			}
		})
		assert.Equal(t, "b=2;a=1;", out)
	})

	t.Run("an empty map is a valid value", func(t *testing.T) {
		out := renderForm(t, prog, "MapProbe", func(form *InputForm) {
			mustField(t, form, "tags") // no entries
		})
		assert.Equal(t, "", out)
	})

	t.Run("an empty key is an error", func(t *testing.T) {
		form := valueForm(t, prog, "MapProbe")
		tags := mustField(t, form, "tags")
		v := BuildField(tags.typ.(*types.Map).Value)
		v.text = "1"
		tags.entries = append(tags.entries, mapEntry{key: "", value: v})
		_, err := form.ToInputValues()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty map key")
	})
}

func TestOptionalValue(t *testing.T) {
	prog := valueProbe(t)

	t.Run("none when not present", func(t *testing.T) {
		out := renderForm(t, prog, "OptionalProbe", nil)
		assert.Equal(t, "empty", out)
	})

	t.Run("some carries the coerced child value", func(t *testing.T) {
		out := renderForm(t, prog, "OptionalProbe", func(form *InputForm) {
			maybe := mustField(t, form, "maybe")
			maybe.present = true
			maybe.child.text = "9"
		})
		assert.Equal(t, "full", out)
	})

	t.Run("a present optional still coerces its child", func(t *testing.T) {
		form := valueForm(t, prog, "OptionalProbe")
		maybe := mustField(t, form, "maybe")
		maybe.present = true
		maybe.child.text = "bogus"
		_, err := form.ToInputValues()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a valid int")
	})
}

func TestClassValue(t *testing.T) {
	prog := valueProbe(t)

	t.Run("coerces every nested field", func(t *testing.T) {
		out := renderForm(t, prog, "ClassProbe", func(form *InputForm) {
			pair := mustField(t, form, "pair")
			subField(t, pair, "a").text = "10"
			subField(t, pair, "b").text = "hi"
		})
		assert.Equal(t, "10:hi\n", out)
	})

	t.Run("a blank required nested int is a validation error", func(t *testing.T) {
		form := valueForm(t, prog, "ClassProbe")
		subField(t, mustField(t, form, "pair"), "b").text = "hi" // leave a blank
		errs := form.Validate()
		require.NotEmpty(t, errs)
		assert.Equal(t, "pair.a", errs[0].path)
	})
}

func TestInterfaceValue(t *testing.T) {
	prog := valueProbe(t)

	t.Run("lists satisfying classes sorted by name", func(t *testing.T) {
		form := valueForm(t, prog, "ShapeProbe")
		shape := mustField(t, form, "shape")
		names := make([]string, 0, len(shape.choices))
		for _, c := range shape.choices {
			names = append(names, c.Name)
		}
		assert.Equal(t, []string{"Circle", "Square"}, names)
	})

	t.Run("choosing a class and filling its fields coerces an object", func(t *testing.T) {
		out := renderForm(t, prog, "ShapeProbe", func(form *InputForm) {
			shape := mustField(t, form, "shape")
			shape.chooseClass(0) // Circle
			assert.Equal(t, "Circle", shape.choices[shape.chosen].Name)
			subField(t, shape, "size").text = "5"
		})
		assert.Equal(t, "5\n", out)
	})

	t.Run("an unchosen interface is a validation error", func(t *testing.T) {
		form := valueForm(t, prog, "ShapeProbe")
		errs := form.Validate()
		require.NotEmpty(t, errs)
		assert.Equal(t, "shape", errs[0].path)
	})
}

func TestToInputValuesDefaultOmission(t *testing.T) {
	prog := valueProbe(t)

	t.Run("blank optional variables are omitted so defaults apply", func(t *testing.T) {
		form := valueForm(t, prog, "DefaultProbe")
		inputs, err := form.ToInputValues()
		require.NoError(t, err)
		assert.NotContains(t, inputs, "count")
		assert.NotContains(t, inputs, "level")
		out, rerr := renderTemplate(prog, "DefaultProbe", inputs)
		require.Nil(t, rerr)
		assert.Equal(t, "3|Low\n", out)
	})

	t.Run("a supplied optional overrides the default", func(t *testing.T) {
		form := valueForm(t, prog, "DefaultProbe")
		mustField(t, form, "count").text = "9"
		inputs, err := form.ToInputValues()
		require.NoError(t, err)
		assert.Equal(t, "9", mustString(t, inputs["count"]))
		assert.NotContains(t, inputs, "level")
		out, rerr := renderTemplate(prog, "DefaultProbe", inputs)
		require.Nil(t, rerr)
		assert.Equal(t, "9|Low\n", out)
	})

	t.Run("supplying the enum member as well", func(t *testing.T) {
		form := valueForm(t, prog, "DefaultProbe")
		mustField(t, form, "count").text = "2"
		level := mustField(t, form, "level")
		level.text = "High"
		inputs, err := form.ToInputValues()
		require.NoError(t, err)
		out, rerr := renderTemplate(prog, "DefaultProbe", inputs)
		require.Nil(t, rerr)
		assert.Equal(t, "2|High\n", out)
	})
}

func mustString(t *testing.T, v eval.Value) string {
	t.Helper()
	s, ok := eval.Stringify(v)
	require.True(t, ok, "expected a stringifiable value")
	return s
}

func TestToInputValuesWrapsVariableErrors(t *testing.T) {
	prog := valueProbe(t)
	form := valueForm(t, prog, "ClassProbe")
	subField(t, mustField(t, form, "pair"), "a").text = "x"
	_, err := form.ToInputValues()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `variable "pair"`)
}

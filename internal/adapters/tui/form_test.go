package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Jh123x/prompiler/internal/ast"
	"github.com/Jh123x/prompiler/internal/types"
)

// testTypes builds the small closed type universe the form tests run over.
func testTypes() (*types.Enum, *types.Class, *types.Interface, *types.Class, *types.Class) {
	level := &types.Enum{Name: "Level", Members: []string{"Low", "High"}}
	pair := &types.Class{
		Name: "Pair",
		Fields: []types.FieldInfo{
			{Name: "a", Type: types.TypeInt},
			{Name: "b", Type: types.TypeString},
		},
	}
	shape := &types.Interface{
		Name:   "Shape",
		Fields: []types.FieldInfo{{Name: "size", Type: types.TypeInt}},
	}
	circle := &types.Class{
		Name:   "Circle",
		Fields: []types.FieldInfo{{Name: "size", Type: types.TypeInt}},
	}
	square := &types.Class{
		Name: "Square",
		Fields: []types.FieldInfo{
			{Name: "size", Type: types.TypeInt},
			{Name: "name", Type: types.TypeString},
		},
	}
	return level, pair, shape, circle, square
}

// fixtureTemplateDecl returns a TemplateDecl whose variables cover every
// drillable type plus defaults of each literal shape.
func fixtureTemplateDecl() *ast.TemplateDecl {
	return &ast.TemplateDecl{
		Name: "Fixture",
		Variables: []ast.VarDecl{
			{Name: "name"}, // string, required
			{Name: "count", Default: &ast.IntLit{Value: 5}, HasDefault: true},
			{Name: "ratio", Default: &ast.FloatLit{Value: 2.5}, HasDefault: true},
			{Name: "active", Default: &ast.BoolLit{Value: true}, HasDefault: true},
			{Name: "title", Default: &ast.StringLit{Value: "hi"}, HasDefault: true},
			{Name: "level", Default: &ast.Ident{Name: "Low"}, HasDefault: true},
			{Name: "maybe", Default: &ast.NoneLit{}, HasDefault: true}, // Optional<int> = none
			{Name: "point"},    // class Pair
			{Name: "shape"},    // interface Shape
			{Name: "nums"},     // int[]
			{Name: "tags"},     // map<string, int>
			{Name: "wrapped"},  // Optional<Pair>
			{Name: "generic"},  // TypeVar (uncoercible)
			{Name: "maybeReq"}, // Optional<int>, no default
		},
	}
}

func fixtureVarTypes(
	level *types.Enum, pair *types.Class, shape *types.Interface,
) map[string]types.Type {
	return map[string]types.Type{
		"name":     types.TypeString,
		"count":    types.TypeInt,
		"ratio":    types.TypeFloat,
		"active":   types.TypeBool,
		"title":    types.TypeString,
		"level":    level,
		"maybe":    &types.Optional{Elem: types.TypeInt},
		"point":    pair,
		"shape":    shape,
		"nums":     &types.Array{Elem: types.TypeInt},
		"tags":     &types.Map{Value: types.TypeInt},
		"wrapped":  &types.Optional{Elem: pair},
		"generic":  &types.TypeVar{Name: "T"},
		"maybeReq": &types.Optional{Elem: types.TypeInt},
	}
}

func fixtureEnv(
	level *types.Enum, pair *types.Class, shape *types.Interface,
	circle *types.Class, square *types.Class,
) *types.Env {
	return &types.Env{Types: map[string]types.Type{
		"Level":  level,
		"Pair":   pair,
		"Shape":  shape,
		"Circle": circle,
		"Square": square,
	}}
}

func buildFixtureForm() *InputForm {
	level, pair, shape, circle, square := testTypes()
	env := fixtureEnv(level, pair, shape, circle, square)
	varTypes := fixtureVarTypes(level, pair, shape)
	return BuildForm(fixtureTemplateDecl(), varTypes, env)
}

func mustField(t *testing.T, form *InputForm, label string) *FormField {
	t.Helper()
	for _, f := range form.Fields {
		if f.label == label {
			return f
		}
	}
	t.Fatalf("no top-level field %q in form (have %d fields)", label, len(form.Fields))
	return nil
}

func TestBuildFormShape(t *testing.T) {
	form := buildFixtureForm()

	t.Run("keeps declaration order", func(t *testing.T) {
		require.NotNil(t, form)
		want := []string{"name", "count", "ratio", "active", "title", "level", "maybe", "point", "shape", "nums", "tags", "wrapped", "generic", "maybeReq"}
		got := make([]string, 0, len(form.Fields))
		for _, f := range form.Fields {
			got = append(got, f.label)
		}
		assert.Equal(t, want, got)
	})

	t.Run("scalar fields start blank and un-defaulted", func(t *testing.T) {
		f := mustField(t, form, "name")
		assert.Equal(t, types.TypeString, f.typ)
		assert.False(t, f.optional)
		assert.Equal(t, "", f.default_)
		assert.True(t, f.isBlank())
	})

	t.Run("literal defaults render for display", func(t *testing.T) {
		assert.Equal(t, "5", mustField(t, form, "count").default_)
		assert.Equal(t, "2.5", mustField(t, form, "ratio").default_)
		assert.Equal(t, "true", mustField(t, form, "active").default_)
		assert.Equal(t, "hi", mustField(t, form, "title").default_)
	})

	t.Run("enum default resolves to the member name", func(t *testing.T) {
		f := mustField(t, form, "level")
		assert.True(t, f.optional)
		assert.Equal(t, "Low", f.default_)
		assert.Equal(t, []string{"Low", "High"}, f.members)
	})

	t.Run("none default renders as none", func(t *testing.T) {
		f := mustField(t, form, "maybe")
		assert.True(t, f.optional)
		assert.Equal(t, "none", f.default_)
		assert.False(t, f.present)
		require.NotNil(t, f.child)
		assert.Equal(t, types.TypeInt, f.child.typ)
	})

	t.Run("class fields are built recursively", func(t *testing.T) {
		f := mustField(t, form, "point")
		assert.Equal(t, "Pair", f.typ.(*types.Class).Name)
		require.Len(t, f.fields, 2)
		assert.Equal(t, "a", f.fields[0].label)
		assert.Equal(t, types.TypeInt, f.fields[0].typ)
		assert.Equal(t, "b", f.fields[1].label)
		assert.Equal(t, types.TypeString, f.fields[1].typ)
	})

	t.Run("interface lists satisfying classes sorted by name, unchosen", func(t *testing.T) {
		f := mustField(t, form, "shape")
		assert.Equal(t, "Shape", f.typ.(*types.Interface).Name)
		assert.Equal(t, -1, f.chosen)
		names := make([]string, 0, len(f.choices))
		for _, c := range f.choices {
			names = append(names, c.Name)
		}
		assert.Equal(t, []string{"Circle", "Square"}, names)
	})

	t.Run("array and map start empty", func(t *testing.T) {
		nums := mustField(t, form, "nums")
		assert.Equal(t, types.TypeInt, nums.typ.(*types.Array).Elem)
		assert.Empty(t, nums.elems)

		tags := mustField(t, form, "tags")
		assert.Equal(t, types.TypeInt, tags.typ.(*types.Map).Value)
		assert.Empty(t, tags.entries)
	})

	t.Run("optional nests a child of its element type", func(t *testing.T) {
		f := mustField(t, form, "maybeReq")
		assert.False(t, f.optional) // no default => required
		require.NotNil(t, f.child)
		assert.Equal(t, types.TypeInt, f.child.typ)
	})

	t.Run("optional of a class nests that class's fields", func(t *testing.T) {
		f := mustField(t, form, "wrapped")
		require.NotNil(t, f.child)
		assert.Equal(t, "Pair", f.child.typ.(*types.Class).Name)
		require.Len(t, f.child.fields, 2)
		assert.Equal(t, "a", f.child.fields[0].label)
		assert.Equal(t, "b", f.child.fields[1].label)
	})

	t.Run("uncoercible types carry an error", func(t *testing.T) {
		f := mustField(t, form, "generic")
		assert.Contains(t, f.err, "cannot coerce T")
	})
}

func TestBuildFieldByType(t *testing.T) {
	t.Run("primitive", func(t *testing.T) {
		f := BuildField(types.TypeBool)
		assert.Equal(t, types.TypeBool, f.typ)
		assert.True(t, f.isBlank()) // blank until given text
	})

	t.Run("enum carries members", func(t *testing.T) {
		f := BuildField(&types.Enum{Name: "Level", Members: []string{"Low", "High"}})
		assert.Equal(t, []string{"Low", "High"}, f.members)
	})

	t.Run("interface without an env has no choices yet", func(t *testing.T) {
		f := BuildField(&types.Interface{Name: "Shape", Fields: []types.FieldInfo{{Name: "size", Type: types.TypeInt}}})
		assert.Equal(t, -1, f.chosen)
		assert.Empty(t, f.choices)
	})

	t.Run("class drills its fields", func(t *testing.T) {
		c := &types.Class{Name: "Point", Fields: []types.FieldInfo{{Name: "x", Type: types.TypeInt}, {Name: "y", Type: types.TypeFloat}}}
		f := BuildField(c)
		require.Len(t, f.fields, 2)
		assert.Equal(t, "x", f.fields[0].label)
		assert.Equal(t, "y", f.fields[1].label)
		assert.Equal(t, types.TypeFloat, f.fields[1].typ)
	})

	t.Run("array element builder", func(t *testing.T) {
		f := BuildField(&types.Array{Elem: types.TypeString})
		el := BuildField(f.typ.(*types.Array).Elem)
		assert.Equal(t, types.TypeString, el.typ)
	})

	t.Run("map value builder", func(t *testing.T) {
		f := BuildField(&types.Map{Value: types.TypeInt})
		v := BuildField(f.typ.(*types.Map).Value)
		assert.Equal(t, types.TypeInt, v.typ)
	})

	t.Run("optional child", func(t *testing.T) {
		f := BuildField(&types.Optional{Elem: types.TypeFloat})
		assert.False(t, f.present)
		require.NotNil(t, f.child)
		assert.Equal(t, types.TypeFloat, f.child.typ)
	})

	t.Run("invalid type is flagged", func(t *testing.T) {
		f := BuildField(types.Invalid{})
		assert.Contains(t, f.err, "cannot coerce")
	})
}

func TestBuildFormNilTemplate(t *testing.T) {
	form := BuildForm(nil, map[string]types.Type{}, &types.Env{Types: map[string]types.Type{}})
	require.NotNil(t, form)
	assert.Empty(t, form.Fields)
}

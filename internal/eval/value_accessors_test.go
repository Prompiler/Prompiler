package eval_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Jh123x/prompiler/internal/eval"
	"github.com/Jh123x/prompiler/internal/types"
)

// These tests exercise the exported read accessors that the TUI pre-fill path
// uses to reconstruct editable form state from a typed value.

func TestValueAsArray(t *testing.T) {
	t.Run("returns elements in order", func(t *testing.T) {
		v := eval.ArrayVal(types.TypeInt, []eval.Value{eval.IntVal(1), eval.IntVal(2)})
		elems, ok := v.AsArray()
		require.True(t, ok)
		require.Len(t, elems, 2)
		s, ok := eval.Stringify(elems[0])
		require.True(t, ok)
		assert.Equal(t, "1", s)
		s, ok = eval.Stringify(elems[1])
		require.True(t, ok)
		assert.Equal(t, "2", s)
	})

	t.Run("not ok for a scalar", func(t *testing.T) {
		_, ok := eval.StringVal("x").AsArray()
		assert.False(t, ok)
	})
}

func TestValueAsMap(t *testing.T) {
	t.Run("returns keys in insertion order with values", func(t *testing.T) {
		v := eval.MapVal(types.TypeString, []string{"b", "a"}, map[string]eval.Value{
			"b": eval.StringVal("2"),
			"a": eval.StringVal("1"),
		})
		keys, vals, ok := v.AsMap()
		require.True(t, ok)
		assert.Equal(t, []string{"b", "a"}, keys)
		s, ok := eval.Stringify(vals["b"])
		require.True(t, ok)
		assert.Equal(t, "2", s)
		s, ok = eval.Stringify(vals["a"])
		require.True(t, ok)
		assert.Equal(t, "1", s)
	})

	t.Run("not ok for a non-map", func(t *testing.T) {
		_, _, ok := eval.IntVal(3).AsMap()
		assert.False(t, ok)
	})
}

func TestValueAsObject(t *testing.T) {
	cls := &types.Class{Name: "Pair", Fields: []types.FieldInfo{
		{Name: "a", Type: types.TypeInt},
		{Name: "b", Type: types.TypeString},
	}}
	t.Run("returns the concrete class and field map", func(t *testing.T) {
		v := eval.ObjectVal(cls, map[string]eval.Value{
			"a": eval.IntVal(1),
			"b": eval.StringVal("x"),
		})
		got, fields, ok := v.AsObject()
		require.True(t, ok)
		assert.Equal(t, "Pair", got.Name)
		s, ok := eval.Stringify(fields["b"])
		require.True(t, ok)
		assert.Equal(t, "x", s)
	})

	t.Run("not ok for a non-object", func(t *testing.T) {
		_, _, ok := eval.BoolVal(true).AsObject()
		assert.False(t, ok)
	})
}

func TestValueAsOptional(t *testing.T) {
	t.Run("none reports absent", func(t *testing.T) {
		present, _, ok := eval.NoneVal(types.TypeInt).AsOptional()
		require.True(t, ok)
		assert.False(t, present)
	})

	t.Run("some reports present and carries the payload", func(t *testing.T) {
		present, inner, ok := eval.SomeVal(eval.IntVal(7)).AsOptional()
		require.True(t, ok)
		assert.True(t, present)
		s, ok := eval.Stringify(inner)
		require.True(t, ok)
		assert.Equal(t, "7", s)
	})

	t.Run("not ok for a non-optional", func(t *testing.T) {
		_, _, ok := eval.StringVal("x").AsOptional()
		assert.False(t, ok)
	})
}

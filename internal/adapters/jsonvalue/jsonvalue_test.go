package jsonvalue_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Jh123x/prompiler/internal/adapters/jsonvalue"
	"github.com/Jh123x/prompiler/internal/domain"
	"github.com/Jh123x/prompiler/internal/eval"
	"github.com/Jh123x/prompiler/internal/types"
)

func TestIntPrecision(t *testing.T) {
	t.Run("preserves 64-bit integers without float64 truncation", func(t *testing.T) {
		src, err := jsonvalue.New([]byte(`{"n": 9007199254740993}`), &types.Env{Types: map[string]types.Type{}})
		require.NoError(t, err)
		inputs, err := src.Provide([]domain.Variable{{Name: "n", Type: types.TypeInt}})
		require.NoError(t, err)
		s, ok := eval.Stringify(inputs["n"])
		require.True(t, ok)
		assert.Equal(t, "9007199254740993", s)
	})

	t.Run("rejects a fractional value for an int", func(t *testing.T) {
		src, err := jsonvalue.New([]byte(`{"n": 1.9}`), &types.Env{Types: map[string]types.Type{}})
		require.NoError(t, err)
		_, err = src.Provide([]domain.Variable{{Name: "n", Type: types.TypeInt}})
		require.Error(t, err)
	})
}

func TestEnumValidation(t *testing.T) {
	level := &types.Enum{Name: "LogLevel", Members: []string{"Low", "High"}}

	t.Run("rejects an invalid enum member", func(t *testing.T) {
		src, err := jsonvalue.New([]byte(`{"level": "BOGUS"}`), &types.Env{Types: map[string]types.Type{}})
		require.NoError(t, err)
		_, err = src.Provide([]domain.Variable{{Name: "level", Type: level}})
		require.Error(t, err)
	})

	t.Run("accepts a valid enum member", func(t *testing.T) {
		src, err := jsonvalue.New([]byte(`{"level": "High"}`), &types.Env{Types: map[string]types.Type{}})
		require.NoError(t, err)
		inputs, err := src.Provide([]domain.Variable{{Name: "level", Type: level}})
		require.NoError(t, err)
		s, ok := eval.Stringify(inputs["level"])
		require.True(t, ok)
		assert.Equal(t, "High", s)
	})
}

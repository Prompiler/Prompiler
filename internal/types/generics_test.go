package types_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Jh123x/prompiler/internal/token"
)

func TestGenericFunctionInference(t *testing.T) {
	t.Run("infers type arguments for a generic function", func(t *testing.T) {
		src := `func identity<T>(x: T): T { return x }
template T {
  variables {
    n: int
    s: string
  }
  prompt {
    {{ identity(n) }}
    {{ identity(s) }}
  }
}`
		assert.Empty(t, typecheckFiles(t, map[string]string{"t.ppl": src}))
	})
}

func TestGenericClass(t *testing.T) {
	t.Run("instantiates a generic class with explicit type arguments", func(t *testing.T) {
		src := `class Pair<A, B> {
  first: A
  second: B
}
template T {
  variables { p: Pair<string, int> }
  prompt {
    {{ p.first }}: {{ p.second }}
  }
}`
		assert.Empty(t, typecheckFiles(t, map[string]string{"t.ppl": src}))
	})
}

func TestGenericMethod(t *testing.T) {
	t.Run("resolves a method on a generic class", func(t *testing.T) {
		src := `class Box<T> {
  value: T
  func get(): T { return this.value }
}
template T {
  variables { b: Box<string> }
  prompt { {{ b.get() }} }
}`
		assert.Empty(t, typecheckFiles(t, map[string]string{"t.ppl": src}))
	})
}

func TestGenericBound(t *testing.T) {
	t.Run("enforces a structural bound", func(t *testing.T) {
		src := `interface Eq<T> {
  func equals(other: T): bool
}
func allEqual<T: Eq<T>>(a: T, b: T): bool {
  return a.equals(b)
}
template T {
  variables {
    n: int
    s: string
  }
  prompt { {{ allEqual(s, s) }} }
}`
		// `string` does not satisfy Eq<T>, so this is an interface_unsatisfied.
		diags := typecheckFiles(t, map[string]string{"t.ppl": src})
		require.NotEmpty(t, diags, "expected a bound violation")
		found := false
		for _, d := range diags {
			if d.Category == token.CatInterfaceUnsatisfied {
				found = true
			}
		}
		assert.True(t, found, "expected interface_unsatisfied, got %v", diags)
	})
}

func TestGenericExplicitAndInferred(t *testing.T) {
	t.Run("supports explicit and inferred type arguments", func(t *testing.T) {
		src := `func head<T>(xs: T[]): Optional<T> {
  if (xs.length == 0) { return none }
  return some(xs[0])
}
template T {
  variables { names: string[] }
  prompt {
    {{ head<string>(names).value() }}
    {{ head(names).value() }}
  }
}`
		assert.Empty(t, typecheckFiles(t, map[string]string{"t.ppl": src}))
	})
}

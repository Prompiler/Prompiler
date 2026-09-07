package types_test

import (
	"testing"

	"github.com/Jh123x/prompiler/internal/token"
)

func TestGenericFunctionInference(t *testing.T) {
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
	diags := typecheckFiles(t, map[string]string{"t.ppl": src})
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestGenericClass(t *testing.T) {
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
	diags := typecheckFiles(t, map[string]string{"t.ppl": src})
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestGenericMethod(t *testing.T) {
	src := `class Box<T> {
  value: T
  func get(): T { return this.value }
}
template T {
  variables { b: Box<string> }
  prompt { {{ b.get() }} }
}`
	diags := typecheckFiles(t, map[string]string{"t.ppl": src})
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestGenericBound(t *testing.T) {
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
	// `string` does not satisfy Eq<T>, so this should be an interface_unsatisfied.
	diags := typecheckFiles(t, map[string]string{"t.ppl": src})
	if len(diags) == 0 {
		t.Fatal("expected a bound violation diagnostic, got none")
	}
	found := false
	for _, d := range diags {
		if d.Category == token.CatInterfaceUnsatisfied {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected interface_unsatisfied, got %v", diags)
	}
}

func TestGenericExplicitAndInferred(t *testing.T) {
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
	diags := typecheckFiles(t, map[string]string{"t.ppl": src})
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

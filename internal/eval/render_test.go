package eval_test

import (
	"strings"
	"testing"

	"github.com/Jh123x/prompiler/internal/ast"
	"github.com/Jh123x/prompiler/internal/builtin"
	"github.com/Jh123x/prompiler/internal/eval"
	"github.com/Jh123x/prompiler/internal/lexer"
	"github.com/Jh123x/prompiler/internal/parser"
	"github.com/Jh123x/prompiler/internal/resolver"
	"github.com/Jh123x/prompiler/internal/token"
	"github.com/Jh123x/prompiler/internal/types"
)

func renderSrc(t *testing.T, files map[string]string, name string, inputs eval.InputValues) (string, *eval.RuntimeError) {
	t.Helper()
	parsed := map[string]*ast.File{}
	for path, src := range files {
		toks, _ := lexer.New(src).Lex()
		file, diags := parser.New(toks).ParseFile()
		if len(diags) != 0 {
			t.Fatalf("%s: parse: %v", path, diags)
		}
		parsed[path] = file
	}
	syms, rdiags := resolver.New(parsed).Resolve()
	if len(rdiags) != 0 {
		t.Fatalf("resolve: %v", rdiags)
	}
	sem, cdiags := types.NewChecker(builtin.NewRegistry()).Check(parsed, syms)
	if len(cdiags) != 0 {
		t.Fatalf("typecheck: %v", cdiags)
	}
	return eval.NewComposer(parsed, sem).Render(name, inputs)
}

func TestInterpolationWithDefault(t *testing.T) {
	src := `enum Difficulty { Easy, Medium, Hard }
template Interpolate {
  variables {
    name: string
    age: int
    level: Difficulty
    greeting: string = 'hello'
  }
  prompt {
    Name: {{ name }}
    Age: {{ age }}
    Level: {{ level }}
    Greeting: {{ greeting }}
  }
}`
	out, err := renderSrc(t, map[string]string{"i.ppl": src}, "Interpolate", eval.InputValues{
		"name":  eval.StringVal("Ada"),
		"age":   eval.IntVal(36),
		"level": eval.EnumVal("Difficulty", "Hard"),
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	want := "Name: Ada\nAge: 36\nLevel: Hard\nGreeting: hello\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestWhitespaceAnchoring(t *testing.T) {
	src := `template W {
  variables {
    code: string
    name: string
  }
  prompt {
    Inline: {{ name }} end
    Block:
    {{ code }}
    Done
  }
}`
	out, err := renderSrc(t, map[string]string{"w.ppl": src}, "W", eval.InputValues{
		"code": eval.StringVal("def foo():\n    return 1"),
		"name": eval.StringVal("Ada"),
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	want := "Inline: Ada end\nBlock:\ndef foo():\n    return 1\nDone\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestLoops(t *testing.T) {
	src := `template L {
  variables { items: string[] }
  prompt {
    Items:
    {% for item in items %}
    - {{ item }}
    {% end %}
    Indexed:
    {% for i in range(items.length) %}
    {{ i }}: {{ items[i] }}
    {% end %}
  }
}`
	out, err := renderSrc(t, map[string]string{"l.ppl": src}, "L", eval.InputValues{
		"items": eval.ArrayVal(types.TypeString, []eval.Value{eval.StringVal("apple"), eval.StringVal("banana"), eval.StringVal("cherry")}),
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	want := "Items:\n- apple\n- banana\n- cherry\nIndexed:\n0: apple\n1: banana\n2: cherry\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestConditionals(t *testing.T) {
	src := `template C {
  variables {
    count: int
    label: string = "default"
  }
  prompt {
    {% if count > 5 %}
    big
    {% else %}
    small
    {% end %}
    {% if label == "default" %}
    default-label
    {% end %}
  }
}`
	out, err := renderSrc(t, map[string]string{"c.ppl": src}, "C", eval.InputValues{"count": eval.IntVal(3)})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	want := "small\ndefault-label\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestOptional(t *testing.T) {
	src := `template O {
  variables {
    nickname: Optional<string> = none
    tag: Optional<string>
  }
  prompt {
    {% if nickname.is_empty() %}
    none
    {% else %}
    {{ nickname.value() }}
    {% end %}
    {% if tag.is_empty() %}
    none
    {% else %}
    {{ tag.value() }}
    {% end %}
  }
}`
	out, err := renderSrc(t, map[string]string{"o.ppl": src}, "O", eval.InputValues{
		"tag": eval.SomeVal(eval.StringVal("beta")),
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	want := "none\nbeta\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestMethods(t *testing.T) {
	src := `interface Named {
  name: string
  func label(): string
  func renamed(new_name: string): Named
}
class Person {
  name: string
  age: int
  func label(): string { return this.name + " (" + string(this.age) + ")" }
  func renamed(new_name: string): Person { return Person { name: new_name, age: this.age } }
}
template M {
  variables { person: Person }
  prompt {
    {{ person.label() }}
    {{ person.renamed("Bob").label() }}
  }
}`
	out, err := renderSrc(t, map[string]string{"m.ppl": src}, "M", eval.InputValues{
		"person": eval.ObjectVal(&types.Class{Name: "Person"}, map[string]eval.Value{"name": eval.StringVal("Ada"), "age": eval.IntVal(36)}),
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	want := "Ada (36)\nBob (36)\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestInclude(t *testing.T) {
	files := map[string]string{
		"main.ppl": `import "header.ppl"
template Main {
  variables { company: string }
  prompt {
    {{ include Header(company: company) }}
    Body
  }
}`,
		"header.ppl": `template Header {
  variables { company: string }
  prompt {
Welcome to {{ company }}!
  }
}`,
	}
	out, err := renderSrc(t, files, "Main", eval.InputValues{"company": eval.StringVal("Acme")})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	want := "Welcome to Acme!\nBody\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestEscaping(t *testing.T) {
	src := `template E {
  variables { name: string }
  prompt {
    Lit: \{\{ name \}\}
    Backslash: C:\\path
    Mixed: {{ name }}
  }
}`
	out, err := renderSrc(t, map[string]string{"e.ppl": src}, "E", eval.InputValues{"name": eval.StringVal("Ada")})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	want := "Lit: {{ name }}\nBackslash: C:\\path\nMixed: Ada\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestRuntimeErrors(t *testing.T) {
	t.Run("division_by_zero", func(t *testing.T) {
		src := `template D {
  variables {
    n: int
    d: int
  }
  prompt { {{ n / d }} }
}`
		_, err := renderSrc(t, map[string]string{"d.ppl": src}, "D", eval.InputValues{"n": eval.IntVal(10), "d": eval.IntVal(0)})
		if err == nil || err.Category != token.CatDivisionByZero {
			t.Fatalf("expected division_by_zero, got %v", err)
		}
	})
	t.Run("index_out_of_bounds", func(t *testing.T) {
		src := `template I { variables { items: string[] } prompt { {{ items[5] }} } }`
		_, err := renderSrc(t, map[string]string{"i.ppl": src}, "I", eval.InputValues{"items": eval.ArrayVal(types.TypeString, []eval.Value{eval.StringVal("a"), eval.StringVal("b")})})
		if err == nil || err.Category != token.CatIndexOutOfBounds {
			t.Fatalf("expected index_out_of_bounds, got %v", err)
		}
	})
	t.Run("unwrap_empty_optional", func(t *testing.T) {
		src := `template U { variables { value: Optional<string> } prompt { {{ value.value() }} } }`
		_, err := renderSrc(t, map[string]string{"u.ppl": src}, "U", eval.InputValues{"value": eval.NoneVal(types.TypeString)})
		if err == nil || err.Category != token.CatUnwrapEmpty {
			t.Fatalf("expected unwrap_empty_optional, got %v", err)
		}
	})
}

func TestStringMethods(t *testing.T) {
	src := `template S {
  variables { name: string }
  prompt {
    {{ name.upper() }}
    {{ name.trim() }}
    {{ name.replace_all("H", "J") }}
  }
}`
	out, err := renderSrc(t, map[string]string{"s.ppl": src}, "S", eval.InputValues{"name": eval.StringVal("Hello")})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	want := "HELLO\nHello\nJello\n"
	if !strings.Contains(out, want[:len("HELLO")]) {
		t.Errorf("got %q", out)
	}
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

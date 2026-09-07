package eval_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Jh123x/prompiler/internal/ast"
	"github.com/Jh123x/prompiler/internal/builtin"
	"github.com/Jh123x/prompiler/internal/eval"
	"github.com/Jh123x/prompiler/internal/lexer"
	"github.com/Jh123x/prompiler/internal/parser"
	"github.com/Jh123x/prompiler/internal/resolver"
	"github.com/Jh123x/prompiler/internal/token"
	"github.com/Jh123x/prompiler/internal/types"
)

func renderSrc(t *testing.T, files map[string]string, name string, inputs eval.InputValues) (string, error) {
	t.Helper()
	parsed := map[string]*ast.File{}
	for path, src := range files {
		toks, _ := lexer.New(src).Lex()
		file, diags := parser.New(toks).ParseFile()
		require.Empty(t, diags, "%s: parse diagnostics", path)
		parsed[path] = file
	}
	syms, rdiags := resolver.New(parsed).Resolve()
	require.Empty(t, rdiags)
	sem, cdiags := types.NewChecker(builtin.NewRegistry()).Check(parsed, syms)
	require.Empty(t, cdiags)
	out, rerr := eval.NewComposer(parsed, sem).Render(name, inputs)
	if rerr != nil {
		return out, rerr
	}
	return out, nil
}

func TestInterpolationWithDefault(t *testing.T) {
	t.Run("renders scalars and applies omitted defaults", func(t *testing.T) {
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
		require.NoError(t, err)
		assert.Equal(t, "Name: Ada\nAge: 36\nLevel: Hard\nGreeting: hello\n", out)
	})
}

func TestWhitespaceAnchoring(t *testing.T) {
	t.Run("anchors multi-line values and dedents the body", func(t *testing.T) {
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
		require.NoError(t, err)
		assert.Equal(t, "Inline: Ada end\nBlock:\ndef foo():\n    return 1\nDone\n", out)
	})
}

func TestLoops(t *testing.T) {
	t.Run("iterates arrays directly and by index via range", func(t *testing.T) {
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
		require.NoError(t, err)
		assert.Equal(t, "Items:\n- apple\n- banana\n- cherry\nIndexed:\n0: apple\n1: banana\n2: cherry\n", out)
	})
}

func TestConditionals(t *testing.T) {
	t.Run("renders if/else branches", func(t *testing.T) {
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
		require.NoError(t, err)
		assert.Equal(t, "small\ndefault-label\n", out)
	})
}

func TestOptional(t *testing.T) {
	t.Run("renders some and none Optionals", func(t *testing.T) {
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
		require.NoError(t, err)
		assert.Equal(t, "none\nbeta\n", out)
	})
}

func TestMethods(t *testing.T) {
	t.Run("calls methods with this and a covariant return", func(t *testing.T) {
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
		require.NoError(t, err)
		assert.Equal(t, "Ada (36)\nBob (36)\n", out)
	})
}

func TestInclude(t *testing.T) {
	t.Run("splices an included template", func(t *testing.T) {
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
		require.NoError(t, err)
		assert.Equal(t, "Welcome to Acme!\nBody\n", out)
	})
}

func TestEscaping(t *testing.T) {
	t.Run("renders prompt escapes literally", func(t *testing.T) {
		src := `template E {
  variables { name: string }
  prompt {
    Lit: \{\{ name \}\}
    Backslash: C:\\path
    Mixed: {{ name }}
  }
}`
		out, err := renderSrc(t, map[string]string{"e.ppl": src}, "E", eval.InputValues{"name": eval.StringVal("Ada")})
		require.NoError(t, err)
		assert.Equal(t, "Lit: {{ name }}\nBackslash: C:\\path\nMixed: Ada\n", out)
	})
}

func TestRuntimeErrors(t *testing.T) {
	t.Run("division by zero", func(t *testing.T) {
		src := `template D {
  variables {
    n: int
    d: int
  }
  prompt { {{ n / d }} }
}`
		_, err := renderSrc(t, map[string]string{"d.ppl": src}, "D", eval.InputValues{"n": eval.IntVal(10), "d": eval.IntVal(0)})
		var rerr *eval.RuntimeError
		require.ErrorAs(t, err, &rerr)
		require.Equal(t, token.CatDivisionByZero, rerr.Category)
	})

	t.Run("index out of bounds", func(t *testing.T) {
		src := `template I { variables { items: string[] } prompt { {{ items[5] }} } }`
		_, err := renderSrc(t, map[string]string{"i.ppl": src}, "I", eval.InputValues{"items": eval.ArrayVal(types.TypeString, []eval.Value{eval.StringVal("a"), eval.StringVal("b")})})
		var rerr *eval.RuntimeError
		require.ErrorAs(t, err, &rerr)
		require.Equal(t, token.CatIndexOutOfBounds, rerr.Category)
	})

	t.Run("unwrap of an empty optional", func(t *testing.T) {
		src := `template U { variables { value: Optional<string> } prompt { {{ value.value() }} } }`
		_, err := renderSrc(t, map[string]string{"u.ppl": src}, "U", eval.InputValues{"value": eval.NoneVal(types.TypeString)})
		var rerr *eval.RuntimeError
		require.ErrorAs(t, err, &rerr)
		require.Equal(t, token.CatUnwrapEmpty, rerr.Category)
	})
}

func TestStringMethods(t *testing.T) {
	t.Run("applies the string method surface", func(t *testing.T) {
		src := `template S {
  variables { name: string }
  prompt {
    {{ name.upper() }}
    {{ name.trim() }}
    {{ name.replace_all("H", "J") }}
  }
}`
		out, err := renderSrc(t, map[string]string{"s.ppl": src}, "S", eval.InputValues{"name": eval.StringVal("Hello")})
		require.NoError(t, err)
		assert.Equal(t, "HELLO\nHello\nJello\n", out)
	})
}

func TestNestedMethodThis(t *testing.T) {
	t.Run("restores this after a nested method call", func(t *testing.T) {
		src := `class Inner {
  x: int
  func get(): int { return this.x }
}
class Outer {
  inner: Inner
  tag: string
  func combined(): string {
    var v = this.inner.get()
    return this.tag + string(v)
  }
}
template T {
  variables {
    o: Outer
  }
  prompt {
{{ o.combined() }}
  }
}`
		inner := eval.ObjectVal(&types.Class{Name: "Inner"}, map[string]eval.Value{"x": eval.IntVal(5)})
		outer := eval.ObjectVal(&types.Class{Name: "Outer"}, map[string]eval.Value{"inner": inner, "tag": eval.StringVal("T")})
		out, err := renderSrc(t, map[string]string{"t.ppl": src}, "T", eval.InputValues{"o": outer})
		require.NoError(t, err)
		assert.Equal(t, "T5\n", out)
	})
}

func TestMapDuplicateKeys(t *testing.T) {
	t.Run("deduplicates a repeated literal key", func(t *testing.T) {
		src := `template T {
  prompt {
{{ {"a": 1, "a": 2}.length }}
  }
}`
		out, err := renderSrc(t, map[string]string{"t.ppl": src}, "T", nil)
		require.NoError(t, err)
		assert.Equal(t, "1\n", out)
	})
}

func TestStringLengthRunes(t *testing.T) {
	t.Run("counts runes, not bytes", func(t *testing.T) {
		src := `template T {
  variables {
    name: string
  }
  prompt {
{{ name.length }}
  }
}`
		out, err := renderSrc(t, map[string]string{"t.ppl": src}, "T", eval.InputValues{"name": eval.StringVal("héllo")})
		require.NoError(t, err)
		assert.Equal(t, "5\n", out)
	})
}

func TestAnchoringColumn(t *testing.T) {
	t.Run("anchors a second inline interpolation at its own column", func(t *testing.T) {
		src := `template T {
  variables {
    a: string
    b: string
  }
  prompt {
X{{ a }}{{ b }}Y
  }
}`
		out, err := renderSrc(t, map[string]string{"t.ppl": src}, "T", eval.InputValues{
			"a": eval.StringVal("1\n2"),
			"b": eval.StringVal("p\nq"),
		})
		require.NoError(t, err)
		assert.Equal(t, "X1\n 2p\n  qY\n", out)
	})
}

func TestRangeStepOverflow(t *testing.T) {
	t.Run("reports overflow instead of looping", func(t *testing.T) {
		src := `template T { prompt { {% for i in range_step(1, 9223372036854775807, 9223372036854775807) %}{{ i }}{% end %} } }`
		_, err := renderSrc(t, map[string]string{"t.ppl": src}, "T", nil)
		var rerr *eval.RuntimeError
		require.ErrorAs(t, err, &rerr)
		require.Equal(t, token.CatOverflow, rerr.Category)
	})
}

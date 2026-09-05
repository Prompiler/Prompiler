# Prompiler examples

This directory contains worked examples for the
[Prompiler language specification](../docs/spec.md). Each example is a
self-contained subdirectory with three artifacts:

1. **Template code** — one or more `.ppl` files (the provisional extension).
2. **`variables.json`** — the typed input values for the entry template's
   variables (what the TUI `ValueSource` would prompt for).
3. **`solution.json`** — the expected rendered prompt **and** expected errors.

There is no compiler in this repository yet, so these examples are authored
*against the spec*, not produced by running a binary. They are intended to be
read as learning material and, once a compiler lands, reused as test fixtures.

## Directory layout

```
examples/
  feature/<name>/     # one minimal syntax feature each
  scenario/<name>/    # realistic prompts exercising several features
  errors/<name>/      # intentionally-failing examples (type + runtime)
```

Each directory contains `<name>.ppl` plus `variables.json` and `solution.json`.
Multi-file scenarios (currently `scenario/onboarding-email`) keep their `.ppl`
files flat in the directory and reference each other via relative
`import "other.ppl"` and `{{ include Other(...) }}`.

## `variables.json`

A single JSON object mapping variable name → value. This is an **input
fixture** — it represents the values the TUI would collect. The spec does *not*
read variables from files; `variables.json` is purely a test/example input.

Type mapping:

| PPL type                | JSON representation                                        |
|-------------------------|------------------------------------------------------------|
| `string` / `int` / `float` / `bool` | JSON string / number / number / boolean          |
| enum                    | JSON string of the bare member name (e.g. `"Hard"`)        |
| class                   | JSON object of `field: value`                              |
| interface               | JSON object with a `"$type"` discriminator + its fields    |
| `T[]`                   | JSON array                                                 |
| `map<string, T>`        | JSON object                                                |
| `Optional<T>`           | `null` = `none`; otherwise the wrapped value = `some(x)`   |

Optional variables (those with a `default`) may be omitted from
`variables.json`; the default then applies (e.g. `interpolation` omits
`greeting`, `enums` omits `level`). `{{ include }}` binds a child template's
variables via named arguments in source, so only the *entry* template's own
variables appear in `variables.json` (see `scenario/onboarding-email`).

## `solution.json`

```json
{
  "template": "Greet",
  "expected_prompt": "    Hello World!\n    End\n",
  "expected_errors": []
}
```

- Valid examples: `expected_errors` is `[]`; `expected_prompt` is the exact
  rendered string, whitespace included.
- Error examples: `expected_prompt` is `null`; `expected_errors` is a list of
  structured entries:

```json
{
  "stage": "typecheck",
  "category": "type_mismatch",
  "line": 8,
  "column": 10,
  "message": "operator + cannot be applied to int and string (no implicit coercion)"
}
```

`stage` is one of `"parse" | "typecheck" | "runtime"`. `line`/`column` and
`message` are **illustrative** — they are a best-effort span and a
human-readable description, intended to be reconciled with real compiler
diagnostics once an implementation exists.

## Whitespace model used by `expected_prompt`

The spec's whitespace rule (§8.4) is preserved exactly. The expected outputs in
this directory assume the following concrete reading:

- Prompt-body literal text is emitted verbatim, each line keeping its written
  indentation and a trailing newline.
- `{{ expr }}` replaces the expression in place. A **single-line** value is
  inserted directly; a **multi-line** value anchors its first line at the
  expression's column and adds that column to each subsequent line's own
  relative indentation (see `feature/whitespace-anchoring`).
- `{% for %}` / `{% if %}` / `{% else %}` / `{% end %}` occupy their own line
  and emit nothing (their trailing newline is consumed).
- `{{ include Child(...) }}` on its own line behaves like a control tag for
  newline purposes: the line is replaced by the child's rendered output, spliced
  at the include's column. To keep splicing unambiguous, included templates in
  `scenario/onboarding-email` write their prompt bodies at column 0.

### Known assumption: map ordering

The spec does not pin down `keys()`/`values()` ordering. Examples that iterate a
map (`feature/maps`, `scenario/weekly-status-report`) assume insertion order
(the order keys appear in `variables.json`) for illustration; a compiler may
need to sort or otherwise normalize these fixtures.

## Coverage map

| Spec section | Covered by |
|---|---|
| §3.1 Comments (line + block) | `feature/interpolation` |
| §3.3 Literals (int/float/string/bool, both quote styles) | `feature/interpolation`, `feature/literals-expressions` |
| §3.4 Composite/optional literals | `feature/maps`, `feature/classes`, `feature/optional` |
| §4.1 Primitives | `feature/interpolation` |
| §4.2 Array | `feature/loops`, `scenario/code-review`, `scenario/quiz-generator` |
| §4.3 Map | `feature/maps`, `scenario/weekly-status-report` |
| §4.4 Enum | `feature/enums`, `scenario/code-review`, `scenario/weekly-status-report` |
| §4.5 Class | `feature/classes`, `feature/methods`, `scenario/code-review`, `scenario/quiz-generator` |
| §4.6 Interface (structural typing: fields + method signatures, covariant returns) | `feature/interfaces`, `feature/methods`, `errors/interface-unsatisfied`, `errors/missing-method` |
| §4.7 Builtin methods + `Optional<T>` | `feature/classes`, `feature/maps`, `feature/optional`, `scenario/commit-message` |
| §5.4 `func` | `feature/functions`, `feature/methods`, `scenario/commit-message` |
| §5.5 `template` + `description` | all examples; `description` in `feature/interpolation` and `scenario/code-review` |
| §6 Variables, defaults, required vs optional | `feature/interpolation`, `feature/enums`, `errors/default-not-assignable` |
| §7 Functions & methods (var/if/for/return, forward refs, `this`) | `feature/functions`, `feature/methods` |
| §8.1 Interpolation string forms | `feature/interpolation` |
| §8.2 Loops | `feature/loops`, `scenario/code-review`, `scenario/quiz-generator`, `scenario/weekly-status-report` |
| §8.3 Conditionals | `feature/conditionals`, `scenario/commit-message`, `scenario/code-review`, `scenario/quiz-generator` |
| §8.4 Whitespace anchoring | `feature/whitespace-anchoring` |
| §8.5 Escaping | `feature/escaping` |
| §8.6 `include` | `scenario/onboarding-email`, `errors/cyclic-include` |
| §9 Expressions + operator precedence | `feature/literals-expressions` |
| §10 Standard library (`range`/`join`/casts) | `feature/standard-library`, `scenario/code-review` |
| §10.1 Loop index via `range` | `feature/loops`, `scenario/code-review`, `scenario/quiz-generator` |
| §2.1 Imports | `scenario/onboarding-email` |
| §11 Type-checking diagnostics | `errors/type-mismatch`, `errors/iterate-non-array`, `errors/unknown-type`, `errors/interface-unsatisfied`, `errors/default-not-assignable`, `errors/duplicate-name`, `errors/cyclic-include`, `errors/unknown-method`, `errors/duplicate-member`, `errors/this-outside-method`, `errors/missing-method` |
| §16.1 Runtime errors | `errors/index-out-of-bounds`, `errors/division-by-zero`, `errors/range-negative`, `errors/failed-cast`, `errors/unwrap-empty-optional` |
| §16.2 Template composition (DAG) | `scenario/onboarding-email`, `errors/cyclic-include` |

## Deliberately-absent features

Per §4.8, Prompiler has no `null`, no union types, no user generics, and no
`any`/`unknown`/`never`. These have no syntax to demonstrate; absence of a value
is expressed with `Optional<T>` (`feature/optional`), and alternatives are
modeled with interfaces + classes (`feature/interfaces`).

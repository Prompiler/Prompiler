# Prompiler — Feature Backlog

Tracks the features and decisions Prompiler still needs, kept separate from the DDD structure ([`architecture.md`](architecture.md)) and the language definition ([`spec.md`](spec.md)).

**Status legend:** `proposed` — not yet decided · `accepted` — decided, needs folding back into `spec.md` · `deferred` — out of scope for now.

## 1. Spec decisions that block implementation (P0)

Ambiguities in `spec.md` that an implementation must resolve. Each is a *decision*, with a proposed resolution and the spec section it belongs to.

| # | Decision | Proposed resolution | Spec § |
|---|---|---|---|
| 1 | Whitespace/newline model | Line-oriented rendering: dedent, then classify each body line as pure-control-tag (emits nothing, drops its newline), own-line include (splice at column, consume newline), or inline-content (replace in place, keep newline). | §8.4 |
| 2 | Field-mutability rule | Element assignment is legal only when the lvalue's root is a local `var`, parameter, loop variable, or template variable bound to an array/map (or an index chain rooted there); any `.field`/`this` path is read-only. | §4.5, §7.2 |
| 3 | `var x = x + 1` self-reference | A `var` initializer resolves the name in the **outer** scope (binding not yet in scope); no outer binding = unknown-name error. | §7.1 |
| 4 | Prompt-text escaping | Exactly `\{`, `\}`, `\\`. Any other backslash in prompt text is a compile error; a lone `{` not starting `{{`/`{%` is literal; a literal `}` must be `\}`. | §8.5 |
| 5 | Literal grammar | Ints `[0-9]+`, floats `[0-9]+.[0-9]+`; no hex/octal/binary/exponent/leading/trailing dot. Negation is the unary `-` operator. Trailing commas legal in enum/array/map/class literals. | §3.3 |
| 6 | Map & Optional variance | Writable containers invariant; read-only envelopes covariant. Maps invariant in value type; `Optional<T>` covariant. | §11.2 |
| 7 | `Optional<T>` equality | `none == none` true; `none == some(_)` false; `some(a) == some(b)` iff `a == b` (structural). | §9.1 |
| 8 | Interface-typed receiver mutability | Interface-typed values are field-read-only and method-callable; element assignment through them is a compile error. | §4.6 |
| 9 | Composite defaults | Array/map/class literals are legal defaults; defaults must be constant-literal expressions only (no identifiers, calls, member access, or operators). | §6.2 |
| 10 | Map iteration order | `keys()`/`values()` in **insertion order**; map equality ignores order. | §4.3, §10 |
| 11 | Name spelling | Canonical: product **Prompiler**, binary `prompiler`, Go module `github.com/Jh123x/promptpiler`, extension `.ppl`. | §1, §15, §17 |
| 12 | Duplicate-name boundary | Resolver owns binding uniqueness (scope/imports/params/locals); TypeChecker owns body-internal uniqueness (enum members, class member set). | §11, §12 |
| 13 | §4.7 `OPEN` cross-listing | List every `OPEN` marker in §17; register the builtin surface behind a replaceable registry. | §17 |

## 2. Agent-harness integration (P1)

Make templates consumable by LLM agent harnesses (Claude Code, LangChain, MCP clients). All are additive — new ports/adapters or a new Static-Analysis query — and do not change the pure core.

1. **JSON Schema export** — map a template's `variables` block to JSON Schema:
   - `string`/`int`/`float`/`bool` → `string`/`integer`/`number`/`boolean`; enum → `enum`; class → object + a `$type` const; interface → `oneOf` of satisfying classes via the `$type` discriminator; `T[]` → `array`; `map<string,T>` → `additionalProperties`; `Optional<T>` → nullable; default → `default`.
   - This turns each template into a callable tool: name + `description` + parameters + rendered-prompt output.
2. **Embeddable library API** — a stable, semver'd public surface: `Analyze`, `Schema`, `Render`, `Templates`. CLI/TUI/LSP/MCP all become adapters over it.
3. **MCP adapter** — expose templates as MCP tools (`render_<template>`) and resources (source + schema); a new adapter in the Application Shell, sibling to LSP.
4. **Typed JSON `ValueSource`** — validate/coerce untyped JSON (e.g. an LLM's tool-call arguments) against a template's schema, returning typed values or positioned diagnostics; generalizes the `variables.json` test harness.
5. **Live discovery** — `promptpiler list --json` and MCP `list_tools`/`list_resources`, computed on demand. **No committed manifest** (it would be derived data; git is the source of truth) and **no `version`/`id`/`tags` metadata** until templates are distributed as a package outside git.

**Open mapping decisions** for items 1 and 4:
- `Optional<T>` ↔ JSON: map `none` → JSON `null` (round-trip fidelity), and rely on `required` for required-ness.
- `interface` ↔ `oneOf`: make `$type` the explicit JSON Schema `discriminator`.

## 3. Deferred items (P2)

| Item | Revisit trigger |
|---|---|
| Language name & `.ppl` extension | before v1.0 |
| Stdlib function-list growth | when a new util is needed |
| Builtin method surface (§4.7) | when a method proves missing in fixtures |
| `entries()` for maps | when map-entry iteration is demanded |
| Interface `extends` | when a shared interface hierarchy appears |
| LSP rename / find-references / workspace symbol | after v1 diagnostics/hover/completion/definition |

## 4. Known edge cases (test plan)

The `examples/errors/` fixtures are the executable catalog; this list names the clusters to keep in view, grouped by stage.

- **Lex/parse:** unterminated string/comment/construct; CRLF vs LF; lone `{`/`}`; control-tag imbalance (`for` without `end`); rejected literal shapes (`1.`, `.5`, `1e3`); span fidelity for LSP.
- **Resolution:** missing/cyclic imports; shadowing vs same-scope duplicate; forward refs; unknown names; `var x = x + 1`.
- **Type check:** structural satisfaction (missing/mistyped field, arity, non-covariant return); no implicit coercion; array/map invariance + Optional covariance; recursive-type guard; `this`/rebind/element-assignment rules; bare `none`; iterate-non-array; include arity/cycle; constant folding hoists §16.1 errors to compile time.
- **Eval/render:** index OOB (read/write); div/mod by zero; 64-bit overflow; failed cast; unwrap-empty; negative `range`; zero `range_step`; multi-line anchoring (trailing newline, CRLF, blank dedent lines); include own-line vs inline; memoize repeated includes; **runtime data cycles** (a mutable array reachable through a recursively-typed class field — bound traversal with a visited set).
- **Value capture:** interface → pick satisfying class; required vs optional Optional; recursive-class drilling; duplicate map keys.
- **Editor:** offset/encoding translation; diagnostics de-dup; go-to-definition across imports.
- **Shell:** `run` on an also-included template; unknown template; output-device failure = I/O error, not a language error.

## 5. Suggested implementation order

1. BC1 front-end (lexer → parser → resolver → checker), gated on `feature/` + `errors/` (parse/typecheck) fixtures.
2. BC2 renderer/evaluator, gated on `solution.json`.
3. BC3 + TUI + the JSON `ValueSource` harness.
4. Library API + JSON Schema export (P1 items 1–2).
5. BC4 LSP.
6. MCP adapter (P1 item 3).
7. Conformance harness in CI.
8. Fold accepted spec decisions (P0) back into `spec.md`.

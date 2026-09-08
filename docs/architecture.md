# Prompiler — Software Architecture (DDD)

## Purpose & status

This document describes the **Domain-Driven Design (DDD)** structure of Prompiler — a strongly-typed, pure, deterministic language for authoring LLM prompt templates, shipped as a compiler/runtime binary, with an LSP language server (BC4) as target design — not yet implemented.

- **Status:** draft. It is a companion to [`docs/spec.md`](spec.md), which remains the normative language specification.
- **Scope:** this document is the DDD structure *only*. Spec decisions, the feature backlog, edge cases, and deferred items live in [`docs/features.md`](features.md).
- **Level:** high-level. Modules and interfaces are described conceptually; there are no concrete Go type signatures. The implementer maps these modules onto the package layout the spec already prescribes (§15).

### Implementation status

- **BC1 (static analysis), BC2 (rendering/evaluation), BC3 (value capture), and BC5 (application shell)** are implemented.
- **BC4 (editor integration / LSP)** is designed but **NOT YET IMPLEMENTED**; its design is retained below as the target.
- In code, the port the spec calls `TemplateStore` is implemented under the name **SourceProvider** (which supersedes the `TemplateStore` name).

---

## 1. Ubiquitous language

All layers — lexer, parser, resolver, type checker, evaluator, LSP, TUI — must use this vocabulary with one meaning. It is drawn verbatim from the spec; where the spec uses a word loosely, this glossary pins it.

### Program & modules
- **Program** — the full resolved, type-checked unit of compilation: every `.ppl` file reachable from a project root plus its imports, assembled and analyzed together. Immutable once produced.
- **Module / source file** — one `.ppl` file; a named container of top-level declarations; the unit of `import`.
- **Project root** — the first ancestor directory containing `prompiler.toml`, walking up from the working directory. *(Not yet implemented: there is no root-marker file mechanism today; the root is simply the `-root` flag, default `.`.)*
- **Import** — a whole-file, path-relative reference that makes a target module's top-level names visible. *Static.*
- **Include** — a template-composition construct `{{ include Child(...) }}` that splices another template's rendered output in place. *Dynamic, per-template.* Distinct from import.
- **Include DAG** — the directed graph of template-includes; must be acyclic.
- **Entry template** — the template targeted by `run`; its variables are prompted.

### Declarations & types
- **Type** — a primitive (`string`/`int`/`float`/`bool`), composite (`T[]`, `map<string,T>`, `Optional<T>`), enum/class/interface type, or builtin. A value object.
- **Enum / member** — a closed set of named constants; each member (`Name.Member`) is its own type.
- **Class** — a nominal record of typed, immutable fields plus methods.
- **Interface** — a structural contract: required fields plus method signatures; nothing declares `implements`.
- **Satisfaction** — the structural test "class *C* satisfies interface *I*": fields ⊇, method arity/param-types match, method returns covariant.
- **Assignability** — the type relation: type equality, or class satisfies interface. No implicit coercion.
- **Cast** — an explicit conversion `int/float/string/bool(x)`; never implicit.
- **Field / method / signature / receiver (`this`)** — class parts; `this` is the read-only implicit receiver, valid only inside a method body.

### Input model
- **Template variable** — a typed input slot declared in a template's `variables` block; **immutable**.
- **Required vs. optional variable** — no `default` vs. has a `default`; optional may be skipped and falls back to the default.
- **Default** — a compile-time-constant literal attached to a variable.
- **Input value** — a concrete typed value supplied for a variable (collected from a user; never read from the environment).

### Prompt body
- **Prompt body** — a template's `prompt { … }` block of literal text plus constructs.
- **Construct** — an interpolation `{{ expr }}`, a control tag `{% for … %}`/`{% if … %}`/`{% else %}`/`{% end %}`, or an include `{{ include … }}`.
- **Render** — produce the final prompt string deterministically from a template + input values.

### Computation
- **Expression / statement** — the precedence-climbing expression tree and the statement forms.
- **Scope / binding / shadow** — lexical name resolution; inner scopes may shadow outer or imported names; same-scope duplicates are errors.
- **Runtime value** — a scalar, enum member, class instance, array/map reference, or Optional at evaluation time.
- **Reference type** — array/map (aliased, element-mutable); everything else is passed by value.
- **Compile-time constant** — an operand computable statically; constant folding hoists certain runtime errors to compile time.
- **Diagnostic** — a positioned problem (span, message, category, severity); the shared currency of all error reporting.
- **Span** — a source range (byte offset, line, column) carried by tokens, AST nodes, and diagnostics.
- **Runtime error** — an evaluation failure that passes type checking but not constant folding.

### Boundaries
- **Port** — an interface the domain core owns and depends on, implemented outside it by an adapter.
- **Adapter** — an outer implementation of a port (TUI, filesystem, LSP JSON-RPC, test harness).

---

## 2. Bounded contexts

Five bounded contexts. The split is justified by *reason to change*: each context answers a different question, fails in a different way, and talks to a different external partner. Where two contexts meet there is explicit translation, never shared mutable state.

### BC1 — Static Analysis ("Authoring & Analysis")

> *"Is this source a well-formed, well-typed Prompiler program?"*

- **Responsibility:** lexing, error-recovering parsing, import resolution, symbol/scoping resolution, type checking, structural-satisfaction checking, constant folding, and the duplicate detection that needs type information.
- **Serves:** `check`, and every other context.
- **Failure mode:** compile-time `Diagnostic`s. **Time domain:** per keystroke / per `check`.
- **Core concepts:** the **Program** aggregate (§3), the analyzed **Template** entity, a type registry, and a **SemanticModel** query surface (type-of-expression, symbol-at-position, satisfying-class enumeration).
- **External interface:** accepts a set of source texts + a root (§4, `SourceProvider`); returns an immutable **Analyzed Program** and a **Diagnostic** list.
- **Why a context:** it holds the strongest invariants ("typedness"; no partial program may leak). It must be consumable by both `check` and the LSP without dragging in evaluation. Because it is pure and deterministic, the same text yields the same Program — cacheable and goroutine-safe for the LSP.

### BC2 — Prompt Rendering & Evaluation ("Rendering")

> *"Given an analyzed template and concrete input values, what is the deterministic output string?"*

- **Responsibility:** tree-walking evaluation over the type-checked AST, the runtime value model, template composition (include-DAG execution with memoization), and the whitespace/layout renderer.
- **Failure mode:** runtime errors (never I/O errors). **Time domain:** one `run` invocation.
- **Core concepts:** the **Template** aggregate (variables + cached `PromptLayout` + include edges); domain services `TemplateComposer`, `Evaluator`, `PromptRenderer`, `ValueStringifier`.
- **External interface:** `Render(analyzedTemplate, InputValueSet) → output string | runtime problem(s)`.
- **Why a context:** determinism and purity are only enforceable if evaluation is a closed pure core with the human/device on the *other side* of a port. It must not know where values came from or where the string goes — that is what lets the TUI, a web/GUI, or the test harness share it untouched. Splitting it from BC1 also matches the spec's two error regimes (compile-time vs. runtime), and lets `check`/LSP avoid carrying an interpreter.

### BC3 — Interactive Value Capture ("Value Collection")

> *"Given an entry template's input model, how do we obtain a valid input value set from a user?"*

- **Responsibility:** turning a template's variables + types into a drillable form (scalar/enum text-or-select; class/interface field drilling; array/map element capture; Optional skip-when-default), enforcing required-vs-optional, and — for interface-typed variables — letting the user pick which *satisfying class* to provide. It produces an **InputValueSet** for BC2.
- **Core concepts:** the **InputForm** aggregate derived from a template's variables; field/element prompts; the collected value tree.
- **External interface:** an outbound **ValueSource** port — the collector *asks* the port "here is the value for this form-field."
- **Why a context:** value capture has its own domain model (forms, drilling, discrimination, defaults) that exists independent of *which* UI shows it. The boundary is exactly the `ValueSource` port the spec names — the seam that makes the TUI replaceable by a web/GUI and by the `variables.json` test harness. Keeping it out of BC2 preserves determinism: BC2 never asks anyone anything.

### BC4 — Editor Integration ("Language Server Tooling")

> *"How do we speak the editor's language about a Prompiler program?"*

- **Responsibility:** the LSP JSON-RPC protocol — mapping text positions ↔ offsets (including encoding differences), buffering unsaved sources, publishing diagnostics, and answering hover/completion/go-to-definition by translating editor coordinates into BC1 queries and back.
- **Core concepts:** protocol DTOs, the workspace view of modules (dirty-buffer overlay), request/response lifecycle.
- **External interface:** consumes an analyzed-program provider and a diagnostics feed; speaks LSP over a transport.
- **Why a context:** the LSP protocol is a *different language* from the prompt language, with its own versioning, encodings, and lifecycle — an external-model collision that DDD puts behind a translation boundary. Its churn must not reach the type checker.

### BC5 — Command & Session Orchestration ("Application Shell")

> *"Which use case is being run, and how are the pure contexts wired to the real world?"*

- **Responsibility:** the use cases — `list`, `check`, `run <template>`, and `lsp` (planned; not yet implemented) — and the sessions behind them. It owns the ports, composes BC1→BC2→BC3 for `run`, wires BC1→BC4 for `lsp`, and hosts the TUI as a view adapter.
- **Core concepts:** application services (`ListTemplates`, `CheckTemplates`, `RunTemplate`, `ServeLanguageServer`); session state (root, open buffers); the port collection.
- **Why a context:** this is the only context that touches I/O, and it must be the *only* one. The spec's principles are enforced by making BC5 the exclusive owner of side effects — DDD's application layer: thin, use-case-named, orchestration over the domain.

### Context map

```
BC5 Shell ──uses──▶ BC1 Static Analysis
   │                        ▲
   ├─▶ BC2 Rendering ◀──────┘
   │        ▲
   ├─▶ BC3 Value Capture ──▶ BC1 (type info only)
   └─▶ BC4 Editor ──────────▶ BC1 (analyzed program) / BC5 (buffers)
```

BC1 and BC2 are pure and know nothing of BC3–BC5. BC3/BC4 translate against BC1's public queries. BC5 is the only context that wires ports.

---

## 3. Domain model

### Entities, value objects, aggregates — the global call

**The Program is the aggregate of the Static Analysis context (its root).** A single declaration is meaningless in isolation: forward references, whole-file imports, cross-template includes, and structural satisfaction mean consistency is only definable over the *whole resolved set*. Its invariants:

- every referenced name resolves;
- no same-scope duplicate declarations;
- no cyclic imports;
- no direct/indirect recursive function or method calls (call graph acyclic);
- no cyclic includes;
- every expression type-checks;
- runtime errors on constant operands are already hoisted to compile time.

Invariants are *enforced by construction*: a Program is only produced by the factory `Analyze(root, sourceSet)`, which runs lexer → parser → resolver → type checker. That factory returns *either* diagnostics *or* a Program — never a half-checked Program. The Program is **immutable**, which is load-bearing for the LSP: one analyzed program is cached across requests and shared safely across goroutines; re-analysis is triggered only by a versioned source change. It is also *value-independent* — the same object BC2 renders and BC3 drills; they never re-derive it.

The Program owns child **entities**: type declarations (enum/class/interface), functions, and templates, each with identity (its fully qualified name). They are *not* separate aggregates because a declaration never changes independently of the Program — there is no partial update; an LSP edit produces a *new* Program, reusing unchanged cached modules.

**The Template is the aggregate of the Rendering context (root of a `run`).** It owns its variables (the input model), its cached **PromptLayout** (computed at analysis time), and its include edges to child Templates. Its invariant — the include DAG is acyclic — is guaranteed by BC1 and so taken as given here. A run is a traversal of this aggregate that memoizes each included child's rendering so a repeated include is computed once.

**Value objects everywhere.** Types (including `T[]` / `map<string,T>` / `Optional<T>` and the enum/class/interface types) are value objects compared by equality, aggregated into an assignability/satisfaction algebra owned by BC1. AST nodes are immutable value objects with spans. `SourceSpan`, `Position`, `Diagnostic`, `PromptLayout`, and the runtime-error classification are all value objects.

**Runtime values split by identity (BC2).** Scalars, enums, and class instances are value objects. **Arrays and maps are entities/refs** — their *identity* is the aliasing that reference semantics depends on; element assignment mutates the referenced collection. This is the one place the model deliberately breaks value-object purity, and it is *why* arrays and maps must be invariant while read-only envelopes may be covariant.

### Domain services

Stateless, pure services — never aggregates.

- **BC1:** `Lexer`; `Parser` (error-recovering); `Resolver` (import graph + scope/symbol table); `TypeChecker` (assignability, satisfaction, constant folding); `SatisfactionChecker`; `SemanticModel` (the query index BC4 and BC3 call).
- **BC2:** `Evaluator`, `TemplateComposer`, `PromptRenderer`, `ValueStringifier`.
- **BC3:** `InputFormBuilder`.
- BC4/BC5 hold protocol/application services rather than domain services.

### The renderer / whitespace engine (the subtle, shared one)

Split in two, because the two halves have different consumers and different value-dependencies:

1. **PromptLayout (value-independent, computable at analysis time).** Dedent computation and line-role classification depend only on the prompt body *text*, not on any runtime value. It is computed once, cached on the **Template** aggregate, owned by the Rendering context, but exposed so the editor context can read it.
2. **PromptRenderer (value-dependent).** Substitutes concrete strings — inline insertion, multi-line anchoring, control-tag line suppression, include splicing. This needs input values, so it lives in BC2 and is driven only by evaluation in v1.

**Why the split:** `check`/`list`/TUI *display* and any future LSP **preview** want to know "how is this construct indented / what does this line render as" without a full evaluation; `PromptLayout` gives them that statically. When LSP preview arrives it will feed the *renderer* synthetic values through the same value interface BC2 already uses — so the renderer must depend on a "here are the values" input, never on a TUI. The whitespace engine is **owned by Rendering, structured as static-layout + dynamic-render, both halves reachable by BC4 without an interactive session.**

---

## 4. Ports & adapters (hexagonal)

### Ports the domain core owns

- **ValueSource** — "give me the typed value for this variable / form-field." The human-in-the-loop is the only non-determinism in a run; making it a port confines it. Adapters: **TUI** (v1), **test-fixture** (reads `variables.json`), future web/GUI.
- **OutputSink** — "emit the rendered prompt." The domain produces a string; where it goes (stdout, file, clipboard, LSP buffer) is adapter-selected. Justification: the output medium is a device concern with its own failure modes — a full disk is not a language error and must not surface inside BC2.
- **SourceProvider** — "resolve a module path to source text; enumerate modules at a root." **Split out of the spec's `TemplateStore`**, because BC1 must compile from **in-memory text** (LSP unsaved buffers). *Store* semantics (returning analyzed templates) belong inside BC1/BC5; the raw-text-with-identity seam must be separate and primary. Adapters: **filesystem**, **in-memory**, and an **LSP workspace overlay** (dirty buffers shadowing disk). *(Not yet implemented: the described root-discovery logic that reads a `prompiler.toml` marker does not exist yet; today the root is taken from the `-root` flag, default `.`.)*

### Argued against: `DiagnosticSink` as a port

BC1/BC2 are pure: they **return** diagnostics as values. A sink would be global output state leaking into the pure core and would make one analysis depend on *who is listening*. Instead, BC5/BC4 adapters own formatting and emission (human/JSON text; LSP `publishDiagnostics`). If streaming progress or telemetry is wanted later, add a *notification* port at BC5 — never inside BC1/BC2.

### The SemanticModel query API (a boundary, not a port)

BC1 exposes a published query surface — satisfying-class enumeration, type-of-position, symbol-at-position — that BC3/BC4 consume. It is an interface *between contexts* but lives inside the core, so it is not an infrastructure port.

### Adapters (all outside the core)

- **Source:** filesystem reader; in-memory reader; LSP workspace (dirty-buffer) overlay.
- **Value collection:** TUI terminal; scripted/test fixture (`variables.json`); future web.
- **Output:** stdout; file; clipboard.
- **Protocol:** LSP JSON-RPC adapter (BC4); CLI command parser (BC5).
- **Diagnostics:** human text printer; JSON printer; LSP publisher.
- **Test/conformance harness** (valuable and cheap *because of* the seams): a `ValueSource` fed by `variables.json` and an `OutputSink`/comparator over `solution.json`. The `examples/` fixtures are a de-facto conformance suite; the port seams let that harness be one more adapter, which both validates the architecture and turns the fixtures into a regression suite.

---

## 5. Why DDD (and hexagonal)

1. **The spec is already a domain statement.** Rich, precise, opinionated vocabulary, named invariants, and a named pipeline. DDD's job is to make that vocabulary the code's vocabulary (ubiquitous language) rather than leaking implementation terms across layers.
2. **Aggregates protect the invariants that make the language trustworthy.** "Strongly typed" is only a guarantee if no half-typed program can reach evaluation or the LSP — so **Program** is an aggregate-with-factory. "Deterministic" is only a guarantee if **Template/render** is a closed aggregate with a total evaluator and the include-DAG invariant already discharged.
3. **Multiple consumers, one core.** A compiler that must serve a CLI, a TUI, and an LSP — from *unsaved buffers* — is textbook hexagonal: the domain compiles from abstract source text and emits abstract diagnostics/output; each consumer adapts.
4. **Bounded contexts sit exactly on the fault lines.** Compile-time vs. runtime errors (BC1 vs BC2); pure computation vs. interactive value capture (BC2 vs BC3 — the determinism boundary); the prompt language vs. the LSP protocol language (BC1/BC4 — an anti-corruption seam); world-touching vs. pure (BC5 vs. everything). Each design principle maps to a boundary: **Pure** → only BC5 touches ports; **Deterministic** → BC2 is a total pure function and asks nothing; **Strongly typed** → the BC1 aggregate factory; **Safe** → all side effects confined behind ports/adapters, so there is *no mechanism* for a program to touch the world.
5. **Immutability + caching that the LSP needs falls out naturally.** An immutable analyzed Program is shareable across editor requests; re-analysis is triggered only by versioned source change. Explicit aggregate identity makes the LSP cache-and-invalidation story a first-class decision rather than an afterthought.
6. **An honest failure taxonomy.** Contexts that fail in different ways (parse/typecheck diagnostics vs. runtime problems vs. I/O errors) keep "compile-time hoisting of constant-triggered runtime errors" expressible without confusion, because all are the same `Diagnostic` currency with a shared category set.

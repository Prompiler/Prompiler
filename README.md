# Prompiler

A strongly-typed, pure, deterministic markup language for authoring LLM prompt templates — shipped as a Go compiler/runtime binary plus an interactive TUI.

> **Status:** Early stage. The language and tooling are a **draft and fluid** — expect breaking changes while the design settles. See [docs/spec.md](docs/spec.md) and [docs/features.md](docs/features.md).

## What is Prompiler

Prompiler is a dedicated markup language for writing LLM prompt templates. A `.ppl` program declares types and `template` blocks, and the compiler renders a final prompt string from typed inputs. The language is **pure** (no I/O or side effects — every template is a function of its inputs), **strongly typed** (every value and expression has a known type, and type errors are reported *before* any evaluation), **deterministic** (same inputs always render the same output), and **safe** (there is no mechanism to execute arbitrary code or touch the world).

## Features

- **Typed template language** — string, int, float, bool, enum, class, interface, arrays, maps, and more.
- **Compile-time type checking** — errors surface when you `check`, not after a model call.
- **Deterministic rendering** — a tree-walking evaluator with no recursion, so evaluation always terminates and output is reproducible.
- **Interactive TUI value capture** — fill in template variables by hand before rendering.
- **stdout / file / clipboard output** — an output adapter picks the sink.
- **Template composition** — `import` and `include` to build prompts from reusable parts.

## Install

Requires **Go 1.27**.

```sh
git clone <repo-url>
cd prompiler
make build          # produces bin/prompiler
```

or install straight to your Go bin path:

```sh
go install ./cmd/promptpiler
```

## Quick start

The [interpolation example](examples/feature/interpolation/) declares an enum, a template with typed variables, and a prompt that renders each variable into its string form:

```prompiler
enum Difficulty {
  Easy,
  Medium,
  Hard,
}

template Interpolate {
  description: "Renders each stringable type into its string form"

  variables {
    name: string
    age: int
    score: float
    active: bool
    level: Difficulty
    greeting: string = 'hello' // single-quoted default (optional variable)
  }

  prompt {
    Name: {{ name }}
    Age: {{ age }}
    Score: {{ score }}
    Active: {{ active }}
    Level: {{ level }}
    Greeting: {{ greeting }}
  }
}
```

Feed it input values (`examples/feature/interpolation/variables.json`):

```json
{
  "name": "Ada",
  "age": 36,
  "score": 3.5,
  "active": true,
  "level": "Hard"
}
```

Render it:

```sh
prompiler run -root examples/feature/interpolation Interpolate
```

Output (written to stdout):

```
Name: Ada
Age: 36
Score: 3.5
Active: true
Level: Hard
Greeting: hello
```

## CLI reference

The binary is `prompiler` (built from the source directory `cmd/promptpiler`). Running with no subcommand starts the interactive TUI.

| Command | Description |
| --- | --- |
| `prompiler` | Start the interactive TUI (root `.`) |
| `prompiler list [-root <dir>]` | List templates found under the root directory |
| `prompiler check [-root <dir>]` | Type-check all templates; reports diagnostics |
| `prompiler run <template> [-root <dir>] [-variables <path>]` | Render a named template to stdout (`-variables` defaults to `<root>/variables.json`) |

## Language

Prompiler programs declare reusable types and `template` blocks that render a prompt from typed variables:

```prompiler
template Greet {
  variables {
    name: string
  }
  prompt {
    Hello, {{ name }}!
  }
}
```

The full draft specification lives in [docs/spec.md](docs/spec.md) (status: **draft and fluid**) with the grammar in [docs/grammar.md](docs/grammar.md).

## Examples

Browse [examples/](examples/) — 43 self-contained example directories (20 `feature/`, 5 `scenario/`, 18 `errors/`), each with a `.ppl` template, `variables.json` inputs, and a `solution.json` with the expected rendered output. See [examples/README.md](examples/README.md) for a tour.

## Development

Useful `make` targets:

| Target | What it does |
| --- | --- |
| `make build` | Compile `prompiler` to `bin/` |
| `make install` | Install `prompiler` to `GOPATH/bin` |
| `make run T=<name> ROOT=<dir>` | Render one template (e.g. `make run T=Interpolate ROOT=examples/feature/interpolation`) |
| `make tui` | Launch the interactive TUI |
| `make lint` | Formatting check and `go vet` |
| `make unit` | Unit tests (`go test ./... -short`) |
| `make integration` | Conformance harness — runs the `examples/**/solution.json` fixtures |
| `make e2e` | CLI smoke tests (`scripts/e2e.sh`) |
| `make test` | Full suite (unit + integration) |
| `make ci` | Everything CI runs (lint, unit, integration, e2e) |

Every example doubles as a conformance test, so `make integration` verifies the compiler's output against each `solution.json`. CI (`.github/workflows/ci.yml`) runs the full suite on **Go 1.27**.

## Documentation & status

- [docs/spec.md](docs/spec.md) — the language specification (**draft and fluid**)
- [docs/grammar.md](docs/grammar.md) — the grammar
- [docs/features.md](docs/features.md) — feature inventory and the roadmap (marked `OPEN` where unsettled)
- [docs/architecture.md](docs/architecture.md) — package layout, ports and adapters

The language is in active development and **fluid**: items marked `OPEN` in the docs are unresolved but not blocking, and the design may change as the implementation matures. The roadmap lives in [docs/features.md](docs/features.md).

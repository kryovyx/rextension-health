# rextension-health — health and readiness endpoints

`github.com/kryovyx/rextension-health`. Part of the REX framework: developed in a `go.work` workspace
alongside its siblings, released as its own module.

## Boundaries

Depends on `rextension` only.

The dependency gate attaches **per route**, not once globally. Attached
globally it silently never fires for the routes that actually need gating,
which is exactly the defect this module was fixed for.

## Working here

- **Never `go build`.** Syntax-check with `go vet ./...`.
- **`go test -race ./...` always.**
- **Tests are per branch, not per coverage number.** Every branch of every
  function gets its own case; the README's coverage figure is recomputed from
  a measurement, never hand-edited.
- **No `replace` directives** in `go.mod`.
- **Commits:** [COMMIT-CONVENTIONS.md](COMMIT-CONVENTIONS.md) — gitmoji, a
  space, an imperative summary; no `type(scope)` prefix. At most one trailer,
  and no generated footers.
- `make check` here runs fmt, vet and race tests for this module alone.
- Default branch is `main`.
- **Contributing:** the framework is in alpha and external contributions open
  at `v1.0.0`. Packages carry a CONTRIBUTING.md with the rules that will
  apply; they are the same rules as these.

Design decisions are numbered (D…/O…/W…) and recorded in the workspace this
module is developed in, not in this repo. If a rule here looks arbitrary, it is
load-bearing — ask before removing it.

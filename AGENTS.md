# task-view: agent working agreement

`task-view` is a Go CLI that checks a coding agent's review file
(`.review/change-<task>.json`, written when it finishes a task) against the filesystem and Git and renders review
views. `specs/init.md` is the source of truth; read it before changing
behaviour. `README.md` describes layout and commands.

## Working agreement

- Read-only by design: only the Git subcommands listed in the spec, run with
  `GIT_OPTIONAL_LOCKS=0`. Never add commands that write to repos or run
  project tooling.
- Every finding uses a code from the catalog in `internal/review/findings.go`
  and carries a hint written for an agent to act on.
- The page is one offline HTML file: no network at view time.
- `TASK_REVIEW.md` in `internal/instructions` is the agent context file from
  the spec; keep it in sync with the schema version.
- `make check` (vet, lint, test) must pass. Lint enforces most of the code
  rules below; every change ends with a self-review of its diff against them.

## Commands

| Command | What it does |
| --- | --- |
| `make build` | Builds `bin/task-view` |
| `make install` | Installs `task-view` into `$(go env GOPATH)/bin` |
| `make test` | Unit tests and golden tests on generated fixture repos |
| `make vet` | `go vet` and the `gofmt` check |
| `make lint` | golangci-lint (`.golangci.yml`) and ESLint on `app.js` (`eslint.config.mjs`, run through `npx`) |
| `make check` | `vet`, `lint` and `test` |
| `make golden` | Rewrites `cmd/task-view/testdata/valid.golden.json` and `gather.golden.json` after an intended output change |
| `make docs` | Regenerates `docs/TASK_REVIEW.md` from `internal/instructions` |
| `make demo` | Live demo pages in `site/` from the reviews in `demo/`; a push to `main` runs it in CI and publishes GitHub Pages |
| `make release` | Cross-compiled binaries, guides from `docs/`, the agent skill and checksums in `release/`; a pushed `v*` tag runs it in CI and publishes a GitHub release |
| `make clean` | Removes build, release and demo output |
| `make bench-prepare`, `make bench`, `make bench-report` | The benchmark in `bench/` (`bench/README.md`); calls models, so never part of `check` |
| `make bench-publish` | Replaces `BENCH.md` and `bench/results/` with the latest run's results and its archived reviews and pages |

## Package boundaries (enforced by depguard)

`gitx`, `langs` and `instructions` depend on nothing internal; `review` →
`langs`; `engine` → `review`, `gitx`, `langs`; `render` → `engine`, `review`,
`gitx`, `langs`; `fixture` is for tests only; `cmd/task-view` wires
everything. Only `gitx` runs Git: `os/exec` is allowed in `gitx`, `fixture`
and `cmd/task-view` (to open the browser) and nowhere else.

`bench/` is a separate Go module, outside these boundaries and the lint
config: it clones projects and runs harness adapters, so it uses `os/exec`
freely. `make vet` and `make test` cover it; keep it on the standard library.

## Code rules

1. **Pure functions at side-effect boundaries.** The edges are `gitx`, the
   file reads in `engine` and `review.Load`, the page output in `render` and
   flags and the browser in `cmd`. When an edge mixes I/O with independently
   testable domain logic (coverage, attribution, status roll-up, diff
   indexing), extract that logic into a function that takes the data as
   parameters. Do not split out trivial logic solely to satisfy this rule.
2. **Spacing.** Readable, well-spaced code; separate distinct logical blocks
   with blank lines; keep closely related statements together. A composite
   literal goes one field per line unless it fits on one short line.
3. **Guard clauses.** Check errors and negative conditions first and return
   or continue early. No `else` after `return`; nesting stays shallow.
4. **Names.** Single-character names only for loop indexes (`i`, `j`, `k`),
   method receivers and `t *testing.T`. Name every other binding after the
   domain value it holds and its intent (`repoState`, `node`, `finding`,
   `fileCoverage`), closure parameters included except trivially obvious
   one-liners. No cryptic abbreviations; `err`, `ok` and `id` are fine.
   Struct fields get real names too; JSON tags stay as they are, since they
   are the contract with the page and with agents.
5. **Magic values.** Prefer a named `const` unless the literal already
   expresses intent (file modes such as `0o644` do).
6. **Named conditions.** Extract complex conditions into bindings named for
   their meaning.
7. **Comments explain why.** Intent, constraints, trade-offs, invariants or
   surprising decisions; never what the code mechanically does.
8. **Doc comments.** Every exported identifier has a doc comment that starts
   with its name and says why it exists and what it does; describe parameters
   and results in prose when the signature does not make them obvious. In
   `app.js`, a top-level function with non-obvious inputs gets a one-line
   comment.
9. **Helper extraction.** Do not extract a short single-caller helper when
   inlining is clearer. Keep a helper when it is a domain operation, a
   pure/edge boundary, reusable or independently testable.
10. **Functional style.** In Go, prefer `slices`, `maps` and `strings`
    helpers and `range` loops over hand-written index loops. In `app.js`,
    prefer `const` and `map`/`filter`/`flatMap` pipelines over loops that
    push. Return new values instead of mutating parameters (a receiver that
    owns its state may change it).

## Conventions

- Shared vocabulary from the spec (review file, repo, node, edge, flow,
  claim, ref, finding, hunk, initial and final state) is used verbatim in
  type names.
- Frontend code is `internal/render/assets/app.{js,css}`; vendored libraries
  live in `internal/render/assets/vendor` and are never edited or linted.
- Golden output: `make golden` after an intended output change, never to make
  a refactor pass.

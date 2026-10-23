# change-view: agent working agreement

`change-view` is a Go CLI that checks a coding agent's review file
(`.review/change.json`) against the filesystem and Git and renders review
views. `specs/init.md` is the source of truth; read it before changing
behaviour. `README.md` describes layout and commands.

- Read-only by design: only the Git subcommands listed in the spec, run with
  `GIT_OPTIONAL_LOCKS=0`. Never add commands that write to repos or run
  project tooling.
- Every finding uses a code from the catalog in `internal/review/findings.go`
  and carries a hint written for an agent to act on.
- The page is one offline HTML file: no network at view time. Frontend code is
  `internal/render/assets/app.{js,css}`; vendored libraries live in
  `assets/vendor`.
- `CHANGE_REVIEW.md` in `internal/instructions` is the agent context file from
  the spec; keep it in sync with the schema version.
- `make vet test` must pass. Golden output: `make golden` after an intended
  output change.

# change-view

After a coding agent makes a change, it writes a structured account of what it
did (`.review/change.json`). `change-view` checks that account against the
repos — every reference, every hunk — and turns it into one self-contained
HTML page for the human reviewer: a graph of the change, flows, a diff map and
claims, with every element marked as **verified**, **contradicted**,
**unresolved** or **the agent's word**.

The checks validate the account, never the work. The tool reads files and runs
read-only Git; it never runs tests, builds or language tooling. See
[specs/init.md](specs/init.md) for the full specification.

## Install

```bash
go install github.com/gtindo/changes-view/cmd/change-view@latest
```

From a checkout: `make install` (puts `change-view` in `$(go env GOPATH)/bin`).
The only runtime dependency is `git`.

## Use it with an agent

1. Install the agent context file in the repo where you work, and point your
   agent at it:

   ```bash
   change-view instructions > CHANGE_REVIEW.md
   ```

   Then add a line to that repo's `AGENTS.md` or `CLAUDE.md`:
   `After any code change, follow CHANGE_REVIEW.md.`
   (Or skip the file and just tell the agent: "run `change-view instructions`
   and write a review file for this change.")

2. Ask the agent to make a change. When it is done it writes
   `.review/change.json`, runs `change-view validate .review/change.json --format json`
   and fixes the file until it validates.

3. Review:

   ```bash
   change-view open .review/change.json
   ```

   The page opens in your browser. Ask the agent to revise the change or the
   account; a new `change.json` gives a fresh page.

You can also ask an agent to describe an existing commit: "Run
`change-view instructions` and write `.review/change.json` for commit
`<sha>`."

## Commands

| Command | What it does |
| --- | --- |
| `change-view validate <file> [--format text\|json] [--scan-siblings]` | Schema, commit chain, reference and diff-level checks. Exit 0 ok, 1 contradicted/unresolved refs, 2 invalid file, 3 environment error |
| `change-view open <file> [--out <path>] [--no-browser] [--scan-siblings]` | Same checks, then writes one offline HTML page and opens it. Exit 1 still renders the page, with a red banner |
| `change-view schema` | JSON Schema of the review file |
| `change-view languages` | Valid language names and their usual file patterns |
| `change-view instructions` | The agent context file for this binary's schema version |
| `change-view version` | Binary and supported schema versions |

Only the repos listed in the review file are read. `--scan-siblings` also
looks at the folders next to them and warns (`unlisted_repo`) about Git repos
with uncommitted changes that the review doesn't list.

## The page

- **Header and sidebar:** the agent's summary, each repo's commit list from
  base to final state, findings grouped by kind (contradicted references,
  unmentioned changes, untested nodes…), and the agent's notes.
- **Graph:** nodes grouped by repo, endpoints between repos. Fill shows the
  change state, the border shows the reference status. Click a node for its
  code excerpt with changed lines, its why, its tests and relations.
- **Flows:** one lane per flow, with tests attached under the steps they cover.
- **Diff map:** every changed file, hunks colored by whether a node covers
  them; expand a file to read its hunks.
- **Claims:** each claim with its evidence and the resolved fact.

Keys: `1`–`4` switch views, `Esc` closes the inspector. Drag the inner edge
of the side panel or the inspector to resize it (double-click resets). The
page follows the system theme; the button in the top bar cycles auto / light /
dark.

## Development

```bash
make test    # unit tests and golden tests on generated fixture repos
make build   # bin/change-view
```

Layout: `cmd/change-view` (CLI), `internal/review` (model, embedded JSON
Schema, structural rules), `internal/engine` (Git states, diff, reference and
coverage checks), `internal/render` (view data and the embedded page:
`assets/app.js`, `assets/app.css`, vendored Cytoscape.js + ELK),
`internal/langs` (Chroma lexers and highlighting), `internal/instructions`
(`CHANGE_REVIEW.md`), `internal/fixture` (test repos).

Golden output for the accurate fixture review lives in
`cmd/change-view/testdata/valid.golden.json`; refresh it with
`go test ./cmd/change-view -update`.

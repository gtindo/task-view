# task-view — spec

Oct 23, 2026 · @Yvan · synced with the implementation Oct 24, 2026

## Idea and principles

When it finishes a task on its own, the coding agent writes a structured account of what it did; the tool turns that account into review views, which a human reviewer uses to validate the work.

The tool's checks serve one purpose: making sure the review file matches exactly what changed, so the agent can correct it before the human looks. They validate the account, never the work; judging the work is the reviewer's job, done through the views.

1. **Views are for the human reviewer.** They show the change as flows, graph, diff map and claims, so the reviewer can validate the work without reading it line by line.
2. **Checks are for the review file.** `validate` tells the agent where its account doesn't match the repos: wrong lines, a missed hunk, a wrong change state. The agent fixes the file until it is accurate. Checks say nothing about whether the code is correct.
3. **The account must match the diff exactly.** Every changed line belongs to a node, every reference resolves to what it claims. An accurate file is the precondition for trustworthy views.
4. **Claims point at facts.** Every element the agent describes carries a reference the tool can resolve: repo, file, line range, line content, commit.
5. **Agent statements stay visible as such.** Some content can't be checked (why a change was made, which tests passed). It is shown, marked as the agent's word, never styled as checked.
6. **Filesystem and Git only.** The tool reads files and runs read-only Git commands. It never runs tests, builds, linters or language tooling.
7. **Language-agnostic.** Every check works on text: files, line ranges, line content, diff hunks.
8. **Local and disposable.** One command, opens in the browser, no server to maintain, no CI.

## Scope and non-goals

The visualizer is a local review aid for a human reading an agent's change, across one or more local repos.

**In scope**

- Changes made by a coding agent in one session, across one or more local repos and packages (for example the mock server package and the client package).
- Any language or file type: the visualizer works on files, lines and diffs.
- Tests referenced as files and line ranges, like any code, with optional agent-provided metadata (command used, result the agent observed) shown as agent statements.

**Non-goals**

- No CI integration, PR comments or gating.
- No automated review, scoring or approval. The visualizer never says a change is good.
- No running of tests, builds or commands other than read-only Git, and no reading of test reports to infer results.
- No language parsing, symbol resolution or call-graph discovery. If the agent didn't describe it, it isn't drawn; it only shows up as unmentioned diff.
- No agent-facing review. `gather` and `validate` help the agent write an accurate account; no command reviews or judges the agent's work, which is the reviewer's job through the views.

## Local workflow

The whole loop runs on the developer's machine in five steps, with no state kept between reviews except the input file.

1. **Agent makes the change** in one or more local repos, including tests, and runs whatever it normally runs.
2. **Agent gathers the evidence and writes the input.** `task-view gather` prints the repos block (commits, languages) and every changed file with its hunks in final-state line numbers, so the agent spends its effort on structuring the change, not on collecting it. The agent writes `.review/change-<task>.json` in the main repo (or a path you choose), following the agent context file. `<task>` names the task, so several reviews can sit side by side in `.review/`. For each repo it records the commits of the change, newest first, so every reference points at an exact state.
3. **Agent validates** with `task-view validate .review/change-<task>.json --format json`, fixes every error, and repeats until the exit code is 0 and no unmentioned\_hunk warning remains (or each remaining one is explained in notes).
4. **Developer runs** `task-view open .review/change-<task>.json`. The binary resolves and verifies every reference using only the filesystem and Git, in seconds.
5. **Browser opens** a single static HTML page. The developer reviews, and can ask the agent to revise the change or the account. The agent rewrites the same file for a revision and `open` regenerates the page; reviews of earlier tasks stay in their own files.

The binary is read-only: it reads files and runs a fixed set of read-only Git subcommands (listed under implementation notes). Nothing else is executed.

## Binary

The visualizer ships as one static Go binary, `task-view`, with no runtime dependency except the `git` executable already present on any machine that has the repos.

### Commands

| Command | What it does | Used by |
| --- | --- | --- |
| `task-view validate <file> [--format text\|json] [--scan-siblings]` | Schema validation, then chain, reference and diff-level checks. No browser. Exit codes below | Agent, after writing the review file |
| `task-view open <file> [--out <path>] [--no-browser] [--scan-siblings]` | Runs the same checks, then writes one self-contained HTML file (default: `generated/<name>.html` next to the review file, so `.review/generated/change-<task>.html`) and opens it in the default browser. Exit 2 or 3: no page, prints the errors. Exit 1: page is generated with a red banner listing the errors, so a reviewer still sees the account | Developer |
| `task-view gather [<path>[=<base>]...] [--base <rev>] [--tip <rev>] [--context <path>]... [--review <file>] [--lines <n>] [--max-hunk-lines <n>] [--format text\|json]` | Prints the evidence a review file is checked against: the `repos` block ready to paste, and every changed file with its hunks, refs and context. Read-only, writes nothing. Exit 0, 2 (bad invocation) or 3 (environment, `bad_base`) | Agent, before writing the review file |
| `task-view schema` | Prints the JSON Schema of the review file | Agent, editors |
| `task-view languages` | Prints valid language names, one per line, with their usual file extensions | Agent |
| `task-view instructions` | Prints the agent context file matching this binary's schema version | Developer, to install or refresh the context file |
| `task-view version` | Binary version and supported schema versions (also `--version`, `-v`) | Both |
| `task-view help` | Usage and exit codes (also `--help`, `-h`) | Both |

The review file path may be absolute or relative to the current directory. Repo paths inside it are resolved as described in the schema rules.

Only the listed repos are read. `--scan-siblings` opts in to the unlisted-repo check, which also looks at the folders next to them (see diff-level checks). The older `--no-sibling-scan` flag is still accepted and ignored. Flags may come before or after the file.

A bad invocation (no command, unknown command or format, wrong arguments) exits 2 with the usage. `open` exits 3 if it cannot write the page; with exit 1 it prints the text report to stderr, writes the page, then prints its path.

### Validate output

`--format json` (for agents) prints one object; `--format text` (default) prints the same as a readable list.

```json
{
  "valid": false,
  "exitCode": 1,
  "schemaVersion": 1,
  "errors": [
    { "code": "change_mismatch", "path": "/nodes/1/ref", "status": "contradicted",
      "repo": "web", "file": "src/orders/OrderSummaryCard.tsx", "lines": [12, 48],
      "message": "node 'card' is marked modified but lines 12-48 have no changed line",
      "hint": "the diff changes lines 50-61 of this file; adjust the range or the change state" }
  ],
  "warnings": [
    { "code": "unmentioned_hunk", "repo": "web", "file": "src/api/client.ts", "lines": [88, 97],
      "message": "changed lines not covered by any node",
      "hint": "add a node with a range covering lines 88-97, or explain in notes" }
  ],
  "summary": { "verified": 14, "contradicted": 1, "unresolved": 0, "agentStatements": 5 }
}
```

| Exit code | Meaning |
| --- | --- |
| 0 | Schema valid, no contradicted or unresolved references (warnings allowed) |
| 1 | Schema valid, at least one contradicted or unresolved reference |
| 2 | Schema invalid or file unreadable; reference checks not run |
| 3 | Environment error: repo path missing, not a Git repo, `git` not found |

Every error and warning carries a stable `code` from the code catalog, a JSON Pointer `path` into the review file when it concerns one element, and a `hint` written for an agent to act on. Diff-level findings (unmentioned hunks, unlisted repos, unreferenced test code) are warnings: the agent should fix them, but they never change the exit code, and `open` shows whatever remains so the reviewer sees what the account left out.

### Gather output

`gather` reads the repos exactly as `validate` will: the same commit chain, `WORKTREE` rule, diff, untracked files, `.review/` exclusion and line attribution. So the agent copies facts instead of collecting them, and an account built from them starts out matching the diff.

- **Repos.** Positional paths are changed repos (default `.`); a folder inside a repo names that repo. `<path>=<base>` sets that repo's start, and `--base` sets it for every repo without one. `--context <path>` adds a repo the agent only read, at its HEAD. Ids come from folder names (`web`, then `web-2` on a repeat). Paths are relative to the Git root of the review file (`--review`, default `.review/change.json` under the current Git root).
- **Commits.** With a base, the first-parent commits from the base to the tip (`--tip`, default HEAD), newest first; the base must be on the tip's first-parent line, else `bad_base`, with the meeting point as hint. `WORKTREE` leads when the tip is HEAD (not given) and there is uncommitted work outside `.review/`. No base means only the uncommitted work. A repo with nothing in range gets `empty_change` and is left out. With `--tip`, uncommitted work and later commits are reported as `uncommitted_changes` and `head_ahead`. To describe one existing commit: `--base <sha>^ --tip <sha>`.
- **Languages.** For each changed text file, the highlighter's language for its name: one `**/*.<ext>` glob when the extension alone picks that language, else a `**/<name>` glob, with name globs first.

```json
{
  "exitCode": 0,
  "schemaVersion": 1,
  "repos": [ { "id": "web", "path": ".", "commits": ["WORKTREE", "e3b8d1f0…"],
               "languages": { "**/*.ts": "typescript" } } ],
  "commits": { "web": [ { "sha": "e3b8d1f0…", "short": "e3b8d1f", "subject": "…", "author": "…", "date": "…" } ] },
  "files": [
    { "repo": "web", "file": "src/a.ts", "status": "modified", "change": "modified",
      "language": "typescript", "added": 2, "removed": 1,
      "ref": { "type": "file", "repo": "web", "file": "src/a.ts" },
      "hunks": [
        { "lines": [50, 51], "oldLines": [50, 50], "commits": ["WORKTREE"],
          "added": ["…", "…"], "removed": ["…"] }
      ] }
  ],
  "errors": [],
  "warnings": []
}
```

The JSON is printed on one line (shown indented here): it is long and read by agents only. Each file carries a `file` ref, and each hunk's `lines` is the smallest final-state range that covers it the way the coverage check reads refs: the added lines, or the line before the removal for a pure deletion. A hunk in a deleted file, or in a file left empty, has no `lines` and is covered by the file's ref; a binary change or a pure rename has no hunks. `status` is Git's (`added`, `modified`, `deleted`, `renamed`, with `oldFile`) and `change` the node change state it implies. `commits` lists the states that wrote a hunk's added lines. With `--lines <n>` (default 0, none) a hunk also carries `context`, `{start, lines}`: the final-state window around it, `n` lines on each side, left out in added files. `added` and `removed` keep at most `--max-hunk-lines` lines (default 200; 0 keeps all); a cut hunk is `truncated` and has no context. `--format text` prints the same as a status line, the `repos` block and a diff-like list.

### Implementation notes

- **Git access:** shell out to the `git` binary with fixed read-only subcommands: `rev-parse` (`--show-toplevel`, `--verify`), `cat-file -e`, `rev-list --first-parent`, `show <sha>:<path>`, `diff --unified=0 -M --no-color --no-ext-diff`, `status --porcelain=v1 -z`, `ls-files --others --exclude-standard -z`, `log --no-walk=unsorted --format`. Only the `gitx` package runs them. Using the user's Git guarantees identical results to what they see, including their config for renames and attributes. Run with `GIT_OPTIONAL_LOCKS=0` so nothing is written to the repos, `GIT_TERMINAL_PROMPT=0` and `LC_ALL=C`. The working tree is read from disk only when a repo's final state is `WORKTREE`.
- **Schema:** JSON Schema 2020-12 file embedded with `go:embed`, validated with `santhosh-tekuri/jsonschema`. The schema is the single source for both `schema` output and structural validation; cross-reference rules are checked in Go after it.
- **Syntax highlighting:** done in Go at `open` time with Chroma; its lexer list (names and aliases) is the source for `languages` and for language validation.
- **HTML:** a page template plus Cytoscape.js with the ELK layout extension, embedded with `go:embed` and inlined into the output file; resolved data (excerpts, hunks, statuses) is injected as one JSON blob. No network access at view time.
- **Context file:** embedded too, so `instructions` always matches the binary's schema version. `make docs` writes a copy to `docs/TASK_REVIEW.md`.
- **Distribution:** `make release` cross-compiles static binaries (`CGO_ENABLED=0`) for Linux amd64/arm64, Windows amd64 and macOS amd64/arm64, with `SHA256SUMS`, the guides, the agent skill and the licenses. A GitHub Actions workflow runs it on a pushed `v*` tag and publishes a GitHub release (a tag with a suffix such as `-rc.1` is a pre-release). Install with `scripts/install.sh` or `scripts/install.ps1` (latest release, checksum check, per-user folder, no admin rights), a manual download, or `go install`.
- **Tests:** fixture repos built by Go test helpers (`internal/fixture`) with pinned dates and config so SHAs are stable: three repos side by side (commits, a worktree change, an untracked file, a rename, a deleted file, a binary file) plus an unrelated dirty sibling. One golden output for an accurate review and one for `gather` on the fixture (`make golden` refreshes both), a test that a review made of every ref `gather` proposes validates with no unmentioned hunk, and a table-driven test that breaks one thing per code in the catalog and checks the code and exit code.

## Input format

The input is one JSON file describing the change as a small graph plus flows and claims; every node, edge and claim may carry a `ref` the visualizer checks.

| Field | Content |
| --- | --- |
| `version` | Schema version, `1` |
| `summary` | The agent's intent in a few sentences: what was asked, what it did |
| `repos[]` | `{ id, path, role?, commits, languages? }`: local path, role, commits and language mapping. `role`: `changed` (default) or `context` for a repo the agent only read, such as the real server behind a mock |
| `repos[].commits` | Full SHAs ordered from last to initial: `commits[0]` is the final state the review describes, the last entry is the first commit of the change. Its parent is the base (initial state). Required, at least one. If the change is not committed, or only partly, the list starts with `"WORKTREE"`, whose parent is the current HEAD: `["WORKTREE"]` alone means nothing was committed and the change starts where HEAD is. For a `context` repo, exactly one SHA: the commit the agent read; no diff is computed for it |
| `nodes[]` | `{ id, kind, label, change, ref?, why?, meta? }`. `kind`: `entry`, `code`, `component`, `endpoint`, `test`, `external`, or any string the agent finds useful. `change`: `added`, `modified`, `deleted`, `unchanged` |
| `nodes[].meta` | Free key-value context from the agent, shown as agent statements. For tests, typically `{ command, observed: "passed" }` |
| `edges[]` | `{ from, to, kind, ref? }`. `kind`: `calls`, `renders`, `requests`, `serves`, `mocks`, `tests`, `depends`, or any string |
| `flows[]` | `{ id, title, path: [nodeId...], tests: [nodeId...] }`: an ordered walk from an entry point through the change, with the tests that cover it |
| `claims[]` | `{ text, about: [id...], evidence: [ref...] }`: statements such as "the mock response now has a discount field" |
| `notes[]` | Free text the agent wants the reviewer to see: doubts, shortcuts, things not done |

`kind` values only choose icons and grouping in the views; the visualizer attaches no meaning to them beyond `test` (used by the test-code checks and the graph's test toggle) and `doc`/`docs` (the graph's doc toggle).

### Languages

The agent declares the language of the files it references, only so views can show code with syntax coloring; the tool never parses code.

- **Per repo:** `repos[].languages` maps glob patterns to language names, matched against repo-relative paths, first match wins: `{ "**/*.tsx": "tsx", "**/*.ts": "typescript", "**/*.go": "go" }`.
- **Per ref:** any `file`, `range`, `site` or `hunk` ref may set `language` to override the repo mapping, for example an extensionless script or a template.
- **Names:** the lexer names of the bundled highlighter, printed by `task-view languages`.
- **Validation:** an unknown name is an error `unknown_language`, with the closest valid names as hint. A referenced file that matches no pattern is a warning `no_language`; it is shown as plain text.

### Schema rules

Structural rules live in the JSON Schema; cross-reference rules are checked by the binary right after it, and both run before any Git access.

| Rule | Code if broken |
| --- | --- |
| `version` is a supported schema version | `unsupported_version` |
| Required: `version`, `summary`, `repos` (≥ 1), `nodes` (≥ 1). Optional: `edges`, `flows`, `claims`, `notes` | `schema_invalid` |
| Ids of repos, nodes and flows match `^[a-z0-9][a-z0-9-]{0,63}$` and are unique within their list | `schema_invalid`, `duplicate_id` |
| `repos[].path` is absolute or relative to the root of the Git repo containing the review file; resolves to the root of a Git repo | `repo_not_found`, `not_repo_root` |
| Every `ref.repo` names a listed repo; refs into a `context` repo are allowed but never count as changed | `unknown_repo` |
| `edges[].from/to`, `flows[].path/tests` and `claims[].about` name existing node ids; `flows[].tests` name nodes of kind `test` | `unknown_node`, `not_a_test` |
| `ref.file` is a repo-relative POSIX path: no leading `/`, no backslash, no `.`, `..` or empty segment | `bad_path` |
| `lines` is `[start, end]`, integers, 1 ≤ start ≤ end; `line` is an integer ≥ 1 | `schema_invalid` |
| `contains` is 1 to 200 characters, single line, matched exactly (case and whitespace as written) | `schema_invalid` |
| `ref.at` is a full SHA (schema), and one of the ref's repo commits, not `WORKTREE` (checked with Git) | `schema_invalid`, `bad_at` |
| A node with `change` other than `unchanged` has a `why` | warning `missing_why` |

Text rules: files are read as UTF-8; CRLF and LF both count as one line break, and a final line break does not start a new line. A binary file (Git's heuristic: a NUL byte in the first 8000 bytes, or Git's diff calls it binary) can only be referenced with a `file` ref; any line-based ref on it is `binary_file`. Renamed files are referenced by their final path; the initial path is taken from `git diff -M` and shown in views. Untracked files under a `WORKTREE` state count as `added`. The review file itself and everything in its `.review/` folder are never part of the diff or of the uncommitted-changes check.

Example, a change across a mock server and a client:

```json
{
  "version": 1,
  "summary": "Add discount to order response; client shows it on the summary card.",
  "repos": [
    { "id": "mock", "path": "../mock-server",
      "languages": { "**/*.ts": "typescript" },
      "commits": ["9c41e7d2a8f03b6e5d1c4a7f8e2b9d0c3a6f1e58"] },
    { "id": "web", "path": ".",
      "languages": { "**/*.tsx": "tsx", "**/*.ts": "typescript" },
      "commits": ["e3b8d1f04c7a92e6b5d8f1a3c0e7b4d29f6a8c15",
                  "4f2a6c9e1b8d73f0a5e2c6b9d4f1e8a7c3b05d62"] },
    { "id": "api", "path": "../api",
      "languages": { "**/*.ts": "typescript" }, "role": "context",
      "commits": ["b7e04d9a2c5f81e3d6a0b4c7f2e9d1a8c5b3f760"] }
  ],
  "nodes": [
    { "id": "summary-page", "kind": "entry", "label": "/orders/:id", "change": "unchanged",
      "ref": { "type": "range", "repo": "web", "file": "src/routes.tsx", "lines": [14, 22] } },
    { "id": "card", "kind": "component", "label": "OrderSummaryCard", "change": "modified",
      "ref": { "type": "range", "repo": "web", "file": "src/orders/OrderSummaryCard.tsx", "lines": [12, 48] },
      "why": "Renders discount line when present" },
    { "id": "get-order", "kind": "endpoint", "label": "GET /orders/:id", "change": "modified" },
    { "id": "mock-handler", "kind": "code", "label": "getOrder handler", "change": "modified",
      "ref": { "type": "range", "repo": "mock", "file": "src/handlers/orders.ts", "lines": [20, 41] } },
    { "id": "server-route", "kind": "code", "label": "GET /orders/:id (api)", "change": "unchanged",
      "ref": { "type": "range", "repo": "api", "file": "src/routes/orders.ts", "lines": [30, 62] } },
    { "id": "t-card", "kind": "test", "label": "shows discount", "change": "added",
      "ref": { "type": "range", "repo": "web", "file": "src/orders/OrderSummaryCard.test.tsx", "lines": [40, 58] },
      "meta": { "command": "npx vitest run src/orders", "observed": "passed" } }
  ],
  "edges": [
    { "from": "summary-page", "to": "card", "kind": "renders",
      "ref": { "type": "site", "repo": "web", "file": "src/orders/OrderPage.tsx", "line": 33, "contains": "<OrderSummaryCard" } },
    { "from": "card", "to": "get-order", "kind": "requests",
      "ref": { "type": "site", "repo": "web", "file": "src/orders/useOrder.ts", "line": 9, "contains": "/orders/${id}" } },
    { "from": "mock-handler", "to": "get-order", "kind": "mocks",
      "ref": { "type": "site", "repo": "mock", "file": "src/handlers/orders.ts", "line": 20, "contains": "/orders/:id" } },
    { "from": "server-route", "to": "get-order", "kind": "serves",
      "ref": { "type": "site", "repo": "api", "file": "src/routes/orders.ts", "line": 30, "contains": "/orders/:id" } },
    { "from": "t-card", "to": "card", "kind": "tests",
      "ref": { "type": "site", "repo": "web", "file": "src/orders/OrderSummaryCard.test.tsx", "line": 44, "contains": "<OrderSummaryCard" } }
  ],
  "flows": [
    { "id": "view-order", "title": "User opens an order", "path": ["summary-page", "card", "get-order", "mock-handler"], "tests": ["t-card"] }
  ],
  "claims": [],
  "notes": ["Server does not return discount yet; the client test runs against the mock only."]
}
```

## Fact references and verification

Each reference type has one deterministic check; every node, edge and claim ends with one of four statuses, and nothing is ever upgraded by the agent's wording.

| Status | Meaning |
| --- | --- |
| Verified | The reference resolved and the check passed |
| Contradicted | The reference resolved and the check failed (line content differs, range not in the diff while marked changed, or the reverse) |
| Unresolved | The reference could not be found (missing repo, file, ref or commit; range past end of file) |
| Agent statement | No reference given, or `meta` and `why` content, which the visualizer only displays |

### Where the visualizer reads

All checks read the exact states listed in the review file: Git objects for committed states, and the working tree only when the list starts with `WORKTREE`.

- **Final state** = `commits[0]`. A SHA is resolved with `git show <sha>:<file>`; `WORKTREE` is resolved by reading files from disk (tracked changes, staged or not, plus untracked files that are not ignored).
- **Initial state** = first parent of the last entry in `commits`. For `["WORKTREE"]` alone, that is the current HEAD. When there is no parent (a root commit, or an unborn branch), it is the empty tree. Refs on `deleted` nodes are resolved there.
- **Diff** = initial state to final state: `git diff <initial> <commits[0]>`, or `git diff <initial>` plus untracked files when the final state is `WORKTREE`.
- **Intermediate states**: any ref may add `at: <sha>`, which must be one of the repo's listed commits, to point at the state after that commit (for example a claim about an approach later replaced). A ref with `at` skips the change-state checks and covers no hunk of the diff. Rarely needed.
- **Snapshot**: `open` copies every resolved file excerpt and hunk into the HTML page, so the view stays accurate if files keep changing, which matters most when the final state is `WORKTREE`. `validate` warns `worktree_changed` if a worktree file it read changed (size or modification time) between the first read and the end of its own run.
- **Context repos** have no diff: their one commit is both the initial and the final state.

Chain checks run before any reference check:

| Check | Result if it fails |
| --- | --- |
| Every SHA exists in the repo | Error `commit_not_found` |
| `commits[i+1]` is the first parent of `commits[i]` for every i (contiguous, newest first, no gaps); `WORKTREE`, if present, is only at index 0 and its parent is the current HEAD | Error `commit_chain_broken`, with the expected order from `git rev-list` as hint |
| A `context` repo lists exactly one SHA, not `WORKTREE` | Error `context_commits` |
| The repo has uncommitted changes but the list does not start with `WORKTREE` | Warning `uncommitted_changes`: not part of the review, shown as unreviewed work |
| The list does not start with `WORKTREE` and the checked-out branch has commits after `commits[0]` | Warning `head_ahead`: later work not covered by the review |

The side panel shows each repo's commit list with messages. For a change over several states, the checks replay each step's diff to record which commit (or `WORKTREE`) last wrote each added line, so the diff map can filter hunks by commit.

### Reference types

| Type | Fields | Check (filesystem and Git only) |
| --- | --- | --- |
| `file` | `repo`, `file` | File exists in the final state (in the initial state for `deleted`). On a node, its `change` matches the diff for that file: added, modified (renames included), deleted, or not in the diff for `unchanged` |
| `range` | `repo`, `file`, `lines` | File exists; `end` ≤ last line. On a node: for `modified` and `added`, at least one changed line of the diff falls in the range, and a `modified` range is not in a new file; for `unchanged`, none does. For `deleted`, the range is read in the initial state and must contain removed lines. A pure deletion counts as a change at the line before it |
| `site` | `repo`, `file`, `line`, `contains` | `contains` appears on `line`: verified. Not on `line` but within ±2 lines (nearest first): verified with warning `site_offset` giving the actual line, for the agent to correct. Otherwise contradicted, with up to three lines that do hold `contains` as hint; a `line` past the end is `range_out_of_bounds` |
| `hunk` | `repo`, `file`, `lines` | The diff has a hunk overlapping these lines (final-state line numbers; initial-state numbers for pure deletions, or the lines around the removal point). A node marked `unchanged` cannot use one |
| `commit` | `repo`, `sha` | The SHA (full, or a prefix of at least 7 characters) is one of the repo's listed commits |

All line-based refs are read in the final state, in the state after `at` when set, and in the initial state for `deleted` nodes. Every type also accepts optional `at` and `language`. Change-state checks apply only to node refs without `at`; edge and claim refs are checked for existence, bounds and content. A node marked changed whose ref points into a `context` repo is `change_mismatch`, since that repo has no diff. A ref into a repo whose commits did not resolve is unresolved without a finding of its own: the chain error already explains it.

When a ref fails several checks, its status is the first failure, raised to unresolved if a later one is unresolved. A claim's status is the worst status of its evidence (unresolved, then contradicted, then verified), or agent statement without evidence.

Tests are nodes with `file` or `range` refs like any code, so the visualizer can confirm the test exists, was added or modified, and touches the component it claims through a `site` ref. Whether it ran or passed is only ever an agent statement in `meta`.

### Diff-level checks

These run regardless of what the agent wrote and are listed in the side panel's findings on every view:

- **Unmentioned changes:** diff hunks in any listed repo not covered by a node's `file`, `range` or `hunk` ref (without `at`). Reported per run of uncovered lines; a binary change or a pure rename has no hunks and needs a `file` ref. A pure deletion is covered by a final-state range touching the removal point or by a `deleted` node's initial-state range.
- **Unlisted repos** (only with `--scan-siblings`): sibling repos (same parent folders as listed repos, one level deep, hidden folders skipped) with uncommitted changes that are not in `repos[]`.
- **Changed tests:** files the agent marked as `test` nodes that changed in the diff, shown next to the code they claim to test.
- **Changed but unreferenced test code:** hunks in files that hold test nodes but fall outside every test node's range (for example an edited assertion the agent didn't mention).

### What the visualizer never checks

Test results, build success, type correctness, runtime behavior, and whether a `site` really is a call rather than matching text. These stay agent statements by design; the reviewer sees exactly which parts of the account are facts and which are the agent's word.

### Code catalog

The complete list of codes `validate` and `gather` can return in schema version 1 (`bad_base` and `empty_change` come only from `gather`); adding a code is a minor version change, renaming one is a breaking change.

| Code | Level | Raised when |
| --- | --- | --- |
| `file_unreadable` | Error, exit 2 | Review file missing or not valid JSON |
| `unsupported_version` | Error, exit 2 | `version` not supported by this binary |
| `schema_invalid` | Error, exit 2 | JSON Schema violation; `path` points at the element |
| `duplicate_id` | Error, exit 2 | Repeated id in repos, nodes or flows |
| `unknown_node` | Error, exit 2 | Edge, flow or claim names a missing node |
| `not_a_test` | Error, exit 2 | `flows[].tests` names a node not of kind `test` |
| `unknown_repo` | Error, exit 2 | `ref.repo` not in `repos` |
| `bad_path` | Error, exit 2 | `ref.file` not a clean repo-relative path |
| `unknown_language` | Error, exit 2 | Language name not known to the highlighter |
| `git_not_found` | Error, exit 3 | No `git` executable on PATH |
| `repo_not_found` | Error, exit 3 | `repos[].path` does not exist |
| `not_repo_root` | Error, exit 3 | Path is not the root of a Git repo (for `gather`: not inside one) |
| `bad_base` | Error, exit 3 | `gather`: base or tip is not a commit, or the base is not on the tip's first-parent line |
| `commit_not_found` | Error, exit 1 | A listed SHA does not exist |
| `commit_chain_broken` | Error, exit 1 | Commits not contiguous first-parent order, or `WORKTREE` misplaced or not on HEAD |
| `context_commits` | Error, exit 1 | `context` repo does not list exactly one SHA |
| `bad_at` | Error, exit 1 | `at` not one of the repo's listed SHAs |
| `file_not_found` | Error, exit 1 | Referenced file absent in the state it is read in |
| `range_out_of_bounds` | Error, exit 1 | Range ends past the last line, or a `site` line is past it |
| `binary_file` | Error, exit 1 | Line-based ref on a binary file |
| `change_mismatch` | Error, exit 1 | Node's `change` contradicts the diff (marked changed with no changed lines in range, marked unchanged with changed lines or on a hunk, wrong added/deleted state, changed node in a `context` repo) |
| `site_mismatch` | Error, exit 1 | `contains` not found within ±2 lines |
| `hunk_not_found` | Error, exit 1 | No hunk overlaps a `hunk` ref |
| `commit_not_listed` | Error, exit 1 | `commit` ref SHA not in the repo's list |
| `site_offset` | Warning | `contains` found up to 2 lines from `line`; hint gives the actual line |
| `unmentioned_hunk` | Warning | Changed lines covered by no node |
| `unreferenced_test_hunk` | Warning | Changed lines in a test file outside every test node |
| `unlisted_repo` | Warning | Sibling repo with uncommitted changes not in `repos`; only with `--scan-siblings` |
| `uncommitted_changes` | Warning | Listed repo has uncommitted changes but no `WORKTREE` entry |
| `head_ahead` | Warning | Branch has commits after `commits[0]` |
| `worktree_changed` | Warning | A referenced file changed during the run |
| `no_language` | Warning | Referenced file matches no language |
| `missing_why` | Warning | Changed node without `why` |
| `empty_change` | Warning | `gather`: a changed repo has no commits in range and no uncommitted work |

## Views

One static HTML page: a top bar, a side panel, four views and an inspector. Verification status is drawn the same way everywhere so it reads at a glance.

### Top bar and side panel

- **Top bar:** the repo ids and review file name, count pills for verified, contradicted, unresolved and agent statements plus total errors and warnings (each opens the matching findings), the side-panel toggle and a theme switch.
- **Error banner:** when `open` exits 1, a red banner gives the error count, says the views show the account as written, and links to the errors.
- **Side panel** (collapsible and resizable): the agent's summary (long ones fold behind "Show more"), one card per repo with its commits from base to final state (messages, a `WORKTREE` row, file and +/− counts, uncommitted paths outside the review), the findings, the agent's notes unedited, and the generation time and binary version. Clicking a commit filters the diff map to it.
- **Findings** are grouped: contradicted, unresolved and other errors; unmentioned changes, unreferenced test code, unlisted repos, work outside the review (`uncommitted_changes`, `head_ahead`) and other warnings; and an informational "Changed tests" group. With none, the panel says the account matches the diff. A group opens its list in the inspector, each item linking to its node, edge, claim or diff map hunk.

### Views

| View | Shows | Interaction |
| --- | --- | --- |
| Graph | Source-code nodes and the edges between them, laid out with ELK. When nodes reference more than one repo, they are grouped in one box per repo (dashed for `context` repos); nodes without a ref, typically endpoints, sit outside the boxes. Test nodes (kind `test`) and doc nodes (kind `doc` or `docs`, or a ref to a `.md`, `.mdx`, `.markdown`, `.rst`, `.adoc` or `.txt` file) and unchanged nodes are hidden until their toggles are on. Edges show their kind as a label | Click a node to open it in the inspector and dim all but its neighbours. Toggle unchanged, test and doc nodes; selecting a hidden node from elsewhere turns its toggle on. Search nodes (`/`, Enter jumps to the first hit, Esc clears), switch the direction (→ or ↓), zoom and fit, fold the legend. Large graphs open centred on the first flow's entry |
| Flows | One lane per flow, left to right along `path`, each connector labelled and colored by the edge that joins the two steps. Tests listed in `flows[].tests` hang under the first step they have a `tests` edge to; the others are listed under the lane | Click a step, test or connector: opens it in the inspector |
| Diff map | Every changed file with a bar of its hunks, colored by whether nodes cover them (covered, partly, not mentioned), and a coverage summary | Filter by commit, or to files that need attention or are covered; expand a file (or all) to read its hunks with their commit, the nodes covering them and the uncovered lines; open any hunk in the inspector |
| Claims | Each claim with its status and evidence | Filter by status; expand a piece of evidence to see the resolved fact (code excerpt, hunk, commit) with the failure reason and hint |

The **inspector** opens on the right (resizable; full screen on narrow viewports) for a node, edge, hunk, finding group or the changed tests. For a node it shows the location (ref type and the state read), the agent's account (`why` and `meta`), its findings, its tests, its relations, and its code. The code box has two tabs when the excerpt is in the final state and has changes: **Diff** (changed lines with the removed ones) and **After** (the referenced range, or the whole file for a `file` ref, as it reads in the final state). A `site` line marks its `contains` text. Excerpts carry three lines of context and stop at 600 rows.

**Navigation:** `1`–`4` switch views, `/` searches the graph, `[` toggles the side panel, Esc closes the inspector. The URL hash (`#view=…&sel=…`) keeps the view and selection, so a link reopens the same place. Theme, panel widths, the code tab and the legend state are kept in local storage.

### Visual encoding

| Status or kind | Encoding |
| --- | --- |
| Verified | Solid border |
| Contradicted | Red border and ✕, reason on hover |
| Unresolved | Amber dashed border and ? |
| Agent statement | Dotted gray border, italic label |
| Changed node | Filled by change: green added, blue modified, red deleted (label struck through); unchanged nodes plain |
| Test node | Rounded; an `observed` value in `meta` is shown as an "agent says" badge (for example "agent says: passed"), green for a pass, red for a failure |
| Edge | Solid line with a verified site, dotted without ref, red for contradicted, dashed amber for unresolved; `tests` edges have their own arrowhead |

The page is one self-contained HTML file (renderer, styles, highlighted code and data inlined; every excerpt and hunk is copied in, so it does not change when files do), works offline, follows the system light or dark theme unless the reader picks one, and can be kept or shared as a file.

## Agent context file

The agent learns the format from one file, `TASK_REVIEW.md`, printed by `task-view instructions`. There are three ways to hand it to an agent:

- **Skill** (recommended): `skills/task-review/SKILL.md`, shipped with every release, tells the agent to run `task-view instructions` when it finishes a task (or is asked to describe an existing commit) and follow what it prints, so the text always matches the installed binary. It falls back to a `TASK_REVIEW.md` in the repo when `task-view` is not on `PATH`.
- **Context file:** `task-view instructions > TASK_REVIEW.md`, referenced from the project's `CLAUDE.md` or `AGENTS.md` (for example "When you finish a task, follow TASK\_REVIEW.md"), refreshed after upgrades.
- **One-off prompt:** ask the agent to run `task-view instructions` and write a review file, for a new task or an existing commit.

Its full content for schema version 1:

```markdown
# Task review file

When you finish a task, write a review file so a human can review your work
with `task-view`. The file describes what you changed and why, as a small
graph.
The tool checks every reference you give against the files and Git; anything
without a reference is shown as "agent says". Be precise, not persuasive.

## When

- Once the task is complete, never during the work or after each edit.
- If you change more for the same task, rewrite its whole file; do not patch
  it.

## Where

`.review/change-<task>.json` in the repo where the task started, unless told
otherwise. `<task>` is a short kebab-case name for the task, for example
`change-rate-limit-login.json`: one file per task, so earlier reviews stay
next to it. Add `.review/` to `.git/info/exclude` if it is not ignored
already.

## Steps

1. Do not commit for the review. Commit only if this repo's own
   instructions (CLAUDE.md, AGENTS.md, CONTRIBUTING) or the user tell you
   to; otherwise leave changes uncommitted. The review covers them through
   the `WORKTREE` entry (step 3).
2. The shape below is complete; `task-view schema` prints the JSON Schema
   only if a validate error needs it.
3. Gather the evidence from the repo where the review file goes:
   `task-view gather --base <start> --format json`, where `<start>` is the
   commit you started from. Leave out `--base` if you committed nothing.
   - Several repos: name each with its own start,
     `task-view gather .=<start> ../client=<start>`. Repos you only read:
     add `--context ../api`.
   - An existing commit: `task-view gather --base <sha>^ --tip <sha>`.
   - Copy its `repos` into the review file as it is: paths, commits
     (`WORKTREE` first when work is uncommitted) and languages are already
     right. If you rename an id, use the new one in every ref.
   - Fix every error with its `hint` and run it again. `uncommitted_changes`
     or `head_ahead` means work the range leaves out; `empty_change` means a
     repo has nothing to review.
   - Never collect commits or the diff another way.
4. Read `files`: every changed file with its hunks. Hunk `lines` are
   final-state numbers, the ones refs use, and `commits` says which state
   wrote them. Every hunk's `lines` must fall inside a node's `range` or
   `file` ref: read the file at the final state and widen each range to the
   whole piece it belongs to (function, component, block). A hunk without
   `lines` (a deleted or emptied file) and a file without hunks (binary,
   pure rename) need the file's `ref`. A `truncated` hunk was cut short.
   Add `--lines 3` to see context around each hunk.
5. Write nodes, edges, flows, claims and notes (rules below).
6. Run `task-view validate .review/change-<task>.json --format json`.
7. Fix every error using its `hint`, then add nodes for every
   `unmentioned_hunk` warning. Repeat until exit code 0 and no
   `unmentioned_hunk` warnings, or explain the remaining ones in `notes`.
8. Tell the user: `task-view open .review/change-<task>.json`. It writes the
   page to `.review/generated/change-<task>.html`.

## File shape

~~~json
{ "version": 1, "summary": "<what was asked, what you did>",
  "repos": [...], "nodes": [...], "edges": [...], "flows": [...],
  "claims": [...], "notes": [...] }
~~~

`version`, `summary`, `repos` and `nodes` are required; the rest may be
left out. Write no other top-level field.

## Nodes

One node per meaningful piece: a function, component, handler, config block,
migration, test case. Fields:

- `id`: short, unique, lowercase kebab-case (`^[a-z0-9][a-z0-9-]{0,63}$`).
- `kind`: `entry`, `code`, `component`, `endpoint`, `test`, `external`,
  or another short word. Use `test` for every test case.
- `label`: what a reviewer would call it.
- `change`: `added`, `modified`, `deleted` or `unchanged`.
- `ref`: `{ "type": "range", "repo", "file", "lines": [start, end] }`
  covering the whole piece (1-based, inclusive, lines at `commits[0]`;
  at the state before the first commit for `deleted`). Use `{ "type": "file", ... }` for whole new or deleted files.
- `why`: one sentence, the reason for the change. Required when changed.
- `meta`: for tests, `{ "command": "<what you ran>", "observed": "passed" | "failed" | "not run" }`.
  Only write a result you saw in this session's output.

Include `unchanged` nodes only when they explain a flow or a boundary,
for example the real server route behind a mock you changed.
Endpoints, queues and external services get a node without `ref`.

## Languages

`gather` sets `languages` for the changed files: glob to language name,
first match wins, e.g. `{ "**/*.tsx": "tsx", "**/*.ts": "typescript" }`.
Add globs for every other file you reference, context repos included. For
a file the globs can't describe, add `"language"` on its ref. Valid names:
`task-view languages`.

## Edges

`{ "from", "to", "kind", "ref" }` with `kind` one of `calls`, `renders`,
`requests`, `serves`, `mocks`, `tests`, `depends`, or another short verb.
Add a `site` ref pointing at the line where the relation happens:

~~~json
{ "type": "site", "repo": "web", "file": "src/orders/OrderPage.tsx",
  "line": 33, "contains": "<OrderSummaryCard" }
~~~

`contains` is a short exact substring of that line. If you can't point at a
line, omit `ref`; never guess. Give each test node a `tests` edge to every
node it exercises. A node with no `tests` edge has no test; the reviewer sees
that, so write no notes explaining it.

## Flows

One flow per user-visible or caller-visible path your change affects:
`{ "id", "title", "path": [...], "tests": [...] }`. `title` says the path
in a reader's words; `path` lists node ids from the entry point through the
changed code to the boundary; `tests` lists the test nodes that exercise it.

## Claims and notes

- `claims`: statements a reviewer might doubt,
  `{ "text", "about": [node ids], "evidence": [refs] }` (usually `site` or
  `range` refs). Without evidence, put it in `notes` instead.
- `notes`: doubts, shortcuts, things not done, assumptions. Plain sentences.
  Say what you didn't verify.

## Never

- Never change a ref only to silence a `contradicted` error. Fix the account
  or the code, and mention it in `notes` if the account was wrong.
- Never claim tests passed unless you saw the output in this session.
- Never leave changed lines out because they seem trivial.
- Never edit `commits` by hand. If you commit or edit files again after
  writing the review file, run `gather` again, update `repos` and every
  affected line number, then validate again.
```

The file is embedded in the binary (`internal/instructions`), versioned with the schema, copied to `docs/TASK_REVIEW.md` by `make docs`, and is the only documentation the agent needs; the spec sections above are for the people building the tool.

## Implementation steps

Three steps, each usable on its own: step 1 lets agents produce accurate review files, step 2 gives reviewers the views, step 3 polishes. Steps 1 and 2 are done; step 3 is partly done.

1. **Validator binary.** Go module, embedded JSON Schema, `schema`, `languages`, `instructions`, `version`, and `validate`: schema rules, chain checks including `WORKTREE`, all reference types (`file`, `range`, `site`, `hunk`, `commit`), diff-level checks, the full code catalog, JSON and text output, exit codes. Fixture repos and golden tests per code. `TASK_REVIEW.md` embedded. Exit: an agent produces a file that validates on a real multi-repo change with uncommitted work.
2. **`open` with views.** Embedded template, Cytoscape.js with ELK, Chroma highlighting; top bar and side panel, graph with repo grouping, flow lanes, diff map, claims panel, inspector; error banner for exit 1. Exit: a reviewer can validate a real change from the page alone.
3. **Extras.** Done: per-commit filtering in the diff map; a release pipeline (`make release` and a tag-triggered GitHub Actions workflow, instead of GoReleaser) with install scripts and a skill (no Homebrew tap); `gather`, so the agent copies the repos block and the hunks instead of collecting them. Not done: side-by-side comparison of two review files.

### Decisions

| Question | Decision for v1 |
| --- | --- |
| Where the review file lives for a multi-repo change | `.review/change-<task>.json` in the repo where the task started; any path works since repo paths resolve from the Git root containing the file |
| Do endpoint nodes get a check | No. Endpoints stay agent statements; each `serves`, `mocks` and `requests` edge is checked through its own `site` ref, which already shows whether each side exists |
| `site` tolerance | Exact `line` expected; ±2 lines accepted with a `site_offset` warning so the agent corrects it; beyond that `site_mismatch` |
| Flows without an entry node | Allowed, no warning: `kind` values carry no meaning for checks except `test`. Views draw the flow from its first node |

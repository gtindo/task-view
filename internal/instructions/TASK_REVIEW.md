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

```json
{ "version": 1, "summary": "<what was asked, what you did>",
  "repos": [...], "nodes": [...], "edges": [...], "flows": [...],
  "claims": [...], "notes": [...] }
```

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

```json
{ "type": "site", "repo": "web", "file": "src/orders/OrderPage.tsx",
  "line": 33, "contains": "<OrderSummaryCard" }
```

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

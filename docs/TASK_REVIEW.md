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
2. Run `task-view schema` if you need the exact JSON Schema.
3. List every repo in `repos`, with `path` relative to this file's repo:
   - repos you changed: `commits` = full SHAs of the change, ordered from
     last to first: `git rev-list --first-parent <start>..HEAD`, where
     <start> is the commit you started from. `commits[0]` must be the final
     state you describe.
   - if the repo has uncommitted changes that belong to the change
     (`git status --porcelain` is not empty), put `"WORKTREE"` first.
     Nothing committed at all: `"commits": ["WORKTREE"]`, and the change
     starts where HEAD is.
   - repos you only read: `"role": "context"` and `commits` = the one SHA
     you read (`git rev-parse HEAD`).
4. Get the real diff for each changed repo, from the initial state (parent
   of the last listed commit, or HEAD for `["WORKTREE"]`):
   `git diff <initial> <commits[0]> --unified=0`, or `git diff <initial>
   --unified=0` plus untracked files when the list starts with `WORKTREE`.
   Every hunk must fall inside a node's `range` or `file` ref.
   Line numbers in refs are read at `commits[0]` (on disk for `WORKTREE`).
5. Write nodes, edges, flows, claims and notes (rules below).
6. Run `task-view validate .review/change-<task>.json --format json`.
7. Fix every error using its `hint`, then add nodes for every
   `unmentioned_hunk` warning. Repeat until exit code 0 and no
   `unmentioned_hunk` warnings, or explain the remaining ones in `notes`.
8. Tell the user: `task-view open .review/change-<task>.json`. It writes the
   page to `.review/generated/change-<task>.html`.

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

Set `languages` on each repo you reference, context repos included: glob to language name,
first match wins, e.g. `{ "**/*.tsx": "tsx", "**/*.ts": "typescript" }`.
Cover every file you reference. For a file the globs can't describe, add
`"language"` on its ref. Valid names: `task-view languages`.

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
`path` lists node ids from the entry point through the changed code to the
boundary; `tests` lists the test nodes that exercise it.

## Claims and notes

- `claims`: statements a reviewer might doubt, each with `evidence` refs
  (usually `site` or `range`). Without evidence, put it in `notes` instead.
- `notes`: doubts, shortcuts, things not done, assumptions. Plain sentences.
  Say what you didn't verify.

## Never

- Never change a ref only to silence a `contradicted` error. Fix the account
  or the code, and mention it in `notes` if the account was wrong.
- Never claim tests passed unless you saw the output in this session.
- Never leave changed lines out because they seem trivial.
- Never list commits out of order or skip one; if you commit or edit files
  again after writing the review file, update `commits` and every affected
  line number, then validate again.

# change-view user guide

`change-view` helps you review a change made by a coding agent. When the agent
finishes, it writes a structured account of what it did (`.review/change.json`).
`change-view` checks that account against your repos and turns it into one HTML
page you read in the browser: a graph of the change, the flows it touches, a
map of every changed line, and the claims the agent makes, each marked as a
checked fact or as the agent's word.

The checks only confirm that the account matches the diff. They never say
whether the code is good: that is your job, and the page is there to make it
faster. The tool reads files and runs read-only Git commands. It never runs
your tests, builds or linters, and it never writes to your repos.

## Contents of this folder

| File | What it is |
| --- | --- |
| `change-view-darwin-arm64` | macOS on Apple silicon (M1 and later) |
| `change-view-darwin-amd64` | macOS on Intel |
| `change-view-linux-amd64` | Linux x86-64 (static, no dependencies) |
| `SHA256SUMS` | Checksums of the three binaries |
| `CHANGE_REVIEW.md` | The instructions your agent follows to write the review file |
| `USER_GUIDE.md` | This guide |

The only thing `change-view` needs at run time is `git` on your `PATH`.

## 1. Install

Pick the binary for your machine, rename it `change-view` and put it on your
`PATH`.

macOS (Apple silicon shown; use `darwin-amd64` on Intel):

```bash
sudo install -m 755 change-view-darwin-arm64 /usr/local/bin/change-view
```

If macOS blocks the binary because it was downloaded ("cannot be opened because
the developer cannot be verified"), clear the quarantine flag once:

```bash
sudo xattr -d com.apple.quarantine /usr/local/bin/change-view
```

Linux:

```bash
sudo install -m 755 change-view-linux-amd64 /usr/local/bin/change-view
```

To install without `sudo`, copy it to `~/.local/bin/change-view` (or any folder
on your `PATH`) and `chmod +x` it.

Check it works and, optionally, the checksum:

```bash
change-view version
shasum -a 256 -c SHA256SUMS
```

## 2. Teach your agent the format

Your agent learns the review file format from `CHANGE_REVIEW.md`. Copy it into
the repo where you work and tell the agent to follow it after every change.

```bash
cp CHANGE_REVIEW.md ~/code/my-app/
```

Then add one line to that repo's `AGENTS.md` (Codex, most agents) or
`CLAUDE.md` (Claude Code):

```markdown
After any code change, follow CHANGE_REVIEW.md.
```

In Claude Code you can also import it directly with a line `@CHANGE_REVIEW.md`
in `CLAUDE.md`.

You don't strictly need the file: the binary prints the same text, so you can
just ask "run `change-view instructions` and write a review file for this
change". The copy in this folder always matches this binary. After you update
`change-view`, refresh it with `change-view instructions > CHANGE_REVIEW.md`.

## 3. Daily workflow

1. **Ask the agent for a change** as usual. It works across one or more local
   repos (for example a client and its mock server).
2. **The agent writes `.review/change.json`** in the repo where the task
   started, then runs `change-view validate .review/change.json --format json`
   and fixes the file until it validates. It adds `.review/` to
   `.git/info/exclude`, so the file is never committed.
3. **You open the review:**

   ```bash
   change-view open .review/change.json
   ```

   The page is written to a temporary folder and opens in your default
   browser. It is one self-contained file that works offline; keep it or share
   it with `--out review.html`.
4. **Decide.** If something looks wrong, ask the agent to fix the code or the
   account. It rewrites `change.json` and you run `open` again for a fresh page.

### Reviewing work that is already committed

You can ask an agent to describe any existing commit, not only work it just
did:

> Run `change-view instructions` and write `.review/change.json` for commit
> `<sha>` (and the commits since `<start>` if several). Don't change any code.

The agent lists those commits in the file. Unrelated uncommitted files in the
repo then show as an `uncommitted_changes` warning, which is expected.

## 4. Reading the page

### Statuses

Every node, edge and piece of evidence has one of four statuses, drawn the same
way everywhere:

| Status | Look | Meaning |
| --- | --- | --- |
| Verified | Solid border, green check | The reference resolved and matches the files and the diff |
| Contradicted | Red border, red ✕ | The reference resolved but the check failed: wrong lines, wrong change state, text not on that line |
| Unresolved | Amber dashed border, ? | The file, line range or commit could not be found |
| Agent statement | Dotted gray border, italic | Nothing to check: no reference, or the agent's explanation (`why`, test results, notes) |

The fill shows the change: green **added**, blue **modified**, red **deleted**,
outlined only for **unchanged**. Test nodes are rounded and carry an
"agent says: passed / failed / not run" badge: the tool never runs tests, so a
test result is always the agent's word.

### Layout

- **Top bar:** the status counts and the number of errors and warnings. Click
  a count to list those findings. The button at the far left hides the side
  panel; the button at the far right switches between auto, light and dark
  themes.
- **Red banner:** only when the review file still has errors. The page still
  shows the account, but anything contradicted or unresolved is not a fact.
  Ask the agent to fix it.
- **Side panel:** the agent's summary, each repo with its commits from the base
  to the final state, the findings grouped by kind, and the agent's notes.
  Drag its right edge to resize it; double-click the edge to reset its width.
- **Inspector (right):** opens when you click a node, an edge, a hunk or a
  finding group. For a node it shows where it is, why it changed in the agent's
  words, which tests cover it, how it relates to other nodes, and the code with
  the changed lines highlighted. The code box has two tabs: **Diff** shows the
  changed lines with what they replaced, **After** shows the whole function
  (or file) as it reads after the change. Drag its left edge to widen it for long code
  lines; double-click the edge to reset its width.

### The four views

| View | Use it to |
| --- | --- |
| **Graph** | See the whole change at once. Nodes are grouped by repo; endpoints sit between repos. Click a node to focus on it and its neighbours. Use the search box (or `/`) to find a node; Enter jumps to it. Toggle unchanged nodes, switch the direction, zoom with the buttons or the trackpad. |
| **Flows** | Follow each user-visible path step by step, left to right, with the tests hanging under the steps they cover. A dotted connector means the agent gave no reference for that link. |
| **Diff map** | Check that nothing was left out. Every changed file has a bar showing its hunks: green covered by a node, amber partly covered, red not mentioned at all. Expand a file to read its hunks. "Needs attention" shows only files with gaps; the commit menu (or clicking a commit in the side panel) shows what each commit wrote. |
| **Claims** | Check statements you might doubt ("the index is unique", "import needs no change"). Expand a piece of evidence to see the exact code or commit it points at. |

### Findings worth your attention

| Finding | What it tells you |
| --- | --- |
| Contradicted or unresolved references | The account is wrong somewhere: send it back to the agent |
| Unmentioned changes | Lines changed that no node describes: the agent didn't tell you about them |
| Unreferenced test code | Test-file changes outside every test case the agent listed (an edited assertion, a helper) |
| Untested changed nodes | Changed source code with no test linked to it; the agent's notes should say why. Docs, data, config and binary files are not checked |
| Work outside the review | Uncommitted files or later commits that this review does not cover |
| Unlisted repos | Only with `--scan-siblings`: neighbouring repos with uncommitted changes that the review doesn't list. Usually unrelated; check if the change could have touched them |

### Keyboard

| Key | Action |
| --- | --- |
| `1` `2` `3` `4` | Graph, Flows, Diff map, Claims |
| `/` | Find a node (graph) |
| `[` | Hide or show the side panel |
| `Esc` | Close the inspector |

## 5. Command reference

| Command | What it does |
| --- | --- |
| `change-view open <file> [--out <path>] [--no-browser]` | Check the file, write the HTML page and open it |
| `change-view validate <file> [--format text\|json]` | Check the file and print errors and warnings (`json` is for agents) |
| `change-view instructions` | Print the agent instructions (`CHANGE_REVIEW.md`) |
| `change-view schema` | Print the JSON Schema of the review file |
| `change-view languages` | Print the language names usable for syntax colouring |
| `change-view version` | Print the binary and supported schema versions |

`open` and `validate` read only the repos listed in the review file. Pass
`--scan-siblings` to also look at the folders next to them and list Git repos
with uncommitted changes that the review doesn't mention, for a change that
may have spilled into a neighbouring repo.

Exit codes for `validate` and `open`:

| Code | Meaning | `open` |
| --- | --- | --- |
| 0 | The account matches the repos (warnings allowed) | Page written |
| 1 | Some references are contradicted or unresolved | Page written, with the red banner |
| 2 | The review file is unreadable or breaks the schema | No page, errors printed |
| 3 | Environment problem: repo path missing, not a Git repo, `git` not found | No page, errors printed |

## 6. Troubleshooting

- **`repo_not_found` / `not_repo_root` (exit 3):** repo paths in the file are
  relative to the root of the Git repo that contains the review file. Run
  `open` from anywhere, but keep the repos where they were when the agent
  wrote the file.
- **The page shows old code after more edits:** the review describes an exact
  state. If you or the agent change files afterwards, ask the agent to
  rewrite and re-validate `change.json`.
- **A large graph is hard to read:** search for a node, click it to focus on
  its neighbours, hide unchanged nodes, or switch to Flows for one path at a
  time.
- **Binary blocked on macOS:** see the `xattr` command in *Install*.

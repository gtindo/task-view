# task-view user guide

`task-view` makes it simpler to review code a coding agent wrote. Instead of
reading a flat list of diffs in your IDE and rebuilding in your head how the
pieces relate and which paths they change, you read the change as a map.

When the agent finishes a task, it describes its change in a review file
(`.review/change-<task>.json`): the functions, components and tests it
touched, why, how they relate, and the flows they form. `task-view` turns
that into one HTML page you read in the browser: a graph of the change, its
flows step by step, a map of every changed line, and each piece's code next
to the agent's reason for it.

It works with any programming language, and with changes that mix several
languages across repos. `task-view` never parses code: everything is built
from files, line ranges and diffs. The language names in the review file
only choose the syntax colouring (about 300 languages, listed by
`task-view languages`); a file in any other language shows as plain text.

Before building the page, `task-view` checks every file, line and commit the
agent points at against your repos, so the map matches the real diff. Those
checks never say whether the code is good: that is your job, and the page is
there to make it easier. The tool reads files and runs read-only Git
commands. It never runs your tests, builds or linters. The only file it
writes is the page, in `.review/generated/` unless you choose another path.

## 1. Install

`task-view` installs for your user only: no `sudo`, no administrator rights.
The install scripts download the binary for your machine from the latest
release, check it against `SHA256SUMS`, put it in a folder in your home
directory and add that folder to your `PATH`. Run the same command again to
update.

The only thing `task-view` needs at run time is `git` on your `PATH`.

### Install script: macOS and Linux

```bash
curl -fsSL https://raw.githubusercontent.com/gtindo/task-view/main/scripts/install.sh | sh
```

[`scripts/install.sh`](https://github.com/gtindo/task-view/blob/main/scripts/install.sh) runs under any POSIX shell
(zsh, bash, dash, busybox). It picks the binary for your system and CPU,
installs it as `~/.local/bin/task-view`, and on macOS clears the quarantine
flag that would otherwise stop the binary with "cannot be opened because the
developer cannot be verified". If `~/.local/bin` is not on your `PATH` yet, it
adds it to your shell's startup file (`~/.zshrc`, `~/.bash_profile` on macOS,
`~/.bashrc` on Linux, otherwise `~/.profile`). If the checksum does not match,
it stops before installing anything.

Open a new terminal afterwards so the `PATH` change applies. To pin a
version or choose another folder:

```bash
curl -fsSL https://raw.githubusercontent.com/gtindo/task-view/main/scripts/install.sh | TASK_VIEW_VERSION=v1.0.0 TASK_VIEW_INSTALL_DIR="$HOME/bin" sh
```

To read the script before running it:

```bash
curl -fsSL https://raw.githubusercontent.com/gtindo/task-view/main/scripts/install.sh -o install.sh
less install.sh
sh install.sh
```

### Install script: Windows

In PowerShell:

```powershell
irm https://raw.githubusercontent.com/gtindo/task-view/main/scripts/install.ps1 | iex
```

[`scripts/install.ps1`](https://github.com/gtindo/task-view/blob/main/scripts/install.ps1) installs `task-view.exe` in
`%LOCALAPPDATA%\Programs\task-view`, removes the "downloaded from the
internet" mark so Windows doesn't block it, and adds the folder to your user
`PATH`. Open a new terminal afterwards. To pin a version:

```powershell
$env:TASK_VIEW_VERSION = "v1.0.0"; irm https://raw.githubusercontent.com/gtindo/task-view/main/scripts/install.ps1 | iex
```

### Other ways to install

#### Manual download

If you'd rather not run a script, download the file for your machine from the
[releases page](https://github.com/gtindo/task-view/releases) and install it
yourself. Every release has these files:

| File | What it is |
| --- | --- |
| `task-view-darwin-arm64` | macOS on Apple silicon (M1 and later) |
| `task-view-darwin-amd64` | macOS on Intel |
| `task-view-linux-amd64` | Linux x86-64 (static, no dependencies) |
| `task-view-linux-arm64` | Linux ARM64, e.g. Raspberry Pi 4/5, Graviton (static) |
| `task-view-windows-amd64.exe` | Windows x86-64 |
| `SHA256SUMS` | Checksums of the five binaries |
| `TASK_REVIEW.md` | The instructions your agent follows to write the review file |
| `SKILL.md` | The same workflow packaged as an agent skill |
| `USER_GUIDE.md` | This guide |
| `LICENSE` | The MIT license of `task-view` |
| `THIRD_PARTY_LICENSES.txt` | Licenses of the libraries embedded in the page (Cytoscape.js, ELK) |

macOS (Apple silicon shown; use `darwin-amd64` on Intel):

```bash
mkdir -p ~/.local/bin
install -m 755 ~/Downloads/task-view-darwin-arm64 ~/.local/bin/task-view
xattr -d com.apple.quarantine ~/.local/bin/task-view 2>/dev/null || true
```

Linux (x86-64 shown; use `linux-arm64` on ARM):

```bash
mkdir -p ~/.local/bin
install -m 755 ~/Downloads/task-view-linux-amd64 ~/.local/bin/task-view
```

If `~/.local/bin` is not on your `PATH`, add this line to `~/.zshrc` or
`~/.bashrc` and open a new terminal:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

To install for every user instead, copy the binary to `/usr/local/bin` with
`sudo`.

Check it works, and optionally the checksum, from the folder holding the
downloads:

```bash
task-view version
shasum -a 256 -c SHA256SUMS --ignore-missing   # or sha256sum on Linux
```

On Windows, compare `certutil -hashfile task-view.exe SHA256` with the
`task-view-windows-amd64.exe` line of `SHA256SUMS`.

#### With Go

If you have Go installed, build and install the latest version into
`$(go env GOPATH)/bin`:

```bash
go install github.com/gtindo/task-view/cmd/task-view@latest
```

From a checkout of the repository, `make install` does the same.

## 2. Teach your agent the format

Your agent learns the review file format from `TASK_REVIEW.md`. Copy it into
the repo where you work and tell the agent to follow it when it finishes a task.

```bash
cp TASK_REVIEW.md ~/code/my-app/
```

Then add one line to that repo's `AGENTS.md` (Codex, most agents) or
`CLAUDE.md` (Claude Code):

```markdown
When you finish a task, follow TASK_REVIEW.md.
```

In Claude Code you can also import it directly with a line `@TASK_REVIEW.md`
in `CLAUDE.md`.

### Or install it as a skill

`SKILL.md` packages the same workflow as an agent skill: the agent loads it
when it finishes a task, and it tells the agent to run `task-view instructions`,
so it always matches the installed binary. For Claude Code, put it in the
repo (or in `~/.claude/skills/` for every repo):

```bash
mkdir -p .claude/skills/task-review
cp SKILL.md .claude/skills/task-review/SKILL.md
```

Other agents that read skills (a `SKILL.md` with `name` and `description`
front matter) take the same file in their own skills folder.

You don't strictly need the file: the binary prints the same text, so you can
just ask "run `task-view instructions` and write a review file for this
task". The copy attached to each release matches that release. After you update
`task-view`, refresh it with `task-view instructions > TASK_REVIEW.md`.

## 3. Daily workflow

1. **Give the agent a task** and let it work on its own. It works across one
   or more local repos (for example a client and its mock server).
2. **When it finishes, the agent writes `.review/change-<task>.json`** in the
   repo where the task started, named after the task. It starts from
   `task-view gather`, which prints the commits, the changed files and their
   hunks exactly as the checks will read them, so it only has to describe
   the change, not collect it. Then it runs
   `task-view validate .review/change-<task>.json --format json` and fixes
   the file until it validates. It adds `.review/` to `.git/info/exclude`, so
   the file is never committed. Each task gets its own file.
3. **You open the review:**

   ```bash
   task-view open .review/change-<task>.json
   ```

   The page is written to `.review/generated/change-<task>.html` and opens in
   your default browser. It is one self-contained file that works offline;
   keep it, share it, or write it elsewhere with `--out review.html`.
4. **Decide.** If something looks wrong, ask the agent to fix the code or the
   account. It rewrites the task's file and you run `open` again for a fresh
   page.

### Reviewing work that is already committed

You can ask an agent to describe any existing commit, not only work it just
did:

> Run `task-view instructions` and write `.review/change-<name>.json` for commit
> `<sha>` (and the commits since `<start>` if several). Don't change any code.

The agent gathers them with `task-view gather --base <sha>^ --tip <sha>`
(or `--base <start>` for several) and lists those commits in the file. Unrelated uncommitted files in the
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

The fill shows the change: green **added**, yellow **modified**, red **deleted**,
outlined only for **unchanged**. A node you marked read carries a check in its
corner and fades back. Test nodes are rounded and carry an
"agent says: passed / failed / not run" badge: the tool never runs tests, so a
test result is always the agent's word.

### Layout

- **Top bar:** the status counts, the number of errors and warnings, and how
  many nodes you marked read. Click a count to list those findings. The button at the far left hides the side
  panel; the button at the far right switches between auto, light and dark
  themes.
- **Red banner:** only when the review file still has errors. The page still
  shows the account, but anything contradicted or unresolved is not a fact.
  Ask the agent to fix it.
- **Side panel:** the agent's summary, each repo with its commits from the base
  to the final state, the findings grouped by kind, and the agent's notes.
  Drag its right edge to resize it; double-click the edge to reset its width.
- **Inspector (right):** opens when you click a node, an edge, a hunk or a
  finding group. For a node it shows where it is and a **Mark read** button
  (or press `r`), then tabs: **Code** first, with the changed lines
  highlighted, then the agent's account (why it changed), its tests, its
  relations to other nodes and its findings. The code box has two tabs of its
  own: **Diff** shows the changed lines with what they replaced, **After**
  shows the whole function (or file) as it reads after the change. Drag its left edge to widen it for long code
  lines; double-click the edge to reset its width.

### The views

| View | Use it to |
| --- | --- |
| **Flows** | Start here: the page opens on this view. Follow what each entry (a screen, a button, a command, an endpoint) reaches, drawn as a small graph from the entry rightwards, with a branch wherever the code fans out. Edge labels say how two steps relate; click one to see the line it rests on. A dotted edge means the agent gave no reference for that link; a dashed outline marks a step the entry does not reach. Tests are not drawn here: read them in the graph or a node's Tests tab. The head counts the steps you read; **Mark reviewed** folds a flow you are done with. |
| **Graph** | See the whole change at once. Nodes are grouped by repo; endpoints sit between repos. Click a node to focus on it and its neighbours. Use the search box (or `/`) to find a node; Enter jumps to it. Toggle unchanged, test and import nodes (tests and import lines start hidden), switch the direction, zoom with the buttons or the trackpad. Docs and standalone config live in their own view. |
| **Diff map** | Check that nothing was left out. Every changed file has a bar showing its hunks: green covered by a node, amber partly covered, red not mentioned at all. Expand a file to read its hunks. "Needs attention" shows only files with gaps; the commit menu (or clicking a commit in the side panel) shows what each commit wrote. A file counts as reviewed once every node covering its hunks is read; mark a hunk no node covers with its own **Mark reviewed** button. The summary shows files reviewed and left to review, and **Hide reviewed** leaves only what you still have to read. |
| **Claims** | Check statements you might doubt ("the index is unique", "import needs no change"). Expand a piece of evidence to see the exact code or commit it points at. |
| **Docs & config** | Read the nodes that never link to code: documentation, and config files no code node uses. Listed by kind; click one to open it in the inspector. Only shown when the review has such nodes. |

Read and reviewed marks are kept in your browser per review file, so they
survive `task-view open` regenerating the page. A mark is dropped when the
code it covered changed since you read it.

### Findings worth your attention

| Finding | What it tells you |
| --- | --- |
| Contradicted or unresolved references | The account is wrong somewhere: send it back to the agent |
| Unmentioned changes | Lines changed that no node describes: the agent didn't tell you about them |
| Code outside the flows | A changed function or component on no flow (`node_not_in_flow`), or a flow step its entry does not reach (`flow_gap`): the flows don't show how that code runs |
| Unreferenced test code | Test-file changes outside every test case the agent listed (an edited assertion, a helper) |
| Work outside the review | Uncommitted files or later commits that this review does not cover |
| Unlisted repos | Only with `--scan-siblings`: neighbouring repos with uncommitted changes that the review doesn't list. Usually unrelated; check if the change could have touched them |

### Keyboard

| Key | Action |
| --- | --- |
| `1` `2` `3` `4` `5` | Flows, Graph, Diff map, Claims, Docs & config |
| `/` | Find a node (graph) |
| `r` | Mark the selected node read or unread |
| `[` | Hide or show the side panel |
| `Esc` | Close the inspector |

## 5. Command reference

| Command | What it does |
| --- | --- |
| `task-view open <file> [--out <path>] [--no-browser]` | Check the file, write the HTML page and open it |
| `task-view validate <file> [--format text\|json]` | Check the file and print errors and warnings (`json` is for agents) |
| `task-view gather [<path>[=<base>]...] [--base <rev>] [--tip <rev>] [--context <path>]` | Print what the agent describes: the repos and commits of a change and every changed file with its hunks. Read-only; for the agent |
| `task-view instructions` | Print the agent instructions (`TASK_REVIEW.md`) |
| `task-view schema` | Print the JSON Schema of the review file |
| `task-view languages` | Print the language names usable for syntax colouring |
| `task-view version` | Print the binary and supported schema versions |

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
  rewrite and re-validate the task's review file.
- **A large graph is hard to read:** search for a node, click it to focus on
  its neighbours, hide unchanged nodes, or switch to Flows for one path at a
  time.
- **Binary blocked on macOS:** see the `xattr` command in *Install*.

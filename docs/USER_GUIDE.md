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

## Contents of this folder

These files are attached to every release on the
[GitHub Releases page](https://github.com/gtindo/task-view/releases).

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

The only thing `task-view` needs at run time is `git` on your `PATH`.

## 1. Install

`task-view` installs for your user only: no `sudo`, no administrator rights.
The scripts below download the binary for your machine from the latest
release, check it against `SHA256SUMS`, put it in a folder in your home
directory and add that folder to your `PATH`. Run the same script again to
update.

### macOS and Linux

Paste this into a terminal. It works in zsh and bash, picks the right binary
for your system and CPU, installs it as `~/.local/bin/task-view`, and on
macOS clears the quarantine flag that would otherwise stop the binary with
"cannot be opened because the developer cannot be verified". If
`~/.local/bin` is not on your `PATH` yet, it adds it to your shell's startup
file (`~/.zshrc`, `~/.bash_profile` on macOS or `~/.bashrc` on Linux).

```bash
(
  set -eu

  version=latest        # or a tag such as v1.0.0
  dir="$HOME/.local/bin"

  case "$(uname -s)" in
    Darwin) os=darwin ;;
    Linux) os=linux ;;
    *) echo "Unsupported system: $(uname -s)" >&2; exit 1 ;;
  esac

  case "$(uname -m)" in
    x86_64 | amd64) arch=amd64 ;;
    arm64 | aarch64) arch=arm64 ;;
    *) echo "Unsupported CPU: $(uname -m)" >&2; exit 1 ;;
  esac

  file="task-view-$os-$arch"
  base="https://github.com/gtindo/task-view/releases/latest/download"
  [ "$version" = latest ] || base="https://github.com/gtindo/task-view/releases/download/$version"

  tmp="$(mktemp -d)"
  echo "Downloading $file ($version)..."
  curl -fsSL "$base/$file" -o "$tmp/$file"
  curl -fsSL "$base/SHA256SUMS" -o "$tmp/SHA256SUMS"

  cd "$tmp"
  if command -v sha256sum >/dev/null 2>&1; then
    grep " $file\$" SHA256SUMS | sha256sum -c -
  else
    grep " $file\$" SHA256SUMS | shasum -a 256 -c -
  fi
  cd - >/dev/null

  mkdir -p "$dir"
  mv "$tmp/$file" "$dir/task-view"
  chmod 755 "$dir/task-view"
  rm -f "$tmp/SHA256SUMS"
  rmdir "$tmp"

  # A binary downloaded by a browser is quarantined by macOS Gatekeeper and
  # refuses to start; clearing the flag is harmless when it is not set.
  if [ "$os" = darwin ]; then
    xattr -d com.apple.quarantine "$dir/task-view" 2>/dev/null || true
  fi

  "$dir/task-view" version

  case ":$PATH:" in
    *":$dir:"*) echo "Installed $dir/task-view" ;;
    *)
      case "${SHELL##*/}" in
        zsh) rc="$HOME/.zshrc" ;;
        bash) [ "$os" = darwin ] && rc="$HOME/.bash_profile" || rc="$HOME/.bashrc" ;;
        *) rc="$HOME/.profile" ;;
      esac
      echo 'export PATH="$HOME/.local/bin:$PATH"' >> "$rc"
      echo "Installed $dir/task-view and added $dir to PATH in $rc."
      echo "Open a new terminal, or run: export PATH=\"$dir:\$PATH\""
      ;;
  esac
)
```

Open a new terminal afterwards so the `PATH` change applies. To pin a
version, change `version=latest` to a tag such as `version=v1.0.0`.

### Windows

Paste this into PowerShell. It installs `task-view.exe` in
`%LOCALAPPDATA%\Programs\task-view`, removes the "downloaded from the
internet" mark so Windows doesn't block it, and adds the folder to your user
`PATH`.

```powershell
& {
  $ErrorActionPreference = "Stop"
  $ProgressPreference = "SilentlyContinue"
  [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

  $version = "latest"   # or a tag such as v1.0.0
  $dir = Join-Path $env:LOCALAPPDATA "Programs\task-view"
  $file = "task-view-windows-amd64.exe"
  $base = "https://github.com/gtindo/task-view/releases/latest/download"
  if ($version -ne "latest") { $base = "https://github.com/gtindo/task-view/releases/download/$version" }

  $tmp = Join-Path $env:TEMP $file
  $sums = Join-Path $env:TEMP "task-view-SHA256SUMS"
  Write-Host "Downloading $file ($version)..."
  Invoke-WebRequest "$base/$file" -OutFile $tmp -UseBasicParsing
  Invoke-WebRequest "$base/SHA256SUMS" -OutFile $sums -UseBasicParsing

  $expected = (Select-String -Path $sums -Pattern " $file$").Line.Split(" ")[0]
  $actual = (Get-FileHash $tmp -Algorithm SHA256).Hash
  Remove-Item $sums
  if ($actual -ne $expected) { Remove-Item $tmp; throw "Checksum mismatch for $file" }

  New-Item -ItemType Directory -Force $dir | Out-Null
  $exe = Join-Path $dir "task-view.exe"
  Move-Item -Force $tmp $exe
  Unblock-File $exe
  & $exe version

  $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
  if (($userPath -split ";") -notcontains $dir) {
    $newPath = if ($userPath) { "$userPath;$dir" } else { $dir }
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    Write-Host "Installed $exe and added $dir to your PATH. Open a new terminal."
  } else {
    Write-Host "Installed $exe"
  }
}
```

Open a new terminal afterwards so the `PATH` change applies.

### Manual install

If you'd rather not run a script, download the file for your machine from the
[releases page](https://github.com/gtindo/task-view/releases) and install it
yourself. macOS (Apple silicon shown; use `darwin-amd64` on Intel):

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
task". The copy in this folder always matches this binary. After you update
`task-view`, refresh it with `task-view instructions > TASK_REVIEW.md`.

## 3. Daily workflow

1. **Give the agent a task** and let it work on its own. It works across one
   or more local repos (for example a client and its mock server).
2. **When it finishes, the agent writes `.review/change-<task>.json`** in the
   repo where the task started, named after the task, then runs
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
| `task-view open <file> [--out <path>] [--no-browser]` | Check the file, write the HTML page and open it |
| `task-view validate <file> [--format text\|json]` | Check the file and print errors and warnings (`json` is for agents) |
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

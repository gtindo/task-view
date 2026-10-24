# task-view benchmark

**What does writing a task-view review cost, for a change of a given size,
on a given model and effort?** This folder answers that with fixed inputs
anyone can rerun: ten real tasks from four open-source projects, the same
brief and prompt for every run, and a runner that measures tokens, cost,
time, validate rounds and how consistent the resulting map is.

It measures the review step only: the agent has already done the task and
gets a brief of it, the way it would still have that context right after
finishing. It does not measure whether the task's code is good. The latest published
results are in [BENCH.md](../BENCH.md).

## The cases

Each case is a task: one or more consecutive commits from the project's
history, replayed as the state an agent leaves when it finishes. Its earlier
commits sit on a branch `task`, and its newest commit's change is left
uncommitted in the working tree. Sizes are judged on the whole task's
changed lines: small up to 80, medium up to 500, big up to 2,000, and
agent-scale beyond that. Agent-scale is the size of change agents produce in
a short time and a person rarely writes by hand in one sitting; flowmap-big
is that case.

| Case | Language | Commits | Files | Changed lines | What the task was |
| --- | --- | --- | --- | --- | --- |
| httpx-small | Python | 1 | 3 | 21 | `iter_text` no longer yields a stray empty string |
| httpx-medium | Python | 1 | 6 | 217 | Deprecate `app=` for explicit WSGI and ASGI transports |
| httpx-big | Python | 1 | 29 | 1,480 | New `SSLContext` API; remove `proxies=`, `app=`, `URL.raw` |
| hono-small | TypeScript | 1 | 2 | 30 | CORS preflight appends `Origin` to `Vary` |
| hono-medium | TypeScript | 1 | 4 | 276 | RegExpRouter wildcard middleware for suffix and parameter routes |
| hono-big | TypeScript | 1 | 12 | 591 | Close XSS holes in server-side JSX escaping |
| axum-small | Rust | 1 | 2 | 33 | Merged `Allow` headers no longer repeat `HEAD` |
| axum-medium | Rust | 3 | 7 | 151 | Remove extra router state clones, with test helpers |
| axum-big | Rust | 4 | 22 | 782 | WebSockets over HTTP/2, `MethodFilter::CONNECT`, a Content-Length fix, plus side work |
| flowmap-big | Go | 6 | 28 | 5,187 | Engine protocol: wire contract, engine session, stdio command and workbench adapter |

The projects are [encode/httpx](https://github.com/encode/httpx) (BSD-3),
[honojs/hono](https://github.com/honojs/hono) (MIT),
[tokio-rs/axum](https://github.com/tokio-rs/axum) (MIT) and
[gtindo/flowmap](https://github.com/gtindo/flowmap) (MIT). The first three
squash-merge their PRs, so each task is a clean first-parent chain; flowmap
adds one agent-scale Go task built over several commits. Full SHAs and sources
are in [`cases.json`](cases.json); the briefs are in [`briefs/`](briefs/).

## What a run does

1. Clones the case's project at the task's end state into a throwaway
   folder, with every other branch, tag and remote removed.
2. Puts a `task-view` wrapper on the agent's PATH (and names its absolute
   path in the prompt). The wrapper logs every call.
3. Sends [`prompt.md`](prompt.md), filled with the case's brief, to the agent
   through a harness adapter, and times it.
4. Afterwards, with the same pinned task-view binary: validates the review
   file, renders its page with `task-view open`, reads its shape, and checks
   the agent changed nothing but the review file.

Each run's prompt, review file, validate output, page, call log and adapter
output are kept in `<results>/runs/<run id>/`.

## Running it

You need Go, Git, network access for the first clone, and a harness. Out of
the box that is [Claude Code](https://code.claude.com) signed in.

```bash
make bench-prepare        # clone the projects, check every case, build task-view
make bench                # run the matrix in matrix.json (180 runs by default)
make bench-report         # write report.md and summary.csv
make bench-publish        # replace BENCH.md and bench/results/ with this run
```

The same commands work from `bench/` as `go run ./cmd/bench prepare|run|report|publish`.

Try one cell before the whole matrix:

```bash
make bench BENCH_ARGS="-cases httpx-small -models haiku -efforts medium -repeats 1 -out .cache/results/smoke"
make bench-report BENCH_OUT=.cache/results/smoke
```

A run with the same `-out` resumes: runs already in `runs.jsonl` are
skipped, except those that ended in a harness error (a usage limit, say),
which run again; reports count only each run's latest result. Flags override every field of [`matrix.json`](matrix.json):

| Flag | Default from matrix.json | Meaning |
| --- | --- | --- |
| `-harness` | `harness/claude-code` | The adapter folder |
| `-models` | `haiku,sonnet,opus` | Model ids, passed to the harness as is |
| `-efforts` | `medium,high` | Effort levels, passed as is |
| `-cases` | `*` | Case id globs, e.g. `httpx-*,axum-big` |
| `-repeats` | `3` | Runs per cell |
| `-parallel` | `4` | Runs at the same time |
| `-timeout` | `20m` | Time limit per run |
| `-out` | `.cache/results/latest` | Results folder |
| `-task-view` | built from this checkout | The task-view binary under test |

## Another harness or model

The runner has no harness built in. An adapter is a folder with a
`harness.json` and one executable; the contract is in
[`harness/harness.md`](harness/harness.md). To try another agent, copy
`harness/claude-code/`, change the one command in `run.sh` and run with
`-harness harness/<yours> -models <its model ids>`. Add a `price` key to
[`prices.json`](prices.json) for each model if the harness does not report
its own cost; otherwise cost is left out and everything else is still
measured.

## What the report shows

Per case, model and effort, over the repeats:

- **Accurate:** runs whose final review validates with exit 0 and no
  unmentioned hunk, without the agent touching the task's code.
- **First validate passed:** runs whose first `validate` already exited 0.
- **Validate runs, minutes, tokens and cost**, as a mean with its range.
  Cost is the harness's own figure when it reports one, else tokens times
  `prices.json`, marked "computed".
- **US$ per 100 changed lines**, so cost can be read against change size.
- **Node and edge agreement:** how alike the repeats' maps are, as the mean
  pairwise overlap (Jaccard) of changed-node ranges and of edges. 1 means
  every repeat drew the same map.

## Known biases

- **One harness, one model family.** We run it with Claude Code and
  Anthropic models. The harness's tools, system prompt and context handling
  shape the result as much as the model does, and none of that is varied
  here. Results from other harnesses are not comparable to ours without
  rerunning both.
- **Fixed briefs.** We wrote every brief from the PR, advisory or changelog
  and the diff. A real agent's memory of its own work is richer and messier.
- **The prompt is ours.** It asks the agent to follow `task-view
  instructions` and to stop at a clean validate; a different hand-over would
  give different numbers.
- **No tests run.** Agents are told not to run project tooling, so test
  results in the reviews are always "not run".
- **Four projects, ten tasks.** httpx, hono and axum are popular,
  well-structured codebases with clean history; flowmap, the one
  agent-scale case, is a project of task-view's author. Messier repos, generated code or huge monorepos are
  not covered.
- **task-view version.** Results hold for the binary they were run with
  (recorded in `meta.json`). The instructions it prints change between
  versions.

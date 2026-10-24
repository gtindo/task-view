# Harness adapters

A harness adapter runs one agent session for one benchmark run. The runner
knows nothing about any particular agent: everything harness-specific lives
in a folder like `claude-code/`. To benchmark another harness, copy that
folder, change the one command in it, and run with `-harness harness/<yours>`.

## Folder

```
harness/<name>/
  harness.json   how the runner calls the adapter and reads its usage
  run.sh         the adapter (any executable works)
```

`harness.json`:

| Field | Meaning |
| --- | --- |
| `name` | Recorded in every result line |
| `command` | The executable to run, relative to the folder |
| `usage_format` | `usage-json` (the generic format below) or `claude-code` (Claude Code's `--output-format json` result) |
| `usage_file` | The file the adapter writes into `$BENCH_OUT`, read with `usage_format` |
| `version_command` | Optional: a shell command, run in the folder, whose first output line is the harness's version; recorded in `meta.json` and `BENCH.md` |

## Call

The runner starts `command` with the work dir as its current directory and
these environment variables:

| Variable | Value |
| --- | --- |
| `BENCH_WORKDIR` | The throwaway clone holding the task, also the current directory |
| `BENCH_PROMPT_FILE` | The rendered prompt; send its whole content to the agent as the first user message |
| `BENCH_MODEL` | The model `id` from the matrix, passed as is |
| `BENCH_EFFORT` | The effort from the matrix, passed as is |
| `BENCH_OUT` | An empty folder for the adapter's files: the usage file, transcripts, logs |
| `BENCH_TIMEOUT` | The run's time limit in seconds; the runner kills the adapter after it |
| `PATH` | Starts with a folder holding `task-view`, a wrapper that logs each call |

The adapter must run the agent non-interactively, wait for it to finish, and
exit 0 when the agent ran (whatever it did). A non-zero exit is recorded as a
harness failure. It must not change the work dir itself.

Map `BENCH_MODEL` and `BENCH_EFFORT` to your harness's own flags. Ids are not
translated by the runner, so a matrix for another harness lists that
harness's model ids (and a `price` key for each, see `prices.json`).

## Usage

With `usage_format: "usage-json"`, write `$BENCH_OUT/<usage_file>` as one JSON
object, every field optional:

```json
{
  "input_tokens": 1200,
  "output_tokens": 5400,
  "cache_read_tokens": 410000,
  "cache_write_tokens": 38000,
  "cost_usd": 0.42,
  "turns": 14,
  "model": "the model id the harness reported"
}
```

`cost_usd`, when given, is reported as the harness's own figure. Without it,
the runner computes the cost from the token counts and the model's `price`
entry in `prices.json`, and marks it as computed. A harness that reports no
tokens at all still gets time, task-view calls and review quality measured.

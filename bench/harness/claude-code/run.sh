#!/bin/sh
# Claude Code adapter for the task-view benchmark; the contract is in
# ../harness.md. It runs one agent session headless in the work dir.
#
# --safe-mode turns off the user's CLAUDE.md, skills, plugins, hooks and MCP
# servers, so nothing installed on this machine (a task-review skill, say)
# leaks into the run. Tools are limited to what the task needs, and edits
# are accepted without a prompt: the work dir is a throwaway clone.
set -eu

cd "$BENCH_WORKDIR"

claude -p "$(cat "$BENCH_PROMPT_FILE")" \
  --model "$BENCH_MODEL" \
  --effort "$BENCH_EFFORT" \
  --output-format json \
  --safe-mode \
  --strict-mcp-config \
  --no-session-persistence \
  --permission-mode acceptEdits \
  --allowedTools "Bash,Read,Write,Edit,Glob,Grep" \
  > "$BENCH_OUT/claude.json"

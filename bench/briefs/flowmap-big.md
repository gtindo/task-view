## What was asked

Implement version `"0"` of the engine protocol in `docs/tdd/0001-flowmap-engine-protocol.md`, a UI-independent JSON-RPC 2.0 protocol for editor integrations, and move the browser workbench onto it without changing the embedded UI.

## What I did

I made six changes, oldest first. The first five are committed; the last is still uncommitted.

1. **Wire contract** (`internal/protocol`): models, JSON-RPC envelopes and error codes, strict `Content-Length` framing, `file:` URIs, and a `Client` that matches responses by id and delivers notifications in order. It imports no analysis code.
2. **Engine session** (`internal/engine`): `Engine.Serve` handles lifecycle and exact version negotiation; workspaces hold one view per language, each running one analysis at a time with best-effort cancellation. Complete snapshots are published after commit, keeping current plus predecessor. Queries address a snapshot and page with self-validating cursors. `StartInProcess` runs a session over in-memory pipes. I copied the summarizer and cache from `internal/server`.
3. **`flowmap engine`** (`cmd/flowmap/main.go`): `runEngine` serves stdio; logs go to stderr.
4. **Workbench adapter** (`internal/server`): `server.New(ctx, client, configs)` replaces the old index-owning constructors. Scans become `analysis/start` plus a wait for `analysis/published` or `analysis/failed`. `convert.go` maps protocol models back to the snake_case HTTP JSON; `httpStatus` maps protocol errors. `serve` starts an in-process engine, and the server's summarizer copy is deleted. Maps, `AGENTS.md`, `README.md` and `USER_GUIDE.md` follow.
5. **TDD doc**: marked implemented, with notes on where version 0 lives.
6. **Tests** for failure and lifecycle paths.

## Decisions

- The `analysis/start` response is written before that analysis's notifications. A per-view `emit` lock keeps notifications in commit order.
- In-process clients cross the same framing as stdio clients.
- A second analysis on the same view maps to HTTP 409. A disconnected scan request cancels its analysis.

## Tests

Tests are in `protocol_test.go`, `client_test.go`, `engine_test.go`, `server_test.go` and `cmd/flowmap/main_test.go`. `TestEngineCommandServesProtocolOverStdio` drives a real Go analysis over stdio.

I did not run them in this session.

## What was asked

Support WebSockets over HTTP/2 (RFC 8441 extended CONNECT) and add `MethodFilter::CONNECT` routing. Also stop `Route` setting `content-length` before middleware runs, which leaves a body-adding middleware with a wrong length and makes Hyper panic.

## What I did

I made four changes, oldest first. The first three are committed; the last is still uncommitted.

1. **HTTP/2 WebSockets** (`axum/src/extract/ws.rs`): for HTTP/1.1 and earlier, `WebSocketUpgrade` keeps the `GET` and `Upgrade` checks. For later versions it requires `CONNECT` and a `:protocol` of `websocket`, with new rejections `MethodNotConnect` and `InvalidProtocolPseudoheader`. `sec_websocket_key` becomes an `Option`, and `on_upgrade` answers HTTP/2 with an empty 200. `serve.rs` enables the connect protocol under `http2`. In `route.rs`, `RouteFuture` stores the request `Method` instead of `strip_body`. A 2xx `CONNECT` response skips `set_content_length`; if it has a body, `content-length` or `transfer-encoding`, the future logs an error and empties the body. Docs and the `websockets` example use `any(...)`; new `examples/websockets-http2`.
2. **Side work, `axum_extra::json!`** (`axum-extra/src/response/erased_json.rs`): a macro that wraps `typed_json::json!` in `ErasedJson`, under the `erased-json` feature.
3. **CONNECT routing** (`method_filter.rs`, `method_routing.rs`): `MethodFilter::CONNECT`, `routing::connect`/`connect_service` and the matching `MethodRouter` methods. A `connect` field joins `with_state`, the layers, `merge_for_path` and `call!` dispatch; `on_endpoint` gets a `CONNECT` `set_endpoint` call. In axum-extra: `RouterExt::typed_connect`.
4. **Content-Length fix** (`route.rs`): `RouteFuture` gains a `top_level` flag. `Route`'s `Service::call` marks its future `not_top_level()`, so only the top-level future sets `Allow` and `content-length` and strips `HEAD` bodies. I dropped the unused `RouteFutureKind` and `Route::layer`'s `MapRequestLayer`.

## Decisions

- WebSocket handlers move from `get` to `any` (HTTP/2 uses `CONNECT`); it reads better than `on(MethodFilter::GET.or(MethodFilter::CONNECT), ..)`.
- `json!` lives in axum-extra so that core does not take the `typed-json` dependency. Returning `ErasedJson` keeps the response `Clone`.

## Tests

New tests: `http2` in `ws.rs` (the echo tests are refactored), `CONNECT` conversion in `method_filter.rs`, and `middleware_adding_body` in `routing/tests/mod.rs`.

I did not run them in this session.

## What was asked

Fix `MethodRouter::merge` listing a method twice in the `Allow` header. `get` registers both `GET` and `HEAD`, so `get(a).merge(head(b))` produced `Allow: GET,HEAD,HEAD` on a 405 response. The same happens through `Router::merge`. The builder path (`get(..).head(..)`) already yields `GET,HEAD`.

## What I did

- **Allow header merge** (`axum/src/routing/method_routing.rs`): in `AllowHeader::merge`, the `Bytes` + `Bytes` arm no longer joins the two byte strings with a comma. It decodes the other side as UTF-8, splits it on `,`, and feeds each method through the existing `append_allow_header`. That helper already skips methods that are present, so the merge now de-duplicates the same way the builder does.
- **Helper signature**: `append_allow_header` now takes `method: &str` instead of `&'static str`, so it accepts the slices split out of the other header. It only copies the bytes, so nothing else changes.
- **Changelog** (`axum/CHANGELOG.md`): a "fixed" entry for #3836.

## Decisions

- If the other header is ever not valid UTF-8, the arm panics in debug builds via `#[cfg(debug_assertions)]`, with a message saying this should never happen. In release builds the other side's methods are dropped and the left-hand value is kept.
- The `None` and `Skip` arms are unchanged. Merging routers with disjoint methods still concatenates them in order.

## Tests

- New test `allow_header_merging_get_into_head` in the `tests` module of `method_routing.rs`. It merges `get(ok)` with `head(created)`, calls with `DELETE`, and asserts `405 Method Not Allowed` with `Allow: GET,HEAD`.
- The existing `allow_header_when_merging` test, which expects `PUT,PATCH,GET,HEAD`, still covers in-order concatenation.

I did not run them in this session.

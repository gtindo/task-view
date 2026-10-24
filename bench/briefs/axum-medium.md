## What was asked

Cut avoidable clones of the router state on the request path. This is a piece pulled out of the larger PR #2865. The second part covers the fallback path with a test first, then removes a clone there.

## What I did

I did the work in three steps, oldest first. The first two are committed; the third is still uncommitted.

1. **Method routes** (`axum/src/routing/route.rs`, `method_routing.rs`): added `Route::oneshot_inner_owned`, which consumes the route instead of cloning its inner service the way `oneshot_inner` does. In `MethodRouter::call_with_state`, the `call!` macro now uses it for both `MethodEndpoint::Route` and `MethodEndpoint::BoxedHandler`. The macro also lost its `$method` argument: it compares `*req.method()` directly instead of a cloned `method` local.
2. **Clone-counting helper** (`axum/src/test_helpers/counting_cloneable_state.rs`, registered in `test_helpers/mod.rs`): `CountingCloneableState` counts clones once `setup_done()` is called and prints a filtered backtrace for each. `state_isnt_cloned_too_much` in `routing/tests/mod.rs` now uses it instead of its own statics.
3. **Fallback** (`axum/src/routing/mod.rs`): `Fallback::call_with_state` now takes `self` instead of `&mut self` and calls `oneshot_inner_owned` on every variant. Both callers (`MethodRouter::call_with_state` and `Router::call_with_state`) already call it on a clone, so no call site changes.

## Decisions

- I kept `oneshot_inner` because `Route`'s `Service::call` still needs the borrowing version.
- The helper shares its counters through an `Arc`, so each test owns its own count instead of using process-wide statics.

## Tests

- `state_isnt_cloned_too_much` (`routing/tests/mod.rs`): the expected count drops from 4 to 3 in step 1.
- New `state_isnt_cloned_too_much_with_fallback` (`routing/tests/fallback.rs`), added in step 2 expecting 4 and lowered to 3 in step 3.

I did not run them in this session.

## What was asked

Deprecate the `httpx.Client(app=...)` and `httpx.AsyncClient(app=...)` shortcuts in favour of the explicit style: `transport=httpx.WSGITransport(app=app)` or `transport=httpx.ASGITransport(app=app)`. The rationale is in issue #947. Docs and `CHANGELOG.md` were part of the ask.

## What I did

- **Clients** (`httpx/_client.py`): `Client.__init__` and `AsyncClient.__init__` now emit a `DeprecationWarning` when `app` is passed. Each message names the matching transport. The `app` parameter still works: `_init_transport` still wraps it in `WSGITransport` / `ASGITransport`.
- **Docs**: `docs/advanced/transports.md` now shows the WSGI example with an explicit `WSGITransport`, adds "Example" and "Configuration" subheadings, and has a new `ASGITransport` section. That section holds the Starlette example, the `raise_app_exceptions` / `root_path` / `client` options and the lifespan note, all moved from `docs/async.md`. `docs/async.md`'s "Calling into Python Web Apps" section is now a single link to that section.
- **Changelog**: a "Deprecated" entry under Unreleased.

## Decisions

- This is a warning only. Removal is left for a later release.
- In `AsyncClient.__init__` I also dropped `app is None` from the `allow_env_proxies` condition, so it is now `trust_env and transport is None`. `Client.__init__` still has `trust_env and app is None and transport is None`. The two clients now differ there.

## Tests

- In `tests/test_wsgi.py` and `tests/test_asgi.py`, every test that used `app=` now builds the transport explicitly and passes `transport=`, so the suite no longer triggers the warning.
- Each file gets a new `test_deprecated_shortcut` that asserts `pytest.warns(DeprecationWarning)` when constructing a client with `app=`.

I did not run them in this session.

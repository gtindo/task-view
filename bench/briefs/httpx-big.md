## What was asked

Prepare version 0.28.0, as the CHANGELOG entry I added says: add an `httpx.SSLContext` class and `ssl_context` parameter, deprecate `verify` and `cert`, and remove the already-deprecated `proxies` and `app` arguments and the `URL.raw` property.

## What I did

- **SSL config** (`httpx/_config.py`): replaced `SSLConfig` with `SSLContext(ssl.SSLContext)`, exported from `httpx/__init__.py`. Its `verify` flag sets the verify mode and hostname check. When verifying, it loads `certifi` and stricter verify flags. It honours `SSLKEYLOGFILE`. `create_ssl_context` stays, but only as a shim that warns and builds an `SSLContext` from the old `verify`/`cert` values.
- **Clients, API and transports** (`_client.py`, `_api.py`, `_transports/default.py`): every entry point takes `ssl_context`. `verify` and `cert` stay as trailing deprecated `typing.Any` arguments forwarded to `HTTPTransport` / `AsyncHTTPTransport`. The transports drop `trust_env`. `proxies` and `app` are gone, and `_get_proxy_map` only accepts a single `proxy`.
- **Removals**: `URL.raw` and `RawURL`; the `CertTypes`, `VerifyTypes` and `ProxiesTypes` aliases; `get_ca_bundle_from_env`; `Timer`; the whole `httpx/_compat.py`.
- **Dependencies**: brotli/zstandard imports moved into `_decoders.py`. Elapsed time now uses `time.perf_counter()`. `sniffio` is no longer a dependency; `ASGITransport` imports it lazily only to detect trio.
- **Other**: CLI builds `SSLContext(verify=verify)`; version bumped; docs rewritten (`docs/advanced/ssl.md`) and CI branch pattern changed to `version-*`.

## Decisions

- `SSL_CERT_FILE`/`SSL_CERT_DIR` are no longer read automatically. The SSL docs show how to load them by hand.
- Some deprecation warnings gained `stacklevel=2`.

## Tests

`tests/test_config.py` now builds `httpx.SSLContext()` and loads certificates directly; it adds keylog, repr and request tests. I deleted tests for removed features in `test_proxies.py`, `test_url.py`, `test_utils.py`, `test_asgi.py`, `test_wsgi.py`, plus the two `conftest.py` fixtures only they used.

I did not run them in this session.

## What was asked

Fix the CORS middleware so an `OPTIONS` preflight keeps any `Vary` values set by earlier middleware (such as `Vary: Accept-Encoding`). Before, it overwrote them with `Vary: Origin`.

## What I did

- **CORS middleware** (`src/middleware/cors/index.ts`): in the `OPTIONS` branch of the `cors` handler, when `opts.origin` is not `'*'`, I replaced `set('Vary', 'Origin')` with `c.res.headers.append('Vary', 'Origin')`. The local `set` helper calls `c.res.headers.set`, which replaced the whole header.

## Decisions

- This matches how the same middleware already adds the other `Vary` values. The preflight branch appends `Access-Control-Request-Headers`, and the non-preflight path after `next()` uses `c.header('Vary', 'Origin', { append: true })`. Only this one call used `set`.
- I used `c.res.headers.append` directly, like the neighbouring `Access-Control-Request-Headers` line, rather than `c.header(..., { append: true })`.
- Nothing else changes: `Access-Control-*` headers are still set with `set`, and `origin: '*'` still adds no `Vary`.

## Tests

- New test in `src/middleware/cors/index.test.ts`: "Append "Origin" to Vary header on OPTIONS preflight, if response has some Vary header". It registers a middleware that appends `Vary: Accept-Encoding` before `cors({ origin: 'http://example.com', allowHeaders: ['X-Custom-Header'] })`. It then sends an `OPTIONS` request with `Origin` and `Access-Control-Request-Headers`, and asserts a 204, the allowed origin, and `Vary` equal to `Accept-Encoding, Origin, Access-Control-Request-Headers`.
- The existing Vary tests for non-preflight requests in the same file still apply.

I did not run them in this session.

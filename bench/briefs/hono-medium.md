## What was asked

Fix two cases where `RegExpRouter` failed to attach wildcard middleware to routes (#5258, #5260, #5261):

1. A suffix wildcard without a slash, such as `/assets*`, must apply to `/assets/app.js` whatever the registration order. Before, `*` was treated as a literal.
2. A trailing wildcard after a parameter, such as `/:name/*`, must apply to `/:id`. Each handler must still receive params under its own name. Custom patterns like `{[0-9]{4}}` must work too.

## What I did

- **Wildcard matching** (`src/router/reg-exp-router/router.ts`): `buildWildcardRegExp`, which is tested against registered paths, now also escapes `?{}|` and turns a trailing `/*` or `*` into the tail or only-wildcard pattern. Each `/:name` label (optionally `{[^/]+}`) matches a label of any name; a custom pattern body is escaped and must match exactly. The three pattern constants are now exported from `node.ts`.
- **Param mapping**: `HandlerWithMetadata` stores the handler's registered path instead of a param count. `#buildMatcher` builds each handler's param map from that path's own param names, by position, against the matched route's capture indexes, mapped through `paramReplacementMap`. The old post-pass over `handlerData` is gone.
- **Registration**: `add` computes the target `methods` once; its wildcard branch updates middleware and routes in one loop. `buildAllMatchers` iterates route methods only. `clearWildcardRegExpCache` is inlined, and `findMiddleware` no longer accepts `undefined`.

## Decisions

- I compacted the code with `for...of`/`for...in` instead of `forEach`, so the new behaviour does not grow the minified bundle.
- All of this runs at registration and build time. The per-request `match()` path is unchanged.

## Tests

- `src/router/common.case.test.ts`: suffix-wildcard ordering cases; new "Trailing wildcard after a label" and "Trailing wildcard after a pattern label" suites (reverse order, `ALL` middleware, nested braces, regexp metacharacters, `{[^/]+}`).
- `src/router/linear-router/router.test.ts` adds the new cases to LinearRouter's skip list.

I did not run them in this session.

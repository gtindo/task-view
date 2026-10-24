## What was asked

Fix advisory GHSA-hxh3-vqpv-xpqv. During server-side rendering, `hono/jsx` emitted plain strings as markup instead of escaping them in several places: children and `fallback` of `Suspense` and `ErrorBoundary`; the only child of a `Context.Provider`; and string or array roots passed to `renderToString()` / `renderToReadableStream()` from `hono/jsx/dom/server`. Values from `raw()` / `html` must stay trusted.

## What I did

- **Shared helpers** (`src/jsx/base.ts`): `renderChildren` renders children through the normal escaping path. `isUntrustedObject` detects plain objects with a `toString` that are not a `JSXNode`, a `Promise`, an array or already escaped. `renderUntrustedObject` escapes such an object's `toString()` result, sync or async. `resolveFunctionComponentResult` now escapes a string that an async component resolves to.
- **Boundaries**: `resolveChildEarly` / `childrenToString` and `ErrorBoundary`'s fallback handling (`components.ts`), `Suspense` children (via a new `serializeBoundaryChild`) and fallback (`streaming.ts`), and the `Context.Provider` render (`context.ts`) now all use these helpers.
- **DOM server** (`dom/server.ts`): a new `prepareRoot` escapes string and array roots in both render functions.
- **Intrinsic elements** (`intrinsic-element/components.ts`): `returnWithoutSpecialBehavior` goes through `renderChildren`.

## Decisions

- Every `String.replace` that splices rendered content into the buffer now takes a replacer function. This covers the `ErrorBoundary`, `Suspense` and head-hoisting paths. Otherwise `$&`-style patterns in user content would be interpreted.
- A direct `Promise` child is left unchanged. An unsupported object inside an element is still omitted.

## Tests

New escaping cases in `base.test.tsx`, `components.test.tsx` (ErrorBoundary), `streaming.test.tsx` (Suspense), `index.test.tsx` (Context), `dom/server.test.tsx` and `intrinsic-element/components.test.tsx`. They cover string and object escaping, trusted `raw` content, replacement patterns and unchanged promise children.

I did not run them in this session.

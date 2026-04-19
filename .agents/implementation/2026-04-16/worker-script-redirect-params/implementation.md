# Fix: Worker Script Redirect Query Parameter Merging

## Problem

When `redirect: "manual"` was added to the Cloudflare worker script (to properly handle redirects without auto-following), the redirect handling code unconditionally overwrote the redirect Location's query string with the original request's query string:

```js
if (url.search) {
    target.search = url.search;
}
```

This caused any query parameters added by the upstream (e.g., OAuth `code`/`state` params on `/callback?code=abc&state=xyz`) to be lost and replaced by the original request's params.

## Root Cause

With the old `fetch(request, {})` (default `redirect: "follow"`), the browser/runtime followed redirects automatically, so most redirects never reached the redirect-handling code. After switching to `redirect: "manual"`, all redirect responses are intercepted, and the blanket overwrite of `target.search` now destroys upstream-added query parameters.

## Fix

Changed `pkg/clouds/pulumi/cloudflare/registrar.go` (the worker script template) to **merge** query parameters instead of overwriting:

- Parse both the original request's params and the redirect target's params using `URLSearchParams`
- Iterate over original params and only append those not already present in the redirect's URL
- Redirect params take precedence over original request params

## Files Changed

- `pkg/clouds/pulumi/cloudflare/registrar.go` — lines 252-263 (worker script template)

# Path rules (`rules.expr`)

File: `/data/rules.expr` (hot-reloaded on mtime). Language: [expr](https://github.com/expr-lang/expr).

Evaluated on each HTTP **response** (so `Via` / CDN headers exist). Return a map:

| Field | Meaning |
|-------|---------|
| `dest` | Catalog destination id → path extra delay vs CF (and host baseline). `cf` → 0 extra |
| `delay_ms` | Absolute one-way sleep (overrides dest formula when > 0) |

## Environment

| Name | Type |
|------|------|
| `phase` | `"response"` (currently) |
| `host` | request host / SNI |
| `path` | request URI path |
| `request` | map of request headers (lower-case keys) |
| `response` | map of response headers |
| `header(map, name)` | helper |
| `match(regex, s)` | helper |
| `lower(s)` | helper |
| `a contains b` | expr infix operator |

## Example

```text
if lower(header(response, "via")) contains "cloudfront" {
  { "dest": "aws-eu-central-1" }
} else if match("^api\\.example\\.com$", host) && match("^/v1/", path) {
  { "dest": "aws-eu-central-1" }
} else if header(request, "x-env") in ["staging", "perf"] {
  { "delay_ms": 150 }
} else {
  { "dest": "cf" }
}
```

Note: `contains` is an **infix** operator in expr (`a contains b`), not a function.

Default if missing/invalid: `{ "dest": "cf" }` (last-mile only).

`GET /v1/rules` returns source and compile error (if any).

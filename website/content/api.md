---
title: API
weight: 20
---

Base URL: `http://<host>:7783` (publish only the API port from the PotatoNetwork container).

Opening `http://<host>:7783/` in a browser **302-redirects** to this OpenAPI section
(`https://kriakiku.github.io/potato-network/api/#openapi`).
JSON API lives under `/v1/…`.

Auth (optional): `Authorization: Bearer <POTATONETWORK_API_TOKEN>`. If the token ENV/file is empty, auth is off. `/v1/health` and `GET /` are always open.

When `POTATONETWORK_API_TOKEN` is set, the API answers CORS preflight (`OPTIONS`) and allows any origin / method / header (so browser UIs can call with the Bearer token).

## OpenAPI {#openapi}

Interactive reference (ReDoc) plus downloadable specs:

- [swagger.json](https://kriakiku.github.io/potato-network/swagger.json)
- [swagger.yaml](https://kriakiku.github.io/potato-network/swagger.yaml)

<div id="redoc-container"></div>
<script src="https://cdn.redoc.ly/redoc/v2.1.5/bundles/redoc.standalone.js"></script>
<script>
  Redoc.init(
    "https://kriakiku.github.io/potato-network/swagger.json",
    {
      scrollYOffset: 60,
      hideDownloadButton: false,
      expandResponses: "200",
    },
    document.getElementById("redoc-container")
  );
</script>

## Endpoints (summary)

| Method | Path | Body / notes |
|--------|------|----------------|
| GET | `/v1/health` | liveness + profile summary |
| GET | `/v1/profile` | current profile (`emulationLimited` / `warning` when host is already slower than the country) |
| PUT | `/v1/profile` | `{"country":"BD","tier":"typical"}` or `{"passthrough":true}` — same fields; limited cases log `WARN` but still apply |
| GET | `/v1/baseline` | `hostRtt`, `probedAt` |
| POST | `/v1/baseline/probe` | TCP RTT probe to catalog destinations |
| PUT | `/v1/baseline` | `{"hostRtt":{"cf":12,"aws-eu-central-1":20}}` |
| GET | `/v1/catalog` | full catalog JSON |
| POST | `/v1/catalog/refresh` | pull catalog from configured URL |
| GET | `/v1/ca.pem` | download root CA |
| GET | `/v1/rules` | `rules.expr` source + compile status |

## Examples

```bash
# Probe host baseline, then emulate Bangladesh typical
curl -s -X POST "$API/v1/baseline/probe"
curl -s -X PUT "$API/v1/profile" -H 'Content-Type: application/json' \
  -d '{"country":"BD","tier":"typical"}'

# Clear shaping
curl -s -X PUT "$API/v1/profile" -H 'Content-Type: application/json' \
  -d '{"passthrough":true}'
```

After container restart, profile defaults to **passthrough**, or to `POTATONETWORK_PROFILE_COUNTRY` / `POTATONETWORK_PROFILE_TIER` when those ENV vars are set. Baseline is stored in `/data/baseline.json` and reloaded on boot; if missing or older than the baseline cron interval it is probed automatically (`POTATONETWORK_BASELINE_CRON`, default every 3 hours).

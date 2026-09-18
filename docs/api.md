# API

Base URL: `http://<host>:7783` (publish only the API port from the PotatoNetwork container).

Opening `http://<host>:7783/` in a browser **302-redirects** to the GitHub Pages API docs
(`POTATONETWORK_DOCS_URL`, default `https://kriakiku.github.io/potato-network/api/`).
JSON API lives under `/v1/…`.

Auth (optional): `Authorization: Bearer <POTATONETWORK_API_TOKEN>`. If the token ENV/file is empty, auth is off. `/v1/health` is always open.

| Method | Path | Body / notes |
|--------|------|----------------|
| GET | `/v1/health` | liveness + profile summary |
| GET | `/v1/profile` | current profile |
| PUT | `/v1/profile` | `{"country":"BD","tier":"typical"}` or `{"passthrough":true}` |
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

After container restart, profile resets to **passthrough**. Baseline is stored in `/data/baseline.json` and reloaded on boot; if missing or older than the baseline cron interval it is probed automatically (`POTATONETWORK_BASELINE_CRON`, default every 3 hours).

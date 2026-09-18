# 🥔 PotatoNetwork

Go-only last-mile network emulator for Docker: run PotatoNetwork as a container, attach apps with `network_mode: service:potatonetwork`, pick a country profile via API, and traffic (including DNS) gets shaped. Transparent MITM applies path delay from an [expr](https://github.com/expr-lang/expr) script. No WireGuard, no web UI.

## Quick start

Runtime image is **`scratch` + UPX-compressed static binary** (catalog and Mozilla CA roots are embedded; no apt packages).

```bash
docker compose up -d --build
curl -s localhost:7783/v1/health
curl -s -X POST localhost:7783/v1/baseline/probe
curl -s -X PUT localhost:7783/v1/profile \
  -H 'Content-Type: application/json' \
  -d '{"country":"BD","tier":"typical"}'
```

Sidecar example:

```yaml
services:
  potatonetwork:
    build: .
    cap_add: [NET_ADMIN]
    ports: ["7783:7783"]
    volumes: ["pn-data:/data"]
  browser:
    image: zenika/alpine-chrome
    network_mode: service:potatonetwork
    depends_on: [potatonetwork]
```

Sidecars share the netns, so DNS is already `127.0.0.1` after PotatoNetwork rewrites resolv.conf. Trust CA from `/data/ca/potatonetwork-ca.pem` or `GET /v1/ca.pem`.

## Data volume (`/data`)

| Path | Purpose |
|------|---------|
| `ca/potatonetwork-ca.pem` | MITM root CA (auto-created) |
| `ca/potatonetwork-ca-key.pem` | CA private key |
| `rules.expr` | Path-delay policy (expr script) |
| `catalog.json` | Country profiles (Radar catalog) |
| `baseline.json` | Host RTT baseline + `probedAt` |

## ENV

| Variable | Default | Notes |
|----------|---------|--------|
| `POTATONETWORK_DATA` | `/data` | |
| `POTATONETWORK_API_ADDR` | `:7783` | Not shaped (7783 ≈ SPUD on a phone keypad) |
| `POTATONETWORK_API_TOKEN` / `_FILE` | empty | Empty = no auth |
| `POTATONETWORK_CATALOG_CRON` | empty | Empty/unset = Tuesday random UTC; `false` = disable; or `M H * * D` |
| `POTATONETWORK_BASELINE_CRON` | empty | Empty/unset = every 3h at random UTC minute; `false` = disable; or `M */N * * *` |
| `POTATONETWORK_UPLINK` | auto | Egress iface for netlink shaping |
| `POTATONETWORK_DOCS_URL` | GitHub Pages API docs | `GET /` on the API port redirects here |

DNS upstream comes from the container’s `/etc/resolv.conf` (Docker embedded `127.0.0.11`, or Compose `dns:`). After boot, resolv is rewritten to `127.0.0.1` so the netns uses PotatoNetwork’s shaped `:53`. Do not set `dns: [127.0.0.1]` on the potatonetwork service — that hides the real upstream.

Boot profile is always **passthrough** until you `PUT /v1/profile`.

## Docs

See [docs/](docs/) — also published via GitHub Pages (API reference, expr rules, profiles gallery, examples).

## License

See repository license.

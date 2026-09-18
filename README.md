# 🥔 PotatoInspector

Single Docker container that makes phones, laptops, and a router (as a WireGuard client) experience a chosen wide-area mobile network — with one web control panel.

Last-mile delay/loss/rate uses Linux `tc` netem on the WireGuard TUN. MITM (custom CA) adds per-request path delay and feeds an Inspector for HTTP, TLS, and DNS.

In-app docs (same content after you open the panel): **Docs** tab, or the markdown under [`docs/`](docs/).

## Docs

| Topic | Link |
|-------|------|
| Overview & limitations | [docs/overview.md](docs/overview.md) |
| WireGuard / lab setup | [docs/wireguard.md](docs/wireguard.md) |
| Profiles data & formulas | [docs/profiles.md](docs/profiles.md) |
| Root CA · Android | [docs/ca-android.md](docs/ca-android.md) |
| Root CA · iOS | [docs/ca-ios.md](docs/ca-ios.md) |
| Root CA · macOS | [docs/ca-macos.md](docs/ca-macos.md) |
| Root CA · Windows | [docs/ca-windows.md](docs/ca-windows.md) |

## Quick start

```bash
docker pull ghcr.io/kriakiku/potato-inspector:latest
docker run --cap-add=NET_ADMIN --device=/dev/net/tun \
  -p 51820:51820/udp -p 8443:8443 \
  -v potatoinspector-data:/data \
  ghcr.io/kriakiku/potato-inspector:latest
```

If `docker pull` asks for login, set the GHCR package to **Public**:  
https://github.com/kriakiku/potato-inspector/pkgs/container/potato-inspector

Or build locally / compose:

```bash
docker compose up -d --build
```

Open **http://\<host\>:8443** (plain HTTP — put TLS on a reverse proxy). No panel login.

Needs: `NET_ADMIN`, `/dev/net/tun`, UDP `51820`, TCP `8443`, a `/data` volume, and host `net.ipv4.ip_forward=1` (compose sets this via `sysctls`).

## Lab (short)

Phone → Potato SSID → router as WG client → PotatoInspector TUN (`tc` + MITM) → NAT → Internet.  
WG UDP handshake stays unshaped. Set **Public WG endpoint**, create a peer, apply `.conf` / QR. Details: [WireGuard docs](docs/wireguard.md).

On-tunnel: **http://potato.local** (CA install → Share after trust).

## Env

| Variable | Default |
|----------|---------|
| `POTATOINSPECTOR_DATA` | `/data` |
| `POTATOINSPECTOR_WG_SUBNET` | `10.8.0.0/24` |
| `POTATOINSPECTOR_WG_PORT` | `51820` |
| `POTATOINSPECTOR_PANEL` | `8443` |
| `POTATOINSPECTOR_UPLINK` | `eth0` |
| `POTATOINSPECTOR_WG_IFACE` | `wg0` |

Persisted under `/data`: settings, peers, catalog pull, MITM rules, CA. Inspector events are in-memory only.

## Panel map

Inspector · Profiles · Baseline · Ignore · MITM · Share · DNS · WireGuard · Docs — see [overview](docs/overview.md). Profiles math: [profiles](docs/profiles.md).

## Catalog refresh (CI)

Weekly / manual workflow **Radar profile catalog** runs `scripts/generate-radar-profiles.py` (needs secret `CLOUDFLARE_API_TOKEN` = Account → Radar → Read). Then **Profiles → Update catalog from GitHub** on the live panel.

## Dev

```bash
cd web && npm install && npm run build && cd ..
go run ./cmd/potatoinspector
```

Full stack needs Linux + TUN (Docker). macOS can serve the panel UI only.

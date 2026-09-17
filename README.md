# PotatoInspector

Single Docker container that makes phones, laptops, and a UniFi gateway (as a WireGuard client) experience a chosen wide-area mobile network — with one web control panel.

Last-mile delay/loss/rate uses Linux `tc` netem on the WireGuard TUN. Optional MITM (custom CA) adds per-request API delay by URL path and feeds an inspector for decrypted HTTP, TLS handshakes, and DNS — all in the same panel (not mitmweb).

## Quick start

Published image (GHCR):

```bash
docker pull ghcr.io/kriakiku/potato-inspector:latest
docker run --cap-add=NET_ADMIN --device=/dev/net/tun \
  -p 51820:51820/udp -p 8443:8443 \
  -v potatoinspector-data:/data \
  ghcr.io/kriakiku/potato-inspector:latest
```

If `docker pull` asks for login, open the package → **Package settings** → make visibility **Public**:
https://github.com/kriakiku/potato-inspector/pkgs/container/potato-inspector

Build locally:

```bash
docker build -t potatoinspector:latest .
docker run --cap-add=NET_ADMIN --device=/dev/net/tun \
  -p 51820:51820/udp -p 8443:8443 \
  -v potatoinspector-data:/data \
  potatoinspector:latest
```

Or:

```bash
docker compose up -d --build
```

Open **http://\<host\>:8443** (plain HTTP — terminate TLS on your reverse proxy). No panel login — protect via reverse proxy / network if needed.

### Required flags

| Flag | Why |
|------|-----|
| `--cap-add=NET_ADMIN` | TUN, qdisc, iptables NAT/TPROXY |
| `--device=/dev/net/tun` | userspace WireGuard (`wireguard-go`) |
| `-p 51820:51820/udp` | WireGuard |
| `-p 8443:8443` | Panel HTTP (TLS via reverse proxy) |
| `-v …:/data` | JSON settings, peers, CA |

Host should allow forwarding (`net.ipv4.ip_forward=1`). Compose sets this via `sysctls`.

Prefer explicit `-p` mappings. `network_mode: host` is not required for the default path.

## Lab topology

1. Phone joins a dedicated Wi-Fi SSID (e.g. **Potato**).
2. That SSID/VLAN is bound on a UniFi gateway.
3. UniFi (or the phone itself) is a WireGuard **client**: `AllowedIPs = 0.0.0.0/0`, endpoint = PotatoInspector host + UDP port.
4. Inner traffic is shaped on the TUN. **WireGuard UDP handshake/keepalive stays unshaped.**
5. Traffic is NAT’d to the real internet.

```
Phone → Potato SSID → UniFi WG client → UDP WG (unshaped)
  → wg TUN → tc netem → optional MITM → NAT → Internet
```

In the panel: set **Settings → Public WG endpoint**, create a **Peer**, download `.conf` or QR, apply on UniFi or the phone.

## Env

| Variable | Default |
|----------|---------|
| `POTATOINSPECTOR_DATA` | `/data` |
| `POTATOINSPECTOR_WG_SUBNET` | `10.8.0.0/24` |
| `POTATOINSPECTOR_WG_PORT` | `51820` |
| `POTATOINSPECTOR_PANEL` | `8443` (plain HTTP; put TLS on the reverse proxy) |
| `POTATOINSPECTOR_UPLINK` | `eth0` |
| `POTATOINSPECTOR_WG_IFACE` | `wg0` |

## Persistence (`/data`, no database)

Atomic JSON writes (temp + rename):

- `settings.json` — WG endpoint, subnet, uplink, active profile, MITM, extra delay, capture, DNS intercept
- `peers.json` — WireGuard peers
- `profiles/catalog.json` — pulled Radar+CloudPing country catalog (also embedded)
- `mitm-rules.json` — path/host regex, bypass SNI
- `ca/` — MITM CA cert + key

Inspector events (HTTP / TLS / DNS) stay in an in-memory ring (~2000 events); they are not written under `/data`.

## Last-mile profiles

Primary model: **country + speed tier** from the Radar catalog (`Profiles` page / Inspector status bar).

- **Last-mile** (download / upload / loss / CF RTT) comes from Cloudflare Radar (or seed when no API token).
- **Path RTT to AWS** ≈ `rtt_cf + CloudPing(nearest_aws(country), dest)` — CloudPing is AWS region↔region backbone, not eyeball→AWS.
- Destinations: `cf` (Cloudflare Edge) plus selected AWS regions (`aws-eu-central-1`, `aws-us-east-1`, …).
- Host baseline probes subtract your container’s RTT to CF / AWS endpoints when applying a profile.
- MITM path rules pick a dest; extra delay is the path delta vs CF (one-way), on top of last-mile netem.
- Refresh: Profiles → “Update catalog from GitHub”, or weekly `.github/workflows/radar-profiles.yml`.

Delay is **one-way** ms. RTT ≈ 2×. Bandwidth is **download** (internet→client) / **upload** (client→internet).

Legacy built-in shaping IDs remain for boot fallback only; the panel uses the country catalog.

### Catalog sources

- Cloudflare Radar Speed / entities (with `CLOUDFLARE_API_TOKEN` in CI)
- [CloudPing](https://www.cloudping.co/) region↔region P50 (`CLOUDPING_URL`)
- Nearest-AWS via country capital → AWS region coords (haversine); geo backbone fallback if CloudPing is down
- Generator: `scripts/generate-radar-profiles.py` → `profiles/radar/catalog.json` + embed

## MITM + inspector

When MITM is **on**:

- Last-mile netem still applies to every packet (including TLS handshake).
- Matching path rules add **extra** delay from dest path delta vs CF (plus optional global extra / override) inside the proxy after decrypt — not a second netem path (HTTP/2 one connection).
- Inspector shows HTTP exchanges and TLS handshake rows (SNI, version, ALPN, leaf CN/SAN).
- UDP/443 (QUIC) is **dropped** so clients fall back to TCP/TLS through MITM.
- Download the CA from the MITM page; install and trust on the phone/laptop. UniFi WG client does **not** install the CA.

When MITM is **off**: HTTPS works via NAT + netem; HTTP/TLS inspector is idle. DNS logging still works if port 53 intercept stays on (Settings).

DoH appears as HTTPS `/dns-query` when those hosts are MITM’d.

**Cert pinning** fails unless the SNI is on the bypass list.

## Limitation blurb

Last-mile shaping hits every packet the same, including TLS handshake. After the container we egress from our region, so a real Cloudflare origin-fetch is short. Country profiles model “the client itself is far”: API TTFB is closer to truth when MITM dest rules add path delay to AWS, but handshake still includes last-mile delay. Static vs API on one hostname cannot be split at packet level. Enable MITM + CA + path rules for dest-based API delay and HTTP inspect. Pinning/QUIC caveats apply.

## Panel

- **Dashboard** — country/tier shape, MITM, capture, peers, qdisc
- **Peers** — add, QR, `.conf`, revoke
- **Profiles** — country catalog (Radar + CloudPing), favorites, apply tiers, pull refresh
- **Baseline** — host RTT to CF/AWS: test, save; persists in settings
- **MITM** — enable, path rules (dest = cf / aws-…), extra delay, bypass, CA download
- **Inspector** — DevTools-style network + country/speed status bar
- **Settings** — public WG endpoint, DNS intercept, capture

No panel auth (use reverse proxy / network ACL). Privacy: MITM decrypts HTTPS on this tunnel; DNS names appear in the in-memory inspector ring while capture is on.

## What this is not

- Not FlowEmu, Toxiproxy-as-last-mile, wg-easy, or mitmweb-as-UI
- Not sqlite / multi-container compose
- Not NixOS / Colmena / sops / Traefik integration

## Dev (host)

```bash
cd web && bun install && bun run build && cd ..
go run ./cmd/potatoinspector
# On macOS, WG/TUN/NAT will warn and skip; panel still serves for UI work.
# Full stack needs Linux + /dev/net/tun (Docker).
```

## Acceptance checklist

- [ ] One `docker run` container; UniFi or phone full-tunnel with passthrough
- [ ] Apply catalog country+tier: last-mile shape; host CF baseline subtracted
- [ ] MITM dest `aws-eu-central-1` vs `cf`: API path gets path-delta extra delay
- [ ] Harsh / poor tier: WG UDP handshake still works
- [ ] Restart: peers/settings restored from `/data` JSON
- [ ] MITM off: HTTPS works; no HTTP/TLS bodies claimed; DNS if 53 intercept on
- [ ] MITM on + CA: dest path rules add TTFB; HTTP + TLS in inspector
- [ ] DNS queries appear; UDP/443 does not skip MITM
- [ ] Profiles → Update catalog from GitHub pulls fresh Radar+CloudPing JSON

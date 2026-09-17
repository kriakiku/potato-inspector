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
  -e POTATOINSPECTOR_PASSWORD=potato \
  ghcr.io/kriakiku/potato-inspector:latest
```

Build locally:

```bash
docker build -t potatoinspector:latest .
docker run --cap-add=NET_ADMIN --device=/dev/net/tun \
  -p 51820:51820/udp -p 8443:8443 \
  -v potatoinspector-data:/data \
  -e POTATOINSPECTOR_PASSWORD=potato \
  potatoinspector:latest
```

Or:

```bash
docker compose up -d --build
```

Open **https://\<host\>:8443** (self-signed panel cert). Default password: `potato` (or `POTATOINSPECTOR_PASSWORD`).

### Required flags

| Flag | Why |
|------|-----|
| `--cap-add=NET_ADMIN` | TUN, qdisc, iptables NAT/TPROXY |
| `--device=/dev/net/tun` | userspace WireGuard (`wireguard-go`) |
| `-p 51820:51820/udp` | WireGuard |
| `-p 8443:8443` | Panel HTTPS |
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
| `POTATOINSPECTOR_PANEL` | `8443` |
| `POTATOINSPECTOR_PASSWORD` | `potato` (first boot only) |
| `POTATOINSPECTOR_UPLINK` | `eth0` |
| `POTATOINSPECTOR_WG_IFACE` | `wg0` |

## Persistence (`/data`, no database)

Atomic JSON writes (temp + rename):

- `settings.json` — password hash, WG endpoint, subnet, uplink, active profile, MITM, extra delay, capture, DNS intercept
- `peers.json` — WireGuard peers
- `profiles/custom.json` — user last-mile profiles
- `mitm-rules.json` — path/host regex, bypass SNI
- `ca/` — MITM CA cert + key

Inspector events (HTTP / TLS / DNS) stay in an in-memory ring (~2000 events); they are not written under `/data`.

## Last-mile profiles

Delay is **one-way** ms. RTT ≈ 2× (UI labels this: e.g. `90 ms one-way ≈ 180 ms ping`). Bandwidth is **download** (internet→client) / **upload** (client→internet).

Built-ins (stable IDs):

| ID | Delay / Down / Up / Loss |
|----|--------------------------|
| `passthrough` | no qdisc |
| `bangladesh-dhaka-4g-europe-stable` | 80 / 40 / 16 / 0.1 |
| `bangladesh-dhaka-4g-europe` | 90 / 32 / 12 / 0.5 (default demo) |
| `bangladesh-khulna-4g-cf-dac-stable` | 25 / 40 / 16 / 0.1 |
| `bangladesh-khulna-4g-cf-dac` | 30 / 32 / 12 / 0.5 |
| `bangladesh-mymensingh-teletalk-weak` | 110 / 2.2 / 1.0 / 2.0 |

Do not “improve” these numbers without saving as a custom profile.

### Sources

- Ookla 4G BD median ~32 Mbps down / ~12 up; Teletalk tail: https://www.ookla.com/articles/bangladesh-4g-qos-q42025
- Dhaka mobile idle ~17 ms is local Speedtest, not Europe: https://www.speedtest.net/global-index/bangladesh?city=Dhaka
- London ↔ BD ~152–159 ms: https://wondernetwork.com/pings/Dhaka/London — 90 ms one-way ≈ 180 ms RTT
- CF PoPs (DAC, CGP): https://www.cloudflare.com/network/
- National mobile ping ~35–40 ms: https://www.speedgeo.net/statistics/bangladesh

## MITM + inspector

When MITM is **on**:

- Last-mile netem still applies to every packet (including TLS handshake).
- Matching API paths get **extra** delay (default 180 ms) inside the proxy after decrypt — not a second netem path (HTTP/2 one connection).
- Inspector shows HTTP exchanges and TLS handshake rows (SNI, version, ALPN, leaf CN/SAN).
- UDP/443 (QUIC) is **dropped** so clients fall back to TCP/TLS through MITM.
- Download the CA from the MITM page; install and trust on the phone/laptop. UniFi WG client does **not** install the CA.

When MITM is **off**: HTTPS works via NAT + netem; HTTP/TLS inspector is idle. DNS logging still works if port 53 intercept stays on (Settings).

DoH appears as HTTPS `/dns-query` when those hosts are MITM’d.

**Cert pinning** fails unless the SNI is on the bypass list.

## Limitation blurb

Last-mile shaping hits every packet the same, including TLS handshake. After the container we egress from our region, so a real Cloudflare origin-fetch is short. The “Europe” profile is “the client itself is far from Europe”: API TTFB is closer to truth, but handshake is inflated too. Static vs API on one CF hostname cannot be split at packet level. Enable MITM + CA + path rules for extra API delay and HTTP inspect. Pinning/QUIC caveats apply.

## Panel

- **Dashboard** — profile, MITM, capture, peers, qdisc
- **Peers** — add, QR, `.conf`, revoke
- **Profiles** — built-ins + custom; Apply; Passthrough
- **MITM** — enable, extra delay, rules, bypass, CA download
- **Inspector** — HTTP / TLS / DNS timeline; filter; clear; optional HAR
- **Settings** — public WG endpoint, DNS intercept, password

Auth: panel password (not world-open). Privacy: MITM decrypts HTTPS on this tunnel; DNS names appear in the in-memory inspector ring while capture is on.

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
- [ ] `bangladesh-dhaka-4g-europe`: ping ~180 ms order; download near ~32 Mbps
- [ ] Harsh profile: WG UDP handshake still works
- [ ] Restart: peers/settings restored from `/data` JSON
- [ ] MITM off: HTTPS works; no HTTP/TLS bodies claimed; DNS if 53 intercept on
- [ ] MITM on + CA: `/api/` extra TTFB; static not extra-delayed; HTTP + TLS in inspector
- [ ] DNS queries appear; UDP/443 does not skip MITM

# Overview

PotatoInspector is a single Docker container that makes phones, laptops, and a UniFi (or similar) gateway — connected as a WireGuard client — experience a chosen wide-area mobile network. One web panel controls shaping, MITM path delay, DNS, and an in-memory Inspector.

## What you can test

- **Last-mile shape** — delay, loss, and rate via Linux `tc` netem on the WireGuard TUN, driven by country/speed profiles.
- **Path-aware API delay** — MITM rules add extra delay by URL path and destination (e.g. Cloudflare vs AWS regions) after decrypt.
- **Inspector** — decrypted HTTP (headers/body), TLS handshakes, and DNS queries in one log (Pause stops recording).
- **DNS** — always-on intercept on the tunnel: rewrite rules, TTL clamp, upstream forwarder.
- **Ignore** — skip MITM / delay for system and custom hosts (pinned apps, vendor domains).

## Recommended lab shape

1. Dedicated Wi-Fi SSID (e.g. **Potato**) on a VLAN.
2. Router runs WireGuard as a **client** (`AllowedIPs = 0.0.0.0/0`) to PotatoInspector.
3. Devices on that SSID get the shaped path without installing WG on each phone.
4. For HTTPS inspect, install and trust the PotatoInspector **root CA** on each client that should be decrypted (the router does not install the CA for you).

## Limitations

- **Shared tunnel** — all peers share one shaped TUN. Traffic from different devices is **not** separated in the Inspector. For a readable Inspect session, use **one active client device at a time** (or pause capture when others are busy).
- **Packet-level shaping is uniform** — every packet on the TUN gets the same netem (including TLS handshake). You cannot split “static vs API” for one hostname at the packet layer; use MITM path rules for dest-based API delay.
- **QUIC** — UDP/443 is dropped so clients fall back to TCP/TLS through MITM.
- **Certificate pinning** — apps that pin will fail unless the host is on the **Ignore** list.
- **Egress region** — after the container you leave from *your* region; country profiles model “the client is far,” not a full remote PoP for every origin hop.

## Next steps

1. [WireGuard setup](/docs/wireguard) — prefer the router; end-device clients are fine too.
2. On a device already on the tunnel, open **http://potato.local** — it detects your OS, shows install steps, and lets you download the CA (no panel required). After the CA is trusted, the same page switches to the live **Share** notepad (`https://potato-share.local`). Platform guides: [Android](/docs/ca-android) · [iOS](/docs/ca-ios) · [macOS](/docs/ca-macos) · [Windows](/docs/ca-windows).
3. Read [Profiles catalog](/docs/profiles) — Radar / CloudPing sources and how delay is calculated — then apply a country+tier in the panel and generate traffic from a single device.

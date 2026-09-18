---
title: Overview
weight: 10
---

**🥔 PotatoNetwork** makes Docker sidecars experience a chosen country’s last-mile network (delay, loss, bandwidth) plus optional HTTPS path delay (e.g. “as if API is farther than CF edge”).

```text
sidecar ──► PotatoNetwork netns
              ├─ DNS :53 (shaped) → Docker/resolv upstream (exempt)
              ├─ netem on uplink via netlink (shaped)
              ├─ transparent MITM 80/443 (nftables redirect) → expr path delay
              └─ API :7783 (exempt — no lab delay)
```

- **No WireGuard** inside PotatoNetwork — for phones, run a separate WG container with `network_mode: service:potatonetwork`.
- **No web UI** — JSON API only.
- **Go only** — no Python / mitmproxy.

## Layers

1. **Last-mile (netlink netem)** — one-way delay ≈ `(country_cf_rtt − host_cf_rtt) / 2`, plus loss and rates from the catalog tier.
2. **Path delay (`rules.expr`)** — after origin response headers, sleep `delay_ms` or PathExtra for `dest` (Frankfurt vs CF, Via/CloudFront, etc.).
3. **TLS handshake delay** — extra sleep before local MITM ServerHello so client-visible SSL time is not ~0 under MITM.

API traffic and DNS **upstream** queries are fwmark-exempt from netem (nftables mark + netlink fw filter). Redirect uses nftables NAT.

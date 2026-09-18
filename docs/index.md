# 🥔 PotatoNetwork

**Last-mile network lab for Docker** — make browsers, scrapers, and APIs feel like they run from Bangladesh, Brazil, or Japan without VPNs, agents, or a heavy proxy stack.

Attach workloads with `network_mode: service:potatonetwork`, pick a country profile over a tiny JSON API, and every packet in that netns — including DNS — gets the delay, loss, and bandwidth of real last-mile. Transparent HTTPS MITM adds path delay on top (edge vs origin), so Waterfall timings look believable under interception.

[Overview](overview.md) · [API](api.md) · [Rules](rules.md) · [CA](ca.md) · [Examples](examples.md) · [Profiles](profiles-gallery.md)

---

## What it does

| Capability | Why it matters |
|------------|----------------|
| **Country last-mile** | Delay, loss, down/up rates from a Radar-backed catalog (stable / typical / poor tiers) |
| **Shared Docker netns** | Sidecars inherit shaping automatically — no SOCKS config, no per-app agents |
| **Shaped DNS** | Local `:53` forwarder so lookups suffer the same last-mile as TCP |
| **Transparent MITM** | nftables REDIRECT of 80/443 → in-process TLS terminator; path delay via [`rules.expr`](rules.md) |
| **Host baseline** | Persisted RTT probe so delay = country − *your* edge, not absolute fiction |
| **API-only control** | `PUT /v1/profile`, CA download, catalog refresh — CI-friendly, no panel |

Typical loop: probe baseline → set `{"country":"BD","tier":"typical"}` → run Playwright / curl / your service in the same network namespace.

Phones and non-Docker clients: run any WireGuard (or other VPN) container with `network_mode: service:potatonetwork` — PotatoNetwork stays the shaping core, not a VPN product.

---

## Stack (deliberately thin)

- **One static Go binary** — no Python, no mitmproxy, no Node UI
- **Dataplane in-kernel** — [netlink](https://github.com/vishvananda/netlink) HTB/netem/IFB for shaping; [google/nftables](https://github.com/google/nftables) for marks, REDIRECT, QUIC drop
- **DNS** — [miekg/dns](https://github.com/miekg/dns) forwarder; upstream from Docker’s resolv / Compose `dns:`
- **Policy** — [expr-lang](https://github.com/expr-lang/expr) scripts for path delay (dest, Via, CloudFront, …)
- **Trust** — embedded Mozilla roots ([breml/rootcerts](https://github.com/breml/rootcerts)); auto-minted MITM CA on the data volume

Control plane is a small HTTP API on `:7783` (SPUD on a phone keypad), fwmark-exempt so labs don’t shape their own remote.

---

## Lightweight on purpose

Runtime image is **`scratch` + UPX-compressed binary**:

- Catalog and CA roots are **embedded** — no `apt`, no `ca-certificates`, no `tc`/`iptables` CLI helpers
- No web UI assets, no sidecar proxy process, no WireGuard daemon inside the image
- Needs only **`NET_ADMIN`** (+ `ifb` on the host kernel) and a data volume

What you *don’t* ship: a GUI, a VPN mesh, or a multi-service compose of “emulator + mitm + DNS + panel”. One container owns the netns; everything else plugs in.

---

## Who it’s for

- **QA / perf** — “does checkout still work on a poor mobile last-mile?”
- **Scraping / automation** — see timeouts and CDN behavior as a distant user would
- **CDN / edge debugging** — path extra latency when the origin sits farther than CF
- **CI** — token-gated API, cron-refreshable catalog and baseline, docs on GitHub Pages

Start with [Overview](overview.md) for the shaping model, or [Examples](examples.md) for a Playwright sidecar.

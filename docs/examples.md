# Examples

## Puppeteer / Playwright sidecar

```yaml
services:
  potatonetwork:
    build: .
    cap_add: [NET_ADMIN]
    ports: ["7783:7783"]
    volumes:
      - ./data:/data
  scraper:
    image: mcr.microsoft.com/playwright:v1.40.0-jammy
    network_mode: service:potatonetwork
    depends_on: [potatonetwork]
    environment:
      NODE_EXTRA_CA_CERTS: /data/ca/potatonetwork-ca.pem
    volumes:
      - ./data:/data:ro
```

1. `POST /v1/baseline/probe`
2. `PUT /v1/profile` with country/tier
3. Run browser tests; install/trust CA via `NODE_EXTRA_CA_CERTS` or OS store
4. DNS: PotatoNetwork rewrites `/etc/resolv.conf` to `127.0.0.1` so the shared netns uses shaped `:53`. To pick a custom upstream, set Compose `dns:` on the **potatonetwork** service (e.g. `1.1.1.1`) — not `127.0.0.1`.

## WireGuard for phones (external)

PotatoNetwork does **not** include WG. Run any WG server/container with:

```yaml
  wireguard:
    image: linuxserver/wireguard
    network_mode: service:potatonetwork
    depends_on: [potatonetwork]
    cap_add: [NET_ADMIN]
    # … peer configs …
```

Phone → WG → same netns as PotatoNetwork → shaped + MITM. Publish WG UDP on the PotatoNetwork service ports section.

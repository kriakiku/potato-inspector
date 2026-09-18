# WireGuard

PotatoInspector terminates WireGuard in the container. Clients (router or device) use a peer config from the **WireGuard** panel (download `.conf` or scan QR).

## Prefer the router

Best lab setup:

1. Create a dedicated SSID/VLAN for testing (e.g. **Potato**).
2. On the gateway (UniFi and similar), add a **WireGuard client** (or site VPN) using the peer config from the panel.
3. Set `AllowedIPs = 0.0.0.0/0` (and `::/0` if present) so that SSID’s traffic is full-tunnelled.
4. Point the SSID/VLAN at that WG client.

Phones and laptops then only join Wi‑Fi — no per-device VPN app. Shaping and DNS intercept apply to everything on that path.

**Note:** A router WG client does **not** install the MITM CA. Install the CA on each device whose HTTPS you want to decrypt ([CA guides](/docs/ca-android)).

## End-device clients

You can run WireGuard directly on a phone or laptop instead:

1. Install an official client from [wireguard.com/install](https://www.wireguard.com/install/).
2. In the panel: **WireGuard** → add a peer → **Conf** or **QR**.
3. Import the config; ensure full tunnel (`AllowedIPs = 0.0.0.0/0`).
4. Set the panel’s **Public WG endpoint** to a host:port reachable from the client.

Use one active tunnel at a time when inspecting traffic (see [Overview](/docs) limitations).

## Panel checklist

1. Set **Public WG endpoint** (e.g. `203.0.113.10:51820`).
2. Add a peer; download `.conf` / QR.
3. Apply on the router or device.
4. Confirm handshake on the WireGuard page.

Then on a phone/laptop on that path, open **http://potato.local** to install the MITM CA (OS auto-detected).

DNS in the peer config uses the WG gateway (tunnel DNS intercept is always on). Upstream DNS for unresolved names is configured on the **DNS** page.

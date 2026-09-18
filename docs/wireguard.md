# WireGuard

PotatoInspector terminates WireGuard in the container. Clients (router or device) use a peer config from the **WireGuard** panel (download `.conf` or scan QR).

## Prefer the router

Best lab setup:

1. Create a dedicated SSID/VLAN for testing (e.g. **Potato**).
2. On the router/gateway, add a **WireGuard client** (or site VPN) using the peer config from the panel.
3. Set `AllowedIPs = 0.0.0.0/0` so that SSID’s traffic is full-tunnelled (IPv4 only — PotatoInspector has no IPv6 MITM/NAT).
4. Enable **SNAT/MASQUERADE** for LAN → WireGuard so Potato only sees the peer address (`10.8.0.x`). Without this, DNS from the router may work while TCP from phone LAN IPs fails.
5. Point the SSID/VLAN at that WG client.

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

Then on a phone/laptop on that path, open **http://potato.local** to install the MITM CA (OS auto-detected). After trust, that page becomes the Share notepad; you can also hit **https://potato-share.local** directly for the API.

DNS in the peer config uses the WG gateway (tunnel DNS intercept is always on). Upstream DNS for unresolved names is configured on the **DNS** page.

## Troubleshooting: DNS works, pages fail (`ERR_CONNECTION_ABORTED`)

Handshake + DNS in the Inspector only prove the tunnel and local DNS intercept. HTTPS still needs a live MITM process **and** working container egress (NAT).

After loading a site on the client, check the Inspector:

| What you see | Likely cause |
|--------------|--------------|
| DNS only (no TLS/HTTP), `potato.local` OK | Often **IPv6**: clients preferred AAAA while MITM/NAT is IPv4-only. Current builds answer AAAA with empty NOERROR (`rewrite:ipv4-only`) and peer configs use `AllowedIPs = 0.0.0.0/0` only — re-import the peer `.conf` / QR after upgrade |
| DNS only (no TLS/HTTP) | mitmdump down while TPROXY still redirects `:80`/`:443` to `:8080`, or TCP never reaches Potato |
| TLS rows, little/no HTTP | MITM accepted the client, then failed dialing the origin — almost always **NAT uplink** |
| Cert / SSL errors | CA not trusted on the device |

Container checks:

1. Startup log: `uplink: "eth0" (env|settings|default-route)` — must be a real iface inside the container (not a stale host name like `eno1`).
2. Startup log: `NAT: MASQUERADE -s 10.8.0.0/24 -o <iface> …`. If you see `NAT uplink failed — clients will see connection aborted`, fix `POTATOINSPECTOR_UPLINK` / redeploy.
3. Inside the container: `iptables -t nat -S POSTROUTING` should list the MASQUERADE rule; `ss -lntp | grep 8080` should show mitmdump when MITM is on.

`http://potato.local` can still open when internet MITM is broken: the portal is excluded from TPROXY and does not need egress.

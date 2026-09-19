---
title: CA
weight: 40
---

PotatoNetwork terminates HTTPS inside the shared netns (transparent MITM). To do that it mints leaf certificates for each host, signed by a **local Root CA** that lives on the data volume. Clients must trust that CA — otherwise TLS validation fails and HTTPS does not work through the emulator.

## Why you must install it

Without MITM, your browser talks TLS straight to `example.com` and checks a chain ending in a public CA (DigiCert, Let’s Encrypt, …).

With PotatoNetwork, nftables redirects `:443` into the process. The client sees a certificate for `example.com` that was **signed by PotatoNetwork Root CA**, not by a public CA. The handshake only succeeds if that root is in the client’s trust store.

| If the CA is trusted | If it is not |
|----------------------|--------------|
| HTTPS works; MITM can apply path delay from [`rules.expr`](rules) | Certificate errors, connection refused, or empty pages |
| Waterfall / TTFB timings include path delay | Apps abort before any useful response |
| You can inspect shaped traffic under real TLS | Only plain HTTP (`:80`) may still work |

So the CA is not optional “extra security theater” — it is the **trust anchor for the fake but necessary MITM certificates**. Without it, transparent HTTPS interception cannot be used.

PotatoNetwork still uses real upstream TLS to origins (with embedded Mozilla roots). The local CA is only between **your client and PotatoNetwork**.

## Files and download

Created on first start if missing:

| Path | Role |
|------|------|
| `/data/ca/potatonetwork-ca.pem` | Root CA certificate (install / mount this) |
| `/data/ca/potatonetwork-ca-key.pem` | Private key (**keep private**; never distribute) |

Download over the API (from the host or any client that can reach the API port):

```bash
curl -fsSL -o potatonetwork-ca.pem http://127.0.0.1:7783/v1/ca.pem
```

`GET /v1/ca.pem` returns the PEM with `Content-Disposition: attachment`.

## Docker / CI (no OS install)

Sidecars that share `network_mode: service:potatonetwork` can trust the CA via env or a mounted file — often enough for Node, Python, and curl:

```yaml
environment:
  NODE_EXTRA_CA_CERTS: /data/ca/potatonetwork-ca.pem
  # or: SSL_CERT_FILE / REQUESTS_CA_BUNDLE pointing at the same PEM
volumes:
  - ./data:/data:ro
```

Browsers and system HTTP stacks usually ignore those variables — use the OS steps below (or a browser profile that imports the CA).

## Install on desktop and phones

Use the PEM from `/v1/ca.pem`. Some UIs prefer `.crt`; renaming `potatonetwork-ca.pem` → `potatonetwork-ca.crt` is fine (same contents).

### macOS

1. Download `potatonetwork-ca.pem`.
2. Open **Keychain Access** → select **System** (machine-wide) or **login** (this user).
3. **File → Import Items…** and choose the PEM.
4. Double-click **PotatoNetwork Root CA** → expand **Trust** → set **When using this certificate** to **Always Trust** (or at least trust for SSL).
5. Close and enter your password if prompted.

CLI (system roots):

```bash
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain potatonetwork-ca.pem
```

Restart the browser if it still warns.

### Windows

1. Download `potatonetwork-ca.pem` (rename to `.crt` if the wizard rejects `.pem`).
2. Double-click the file → **Install Certificate…**
3. Store location: **Local Machine** (needs admin) or **Current User**.
4. Choose **Place all certificates in the following store** → **Trusted Root Certification Authorities** → Finish.

CLI (admin PowerShell / cmd):

```bat
certutil -addstore -f ROOT potatonetwork-ca.pem
```

### iOS / iPadOS

1. On the device, open the CA URL in **Safari** (AirDrop / Files also work), e.g. `http://<host>:7783/v1/ca.pem`.
2. Allow the download → **Settings → Profile Downloaded** (or **General → VPN & Device Management**) → **Install** the PotatoNetwork profile. Enter the passcode.
3. **Required second step:** **Settings → General → About → Certificate Trust Settings** → enable full trust for **PotatoNetwork Root CA**.

Without step 3, Safari and apps still treat the cert as untrusted. MDM / supervised devices can push the root as a trusted payload instead.

### Android

1. Download the PEM/CRT to the device (browser, Drive, `adb push`, …).
2. **Settings → Security** (wording varies) → **Encryption & credentials** / **Install a certificate** → **CA certificate** → select the file → confirm the warning.

Notes:

- **Chrome / system WebView** generally honor user-installed CAs.
- From Android 7 onward, **most apps ignore user CAs** unless they use a custom `network_security_config` that trusts user certificates, or the CA is installed in the **system** store (requires root / custom ROM / debug build).
- For phones that only need a browser through WireGuard into PotatoNetwork, a user CA is usually enough. For native apps under test, prefer a debug build that trusts user CAs, or inject the CA into the system store on a lab device.

## Security notes

- Anyone who trusts this CA will accept MITM certs for **any** hostname signed by it — treat the CA like a lab secret, not a public root.
- Do not commit `potatonetwork-ca-key.pem`. Rotating: delete the CA files under `/data/ca/`, restart PotatoNetwork (it regenerates), then re-install the new PEM on every client.
- Certificate pinning (apps that pin public CA SPKIs) will still fail even after you install this root — disable pinning in test builds or exclude those hosts from interception if you control the datapath.

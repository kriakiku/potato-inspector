# Install root CA · Android

The MITM root certificate lets Android trust HTTPS intercepted by PotatoInspector.

**Preferred:** on a device already on the WireGuard tunnel, open **[http://potato.local](http://potato.local)** — Android steps and a one-tap CA download. After the CA is trusted, that page becomes the Share notepad.

You can also download from this panel: **[ca.crt](/api/mitm/ca.crt)** (MITM page).

OEM menus differ slightly; steps below cover stock-style Android / Pixel and common variants.

## Steps

1. On the phone (while on the tunnel), open **[http://potato.local](http://potato.local)** and tap **Download CA certificate**, or use **[ca.crt](/api/mitm/ca.crt)** from the panel.
2. Open the downloaded `.crt` / `.cer` file, or go to **Settings → Security → Encryption & credentials → Install a certificate → CA certificate** (wording varies: *Trusted credentials*, *Install from storage*).
3. Confirm the warning that a CA can monitor traffic — that is expected for this lab.
4. Name the certificate (e.g. `PotatoInspector`) and finish install. It should appear under **User** trusted credentials (not System).
5. Load an HTTPS site. In **Inspector** you should see decrypted HTTP when MITM is up.

## Important

- Many apps (banking, some Google services) **ignore user CAs**. Put those hosts on the **Ignore** list, or accept that they will fail / stay opaque.
- After **Regenerate CA** on the MITM page, remove the old user CA and install the new file — the old root stops working.
- Work profile / corporate MDM may block user CA install.

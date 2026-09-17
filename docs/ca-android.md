# Install root CA · Android

The MITM root certificate lets Android trust HTTPS intercepted by PotatoInspector. Download it from this panel: **[ca.crt](/api/mitm/ca.crt)** (also on the **MITM** page).

OEM menus differ slightly; steps below cover stock-style Android / Pixel and common variants.

## Steps

1. On the phone, open **[Download CA cert](/api/mitm/ca.crt)** in Chrome (or copy the file from another device).
2. Open the downloaded `.crt` / `.cer` file, or go to **Settings → Security → Encryption & credentials → Install a certificate → CA certificate** (wording varies: *Trusted credentials*, *Install from storage*).
3. Confirm the warning that a CA can monitor traffic — that is expected for this lab.
4. Name the certificate (e.g. `PotatoInspector`) and finish install. It should appear under **User** trusted credentials (not System).
5. Connect via WireGuard (router SSID or on-device client) and load an HTTPS site. In **Inspector** you should see decrypted HTTP when MITM is up.

## Important

- Many apps (banking, some Google services) **ignore user CAs**. Put those hosts on the **Ignore** list, or accept that they will fail / stay opaque.
- After **Regenerate CA** on the MITM page, remove the old user CA and install the new file — the old root stops working.
- Work profile / corporate MDM may block user CA install.

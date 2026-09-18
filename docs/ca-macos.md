# Install root CA · macOS

**Preferred:** while connected to the WireGuard tunnel, open **[http://potato.local](http://potato.local)** in a browser — macOS steps and CA download. After Always Trust, the same page switches to Share.

You can also download from this panel: **[ca.crt](/api/mitm/ca.crt)** (MITM page).

## Steps

1. Download via **[http://potato.local](http://potato.local)** or **[ca.crt](/api/mitm/ca.crt)**.
2. Open the file — **Keychain Access** should prompt to add it. Choose the **login** or **System** keychain (System needs an admin password and applies to all users).
3. Find the certificate (search `Potato` or the CA common name).
4. Double-click it → expand **Trust**.
5. Set **When using this certificate** to **Always Trust**.
6. Close the window and authenticate if asked. The cert should show a blue “trusted” indicator.

Alternatively: drag `ca.crt` into Keychain Access, then set Always Trust as above.

## Important

- Browsers using the system trust store (Safari, Chrome) pick this up after Always Trust. Restart the browser if a site still fails.
- After **Regenerate CA**, delete the old certificate from Keychain and install the new one with Always Trust again.
- For WireGuard on Mac, see [WireGuard](/docs/wireguard) and [wireguard.com/install](https://www.wireguard.com/install/).

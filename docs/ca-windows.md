# Install root CA · Windows

**Preferred:** while connected to the WireGuard tunnel, open **[http://potato.local](http://potato.local)** — Windows steps and CA download.

You can also download from this panel: **[ca.crt](/api/mitm/ca.crt)** (MITM page).

## Steps (Certificate Manager)

1. Download via **[http://potato.local](http://potato.local)** or **[ca.crt](/api/mitm/ca.crt)** and save it (e.g. Downloads).
2. Press Win+R, run `certmgr.msc` (current user) or `certlm.msc` (local machine — needs admin).
3. Expand **Trusted Root Certification Authorities** → right-click **Certificates** → **All Tasks → Import…**.
4. **Next** → **Browse…** → select the `.crt` (change filter to *All Files* if needed) → **Next**.
5. Store location should be **Trusted Root Certification Authorities** → **Next** → **Finish**.
6. Confirm the security warning → **Yes**.

## Steps (double-click)

1. Open the downloaded `ca.crt`.
2. **Install Certificate…** → **Current User** or **Local Machine**.
3. Choose **Place all certificates in the following store** → **Browse…** → **Trusted Root Certification Authorities**.
4. Finish and accept the warning.

## Important

- Restart the browser after install if HTTPS still fails.
- Some apps use their own trust stores; pinning still requires **Ignore** for those hosts.
- After **Regenerate CA**, remove the old root from Trusted Root Certification Authorities and import the new file.
- WireGuard for Windows: [wireguard.com/install](https://www.wireguard.com/install/).

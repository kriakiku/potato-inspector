# Install root CA · iOS / iPadOS

**Preferred:** on the iPhone/iPad already on the WireGuard tunnel, open Safari to **[http://potato.local](http://potato.local)** — iOS steps and CA download. After Full Trust, reopen potato.local for the Share notepad.

You can also download from this panel: **[ca.crt](/api/mitm/ca.crt)** (MITM page). On iOS you must both **install** the profile and enable **full trust**.

## Steps

1. On the iPhone/iPad (on the tunnel), open Safari → **[http://potato.local](http://potato.local)** → **Download CA certificate** (or use **[ca.crt](/api/mitm/ca.crt)** from the panel).
2. When prompted, allow the configuration profile download. Tap **Close**.
3. Open **Settings → General → VPN & Device Management** (older iOS: *Profiles & Device Management*).
4. Under **Downloaded Profile**, tap the PotatoInspector / CA profile → **Install** → enter passcode → **Install** again → **Done**.
5. Enable trust: **Settings → General → About → Certificate Trust Settings**.
6. Under **Enable Full Trust for Root Certificates**, turn **on** the PotatoInspector CA toggle. Confirm the warning.

Without step 5–6, Safari and apps will still reject the MITM certificates.

## Important

- Certificate pinning still fails for some apps unless the host is on **Ignore**.
- After **Regenerate CA**, delete the old profile under VPN & Device Management, then install and re-enable full trust for the new CA.
- Use one device at a time for a clean Inspector session ([Overview](/docs)).

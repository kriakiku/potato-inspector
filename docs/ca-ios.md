# Install root CA · iOS / iPadOS

Download the MITM root from this panel: **[ca.crt](/api/mitm/ca.crt)** (also on the **MITM** page). On iOS you must both **install** the profile and enable **full trust**.

## Steps

1. On the iPhone/iPad, open Safari and download **[ca.crt](/api/mitm/ca.crt)**.
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

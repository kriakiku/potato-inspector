#!/usr/bin/env python3
"""Generate docs/profiles-gallery.md from catalog.json."""
from __future__ import annotations

import json
import pathlib
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
CATALOG = ROOT / "internal" / "catalog" / "data" / "catalog.json"
OUT = ROOT / "docs" / "profiles-gallery.md"


def main() -> int:
    cat = json.loads(CATALOG.read_text())
    lines = [
        "# Profiles gallery",
        "",
        f"Generated from catalog `source={cat.get('source')}` at `{cat.get('generatedAt')}`.",
        "",
        "RTT values are milliseconds to destinations (`cf` = Cloudflare edge). "
        "Last-mile one-way delay at runtime ≈ `(rtt_cf − host_cf) / 2`.",
        "",
    ]
    for c in sorted(cat.get("countries") or [], key=lambda x: x.get("name") or ""):
        flag = c.get("flag") or ""
        name = c.get("name") or c.get("id")
        cid = c.get("id")
        lines.append(f"## {flag} {name} (`{cid}`)")
        lines.append("")
        lines.append("| Tier | ↓ Mbps | ↑ Mbps | Loss % | CF RTT | Nearest AWS RTT |")
        lines.append("|------|--------|--------|--------|--------|-----------------|")
        tiers = c.get("tiers") or {}
        for tier in ("stable", "typical", "poor"):
            t = tiers.get(tier) or {}
            rtt = t.get("rttToDest") or {}
            aws = c.get("nearestAws") or ""
            aws_rtt = rtt.get(aws, "—")
            lines.append(
                f"| {tier} | {t.get('downloadMbps', '—')} | {t.get('uploadMbps', '—')} | "
                f"{t.get('lossPercent', '—')} | {rtt.get('cf', '—')} | {aws_rtt} |"
            )
        lines.append("")
    OUT.write_text("\n".join(lines) + "\n")
    print(f"wrote {OUT}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())

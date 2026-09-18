#!/usr/bin/env python3
"""Generate docs/profiles-gallery.md from catalog.json."""
from __future__ import annotations

import json
import pathlib
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
CATALOG = ROOT / "internal" / "catalog" / "data" / "catalog.json"
OUT = ROOT / "docs" / "profiles-gallery.md"

# Pseudo / sensitive codes: no national flag — neutral bullet.
NEUTRAL_CODES = frozenset({"AP", "AN", "BY", "RU"})
NEUTRAL_EMOJI = "●"

# Compact column headers for rttToDest keys.
DEST_SHORT = {
    "cf": "CF",
    "aws-eu-central-1": "FRA",
    "aws-eu-west-1": "DUB",
    "aws-us-east-1": "IAD",
    "aws-us-west-2": "PDX",
    "aws-ap-southeast-1": "SIN",
    "aws-ap-northeast-1": "NRT",
    "aws-ap-south-1": "BOM",
}


def flag_emoji(code: str) -> str:
    """ISO 3166-1 alpha-2 → regional-indicator flag emoji."""
    cc = (code or "").strip().upper()
    if cc in NEUTRAL_CODES:
        return NEUTRAL_EMOJI
    if len(cc) != 2 or not cc.isalpha():
        return NEUTRAL_EMOJI
    return "".join(chr(0x1F1E6 + ord(c) - ord("A")) for c in cc)


def dest_order(destinations: dict) -> list[str]:
    keys = list(destinations.keys()) if destinations else []
    preferred = list(DEST_SHORT.keys())
    ordered = [k for k in preferred if k in keys]
    ordered.extend(k for k in keys if k not in ordered)
    return ordered or preferred


def short_label(dest_id: str, destinations: dict) -> str:
    if dest_id in DEST_SHORT:
        return DEST_SHORT[dest_id]
    meta = destinations.get(dest_id) or {}
    if meta.get("awsRegion"):
        return str(meta["awsRegion"])
    return dest_id


def main() -> int:
    cat = json.loads(CATALOG.read_text())
    destinations = cat.get("destinations") or {}
    dests = dest_order(destinations)
    dest_headers = [short_label(d, destinations) for d in dests]

    legend = ", ".join(
        f"`{short_label(d, destinations)}` = {(destinations.get(d) or {}).get('label') or d}"
        for d in dests
    )

    lines = [
        "# Profiles gallery",
        "",
        f"Generated from catalog `source={cat.get('source')}` at `{cat.get('generatedAt')}`.",
        "",
        "RTT columns are milliseconds to catalog destinations. "
        "Last-mile one-way delay at runtime ≈ `(rtt_cf − host_cf) / 2`.",
        "",
        f"Destinations: {legend}.",
        "",
    ]

    base_headers = ["Tier", "↓ Mbps", "↑ Mbps", "Loss %", *dest_headers]
    sep = ["---"] * len(base_headers)

    for c in sorted(cat.get("countries") or [], key=lambda x: x.get("name") or ""):
        cid = c.get("id") or ""
        name = c.get("name") or cid
        emoji = flag_emoji(cid)
        nearest = c.get("nearestAws") or ""
        nearest_note = f" · nearest AWS `{short_label(nearest, destinations)}`" if nearest else ""
        lines.append(f"## {emoji} {name} (`{cid}`){nearest_note}")
        lines.append("")
        lines.append("| " + " | ".join(base_headers) + " |")
        lines.append("| " + " | ".join(sep) + " |")
        tiers = c.get("tiers") or {}
        for tier in ("stable", "typical", "poor"):
            t = tiers.get(tier) or {}
            rtt = t.get("rttToDest") or {}
            cells = [
                tier,
                str(t.get("downloadMbps", "—")),
                str(t.get("uploadMbps", "—")),
                str(t.get("lossPercent", "—")),
            ]
            for d in dests:
                cells.append(str(rtt.get(d, "—")))
            lines.append("| " + " | ".join(cells) + " |")
        lines.append("")

    OUT.write_text("\n".join(lines) + "\n")
    print(f"wrote {OUT}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())

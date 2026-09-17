#!/usr/bin/env python3
"""Test a Python re pattern the same way PotatoInspector MITM rules do."""

from __future__ import annotations

import json
import re
import sys


def main() -> int:
    raw = sys.stdin.read()
    try:
        req = json.loads(raw)
    except json.JSONDecodeError as e:
        json.dump({"ok": False, "matched": False, "error": f"bad json: {e}"}, sys.stdout)
        return 0

    pattern = req.get("pattern")
    if pattern is None:
        pattern = ""
    text = req.get("text")
    if text is None:
        text = ""
    ignore_case = bool(req.get("ignoreCase", False))

    flags = re.IGNORECASE if ignore_case else 0
    try:
        rx = re.compile(str(pattern), flags)
    except re.error as e:
        json.dump({"ok": False, "matched": False, "error": str(e)}, sys.stdout)
        return 0

    m = rx.search(str(text))
    if not m:
        json.dump({"ok": True, "matched": False, "span": None, "error": ""}, sys.stdout)
        return 0

    json.dump(
        {
            "ok": True,
            "matched": True,
            "span": [m.start(), m.end()],
            "group": m.group(0),
            "error": "",
        },
        sys.stdout,
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

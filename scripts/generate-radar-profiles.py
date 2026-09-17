#!/usr/bin/env python3
"""Build profiles/radar/catalog.json from Cloudflare Radar + CloudPing AWS matrix.

No Globalping. Path RTT to AWS:
  rtt_dest = rtt_cf + cloudping(nearest_aws(country), dest_region)

Env:
  CLOUDFLARE_API_TOKEN — optional; without it uses seed last-mile for a country subset
  CLOUDPING_URL        — default https://www.cloudping.co/api/latencies
"""
from __future__ import annotations

import json
import math
import os
import sys
import time
import urllib.error
import urllib.request
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "profiles" / "radar" / "catalog.json"
EMBED = ROOT / "internal" / "catalog" / "data" / "catalog.json"

# Destinations exposed in the product (cf + selected AWS regions).
AWS_DESTS = {
    "aws-eu-central-1": {
        "label": "AWS Frankfurt",
        "region": "eu-central-1",
        "lat": 50.11,
        "lon": 8.68,
        "target": "ec2.eu-central-1.amazonaws.com",
    },
    "aws-eu-west-1": {
        "label": "AWS Ireland",
        "region": "eu-west-1",
        "lat": 53.35,
        "lon": -6.26,
        "target": "ec2.eu-west-1.amazonaws.com",
    },
    "aws-us-east-1": {
        "label": "AWS N. Virginia",
        "region": "us-east-1",
        "lat": 39.04,
        "lon": -77.49,
        "target": "ec2.us-east-1.amazonaws.com",
    },
    "aws-us-west-2": {
        "label": "AWS Oregon",
        "region": "us-west-2",
        "lat": 45.87,
        "lon": -119.69,
        "target": "ec2.us-west-2.amazonaws.com",
    },
    "aws-ap-southeast-1": {
        "label": "AWS Singapore",
        "region": "ap-southeast-1",
        "lat": 1.35,
        "lon": 103.82,
        "target": "ec2.ap-southeast-1.amazonaws.com",
    },
    "aws-ap-northeast-1": {
        "label": "AWS Tokyo",
        "region": "ap-northeast-1",
        "lat": 35.68,
        "lon": 139.69,
        "target": "ec2.ap-northeast-1.amazonaws.com",
    },
    "aws-ap-south-1": {
        "label": "AWS Mumbai",
        "region": "ap-south-1",
        "lat": 19.08,
        "lon": 72.88,
        "target": "ec2.ap-south-1.amazonaws.com",
    },
}

# Capitals / approximate centroids for nearest-AWS mapping + seed.
COUNTRY_META: dict[str, tuple[str, str, float, float]] = {
    # code: name, flag, lat, lon
    "AF": ("Afghanistan", "🇦🇫", 34.5, 69.2),
    "AL": ("Albania", "🇦🇱", 41.3, 19.8),
    "DZ": ("Algeria", "🇩🇿", 36.7, 3.1),
    "AR": ("Argentina", "🇦🇷", -34.6, -58.4),
    "AM": ("Armenia", "🇦🇲", 40.2, 44.5),
    "AU": ("Australia", "🇦🇺", -35.3, 149.1),
    "AT": ("Austria", "🇦🇹", 48.2, 16.4),
    "AZ": ("Azerbaijan", "🇦🇿", 40.4, 49.9),
    "BH": ("Bahrain", "🇧🇭", 26.2, 50.6),
    "BD": ("Bangladesh", "🇧🇩", 23.8, 90.4),
    "BY": ("Belarus", "🇧🇾", 53.9, 27.6),
    "BE": ("Belgium", "🇧🇪", 50.8, 4.4),
    "BO": ("Bolivia", "🇧🇴", -16.5, -68.1),
    "BA": ("Bosnia and Herzegovina", "🇧🇦", 43.9, 18.4),
    "BR": ("Brazil", "🇧🇷", -15.8, -47.9),
    "BG": ("Bulgaria", "🇧🇬", 42.7, 23.3),
    "KH": ("Cambodia", "🇰🇭", 11.6, 104.9),
    "CM": ("Cameroon", "🇨🇲", 3.9, 11.5),
    "CA": ("Canada", "🇨🇦", 45.4, -75.7),
    "CL": ("Chile", "🇨🇱", -33.4, -70.7),
    "CN": ("China", "🇨🇳", 39.9, 116.4),
    "CO": ("Colombia", "🇨🇴", 4.7, -74.1),
    "CR": ("Costa Rica", "🇨🇷", 9.9, -84.1),
    "HR": ("Croatia", "🇭🇷", 45.8, 16.0),
    "CZ": ("Czechia", "🇨🇿", 50.1, 14.4),
    "DK": ("Denmark", "🇩🇰", 55.7, 12.6),
    "DO": ("Dominican Republic", "🇩🇴", 18.5, -69.9),
    "EC": ("Ecuador", "🇪🇨", -0.2, -78.5),
    "EG": ("Egypt", "🇪🇬", 30.0, 31.2),
    "EE": ("Estonia", "🇪🇪", 59.4, 24.8),
    "ET": ("Ethiopia", "🇪🇹", 9.0, 38.7),
    "FI": ("Finland", "🇫🇮", 60.2, 24.9),
    "FR": ("France", "🇫🇷", 48.9, 2.3),
    "GE": ("Georgia", "🇬🇪", 41.7, 44.8),
    "DE": ("Germany", "🇩🇪", 52.5, 13.4),
    "GH": ("Ghana", "🇬🇭", 5.6, -0.2),
    "GR": ("Greece", "🇬🇷", 37.98, 23.7),
    "GT": ("Guatemala", "🇬🇹", 14.6, -90.5),
    "HK": ("Hong Kong", "🇭🇰", 22.3, 114.2),
    "HU": ("Hungary", "🇭🇺", 47.5, 19.0),
    "IS": ("Iceland", "🇮🇸", 64.1, -21.9),
    "IN": ("India", "🇮🇳", 28.6, 77.2),
    "ID": ("Indonesia", "🇮🇩", -6.2, 106.8),
    "IQ": ("Iraq", "🇮🇶", 33.3, 44.4),
    "IE": ("Ireland", "🇮🇪", 53.3, -6.3),
    "IL": ("Israel", "🇮🇱", 31.8, 35.2),
    "IT": ("Italy", "🇮🇹", 41.9, 12.5),
    "JM": ("Jamaica", "🇯🇲", 18.0, -76.8),
    "JP": ("Japan", "🇯🇵", 35.7, 139.7),
    "JO": ("Jordan", "🇯🇴", 31.9, 35.9),
    "KZ": ("Kazakhstan", "🇰🇿", 51.2, 71.4),
    "KE": ("Kenya", "🇰🇪", -1.3, 36.8),
    "KW": ("Kuwait", "🇰🇼", 29.4, 47.98),
    "LV": ("Latvia", "🇱🇻", 56.9, 24.1),
    "LB": ("Lebanon", "🇱🇧", 33.9, 35.5),
    "LT": ("Lithuania", "🇱🇹", 54.7, 25.3),
    "LU": ("Luxembourg", "🇱🇺", 49.6, 6.1),
    "MY": ("Malaysia", "🇲🇾", 3.1, 101.7),
    "MX": ("Mexico", "🇲🇽", 19.4, -99.1),
    "MD": ("Moldova", "🇲🇩", 47.0, 28.9),
    "MA": ("Morocco", "🇲🇦", 34.0, -6.8),
    "MM": ("Myanmar", "🇲🇲", 16.8, 96.2),
    "NP": ("Nepal", "🇳🇵", 27.7, 85.3),
    "NL": ("Netherlands", "🇳🇱", 52.4, 4.9),
    "NZ": ("New Zealand", "🇳🇿", -41.3, 174.8),
    "NG": ("Nigeria", "🇳🇬", 9.1, 7.5),
    "NO": ("Norway", "🇳🇴", 59.9, 10.7),
    "OM": ("Oman", "🇴🇲", 23.6, 58.5),
    "PK": ("Pakistan", "🇵🇰", 33.7, 73.1),
    "PA": ("Panama", "🇵🇦", 9.0, -79.5),
    "PE": ("Peru", "🇵🇪", -12.0, -77.0),
    "PH": ("Philippines", "🇵🇭", 14.6, 121.0),
    "PL": ("Poland", "🇵🇱", 52.2, 21.0),
    "PT": ("Portugal", "🇵🇹", 38.7, -9.1),
    "QA": ("Qatar", "🇶🇦", 25.3, 51.5),
    "RO": ("Romania", "🇷🇴", 44.4, 26.1),
    "RU": ("Russia", "🇷🇺", 55.8, 37.6),
    "SA": ("Saudi Arabia", "🇸🇦", 24.7, 46.7),
    "RS": ("Serbia", "🇷🇸", 44.8, 20.5),
    "SG": ("Singapore", "🇸🇬", 1.35, 103.8),
    "SK": ("Slovakia", "🇸🇰", 48.1, 17.1),
    "SI": ("Slovenia", "🇸🇮", 46.1, 14.5),
    "ZA": ("South Africa", "🇿🇦", -25.7, 28.2),
    "KR": ("South Korea", "🇰🇷", 37.6, 127.0),
    "ES": ("Spain", "🇪🇸", 40.4, -3.7),
    "LK": ("Sri Lanka", "🇱🇰", 6.9, 79.9),
    "SE": ("Sweden", "🇸🇪", 59.3, 18.1),
    "CH": ("Switzerland", "🇨🇭", 46.9, 7.4),
    "TW": ("Taiwan", "🇹🇼", 25.0, 121.5),
    "TH": ("Thailand", "🇹🇭", 13.8, 100.5),
    "TR": ("Turkey", "🇹🇷", 39.9, 32.9),
    "UA": ("Ukraine", "🇺🇦", 50.5, 30.5),
    "AE": ("United Arab Emirates", "🇦🇪", 24.5, 54.4),
    "GB": ("United Kingdom", "🇬🇧", 51.5, -0.1),
    "US": ("United States", "🇺🇸", 38.9, -77.0),
    "UY": ("Uruguay", "🇺🇾", -34.9, -56.2),
    "UZ": ("Uzbekistan", "🇺🇿", 41.3, 69.2),
    "VE": ("Venezuela", "🇻🇪", 10.5, -66.9),
    "VN": ("Vietnam", "🇻🇳", 21.0, 105.8),
}

# Seed last-mile when Radar token missing (down, up, loss, rtt_cf).
SEED_LASTMILE = {
    "BD": (32, 12, 0.5, 48),
    "IN": (40, 15, 0.4, 35),
    "PH": (28, 10, 0.6, 30),
    "NG": (18, 8, 0.8, 55),
    "BR": (50, 20, 0.3, 25),
    "DE": (80, 30, 0.1, 12),
    "US": (100, 40, 0.1, 15),
    "GB": (75, 28, 0.15, 14),
    "SG": (90, 35, 0.1, 8),
    "JP": (85, 35, 0.1, 12),
    "ID": (35, 12, 0.5, 28),
    "PK": (25, 10, 0.7, 50),
    "EG": (30, 10, 0.6, 40),
    "ZA": (40, 15, 0.4, 35),
    "AU": (70, 25, 0.2, 20),
    "CA": (90, 35, 0.1, 18),
    "MX": (45, 15, 0.4, 30),
    "FR": (80, 30, 0.1, 14),
    "IT": (70, 25, 0.2, 18),
    "ES": (70, 25, 0.2, 20),
    "NL": (100, 40, 0.1, 10),
    "KR": (90, 35, 0.1, 12),
    "TH": (45, 15, 0.4, 28),
    "VN": (40, 12, 0.5, 32),
    "TR": (50, 18, 0.3, 30),
    "PL": (70, 25, 0.2, 18),
    "AE": (80, 30, 0.2, 20),
    "SA": (60, 20, 0.3, 35),
    "AR": (40, 15, 0.5, 40),
    "CO": (35, 12, 0.5, 45),
}


def http_json(url: str, headers: dict | None = None) -> dict:
    req = urllib.request.Request(url, headers=headers or {})
    with urllib.request.urlopen(req, timeout=60) as resp:
        return json.loads(resp.read().decode())


def haversine_km(lat1: float, lon1: float, lat2: float, lon2: float) -> float:
    r = 6371.0
    p1, p2 = math.radians(lat1), math.radians(lat2)
    dphi = math.radians(lat2 - lat1)
    dlmb = math.radians(lon2 - lon1)
    a = math.sin(dphi / 2) ** 2 + math.cos(p1) * math.cos(p2) * math.sin(dlmb / 2) ** 2
    return 2 * r * math.asin(math.sqrt(a))


def nearest_aws(lat: float, lon: float) -> str:
    best, best_d = "aws-eu-central-1", 1e18
    for dest_id, meta in AWS_DESTS.items():
        d = haversine_km(lat, lon, meta["lat"], meta["lon"])
        if d < best_d:
            best, best_d = dest_id, d
    return best


def geo_backbone_ms(a: dict, b: dict) -> float:
    """Fiber RTT floor * inflation between two AWS dest metas."""
    km = haversine_km(a["lat"], a["lon"], b["lat"], b["lon"])
    return (2 * km / 200.0) * 1.5  # ~200 km/ms RTT floor, ×1.5 routing


def fetch_cloudping(url: str) -> dict[str, dict[str, float]]:
    raw = http_json(url)
    data = raw.get("data") or {}
    out: dict[str, dict[str, float]] = {}
    for src, row in data.items():
        if not isinstance(row, dict):
            continue
        out[src] = {}
        for dst, val in row.items():
            try:
                out[src][dst] = float(val)
            except (TypeError, ValueError):
                continue
    return out


def backbone_ms(matrix: dict[str, dict[str, float]], src_region: str, dst_region: str) -> float:
    if src_region == dst_region:
        return 0.0
    row = matrix.get(src_region) or {}
    if dst_region in row:
        return max(0.0, float(row[dst_region]))
    # try reverse
    rev = (matrix.get(dst_region) or {}).get(src_region)
    if rev is not None:
        return max(0.0, float(rev))
    # geo fallback between known dests
    src_meta = next((m for m in AWS_DESTS.values() if m["region"] == src_region), None)
    dst_meta = next((m for m in AWS_DESTS.values() if m["region"] == dst_region), None)
    if src_meta and dst_meta:
        return geo_backbone_ms(src_meta, dst_meta)
    return 80.0


def radar_locations(token: str) -> list[dict]:
    url = "https://api.cloudflare.com/client/v4/radar/entities/locations?format=json&limit=500"
    raw = http_json(url, headers={"Authorization": f"Bearer {token}"})
    return (raw.get("result") or {}).get("locations") or []


def radar_summary(token: str, country: str) -> dict | None:
    url = (
        "https://api.cloudflare.com/client/v4/radar/quality/speed/summary"
        f"?location={country}&format=json"
    )
    try:
        raw = http_json(url, headers={"Authorization": f"Bearer {token}"})
    except Exception as e:
        print(f"radar {country}: {e}", file=sys.stderr)
        return None
    if not raw.get("success"):
        return None
    return (raw.get("result") or {}).get("summary_0") or {}


def tier_rtt(base: dict[str, int], mult: float) -> dict[str, int]:
    return {k: max(1, int(round(v * mult))) for k, v in base.items()}


def build_tiers(down: float, up: float, loss: float, rtt: dict[str, int]) -> dict:
    return {
        "stable": {
            "downloadMbps": round(down * 1.25, 1),
            "uploadMbps": round(up * 1.25, 1),
            "lossPercent": round(max(0.05, loss * 0.3), 2),
            "rttToDest": tier_rtt(rtt, 0.85),
        },
        "typical": {
            "downloadMbps": round(down, 1),
            "uploadMbps": round(up, 1),
            "lossPercent": round(loss, 2),
            "rttToDest": dict(rtt),
        },
        "poor": {
            "downloadMbps": round(max(1.5, down * 0.2), 1),
            "uploadMbps": round(max(0.5, up * 0.2), 1),
            "lossPercent": round(max(1.0, loss * 3), 2),
            "rttToDest": tier_rtt(rtt, 1.4),
        },
    }


def build_destinations() -> dict:
    dests = {
        "cf": {
            "label": "Cloudflare Edge",
            "calibrate": "cf",
            "target": "speed.cloudflare.com",
        }
    }
    for dest_id, meta in AWS_DESTS.items():
        dests[dest_id] = {
            "label": meta["label"],
            "calibrate": dest_id,
            "target": meta["target"],
            "awsRegion": meta["region"],
        }
    return dests


def country_list(token: str | None) -> list[tuple[str, str, str, float, float]]:
    """Return list of (code, name, flag, lat, lon)."""
    out = []
    if token:
        try:
            locs = radar_locations(token)
            for loc in locs:
                code = (loc.get("alpha2") or "").upper()
                if len(code) != 2:
                    continue
                name = loc.get("name") or code
                flag, lat, lon = "", 0.0, 0.0
                if code in COUNTRY_META:
                    _, flag, lat, lon = COUNTRY_META[code]
                else:
                    try:
                        lat = float(loc.get("latitude") or 0)
                        lon = float(loc.get("longitude") or 0)
                    except (TypeError, ValueError):
                        lat, lon = 0.0, 0.0
                out.append((code, name, flag, lat, lon))
            if out:
                return sorted(out, key=lambda x: x[0])
        except Exception as e:
            print(f"radar locations failed: {e}", file=sys.stderr)
    # fallback: COUNTRY_META keys that we care about (seed + extras)
    for code, (name, flag, lat, lon) in sorted(COUNTRY_META.items()):
        out.append((code, name, flag, lat, lon))
    return out


def main() -> int:
    token = os.environ.get("CLOUDFLARE_API_TOKEN", "").strip()
    cloudping_url = os.environ.get(
        "CLOUDPING_URL", "https://www.cloudping.co/api/latencies"
    )

    print("fetching CloudPing matrix…", file=sys.stderr)
    try:
        matrix = fetch_cloudping(cloudping_url)
        print(f"cloudping regions: {len(matrix)}", file=sys.stderr)
        source = "radar+cloudping" if token else "seed+cloudping"
    except Exception as e:
        print(f"cloudping failed ({e}), using geo backbone", file=sys.stderr)
        matrix = {}
        source = "radar+geo" if token else "seed+geo"

    countries_out = []
    for code, name, flag, lat, lon in country_list(token if token else None):
        if lat == 0 and lon == 0 and code in COUNTRY_META:
            _, flag, lat, lon = COUNTRY_META[code]

        down, up, loss, rtt_cf = SEED_LASTMILE.get(code, (40, 15, 0.4, 40))
        rtt_source = "seed"

        if token:
            summary = radar_summary(token, code)
            time.sleep(0.15)
            if summary:
                try:
                    down = float(summary.get("bandwidthDownload") or down)
                    up = float(summary.get("bandwidthUpload") or up)
                    loss = float(summary.get("packetLoss") or loss)
                    idle = float(summary.get("latencyIdle") or rtt_cf)
                    rtt_cf = max(1, int(round(idle)))
                    rtt_source = "radar"
                except (TypeError, ValueError):
                    pass

        nearest_id = nearest_aws(lat, lon) if (lat or lon) else "aws-eu-central-1"
        nearest_region = AWS_DESTS[nearest_id]["region"]

        rtt: dict[str, int] = {"cf": max(1, int(rtt_cf))}
        for dest_id, meta in AWS_DESTS.items():
            bb = backbone_ms(matrix, nearest_region, meta["region"])
            rtt[dest_id] = max(rtt["cf"], int(round(rtt["cf"] + bb)))

        countries_out.append(
            {
                "id": code,
                "name": name,
                "flag": flag,
                "nearestAws": nearest_id,
                "rttSource": rtt_source,
                "tiers": build_tiers(down, up, loss, rtt),
            }
        )

    catalog = {
        "generatedAt": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "source": source,
        "destinations": build_destinations(),
        "countries": countries_out,
    }
    text = json.dumps(catalog, indent=2, ensure_ascii=False) + "\n"
    OUT.parent.mkdir(parents=True, exist_ok=True)
    OUT.write_text(text, encoding="utf-8")
    EMBED.parent.mkdir(parents=True, exist_ok=True)
    EMBED.write_text(text, encoding="utf-8")
    print(f"wrote {OUT} ({len(countries_out)} countries, source={source})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

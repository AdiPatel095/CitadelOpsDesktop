#!/usr/bin/env python3
"""Download GGE items JSON and write Server/Models/decoration_catalog.json.

Filter matches GeneralsCamp "GGE Decorations" (Deco + public order > 0, non-test comments):
https://generalscamp.github.io/forum/overviews/decorations/index.html

Regenerate: python3 Server/scripts/gen_decoration_catalog.py
"""
from __future__ import annotations

import json
import re
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "Models" / "decoration_catalog.json"

VER_URL = "https://empire-html5.goodgamestudios.com/default/items/ItemsVersion.properties"
ITEMS_BASE = "https://empire-html5.goodgamestudios.com/default/items"
UA = "CitadelOpsDecorationGen/1.0"


def fetch(url: str) -> bytes:
    req = urllib.request.Request(url, headers={"User-Agent": UA})
    with urllib.request.urlopen(req, timeout=120) as r:
        return r.read()


def get_po(item: dict) -> int:
    if item.get("decoPoints") is not None:
        try:
            return int(str(item["decoPoints"]).strip())
        except (ValueError, TypeError):
            pass
    if item.get("initialFusionLevel") is not None:
        try:
            level = int(str(item["initialFusionLevel"]).strip())
            return 100 + level * 5
        except (ValueError, TypeError):
            pass
    return 0


def is_deco(item: dict) -> bool:
    if str(item.get("name", "")).lower() != "deco":
        return False
    if get_po(item) <= 0:
        return False
    c1 = str(item.get("comment1") or "").lower()
    c2 = str(item.get("comment2") or "").lower()
    if "test" in c1 or "test" in c2:
        return False
    return True


def main() -> None:
    ver_text = fetch(VER_URL).decode()
    m = re.search(r"CastleItemXMLVersion=(\d+\.\d+)", ver_text)
    if not m:
        raise SystemExit("Could not parse CastleItemXMLVersion")
    version = m.group(1)
    items_url = f"{ITEMS_BASE}/items_v{version}.json"
    data = json.loads(fetch(items_url).decode("utf-8"))
    buildings = data.get("buildings") or []

    by_wid: dict[str, dict] = {}
    for b in buildings:
        if not is_deco(b):
            continue
        try:
            wid = int(b["wodID"])
        except (KeyError, TypeError, ValueError):
            continue
        by_wid[str(wid)] = b

    payload = {"version": version, "byWid": by_wid}
    OUT.write_text(json.dumps(payload, ensure_ascii=False, separators=(",", ":")) + "\n", encoding="utf-8")
    print(f"Wrote {OUT} ({len(by_wid)} entries, catalog {version})")


if __name__ == "__main__":
    main()

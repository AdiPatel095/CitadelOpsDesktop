from __future__ import annotations

import json
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
GAME_DATA_DIR = ROOT / "Client" / "public" / "game-data"

RESOURCE_IMAGE_MAP = {
    "currency1": "/assets/Resources/Sceat.png",
    "currency2": "/assets/Resources/ImperialDucat.png",
    "wood": "/assets/Resources/Wood.png",
    "stone": "/assets/Resources/Stone.png",
    "food": "/assets/Resources/Food.png",
    "coal": "/assets/Resources/Charcoal.png",
    "oil": "/assets/Resources/OliveOil.png",
    "glass": "/assets/Resources/Glass.png",
    "aquamarine": "/assets/Resources/Aquamarine.png",
    "iron": "/assets/Resources/Iron_Ore.png",
    "honey": "/assets/Resources/Honey.png",
    "mead": "/assets/Resources/Mead.png",
    "beef": "/assets/Resources/Beef.png",
}


def read_json(path: Path):
    with path.open("r", encoding="utf-8") as f:
        return json.load(f)


def write_json(path: Path, payload) -> None:
    with path.open("w", encoding="utf-8", newline="\n") as f:
        json.dump(payload, f, indent=2, ensure_ascii=True)
        f.write("\n")


def sync_index_images(category: str) -> None:
    category_dir = GAME_DATA_DIR / category
    image_dir = category_dir / "images"
    index_path = category_dir / "index.json"
    items = read_json(index_path)

    for item in items:
        item_id = item.get("id")
        image_path = image_dir / f"{item_id}.png" if item_id is not None else None
        item["image"] = f"/game-data/{category}/images/{item_id}.png" if image_path and image_path.exists() else None

    write_json(index_path, items)


def sync_item_image_fields(category: str) -> None:
    category_dir = GAME_DATA_DIR / category
    image_dir = category_dir / "images"
    items_path = category_dir / "items.json"
    items = read_json(items_path)

    for item in items:
        item_id = item.get("wodID")
        image_path = image_dir / f"{item_id}.png" if item_id is not None else None
        local_url = f"/game-data/{category}/images/{item_id}.png" if image_path and image_path.exists() else None
        item["image_url"] = local_url
        item["image_local"] = local_url

    write_json(items_path, items)


def sync_resources() -> None:
    category_dir = GAME_DATA_DIR / "resources"
    index_path = category_dir / "index.json"
    items_path = category_dir / "items.json"

    index_items = read_json(index_path)
    for item in index_items:
        item["image"] = RESOURCE_IMAGE_MAP.get(item.get("name"))
    write_json(index_path, index_items)

    resource_items = read_json(items_path)
    for item in resource_items:
        local_url = RESOURCE_IMAGE_MAP.get(item.get("name"))
        item["image_url"] = local_url
        item["image_local"] = local_url
    write_json(items_path, resource_items)


def main() -> None:
    for category in ("troops", "tools", "decorations"):
        sync_index_images(category)
        sync_item_image_fields(category)
    sync_resources()


if __name__ == "__main__":
    main()

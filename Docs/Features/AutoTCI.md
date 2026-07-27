# Auto TCI

Manages construction items (TCI): keeps the configured items equipped at the
configured levels, upgrading and buying them as needed.

Source: `Server/Automation/ConstructionPolicy.go`,
`Server/App/ConstructionIntents.go`, `Server/GameParser/CidTrivialProduct.go`.

## Identity

| | |
|---|---|
| Policy ID | `autoTCI` |
| Enabled key | `auto_tci` |
| Priority | 70 (`PriorityAutoTCI`) |
| Config section | `automation.constructionItems` |

Note the mismatch: the policy ID is `autoTCI` but the config section is
`automation.constructionItems`. Both are load-bearing — the ID drives the
schedule and enabled lookups, the section drives wake and fingerprinting.

## Settings

`constructionSettings` holds `targets` — the construction items to maintain,
each with the building it belongs to and the level to reach.

## Wake triggers

Domains: `construction-items`, `construction-offers`, `inventory`. Section:
`automation.constructionItems`.

## Decision ladder

1. **No targets configured** — `waiting`.
2. **Inventory stale** — `construction.inventory.refresh`.
3. **Slots at a castle unknown** — `game.focus_castle` to read them.
4. **An item can be upgraded** — `construction.upgrade` to the target level.
5. **An item is owned but not equipped** — `construction.equip`.
6. **Shop offers stale** — `construction.shop` to refresh live offers.
7. **A needed item is purchasable** — `construction.purchase`.
8. **No matching live or official shop entry** — `blocked`.
9. **Otherwise** — `idle`.

## Guards

- **CID namespace discipline.** This is the feature's biggest correctness
  hazard, documented as a hard rule in `AGENTS.md` §5: a construction item's
  **CID** is a different id namespace from a building/decoration **wodID**. The
  same number means different things in the two catalogs, so a TCI CID must
  never be resolved through `GetBuildingInfo` or the decoration catalogs.
  Levels come only from `construction_items/items.json`.
- **`gbc` payload trap.** In the outgoing open-purchase-list payload, the field
  named `CID` is the **castle instance id (AID)**, not a construction item id.
- **Live offers before purchase.** The feature refreshes live shop offers rather
  than trusting the shipped package map, and blocks explicitly when neither a
  live nor an official offer matches — rather than guessing a product id.
- **Order of operations.** Upgrade before equip before buy, so the feature does
  not purchase something it already owns unequipped.

## Data sources

The trivial-shop product map is built from `Server/Data/packages/items.json`
(official game data). A per-instance `CidTrivialProduct.json` in the DataDir can
override or extend it — that override lives next to the binary, **not** under
`Server/Data/`, which is reserved for mirrored official game data.

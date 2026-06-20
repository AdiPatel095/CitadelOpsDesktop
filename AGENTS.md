# AGENTS.md — CitadelOpsDesktop

Conventions for humans and AI assistants working in this repository. **Cursor rule files** under `[.cursor/rules/](.cursor/rules/)` are authoritative where they repeat or specialize these points.

---

## 1. Execution and environment

- This is a **real** workspace: **run** commands, read files, and fix issues yourself. Do not stop after a single failed attempt; try alternatives and diagnose.
- When investigation needs large logs (e.g. `websocket_game.log`), use **streaming**, **line ranges**, or **grep/Select-String**—do not load entire multi‑GB files into a single read or string.
- **Today’s date** for user-facing or search defaults: use the **“Today’s date”** value from the active session / user_info (do not assume an old year).
- This is **not a production application**. Testing is **not required** as a default definition of done; run tests only when the user explicitly asks, when diagnosing a test failure, or when a narrowly scoped check is clearly useful.

---

## 2. Scope of code changes (hard)

- Change **only** what the task requires: **no** drive-by refactors, **no** unrelated files, **no** new markdown or docs **unless the user asked** (this file and `.cursor/rules` updates are an exception when explicitly requested).
- New code should **match** the surrounding package: imports, error handling density, and comment style of that area.
- **All new** hand-written source files use **PascalCase** basenames (see `[.cursor/rules/pascalcase-filenames.mdc](.cursor/rules/pascalcase-filenames.mdc)`). Ecosystem files (`go.mod`, `package.json`, `README.md`, etc.) stay as required by tools.
- GCA-related parser sources use **PascalCase** basenames: `GcaWodBuildings.go`, `GcaConstructionItems.go`, `GcaConstructionLevel.go`, `GcaJSONInt.go`, and matching `*_test.go` files.

---

## 3. Go tooling (non-shipped scripts)

- **Durable** dev scripts, one-off repo tools, and logic-heavy automation **not** part of the shipped product must be **Go** (`go run`, `go test`, or a `cmd/<tool>` binary), not ad-hoc Python/Node for the same job—see `[.cursor/rules/dev-tooling-golang.mdc](.cursor/rules/dev-tooling-golang.mdc)`.
- **Import cycles:** do **not** create a cycle between `**GameParser`** and `**GameFeatures`** (e.g. `GameFeatures/AutoBird.go` already imports `GameParser`). TCI/CI level maps used only in parsing live under `GameParser` to avoid that cycle.

---

## 4. Server models and `GameState`

- Layout of castles, focus, and related types: `[.cursor/rules/gamestate-and-server-models.mdc](.cursor/rules/gamestate-and-server-models.mdc)`.
- Each `PlayerCastleInfo` includes **construction item slots** parsed from the game: `ConstructionByBuilding` (`GCAConstructionBuilding` per building **OID**, with `GCAConstructionSlot`: **CID**, **S**, **remainingSec**, **level**). Sources: **jaa** `gca.CI`, and **rpc** / **ubc** responses with a root `CI` array (applied to the **focused** castle).

**Data directories:** The tree at `Server/Data/` (and the copies distributed with the app) is reserved for **official in-game JSON** mirrored or fetched from the game’s public data endpoints. It is not for Citadel-specific config, presets, or user maps; those belong under `Server/Paths` **DataDir** (per instance, next to the binary or `CITADEL_DATA_DIR`).

---

## 5. Construction items (TCI) — IDs and wire format (hard)

These rules avoid confused types and bad automation.

1. **CID / `constructionItemID`:** The id in JAA `gca.CI[].CIL[].CID`, AutoTCI settings, and `Server/Data/construction_items/items.json` is the **construction item definition id**. It is **not** the same namespace as BG/BD row `[0]` (**building/decoration wodID**). The **same number** can mean different things in `decorations/items.json`—**never** interpret a TCI **CID** with `GetBuildingInfo` / decor catalogs.
2. **Level** for a given **CID** comes **only** from `construction_items/items.json` (or helpers that read that data), not from the static building id map.
3. `**RS` in CIL:** On the wire, **RS** is **remaining time in seconds** for timed TCI. Internal/model JSON uses `**remainingSec`** (pointer when absent/optional). Parsing fills `**Level`** from the items catalog where possible.
4. `**gbc` (open trivial purchase list):** In the **outgoing** payload, field `**CID`** is the **castle instance id (AID)**, *not* a construction item id. `**sbp`** completes a purchase (see `Server/GameCommands/Commands.go` and live captures under `Logs/` if present).
5. **EmpireEx_21 commands** for TCI (shapes in code comments): **rpc** (equip), **ubc** (upgrade), **gbc** (list), **sbp** (buy). **ubc** responses may include **BCID** (new CID after upgrade) plus full `CI` snapshot.
6. **SIN** and similar “inventory” `RD` rows: the first column is sometimes called “wodID” in general docs; for **construction-item** segments that id aligns with **CID**, not with arbitrary decoration wodIDs.
7. **Trivial shop PID+AMT (AutoTCI buy/rebuy):** Shipped map is built from **`Server/Data/packages/items.json`** (Central Silver Shop construction-item packages); optional per-instance `CidTrivialProduct.json` in **DataDir** overrides/extends (see `Server/GameParser/CidTrivialProduct.go`, `CidTrivialProductPackages.go`, `Server/Paths`), not under `Server/Data`. An **empty** JSON shape is `Server/AppConfig/CidTrivialProduct.Example.json`.

---

## 6. UI / client pointers

- **Castle focus** and merged BG+BD: see `Client` types and comments referencing **GameParser** / `GcaWodBuildings` where applicable.

---

## 7. Communication style (when responding to the user)

- When citing **existing** code in this repo, use a single **code-reference block** with start line, end line, and full file path (opening fence on its own line, not merged with list items).
- Write in **full sentences**; keep length proportional to the task; avoid engagement-style sign-offs; use **sparing** bold/backticks in prose.

---

## 8. Skills and rules precedence

- Follow **user instructions**, **tool constraints**, **this file**, and `**.cursor/rules/*.mdc`**. If a skill file applies to a task, read and follow it.

---

## 9. What “done” means for TCI work (product direction)

- **State:** Server can store equipped CIs, **remaining time**, and **level** from frames (see `GameParser` and `Server/Models/Castle/ConstructionSlots.go`).
- **Automation:** `AutoTCI` and related logic should eventually use that state plus settings; **outgoing** helpers exist in `GameCommands`—wire real **SUC** / purchase flows from captures when implementing upgrades and buys.

This section is product intent, not a guarantee that every path is already implemented in the app loop.

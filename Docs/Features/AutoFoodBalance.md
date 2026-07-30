# Auto Food Balance

Keeps every castle above a configured food reserve by shipping food from castles
that have a surplus, using market barrows within a kingdom and kingdom transport
across kingdoms.

Source: `Server/Automation/FoodBalancePolicy.go`.

## Identity

| | |
|---|---|
| Policy ID | `autoFoodBalance` |
| Enabled key | `auto_food_balance` |
| Priority | 10 (background) |
| Config section | `automation.autoFoodBalance` |

## Settings

| Field | Meaning |
|---|---|
| `safetyHours` | Hours of food a destination castle must keep |
| `sourceSafetyHours` | Hours a source castle must retain after donating |
| `minimumShipmentSize` | Floor below which a shipment is not worth sending |
| `minimumSourceReserve` | Absolute floor a source keeps |
| `minimumCoinReserve` | Coin floor for shipment costs |
| `autoKingdomTransport` | Allow cross-kingdom transport |
| `useKingdomTimeSkips` + `allowedTimeSkips` + `timeSkipReserve` | Whether and which time skips may speed transports, and their reserves |
| `checkIntervalSec` / `stateRefreshIntervalSec` / `logisticsRefreshIntervalSec` | Pacing |

## Wake triggers

Domains: `currencies`, `kingdom-transport`, `market`, `movements`, `resources`,
`units`. Section: `automation.autoFoodBalance`.

## Decision ladder

1. **Not configured** — `waiting`.
2. **Official game data unavailable** — `waiting`.
3. **Leased market barrows still out** — `waiting` for them to return before
   refreshing logistics.
4. **Logistics stale** — `resource.logistics.refresh`.
5. **Food production unknown at a castle** — `game.focus_castle` to read it.
6. **All castles above their reserve** — `idle`.
7. **An in-flight shipment already protects the castle** — `waiting`.
8. **No safe source available** — `waiting`.
9. **Food state stale at a castle** — `game.focus_castle` to refresh.
10. **Same-kingdom shipment possible** — send it.
11. **Kingdom resource destination not refreshed** — `waiting`.
12. **Cross-kingdom shipment possible** — `resource.ship` by kingdom transport.

## Guards

- **Two-sided safety.** A shipment must leave the *source* above
  `sourceSafetyHours` and `minimumSourceReserve`, and bring the *destination*
  above `safetyHours`. A castle cannot be drained to save another.
- **In-flight awareness.** A castle with a shipment already inbound is skipped,
  so the feature never double-sends into the same deficit.
- **Barrow leases.** Market barrows are leased state
  (`Server/State/MarketBarrowLeases.go`). The feature waits for leased barrows
  to return before refreshing, so it never plans against barrows it has already
  committed.
- **Minimum shipment size.** Avoids a stream of tiny transfers.
- **Coin reserve.** Shipments cost coin; the floor is respected.
- **Time-skip allowlist.** Only skips in `allowedTimeSkips` may be used, and
  never below `timeSkipReserve`.

## Shared machinery

`resource.logistics.refresh` and the barrow-lease model are shared with
[Auto Sceat Resources](AutoSceatResources.md), which is why both features
declare the `market` and `kingdom-transport` wake domains and both wait on
leased barrows.

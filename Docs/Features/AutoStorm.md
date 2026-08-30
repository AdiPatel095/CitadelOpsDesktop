# Auto Storm

The largest feature by far (~2,000 lines). Runs the Storm islands event
end-to-end: build out the storm base, import troops, buy from the aquamarine
shop, and attack forts and islands in a configured priority order.

Source: `Server/Automation/AutoStormPolicy.go`, `Server/App/StormIntents.go`.

## Identity

| | |
|---|---|
| Policy ID | `autoStorm` |
| Enabled key | `auto_storm` |
| Priority | 10 (background) |
| Config section | `automation.autoStorm` |

Its declared `WakeSections()` is `decorations.presets` — the decoration preset
used for the base layout. The main `automation.autoStorm` section is read
directly at evaluation time.

## Settings

Storm has the richest configuration of any feature; it is split into
sub-sections.

| Group | Fields |
|---|---|
| Top level | `target`, `decorationPresetCastleId`, `decorationPresetId`, `targetPriority`, `combatOrder`, `checkIntervalSec`, `mapRefreshIntervalSec`, `dailyAttackLimit`, `horseTravelBoostId` |
| `build` | `allowPremium`, `allowDemolition`, `allowResourceTransport`, `allowTimeSkips`, `resourceReserves`, `timeSkipReserve` |
| `harbor` | `enabled`, `targetLevel` |
| `forts` | `enabled`, `levels`, `minimumWins`, `presetId` |
| `islands` | `enabled`, `resources`, `sizes`, `presetId`, `defenseUnits` |
| `troopImport` | `enabled`, `donorCastleIds` |
| `aquamarine` | `reserve`, `shopTableId`, `purchases` |

Each `enabled` flag is an independent switch — the harbor, forts, islands,
troop import, and shop sub-features can each be turned off without disabling
the feature.

## Wake triggers

Domains: `attacks`, `buildings`, `castles`, `construction-items`,
`construction-offers`, `inventory`, `map`, `movements`, `reports`, `resources`,
`storm`, `units`, `kingdom-transport`.

The breadth reflects how much of the game Storm touches — it is the only
feature that both builds and attacks.

## Main activities

1. **Base build-out** — construct and upgrade the storm base toward the
   decoration preset, optionally demolishing, transporting resources, and
   applying time skips.
2. **Harbor** — upgrade to `harbor.targetLevel`.
3. **Troop import** — pull troops from `donorCastleIds` via kingdom transport,
   then confirm and optionally time-skip the transfer.
4. **Aquamarine shop** — `storm.shop.purchase` against the configured shop
   table, respecting a currency reserve.
5. **Combat** — `storm.attack` against forts and islands in `combatOrder` /
   `targetPriority` order.

## Decision ladder (combat and transport tail)

- **Shop purchase available** — `storm.shop.purchase`.
- **Attack ready** — `storm.attack`.
- **Arriving troop transfer** — `troops.kingdom.refresh` to confirm it and
  refresh capacity.
- **Transfer skippable** — `troops.kingdom.skip` to apply a time skip.
- **Transfer in flight** — `waiting`, with remaining seconds.

## Guards

- **Reserves everywhere.** `build.resourceReserves`, `build.timeSkipReserve`,
  and `aquamarine.reserve` all act as floors that the feature will not spend
  below.
- **Opt-in destructive actions.** `allowPremium`, `allowDemolition`,
  `allowResourceTransport`, and `allowTimeSkips` all default off. Demolition in
  particular is irreversible, so it never happens unless explicitly enabled.
- **Fort minimum wins.** `forts.minimumWins` prevents attacking a fort tier
  until enough wins have been banked at the current tier.
- **Level and size filters.** `forts.levels`, `islands.sizes`, and
  `islands.resources` restrict targeting to what the user configured.
- **Storm-specific dialog availability.** The CRA send guard calls
  `stormAttackDialogUnavailable` and refuses to launch when the dialog reports
  the storm target unavailable — plus the ordinary `stormTargetCooldownRemaining`
  cooldown check.
- **Daily attack limit.** Verified as a plan step at launch.
- **Transfer confirmation.** An arriving troop transfer is confirmed against the
  game's own capacity reading before the troops are counted as available.

## Note

Given its size, this doc is deliberately a map rather than an exhaustive trace.
For a specific behaviour, start from `AutoStormPolicy.go` and the
`autoStormSettings` sub-struct that governs it — the sub-structs partition the
file cleanly.

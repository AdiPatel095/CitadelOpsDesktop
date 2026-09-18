# Auto Storm

The largest feature by far (~2,000 lines). Runs the Storm islands event
end-to-end: build out the storm base, import troops, buy from the aquamarine
shop, and attack forts and islands in a configured priority order.

Source: `Server/Automation/AutoStormPolicy.go`,
`Server/Automation/AutoStormBuildPhases.go`, and `Server/App/StormIntents.go`.

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
`construction-offers`, `inventory`, `storage`, `map`, `movements`, `reports`,
`resources`, `storm`, `units`, `kingdom-transport`.

The breadth reflects how much of the game Storm touches — it is the only
feature that both builds and attacks.

## Main activities

1. **Base build-out** — reconcile the captured Storm target through the strict
   phase order below. Each blocked phase waits without spending on a later
   phase.
2. **Harbor** — upgrade to `harbor.targetLevel` or the captured Harbor level.
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

## Builder phase order

The build lane recomputes these phases from each fresh authoritative state and
emits at most one mutation before evaluating again:

1. Upgrade the configured or captured Harbor. Harbor is never selected for
   cleanup. Premium Harbor levels still require `allowPremium`.
2. Upgrade observed and captured Storehouses through the highest official
   level at or below level 7. An already higher Storehouse is preserved and is
   never downgraded or removed.
3. Buy every captured expansion in official level order. If the next expansion
   is unaffordable, the lane waits or uses explicitly enabled resource
   transport. Capacity prerequisites are also restricted to Storehouse level 7
   or below.
4. For exact targets, store or explicitly demolish non-target objects. Invalid
   or overlapping target geometry blocks cleanup. Non-exact targets preserve
   unmanaged objects.
5. Move retained target objects to captured positions, one confirmed move at a
   time.
6. Place target decorations that are confirmed in ordinary decoration storage.
   The lane counts placed and stored multiplicities. A fresh storage snapshot
   proving an item absent produces an amber warning and skips that decoration;
   unknown or stale storage is refreshed instead. Storage changes and a bounded
   refresh retry make skipped decorations eligible later.
7. Establish every requested Cargo ship at level 1 before upgrading any Cargo
   ship, then finish Cargo and remaining target upgrades. Cargo definitions use
   the game's decoration ground type, but are retained as build targets in all
   capture modes.

A Harbor-only configuration performs the Harbor and existing Storehouse phases
without creating an implicit full-castle target. Captured coordinates remain
authoritative; missing decorations do not become Cargo slots.

## Guards

- **Reserves everywhere.** `build.resourceReserves`, `build.timeSkipReserve`,
  and `aquamarine.reserve` all act as floors that the feature will not spend
  below.
- **Opt-in destructive actions.** `allowPremium`, `allowDemolition`,
  `allowResourceTransport`, and `allowTimeSkips` all default off. Demolition in
  particular is irreversible, so it never happens unless explicitly enabled.
- **Strict phase gates.** A disabled permission, insufficient balance,
  occupied queue, unavailable prerequisite, or stale inventory leaves the
  builder waiting in the current phase. It does not fall through to cheaper
  later work.
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

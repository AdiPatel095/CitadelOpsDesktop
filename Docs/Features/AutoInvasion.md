# Auto Invasion

Farms invasion events (Foreign Lords, Bloodcrow) by scanning for eligible
invasion castles around a source castle and attacking them until a score target
is reached.

Source: `Server/Automation/AutoInvasionPolicy.go`, `Server/App/InvasionIntents.go`.

## Identity

| | |
|---|---|
| Policy ID | `autoInvasion` |
| Enabled key | `auto_invasion` |
| Priority | 10 (background) |
| Config section | `automation.autoInvasion` |

## Settings

| Field | Meaning |
|---|---|
| `sourceCastleId` | Castle attacks launch from |
| `presetId` | Attack formation |
| `foreignLordsDifficultyId` / `bloodcrowDifficultyId` | Difficulty per invasion type |
| `scoreTarget` | Stop once event score reaches this |
| `minimumRemainingSec` | Stop launching when the event is nearly over |
| `dailyAttackLimit` | Server daily attack cap |
| `mapRefreshIntervalSec` | How stale an invasion map scan may be |
| `horseTravelBoostId` | Travel boost |
| `fortifyCurrency` | Currency used for fortification |

## Wake triggers

Domains: `attacks`, `map`, `movements`, `commanders`, `units`, `events`,
`event-scores`, `invasion`, `achievements`, `player-protection`. Section:
`automation.autoInvasion`.

## Decision ladder

1. **Protection Mode preparing or active** — `protected`; attacks stand down.
2. **Difficulty not selected** — `invasion.difficulty.select`.
3. **Score target reached** — `complete`.
4. **Event nearly over** — `idle`, no new attacks.
5. **No commanders assigned** — `waiting`.
6. **Map scan stale** — `invasion.map.scan` around the source castle.
7. **No eligible invasion castle in the latest scan** — `idle`.
8. **Preset inventory cannot be calculated** — `waiting` with the reason.
9. **Ready** — `invasion.attack` against the chosen castle.

## Guards

- **Protection Mode.** Like Auto Bird, the feature refuses entirely while
  Protection Mode is preparing or active.
- **Event window.** `minimumRemainingSec` stops new launches before the event
  ends, so troops are not committed to a hit that lands after scoring closes.
- **Score target.** A hard stop, checked before anything else costly.
- **Commander assignment.** Refuses when no commander is assigned to the feature.
- **Preset capacity.** Refuses when the source castle cannot field the preset.
- **Daily attack limit.** Verified as a plan step at launch.
- **Map freshness.** Targets are re-scanned rather than attacked from a stale
  observation.

## Failure behaviour

With Auto Towers, failed Auto Invasion operations always take the full 30 s
retry floor (`Coordinator.go:678`).

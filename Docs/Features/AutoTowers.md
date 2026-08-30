# Auto Towers

Maintains a queue of kingdom tower targets and attacks them in rotation,
refreshing cooldowns and rotating stale targets back into the queue.

Source: `Server/Automation/AutoTowerPolicy.go`, `Server/App/TowerIntents.go`,
`Server/App/TowerQueueIntents.go`.

## Identity

| | |
|---|---|
| Policy ID | `autoTowers` |
| Enabled key | `auto_towers` |
| Priority | 10 (background) |
| Config section | `automation.autoTowers` |

Auto Towers is the only policy that implements
`ConfigurationDerivedStatePolicy`: its target queue is materialized into state
from settings, so when `automation.autoTowers` changes the coordinator
invalidates and rebuilds that queue before re-evaluating
(`ResetConfigurationDerivedState`).

It is also one of two policies given a bounded operation context —
`autoTowerIntentTTL` of 2 minutes (`Coordinator.go:24`) — so a tower operation
cannot occupy the lane indefinitely.

## Settings

| Field | Meaning |
|---|---|
| `castles` | Tower castles to work, with their per-castle configuration |
| `checkIntervalSec` | Pacing |
| `mapRefreshIntervalSec` | How stale a tower observation may be |
| `dailyAttackLimit` | Server daily attack cap |
| `horseTravelBoostId` | Travel boost |

## Wake triggers

Domains: `attacks`, `commanders`, `map`, `movements`, `tower-cooldowns`,
`tower-queue`, `units`. Section: `automation.autoTowers`.

## Decision ladder

1. **No tower castles configured** — `waiting`.
2. **Unsupported horse travel boost** — `waiting`.
3. **No commanders assigned** — `waiting`.
4. **Cooldown refresh needed after a tower battle** — `map.query` at that
   position.
5. **Tower map stale** — `tower.queue.scan` to rebuild the complete map around
   the castle.
6. **Queued target needs verification** — `tower.queue.target.refresh`, which
   re-pings the target and rotates it to the back of the queue if it is
   unchanged.
7. **Ready** — `tower.attack` against the queued target.
8. **Otherwise** — `waiting` / `idle`.

## Guards

- **Post-battle cooldown refresh.** After a tower battle the target's cooldown
  is re-read before it can be queued again. `TowerCooldowns[key].PendingCooldownRefresh`
  marks the window, and the CRA send guard refuses to launch at a tower awaiting
  that refresh.
- **Cross-feature settlement guard.** The CRA dependency resolver refuses to
  build an attack when *any* feature has a prior Auto Towers attack on the same
  target awaiting settlement (`CommandContexts.go:200`) — this is the one guard
  that reaches across features by name, and it prevents double-committing a
  tower.
- **Commander required.** A tower CRA without an identified commander is
  rejected by the send guard.
- **Rotation.** An unchanged target rotates to the back rather than being
  hammered, which spreads attacks across the queue.
- **Operation TTL.** 2-minute ceiling on any single tower operation.
- **Daily attack limit.** Verified as a plan step at launch.

## Failure behaviour

Auto Towers is one of two policies (with Auto Invasion) whose failed operations
always take the full 30 s retry floor regardless of the decision's own
`NextCheckAt` (`Coordinator.go:678`) — tower failures are usually structural
rather than transient, so fast retries are not useful.

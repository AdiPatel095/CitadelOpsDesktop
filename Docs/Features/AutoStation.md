# Auto Station

Evacuates troops to protected alliance castles when a player attack is incoming,
and recalls them once the threat has passed. This is the defensive feature every
other combat feature yields to.

Source: `Server/Automation/AllianceStationPolicies.go:139`.

## Identity

| | |
|---|---|
| Policy ID | `autoStation` |
| Enabled key | `auto_station` |
| Priority | 90 (`PriorityAutoStation`) — the highest automation band |
| Config section | `automation.autoStation` |

The high priority is deliberate: when an attack is inbound, evacuating troops
outranks every economic and farming feature for claims and dispatch slots.

## Settings

| Field | Default | Meaning |
|---|---|---|
| `leadTimeSec` | 60 | How long before impact the evacuation window opens. Clamped to 60–3600. |
| `recallWhenClear` | true | Recall troops once no attack remains |
| `minRPTDays` | 3 | Minimum remaining protection days for an alliance castle to count as a safe target |
| `openGateFallback` | false | Open gates when Protection Mode blocks stationing |
| `settings` | — | Per-castle unit reserves that stay home |

## Wake triggers

Domains: `alliance`, `movement-snapshot`, `movements`, `player-protection`,
`stationing`, `units`. Section: `automation.autoStation`.

`movements` and `movement-snapshot` are the important ones — an incoming attack
appears as a movement, so the feature wakes on the frame that reveals the threat.

## The loop

```
no threat ──▶ armed, watching movement snapshots
    │
threat detected
    │
    ├─ Protection Mode active? ──▶ open-gate fallback path
    ├─ alliance roster stale?  ──▶ refresh it first
    ├─ impact further out than leadTimeSec? ──▶ wait, wake at the window
    └─ inside the window ──▶ station troops to the nearest protected holding
                                   │
                          attacks land / expire
                                   │
                             recall troops home
```

## Decision ladder

1. **Player-protection refresh required** — refresh first; every other decision
   is capped at the next protection refresh.
2. **Threat + Protection Mode** — stationing is suppressed by the game. Either
   open gates (if `openGateFallback` and the gate period covers the final
   impact) or report why not.
3. **Threat + stale alliance roster** — `alliance.refresh` before choosing a
   destination.
4. **Threat + no protected targets** — refresh and report.
5. **Threat, impact beyond the lead time** — `threat`, wake when the window opens.
6. **Threat, inside the window** — `troops.station` per threatened castle, to
   the nearest protected holding, skipping castles already stationed.
7. **Threat, already protected** — nothing to move.
8. **No threat, troops still out** — wait for a fresh movement snapshot
   (`game.refresh_movements`), then `movement.recall`.
9. **Idle** — `armed`, monitoring snapshots.

## Guards

- **Fresh movement snapshot before recall.** The policy explicitly refreshes
  movements and refuses to recall until the snapshot postdates the request, so
  troops are never recalled based on a stale view that has since gained a new
  incoming attack.
- **Final-impact tracking.** Troops stay stationed until the *last* observed
  attack has landed, not the first.
- **Protection days.** A destination must have at least `minRPTDays` remaining,
  so troops are not parked somewhere about to lose protection.
- **Reserves.** Per-castle unit reserves never leave.
- **Already-tracked stations.** A castle with an active tracked station is
  skipped rather than double-stationed.
- **Open-gate is conditional.** The fallback only fires when the game-reported
  gate duration actually covers the final incoming impact; otherwise it reports
  that it cannot help rather than opening gates pointlessly.

## Interaction with other features

Auto Khan, and any feature using `incomingThreats`, stands down while Auto
Station is active — both on an incoming player attack and while stationing
movements are in flight. That check is in the other feature's lane context and
in its in-plan guard, not here.

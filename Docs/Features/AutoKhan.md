# Auto Khan

Runs the Khan camp loop the way a player does it by hand: attack the Khan camp
continuously, skip the cooldown after every landed report, taunt the moment the
rage bar fills, and restock the wall after every retaliation lands.

Source: `Server/Automation/AutoKhanPolicy.go`, `Server/App/AutoKhanIntents.go`,
`Server/Khan/`.

## Identity

Four independent lanes, one feature. All share the enabled key, schedule, and
actor, so the user sees and controls one thing.

| Lane | Policy ID | Purpose |
|---|---|---|
| Attack | `autoKhan` | The chain attack, plus event-level safety |
| Cooldown | `autoKhan:cooldown` | Report-linked time skips |
| Rage | `autoKhan:rage` | The taunt |
| Defense | `autoKhan:defense` | Post-taunt wall restock |

| | |
|---|---|
| Enabled key | `auto_khan` |
| Schedule key | `autoKhan` |
| Actor | `automation:autoKhan` (all four lanes, via `ActorID()`) |
| Priority | 35 (`PriorityAutoKhan`) |
| Config section | `automation.autoKhan` |
| Event | Nomad event id 72, camp type id 35 |

> The lanes **must** declare `ActorID()`. Without it the coordinator would
> attribute `autoKhan:rage` to an actor the priority table does not recognise
> and its operations would run at background priority (10).

## Why four lanes

The four activities are asynchronous and none may block another. The coordinator
runs one operation per lane, so anything sharing a lane is serialized — which is
why the taunt has a lane to itself. They are kept independent at the intent
layer too, through claim naming (see Guards below).

The one thing that stops all four is the wall guard.

## The loop

```
attack ──lands──▶ battle report ──▶ cooldown lane skips the cooldown ──▶ attack ...
   │
   └── each landed hit generates rage ──▶ bar fills ──▶ rage lane taunts
                                                          │
                              Khan attacks the main castle │
                                                          ▼
                                        defense lane restocks the wall
                                        (the landing attack also grants rage)
```

## Settings

`autoKhanSettings` (`AutoKhanPolicy.go:33`). Key fields:

| Field | Meaning |
|---|---|
| `sourceCastleId` | Great Empire castle the chain attacks from |
| `attackPresetId` / `defensePresetId` | Attack formation, and the wall preset restocked after each taunt |
| `attackLaunchesEnabled` | When false, blocks only automatic `khan.attack` launches while map, cooldown, rage, and defense maintenance stays active for manual chains |
| `triggerRage` | When false, blocks automatic `khan.taunt` dispatch while attacks and cooldown handling continue |
| `skipCooldowns` | Enables the cooldown lane at all |
| `timeSkipReserve` | Per-skip-item reserves the lane will not spend below |
| `openGateProtection` + `offensiveUnitThreshold` | The wall guard |
| `nomadPointThreshold` | Hard stop once event points reach this |
| `dailyAttackLimit` | Server daily attack cap |
| `horseTravelBoostId` | Travel boost applied to launches |
| `replenishDefenseTools` | Buy missing wall tools from the shop |
| `maxRageChain` | Authoritative retaliation count at which new camp attacks stop; `0` is unlimited and the taunt lane always continues |
| `requireActiveRageBooster` | Require active `boi` booster ID 27 before a new automatic camp attack |
| `checkIntervalSec` / `defenseRefreshIntervalSec` / `mapRefreshIntervalSec` | Pacing, all default 30 s |

## Wake triggers

| Lane | Domains |
|---|---|
| Attack | achievements, attacks, boosters, commanders, currencies, defense, events, event-scores, inventory, khan, map, movement-snapshot, movements, nomad-camps, resources, stationing, units |
| Cooldown | currencies, defense, events, event-scores, khan, map, movements, nomad-camps, stationing |
| Rage | defense, events, event-scores, inventory, khan, map, movement-snapshot, movements, stationing |
| Defense | defense, events, event-scores, inventory, khan, map, movements, stationing |

The rage lane declares `UrgentWakeDomains() = ["khan"]`, so a rage update
bypasses the 250 ms coalescing window. The `rpr` frame that reports a full bar
is pushed by the game unsolicited, so the taunt evaluates on the frame that
reports it rather than up to 250 ms later.

## Shared lane context

All three async lanes first run `autoKhanAsyncLaneContext`
(`AutoKhanPolicy.go:760`), which refuses in this order:

1. Settings missing, or no source castle / attack preset / defense preset
2. Negative time-skip reserves; open-gate protection without a positive threshold
3. Official game data unavailable
4. Source castle not an available Great Empire castle; main castle unavailable
5. Nomad event not active
6. Defense preset fails to decode
7. **Nomad point threshold reached** — the protection lane is stopping the feature
8. **Incoming player attack** — yields to Auto Station (2 s)
9. **Auto Station is moving troops** — yields (2 s)
10. **Protection lock active** — the soft disable is engaged
11. **Main castle gates are open** — paused until they close
12. **Offensive wall units at/over threshold** — waiting for gate protection

Conditions 7–12 are the safety envelope. Everything below them is normal work.

> Note that 8 and 9 are *player* attacks and Auto Station activity — the Khan's
> own retaliation does not trip them (`State.IsIncomingPlayerAttack` requires a
> player-owned source castle).

## Decision ladders

### Attack lane (`autoKhan`)

In order: protection lock states → point-limit stop → event expiring → incoming
player attack / stationing yields → gates open → **wait for the defense lane to
restock after a taunt** → defense stale, refresh → defense tools missing,
replenish → offensive units over threshold, open gates and lock → apply defense
preset → unsafe arrival order, stop → target not found, map jump → target
observation stale, refresh → cooldown pending (defer to cooldown lane, or skip it
inline when no report exists yet) → live cooldown wait or skip → automatic
attacks disabled → maximum accepted rage-chain count → daily attack limit →
commander availability → uncommitted skips available → preset copies/capacity
→ required Rage booster → **launch** (`khan.attack`).

When `attackLaunchesEnabled` is false, the lane still locates and refreshes the
camp and clears eligible cooldowns, then stops before commander selection and
`khan.attack`. This lets the other lanes maintain a manually driven chain.

### Cooldown lane (`autoKhan:cooldown`)

Skips disabled → no confirmed report yet → re-ping the camp for the report's MSD
→ camp observation stale, re-ping → cooldown already clear, resolve the reports
→ no skip available above reserves → **apply one report-linked time skip**
(`nomad.cooldown.minute_skip`).

### Rage lane (`autoKhan:rage`)

Target not located → **taunt due, dispatch** (`khan.taunt`) → active taunts,
track to resolution → idle, watching rage. The attack lane's rage-chain
threshold never blocks this lane.

The taunt cursor stores both total rage and the authoritative event occurrence.
This lets the first full bar of a new Nomad occurrence fire even when its total
rage is equal to or lower than the previous event, while a same-occurrence rage
correction cannot reopen an already accepted fill.

The rage-chain count is `EventActivityState.KhanDefense.Launches`: incoming
retaliation movements actually published by the game. `khan.taunt` keeps its
lane claim while `lta` waits for the correlated committed `gam`, and advances
the occurrence-scoped rage cursor only after code-zero acceptance or the same
movement is observed independently. A rejection or dropped response leaves the
fill eligible for retry. At the configured count, only new camp attacks stop;
the taunt, cooldown, and defense lanes continue chaining for the rest of the
occurrence.

When `triggerRage` is false, the lane remains idle and never submits
`khan.taunt`; the attack and cooldown lanes continue independently.

### Defense lane (`autoKhan:defense`)

Defense observation is newer than the last taunt boundary → idle. Otherwise
**reapply the preset** (`defense.preset.apply`). The boundary is the later of
the last accepted taunt and the last taunt resolution.

## Intents and claims

| Intent | Claims |
|---|---|
| `khan.attack` | `attack-context`, `attack-inventory:<src>`, `khan-lane:attack`, `commander:<id>`, `khan-target:<k>:<x>:<y>`, plus `castle-focus` **only when the source castle is not the main castle** |
| `nomad.cooldown.minute_skip` | dungeon skip claim, `account-resources`, `khan-lane:cooldown` |
| `khan.taunt` | `khan-lane:taunt` |
| `defense.preset.apply` | `castle-focus`, `defense:<castle>`, `khan-lane:defense` |
| `khan.open_gate` | defense claims, `khan-protection`, `khan-lane` |
| `khan.point_limit.protect` | `khan-protection`, `khan-lane`, `castle:<main>` |
| `khan.protection.clear` | `khan-protection`, `khan-lane` |
| `khan.cooldown.reports.resolve`, `khan.map.jump`, `map.query` | read/refresh claims |

**The claim design is the whole concurrency story.** Each lane claims
`khan-lane:<lane>`, which collides only with itself. The three protection
intents claim bare `khan-lane`, which wildcard-matches every lane at once. So
the four lanes run concurrently, and the safety path still excludes all of them.

`castle-focus` appears only where focus genuinely moves. Chaining from the main
castle leaves focus where the rest of the loop already needs it; a separate
attack castle takes the lock while it switches.

## Guards

### In-plan (level 4)

`khan.lane.guard` (`AutoKhanIntents.go:503`) is a step inside every lane's plan,
immediately before its effect. It reads **live** state, not the planning
snapshot, and refuses when the main castle is missing, a player attack is
incoming, Auto Station is moving troops, gates are open, the protection lock is
active, the point threshold is reached, or the defense preset would put too many
offensive units on the wall.

The defense preset plan runs it repeatedly — before the refresh, and again
between each of the wall, moat, and keep commands.

This recheck, not the claim, is what actually enforces safety. Claims order
work; the guard decides whether the work is still a good idea.

### The wall guard (the soft disable)

The one condition that stops everything. When offensive wall units reach
`offensiveUnitThreshold`, the attack lane submits `khan.open_gate`, which opens
the main castle gates for six hours and sets the protection lock. Every lane
then refuses at shared-context step 10 until defense recovers, at which point
`khan.protection.clear` lifts it.

### Chain-specific

- **Unsafe arrival order** — arrival comparisons use the game wire's one-second
  movement precision, so sub-second local projection jitter is simultaneous.
  When a newly available faster commander could materially overtake the last
  in-flight hit, only the attack lane waits until that hit lands; taunt,
  cooldown, and defense continue, and historical error markers self-recover.
- **Daily attack limit** — verified again as a plan step at launch time.
- **Maximum rage chain** — the event-occurrence count is checked by the attack
  policy, the initial attack planner, and again by the deferred CRA resolver;
  it never gates taunt, cooldown, or defense intents.
- **Rage booster** — when required, active `boi` ID 27 is checked before policy
  admission, at attack admission, and again by the deferred CRA resolver. It
  never gates taunt, cooldown, or defense intents.
- **Committed skips** — the attack lane will not launch a hit it cannot cover
  with an uncommitted cooldown skip (`availableSkips - outstandingSkips`).
- **Preset capacity** — refuses when the source castle cannot field the preset.

## Known sharp edges

`khanAttackContext` (`AutoKhanIntents.go:328`) wraps two durable conditions —
camp on cooldown, and the authoritative dialog on cooldown — in
`Intent.ErrPlanStale`, which means "retry immediately". The planner reads the
**map** cooldown and the resolver reads the **attack dialog** cooldown, so when
those disagree the planner re-approves work the resolver rejects, and each retry
re-sends the `gbl`/`adi`/`gas` dependency chain. One operation reached 259
attempts in 65 seconds this way.

The engine now caps in-place stale replans at 3, which bounds it. The real fix
is for durable conditions to be plain planning failures so the policy reroutes
to the cooldown lane instead of retrying at all.

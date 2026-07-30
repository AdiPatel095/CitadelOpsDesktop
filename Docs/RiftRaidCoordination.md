# Rift Raid Coordination — Design Spec

> **Status: design, not implemented.** Nothing here is built. The event mechanics
> in Part A are read from official game data and are reliable. The feature design
> in Part C is a worked-through proposal, not a committed plan. Part D lists
> hard prerequisites; Part E lists what is still unknown and what each unknown
> blocks.
>
> Sources are tagged: **[data]** from `Data/GameData/Items/Items-v780.01.json`
> and the shipped language file, **[play]** from how the alliance actually plays
> it, **[inferred]** my reasoning from the other two.

Related: [`ArchitectureReview.md`](ArchitectureReview.md) §6.1 (headless
transport, which this depends on), [`Features/README.md`](Features/README.md),
[`IntentEngineCommandGuards.md`](../IntentEngineCommandGuards.md).

---

## Part A — Event mechanics reference

### Two events, not one **[data]**

| Event ID | Type | Role |
|---|---|---|
| **133** | `AllianceRaidbossEvent` (ARE) | The Rift Raid itself. Min level 70. |
| **137** | `AREAllianceMobilizationEvent` (ARME) | Alliance division/ranking layer added July 2026 |

Division names from `popup_arme_rewards_division_name_1..6`: **Copper, Glass,
Bronze, Silver, Gold, Diamond**. Steady-state round (`divisionRoundID 11`):
subdivisions 16/8/5/3/2/1, sizes unlimited/50/30/16/8/5, promote 8/5/3/2/1/0,
relegate 0/16/8/5/3/2. Diamond is a single subdivision of 5 alliances globally.

### The three bosses **[data]**

Internal names map to display names via `are_boss_name_*`:

| Internal | Display | Rarity | Levels | Wall regen | Courtyard | minPts for boss reward |
|---|---|---|---|---|---|---|
| `Necromancer` | **Mistress of Decay** | 2 | 20 | 1200s | 50k → 2.2M | 300 → 7,900 |
| `FungalSwarm` | **Mycelial Sovereign** | 1 | 20 | 180–300s | 100k → 3M | 300 → 13,000 |
| `LegendaryDragon` | **Ashen Tyrant** | 4 | 20 | 600s | 10k → 75k | 450 → 155,000 |

### Universal rules **[data]**

- **Courtyard is worth exactly 10× the wall.** `courtyardPointFactor` is 10×
  `wallPointFactor` on all 397 stages, without exception.
- **Points are proportional** to the share of the target area you defeat
  (`dialog_are_activityreward_tooltip`), not absolute per kill. A bigger
  courtyard is therefore strictly worse per attack.
- **The wall resets on stage boundaries.** Effect `444 raidBossWallRegeneration`
  fires at the start of most stages, in addition to the timer. Pushing the boss
  into its next health stage closes the current window early.
- **Three independent sections** — left wall, gate, right wall. Ladders debuff
  walls, Rams debuff the gate.
- Generals are per boss: 128–130 / 131–133 / 134–136.

### Section geometry **[data]**

Mistress of Decay stage-1 wall, representative levels:

| Level | Left | Gate | Right | Gate vs flank |
|---|---|---|---|---|
| 1 | 900 | 1,350 | 900 | 1.5× |
| 10 | 20,000 | 60,000 | 20,000 | 3.0× |
| 20 | 37,000 | 83,000 | 37,000 | 2.24× |

- Flanks are **mirrored totals with inverted composition** — left is ranged-heavy,
  right is melee-heavy. Choosing a flank is a counter-composition decision, not a
  cost one.
- **L14 and L18 are outliers** where the right flank is genuinely cheaper
  (L18: 24,600 vs 31,500, 22% less).
- Wall composition is **constant across stages** within a level.
- `wallBonus` == `gateBonus` and front defence == flank defence at every level,
  so sections differ only in unit count.

### Attacker suppression flips **[data]**

`offensiveRangeMalus` and `offensiveMeleeMalus` are mutually exclusive — the boss
suppresses one or the other, and which one changes by level.

| Boss | Pattern |
|---|---|
| Mistress of Decay | ranged −30→−60 (L1–11), then **melee** −60→−80 (L12–20) |
| Mycelial Sovereign | none (L1–13), melee −30/−40 (L14–15), ranged −40→−80 (L16–19), **melee −80 (L20)** |
| Ashen Tyrant | **never suppresses — all 20 levels clean** |

Mycelial alternates; L19→L20 flips from ranged −80 to melee −80, requiring the
opposite army one level apart. This is a lookup the tool should own.

Courtyard melee share also differs: **70% / 50% / 30%**. There is no single
"rift setup" — composition is per boss and per level.

### Per-boss gimmicks **[data]**

**Mistress of Decay — infection.** Defeated attacker units convert into her
garrison. Rate is **flat per level and constant across all six stages** (60% at
L10, 120% at L20) — there is no in-fight escalation. Countered by Decay Antidote
(Raw / Refined / Masterwork). Point factor is flat 1,000 at every level, so
climbing levels buys more work and more losses for identical point rate.

**Mycelial Sovereign — silent accumulation, then a cliff.** Every wall
regeneration adds dormant spores to the reserve. `mutateReserveUnit` fires only
at the **final health stage (17%)**, converting the entire accumulated stock at
once. Cullable while dormant via Sporebane tools.

**Ashen Tyrant — continuous conversion.** Lifecycle:

```
Dormant Rift-Egg → Dormant Rift-Wyrmling → Lesser Rift-Dragon → Great Rift-Dragon
   ↑ Magmatic          ↑ Magmatic              ↑ no tool           ↑ no tool
     Grenade Flask       Flintlock Gun
```

Each wall regeneration spawns a cohort; tools reach only the two **dormant**
stages. All culling tools are **"Refunded if unused"**, so carrying them is free
insurance. Because every regeneration spawns more brood, Ashen wants **fewer,
longer windows** — the opposite of the other two.

### Tool families **[data]**

| Family | Purpose |
|---|---|
| Ladder (Purging / Ascendant / Reclaimer / Tempered) | Wall (flank) debuff |
| Ram (same tiers) | Gate debuff |
| Decay Antidote (Raw/Refined/Masterwork) | Reduce infection — Mistress only |
| Sporebane Flask / Gun (Bronze/Silver/Gold) | Cull dormant spores — Mycelial only |
| Magmatic Flask / Gun (Bronze/Silver/Gold) | Cull eggs / wyrmlings — Ashen only |
| Spore\* / Obsidian\* regen tools | **+N seconds to regeneration cooldown** |
| Dominion Banner | Event currency boost |

### How it is played **[play]**

- One player breaks the wall; everyone else crams courtyard attacks into the
  open window. Mistress is medium difficulty of the three.
- **Open sections modify attack strength**: 1 section = debuff, 2 = neutral,
  3 = buff. The modifier depends on **which sections a given attack's troops
  travelled through unscathed**, not on what the alliance breached globally.
- Consequence **[inferred]**: fill your lanes to *exactly* the breached set.
  Fewer lanes self-inflicts a lower modifier; more lanes loses troops on intact
  wall and doesn't count anyway. Loss-minimising and buff-maximising coincide.
- Higher levels trend toward opening a single section, to save tools. Tools are
  the scarce commodity across all three bosses.
- Attack lanes map **directly** to wall sections — left fights left, centre
  fights centre, right fights right.
- The breach is effective essentially immediately; a **5-second buffer** after it
  gives room to recall if it fails. Following attacks want **1 second apart** so
  the server processes each cleanly.

---

## Part B — What exists today

Almost nothing. `RiftState` holds saved launch templates only:

```go
type RiftLaunch struct {
    ID, DisplayName   string
    CommanderID       CommanderID
    SourceX/Y, TargetX/Y, KingdomID
    WaveCount         int
    UseTravelFeather  bool
    OneWayTTSeconds   int      // learned from the movement response
    ...
}
```

Ingestion covers exactly one opcode — `cra`, for launch capture and ack
(`CoreReducers.go:88,133`). **There is no boss, wall, stage, or points state.**

Two useful pieces already exist:

- `riftReplayTiming` (`Server/App/AttackIntents.go:617`) — the land-at-T
  primitive. Give it a desired arrival, it returns launch time or refuses if the
  arrival is unreachable.
- Preset lanes map 1:1 onto wall sections, and carry tools as well as troops:

```go
type Wave struct { Left, Middle, Right Lane }   // up to 30 waves
type Lane struct { Troops []Slot; Tools []Slot }
```

Masking a lane therefore saves its tools too — which is where most of the
savings are, since tools are the scarce resource.

---

## Part C — Feature design

### Shape

A dedicated **scheduler account** — a headless instance in the alliance that
never attacks. It reads alliance-visible boss and wall state authoritatively,
collects private per-member data through a handshake, computes a global
schedule, and broadcasts each member their own array.

**Centralised planning, fully independent execution, centralised monitoring.**
Once an instance holds its array it launches without reference to any other
instance, so a scheduler outage does not stop the attack.

### Protocol

```
0  REGISTER    members announce to scheduler (account, alliance, instance id)
1  PRIME       leader triggers
               scheduler → members: boss target, preset, report your set
               members  → scheduler: per-commander { TT, riftEligible, troopsReady }
2  PLAN        leader picks T, section assignments, policy
               scheduler allocates landing slots
3  DISPATCH    scheduler → each member: [{ landAt, launchAt, commander, preset, lanes }]
4  EXECUTE     instances arm local schedules; local guards validate at dispatch
5  FEEDBACK    instances report observed ArrivesAt; scheduler detects drift,
               re-plans remaining waves
```

Priming is required because commander rosters and travel times are private
per-account — the scheduler account cannot see them.

### The gate chain

One scheduler for all three bosses. Only the chain differs.

```
Gate 1  BREACH    exclusive; must confirm before Gate 2 opens
Gate 2  CULL      ordered — cullers land in assigned sequence
Gate 3  OPEN      unordered free-for-all, fill remaining window
Gate 4  RESET     hard stop; nothing lands past it
```

Mistress and Mycelial have an empty Gate 2. The structural difference between
phases is **ordering strictness**: Gate 2 is order-enforcing (each tool's effect
depends on the reserve state the previous left), Gate 3 is order-indifferent
(only membership in the window matters).

### Recall cascade

Failure at gate N recalls everything still in flight scheduled for gates > N.

- breach fails → recall cullers **and** free-for-all
- culler lands out of sequence → recall remaining cullers; decide whether Gate 3 proceeds
- approaching reset → recall anything that would land past it

Recall is issued **by the owning instance** (`mcm` only operates on your own
movements), so the scheduler detects and instructs; the instance executes.

**Detect at T, do not poll.** The scheduler knows exactly when the breach should
land. One deliberate wall-state check at T leaves nearly the full 5-second buffer
for the recall round trip; a 2-second poll interval would consume 40% of it
before doing anything. Polling remains a background sanity check only.

Ordering priority — only 1 and 2 justify a recall:

1. Nothing lands before the breach *(hard — troops die on intact wall)*
2. Nothing lands after the window *(hard — same loss)*
3. Slot overlap between courtyard attacks *(soft — a processing nicety)*

### Allocator

Constraints:

- landing slots unique, 1s apart, within `[T+5, T+W]`
- per member, their own slots must map to launches **≥4s apart** (see Part D)
- rift-eligible commanders are a priority class — guarantee them slots
- courtyard lanes masked to exactly the breached section set

**The 4s floor is mostly solved by interleaving.** Allocating slots round-robin
across instances gives each member consecutive slots N seconds apart, where N is
the participant count — so with **N ≥ 4 the floor never binds**. 1-second spacing
is a property of the alliance, not of any instance.

**Travel-time diversity is an asset.** Commanders with different TTs produce
launch slack automatically:

```
commander A  TT=240s → land T+5 → launch T−235
commander B  TT=250s → land T+6 → launch T−244   (9s apart, floor satisfied)
```

`W` is an input, not a constant: `W_base + regen extensions applied`. Extension
spend and section-count spend compete for the same tool budget, both leadership
decisions, both changing the schedule.

### Two allocation objectives

The leader picks; they conflict on a tight window.

- **Maximise damage** — pack slots with whoever kills most
- **Maximise qualifying members** — preferentially slot players below their
  `minPointsForBossRewards` threshold so more people collect boss-defeat rewards

### Control plane

**Central proposes, local guards dispose.** The scheduler decides what to send,
when, with which commander and preset. The member instance validates against its
own hard limits immediately before dispatch and refuses if exceeded — the
`khan.lane.guard` pattern extended across the network. A stale or misconfigured
scheduler cannot dump someone's army, and the system degrades safely.

| Leader / scheduler | Member instance (hard caps) |
|---|---|
| target boss + level, breach time T | max commanders committed |
| section assignments per breaker | max troops / tools committed |
| slots per player, waves per player | rift-set commanders usable? |
| preset per player class | allowed source castles |
| re-slot policy on recall | opt out / pause |
| points gating, priority classes | |

---

## Part D — Prerequisites

Ordered by what blocks what.

1. **Rift state ingestion.** Wall segment status, regen countdown, boss
   health/stage/level, and the **current-attacks panel**
   (`dialog_are_currentAttacks_title` proves it exists — it gives the scheduler
   authoritative visibility of every alliance attack in flight, far stronger
   than instance self-reporting). **Needs a live-event capture; these opcodes are
   not in any existing log.**

2. **Travel-time estimator.** Priming asks every member for per-commander times,
   which is only feasible if TT is *computed* — you cannot ask 50 players to fire
   probe attacks, and `adi` does not carry travel time. All inputs exist: 971/999
   units carry `speed`, coordinates are in state, `TravelSpeedResolver` already
   derives per-commander `AppliedPercent`, and `AttackTravelBoost` models boosts.
   Validate in shadow mode against observed `TT` on ordinary attacks before
   anything depends on it. **Critical path.**

3. **Attack-lane delay.** `RuntimePolicy.go:41` clamps `minAttackDelay` to a
   4-second floor and randomises to 6s. The floor is schedulable; **the
   randomisation is not** — you cannot compute a launch time when the router will
   hold the command for an unpredictable duration. Needs to become exact, or
   bypassable on the rift lane. This is a deliberate pacing control, so
   loosening it is a decision, not a fix.

4. **Minute rounding.** `riftReplayTiming` applies `roundUpUnixMinute`. This was
   introduced by us, not the game, and can go to seconds.

5. **Clock alignment across instances.** Single-instance timing self-corrects
   (the server reports actual `ArrivesAt`, so the first launch calibrates). Across
   instances it does not — NTP is sufficient for 1s slots but should be an
   explicit startup check.

6. **Trust boundary.** This turns a single-user desktop app into a networked
   multi-account system issuing launch commands to other people's instances.
   Pairing and auth belong in the design from the start.

---

## Part E — Open questions

| Question | Blocks |
|---|---|
| Does yield vary across the window? If the courtyard depletes, later slots may score differently. | Whether slot *order* matters, and where scarce rift commanders should be placed |
| Is the 1s spacing per-player or alliance-wide? | Whether a global slot allocator is required or per-instance staggering suffices |
| Does the return trip equal the outbound? | Wave cadence — `roundtrip = TT_out + TT_back` drives the entire wave count |
| Exact section-count modifier values (1 = debuff, 3 = buff) | The break-even between tool spend on breaching vs on window extension |
| Can the scheduler read per-member points from the alliance view? | Whether points-gated allocation needs a member report during priming |
| Does Mycelial need Sporebane culling in practice, or is it killed before the 17% cliff matters? | Whether Gate 2 is populated for that boss |

### Sizing note **[inferred]**

Window capacity is `floor(W / (TT_out + TT_back)) + 1` waves per commander.
Mycelial's 180s window is shorter than a typical 240s travel time, so **no round
trip is possible — one pre-launched wave is the entire contribution**, and
re-slotting after a recall is pointless there. Mistress's 1200s window supports
three or more waves and makes re-slotting worthwhile.

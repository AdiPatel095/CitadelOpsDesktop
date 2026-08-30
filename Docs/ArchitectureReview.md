# Architecture Review — Findings and Open Questions

> **Status: discussion input, with one decision recorded.** Sections 1–5 and 7
> are findings and options — nothing there is decided. The single locked
> decision is the headless transport in §6.1. Everything else in §6, including
> playing alongside a hosted bot, is explicitly undecided. Every "would" outside
> §6.1 is a proposal, not a plan.
>
> **Decision log**
>
> | Date | Decision | Status |
> |---|---|---|
> | 2026-07-26 | Headless transport (§6.1) | Locked in; not scheduled or started |
> | 2026-07-26 | Play alongside hosted bot (§6.2) | Under consideration, undecided |
> | 2026-07-26 | Rift Raid coordination (§6.3) | Spec written, undecided |
> | 2026-07-26 | Alliance Observer (§6.4) | Noted, undecided |
> | 2026-07-26 | Auto Builder (§6.5) | Noted, undecided |
> | 2026-07-26 | LLM control plane (§6.6) | Noted, undecided |

Reviewed against `codex/version-2.0.0` on 2026-07-25/26, prompted by the Auto
Khan taunt-latency work. Companion documents:

- [`Architecture.md`](../Architecture.md) — the intended 2.0 boundary
- [`IntentEngineCommandGuards.md`](../IntentEngineCommandGuards.md) — how commands and guards work today
- [`Docs/Features/`](Features/README.md) — per-feature behaviour and guards
- [`SecondSystemDesign.md`](SecondSystemDesign.md) — what a from-scratch build
  would change, and the incremental ordering to get there without one

---

## 1. Headline

The architecture is sound and worth keeping. The implementation has a small
number of expensive hot spots, and the guard model has three design-level gaps.

Those are different kinds of problem and deserve different treatment:

- **Performance items cost milliseconds and allocation churn.** Fixing them
  makes the app faster. They prevent no bugs.
- **Design gaps cost correctness.** Every significant bug found this week traces
  to one of them — not to the performance items.

---

## 2. What was measured

Apple M4, against the real state fixture used by the existing
`Server/State/Performance_test.go` and `Server/Intent/Performance_test.go`
benchmarks.

| Operation | Cost | Allocations |
|---|---|---|
| `ReadOnlyView()` / `PlanningView()` | **74–114 ns** | 0 B |
| `Snapshot()` (deep clone) | **5.8–6.5 ms** | 47,777 (14 MB) |
| `ApplyWithoutMapMutation` (per changing frame) | 1.08 ms | 15,027 (3.7 MB) |
| `Apply` incl. world map (per changing frame) | 5.8 ms | 47,801 (14 MB) |
| `Engine.Submit` (no durable store) | 4.8 µs | 47 |
| `Engine.Submit` through the outbound router | 8.0 µs | 60 |
| `Engine.Submit` with the SQLite operation store | **348 µs** | 164 |

Live instance during a Khan chain: **~10 state revisions/sec** sustained, CPU
8.6% idle rising to **47%** mid-chain.

Coordinator wake latency, before and after the urgent-wake change: **260 ms →
10 ms** (stable across repeated runs).

### Measurement caveat

The running process was **not profiled**. The 47% CPU figure is not attributed.
Everything in §3 is microbenchmarks plus call-site counting — strong enough to
act on for items 1 and 3, but a pprof capture during a live chain would settle
attribution properly and should probably happen before anyone invests in item 4.

---

## 3. Performance findings (implementation level)

All four preserve every guarantee documented in `IntentEngineCommandGuards.md`.

### 3.1 `Snapshot()` in read-only guards — largest measured win

46 call sites in `Server/App` use `State.Snapshot()`; 2 use `ReadOnlyView()`.
The deep clone is **~78,000× more expensive** than the read-only view and
allocates 14 MB per call.

`guardKhanLane` (`AutoKhanIntents.go:516`) uses it, and the defense preset plan
invokes that guard 4–5 times per operation — roughly **29 ms and 70 MB of
garbage per wall restock**, to read a handful of fields it never mutates.
`guardCRASend` and `guardDailyAttackLimit` do the same, so one `khan.attack`
burns an estimated 12–18 ms and ~40 MB on defensive copies.

- **Change:** convert read-only call sites to `ReadOnlyView()`.
- **Preserves:** everything. Guards are pure readers.
- **Risk:** `GameState` by value shares its maps, so each converted site must be
  confirmed never to write into one. Mechanical, reviewable per site, but it is
  46 sites and not a blind find-and-replace.

### 3.2 Claims held across the 4–6 s attack-lane sleep

`executeCommandDependencies` waits out `NextAllowed(LaneAttackLaunch)` *inside*
dependency resolution, while the operation already holds `attack-context`,
`attack-inventory`, the commander claim, its admission slot, and sometimes
`castle-focus`. Up to six seconds of exclusive holds doing nothing.

- **Change:** perform the lane wait at admission time, before claims are acquired.
- **Preserves:** the dependency chain still runs immediately before the `cra`,
  so the freshness guarantee (`IntentEngineCommandGuards.md` §5.4) is unchanged.
- **Risk:** low. Admission already consults lane availability, so this moves the
  wait to where the concept already lives.

### 3.3 Coordinator computes fingerprints before the cheap early-out

`Coordinator.go:278-279` builds both configuration fingerprints — string joins
over section JSON — for **every** policy on **every** evaluate, and only then
checks `now.Before(current.nextCheck)` to skip. 19 lanes × 2 fingerprints, at
minimum every 2 s plus every debounced state change.

- **Change:** `evaluatedConfigRevision` is already tracked. If
  `configuration.Revision` is unchanged, the fingerprint provably cannot have
  changed — skip computing it.
- **Preserves:** identical semantics.
- **Risk:** very low.

### 3.4 The operation store is 98.6% of engine overhead

`synchronous=FULL` with `SetMaxOpenConns(1)`, so all 19 lanes serialize on one
fsync'd connection, at roughly 10–15 writes per attack operation.

The guarantee that actually lives here is the **pre-dispatch `dispatching` write
for write and launch effects** — the crash-recovery record. Read-effect intents
do not need it, post-dispatch phases do not gate the wire, and `Get`/`Recent`
reads currently queue behind writes for no reason.

- **Change:** scope `FULL` to where the invariant lives; separate read
  connection; consider coalescing post-dispatch phase writes.
- **Preserves:** the crash-recovery guarantee, if scoped carefully.
- **Risk:** this is the one that needs a deliberate durability decision, not a
  mechanical edit. **Open question — see §6.**

### 3.5 Per-frame clone-on-write — recommend leaving alone

1.08 ms per changing frame is the cost of the immutable-generation design, and
that design is precisely what makes `ReadOnlyView()` free and planning
lock-free. The `WithoutMapMutation` split already recovers 5.4×. The remaining
lever is structural sharing per domain — a large change for a modest win.

---

## 4. Design-level gaps

These are not performance issues. Each one produced a real bug this week.

### 4.1 Claims have no shared/exclusive mode

Every claim is exclusive. There is no way to say "I observe this, exclude only
writers."

**What it caused.** `khan-protection` was a *read* guard held by four lanes, so
it serialized the entire feature. A full rage bar queued behind whatever was in
flight, which was the original complaint.

**Current workaround.** Per-lane claims `khan-lane:<lane>` plus a bare
`khan-lane` that wildcard-matches all of them, so protection intents can exclude
every lane at once. It works, but it encodes a read/write relationship in a
naming convention that nothing validates.

**Option.** Give `ResourceKey` a mode. "Readers observe, writers exclude"
becomes expressible directly and cannot be got wrong by choosing the wrong
string. Touches the claim manager's conflict rules and every plan's claim list.

### 4.2 `ErrPlanStale` conflates "retry" with "impossible"

One error means both "the world moved, rebuild and try again" and "this cannot
happen right now."

**What it caused.** A Khan camp cooldown — a durable fact — was reported as
stale, so the engine retried immediately, forever. One operation reached **259
attempts in 65 seconds**, re-sending the `gbl`/`adi`/`gas` dependency chain each
pass. The same pattern exists for preset shortages.

**Current mitigation.** `maximumStaleReplans = 3` bounds the loop
(`Engine.go:28`). It caps the damage; it does not fix the cause. Measured effect:
max attempts 259 → 17, runaway bursts gone.

**Option.** Split the taxonomy — `retry` (transient race) / `blocked` (durable,
reroute) / `failed`. A durable condition then fails once and the policy reroutes
to the time-skip lane instead of retrying at all. Would make the replan cap
redundant.

### 4.3 No channel for a resolver to teach the planner

The planner reads the **map** cooldown; the resolver reads the **attack dialog**
cooldown. When they disagree, nothing propagates the resolver's discovery back,
so the planner re-approves work the resolver has already rejected.

**What it caused.** The other half of the `adi` storm. The loop cannot converge
because neither side learns.

**Option.** Let a failed resolver record the fact it learned — either into state
or as a structured result the next planning pass consumes. Interacts with the
"planners must be pure" rule, so it needs care: the recording would have to
happen outside the planner, in the engine or an action.

### 4.4 Actor identity is an exact string match

`DefaultPriority` matches the actor suffix exactly. A lane named
`autoKhan:rage` produces `automation:autoKhan:rage`, matches nothing, and
silently runs at background priority (10) instead of 35.

**What it caused.** The taunt — the most latency-critical operation in the
feature — ran at the lowest priority in the system, losing every claim and
dispatch tie. Silent; no error, no log.

**Current mitigation.** All Khan lanes declare `ActorID()`. A test now pins it.

**Option.** A typed feature identity rather than a string suffix lookup would
make the failure unrepresentable rather than merely tested-for.

---

## 5. Cross-feature mechanism inventory

What each feature implements to run as intended. The point of this table is the
**duplication**: mechanisms implemented independently in many features are
candidates to become platform primitives — which is the main input to the
architecture conversation.

| Mechanism | Features implementing it | Today |
|---|---|---|
| **Freshness / staleness intervals** | Khan (defense, map), Nomad (camp), Towers (map), Storm (map), FoodBalance (state, logistics), Crafting (queues), TCI (inventory), Beri (capacity), Advisor (map) | Hand-rolled per feature; each has its own `*IntervalSec` setting and its own `observedAt` comparison |
| **Post-action confirmation re-read** | Khan, Nomad, Towers (`PendingCooldownRefresh`), Beri (capacity), Storm (transfer confirm), Hospital (queue) | Partly shared (`PendingCooldownRefresh` on cooldown state), partly per-feature |
| **Reserve floors** | Khan, Nomad, Advisor, Storm, FoodBalance, Crafting, Station, Bird | Hand-rolled; every feature has its own reserve shape |
| **Commitment accounting** (don't spend what in-flight work already claimed) | Khan (`outstandingSkips`), Nomad (uncommitted skips), FoodBalance (in-flight shipment), Crafting + FoodBalance (barrow leases) | Three different implementations of the same idea; leases are the only shared one |
| **In-flight de-duplication** | Station (`activeTrackedStation`), Bird (`birdReturnUnixMs`), FoodBalance (in-flight shipment), Towers (cross-feature settlement guard) | Four different approaches |
| **Capacity: computed vs read from game** | Recruit/Tool (computed from base + effect 189, skip when unknown), Beri (read), Hospital (read, distinguishes unknown) | Inconsistent — some compute, some ask |
| **Event window guards** | Khan, Nomad, Invasion, Advisor (`minimumRemainingSec`, `scoreTarget`) | Consistent pattern, duplicated code |
| **Opt-in destructive / premium** | Storm (`allowPremium`, `allowDemolition`, `allowTimeSkips`), Crafting (`allowRubyRecipes`, `useRubyOverflowSkip`) | Per-feature flags, no shared convention |
| **Yield to a higher-priority feature** | Khan → Station; Bird, Invasion → Protection Mode | Hard-coded by name in the yielding feature |
| **Soft disable / safety lock** | Khan (open gate + protection lock) | Only Khan has one |
| **Rotation / fairness** | Recruit (per-castle cursor), Towers (queue rotation) | Two implementations |
| **Arrival-order safety** | Khan chain, Nomad RBC trial | Duplicated |
| **Daily attack limit** | Khan, Nomad, Invasion, Towers, Storm | Shared (`appendDailyAttackLimitGuard`) — a good example of the pattern done right |
| **Live recheck before effect** | `khan.lane.guard`, `attack.cra.send.guard`, `defense.preset.verify` | Shared mechanism (guard actions), per-feature content — also done right |
| **Id-namespace discipline** | TCI (CID vs wodID), Beri (`wireCastleId`) | Documented in `AGENTS.md` and feature docs; not enforced by types |

**Reading of the table.** Two mechanisms are already platform primitives done
well — the daily attack limit guard and the in-plan live recheck. The rest are
implemented between two and nine times each, with slightly different semantics
every time. Freshness intervals, reserve floors, and commitment accounting are
the three biggest duplications and the most obvious candidates for shared
primitives.

**Not a recommendation yet.** Consolidating these is exactly the kind of change
worth discussing before committing, because each feature's variant may encode a
real difference rather than an accident. Section 6 lists the questions.

---

## 6. Future state

### 6.1 Headless transport — DECIDED (2026-07-26)

Run the binary headless, owning the game websocket directly, with no Chrome tab
and no game client.

**Status: locked in.** Not yet scheduled or started.

Why it is tractable: `Transport` (`Server/Session/Transport.go:39`) is already
an interface with two implementations — `ChromiumTransport` and
`ReplayTransport` — so a `DirectTransport` slots in beside them and nothing
above the seam changes. The engine, ingest pipeline, coordinator, and all 19
policies consume `RawFrame` and call `Send([]byte)`; none of them know what is
underneath. `Protocol.Encode`/`Decode` already owns the `%xt%` wire format in
both directions, and the real server URL is already captured.

What has to be built — the work the browser currently does for us:

1. Login and auth handshake, then the socket handshake and bootstrap frames.
2. Keepalive/heartbeat.
3. Reconnect and relog state machine. Today this is "reload the tab".
   Consumers are already prepared: `ConnectionGeneration` exists and the engine
   guards on it.
4. Replication of any value the game client computes locally.

Item 4 is the open risk and the reason no estimate is recorded here yet. See
§6.3.

Expected payoff: Chrome plus the game client is effectively the entire resource
footprint. Removing it also retires the `websocketTrafficTimeout` tab-reload
watchdog.

### 6.2 Playing alongside a hosted bot — UNDECIDED

Splits into two projects with very different costs. **Neither is decided.**

**6.2a — hand-over.** The hosted bot suspends automation while the user is
playing and resumes when they stop. Cheap once §6.1 exists, because the
primitive is already built and wired end to end: `directTrafficUntil` +
`AutomationLocked()` (`Server/Session/Controller.go:282`), the dispatch gate
returning `ErrAutomationLocked` (`Controller.go:293`), and the engine pausing
the operation at a safe step boundary and resuming it. This would be wiring an
existing mechanism to a new trigger.

**6.2b — true simultaneity.** Bot and human share one multiplexed socket via a
hosted relay. Requires §6.1 first: you cannot relay a socket the browser owns.

The hard part is not effort. The game exposes one focused castle, one attack
dialog, and one world-map context, so bot and human contend for the same
context by construction. The dispatch permit already makes this *safe* —
a focus change is detected and the plan is rebuilt — but the bot would replan
continuously while someone plays, and the human's client would receive
responses to commands it never issued. Also adds the user's round trip to the
hosted relay to every action, and changes the connection pattern for the
account.

Assessment: 6.2a captures most of the value. 6.2b is separable and deferrable.

### 6.3 Rift Raid coordination — SPEC WRITTEN, UNDECIDED

A multi-instance alliance scheduling feature for the Rift Raid event: a headless
scheduler account primes participating members for their per-commander travel
times, allocates global landing slots, and broadcasts each member an independent
launch array.

Full design in [`RiftRaidCoordination.md`](RiftRaidCoordination.md). **Not
decided and not scheduled.** It depends on §6.1 (the scheduler account is a
headless instance) and has its own hard prerequisites, chiefly a live-event
capture for the rift opcodes and a travel-time estimator accurate to the second.

### 6.4 Alliance Observer — NOTED, UNDECIDED

Watch for incoming attacks and for capture of alliance outposts, metropolises and
capitals; alert alliance-wide through a Discord channel that other instances can
also read for sync; optionally launch a clearing attack to wipe occupying forces.

Notes in [`AllianceObserver.md`](AllianceObserver.md). **Not decided, not
scheduled.** Cheaper than first assumed: `AllianceHolding` already carries
`SlotType`, so capture detection is a holdings diff rather than map polling.
Alliance-wide *incoming* alerts (as opposed to capture alerts) carry the same
multi-instance dependency as §6.3.

### 6.5 Auto Builder — NOTED, UNDECIDED

Per-castle building level targets, queued automatically as resources and build
slots free up. **Not decided, not scheduled.** The action layer is already
complete — 11 building intents including `building.upgrade`, `building.refresh`
and `building.skip_time`, with state for buildings, queue slots and resources,
and a catalog carrying the full `upgradeWodID` level chain with costs, durations
and player-level gates. What is missing is a target model, a chain resolver, a
policy, and UI — and there is no building UI in the client today, so that is
likely the bulk of the work.

Open design point: the ordering rule when several buildings are below target and
resources bind. Cheapest-first quietly starves expensive targets forever.

### 6.6 LLM control plane — NOTED, UNDECIDED

Attach a language model that can adjust settings in natural language, and later
take game actions. Notes in [`LLMControlPlane.md`](LLMControlPlane.md). **Not
decided, not scheduled.**

Cheaper than it sounds, because the intent engine is already a tool-use API that
was not labelled as one. `GET /api/v2/intents` enumerates 111 definitions
(29 read / 56 write / 20 launch / 6 external) each carrying name, description
and `Effect`; `POST /api/v2/intents/{name}` submits one; `Effect` is a ready-made
permission tier; and `Request.DryRun` (`Server/Intent/Engine.go:347`) already
plans in full and returns the plan *without dispatching*, which is exactly the
propose-then-confirm primitive this needs. A model proposes; the engine's guard
stack still decides — the same split as §6.3.

Two observations worth carrying forward:

- **A settings assistant is one tool, not 111.** Every configuration write goes
  through the single `config.update` intent, which is already claim-scoped per
  section and CAS-guarded via `UpdateConditional`. That makes a useful first
  release small and independent of the full tool-surface work.
- **Do not feed it `Snapshot()`.** The obvious "give the model current state"
  implementation lands straight on the §3.1 deep-clone path. Read-tier intents
  as on-demand tools, over a cached `Docs/Features/` prefix, is the cheaper
  shape.

The part that does not get easier with time is prompt injection: this would be
the first time attacker-controlled text (chat, mail, player and castle names)
enters a decision path. Launch tier should never auto-execute.

### 6.7 Open question gating the §6.1 estimate

Does any outbound payload carry a value computed by the game client — a
signature, nonce, or per-message token that cannot simply be replayed from a
server response?

If no, §6.1 is bounded protocol replication against frames we already capture in
`Data/Logs/channels/`. If yes, the difficulty changes materially. This is
answerable cheaply from existing captures and should be settled before any
scheduling estimate is given.

---

## 7. Open questions for the future-plans discussion

**Durability.** How much crash-recovery do we actually want? `synchronous=FULL`
on every write is the strictest option and costs 348 µs/op. Scoping it to
pre-dispatch writes on write/launch effects keeps the reconciliation guarantee
for the cases that can duplicate a game action. Is that the right line?

**Claim modes.** Is a shared/exclusive mode worth the change to the claim
manager and every plan's claim list, or is the `khan-lane` wildcard convention
good enough with a lint or test to enforce it?

**Error taxonomy.** Splitting `ErrPlanStale` touches every planner and resolver
that returns it. Do it as one sweep, or introduce the new kinds alongside and
migrate feature by feature?

**Resolver → planner feedback.** Does the discovered fact belong in state (which
makes planners see it naturally but adds writes on a failure path), or in a
per-operation side channel (cheaper, but invisible to other operations)?

**Mechanism consolidation.** For each of the three big duplications — freshness
intervals, reserve floors, commitment accounting — are the per-feature variants
meaningfully different, or accidental? Consolidating an accidental difference is
a win; consolidating a real one creates a leaky abstraction.

**Parallel policy evaluation.** The coordinator evaluates 19 lanes serially in
one goroutine, and one slow policy delays the rest. Policies are pure functions
of a snapshot, so they are trivially parallelizable — but the runtime
bookkeeping around them is mutable, and serial is far easier to reason about.
Worth doing, or a solution looking for a problem?

**Sequencing.** Items 3.1–3.3 are safe now and independent of any architecture
decision. Items 4.1–4.3 should probably wait for this discussion. Is there
appetite to do the safe performance work first, or to hold everything until the
design direction is settled?

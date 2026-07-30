# Second-System Design — How I Would Build This From Scratch

> **Status: design input, nothing decided, nothing scheduled.** This answers a
> specific question: given the measured behaviour in
> [`ArchitectureReview.md`](ArchitectureReview.md), the guard inventory in
> [`IntentEngineCommandGuards.md`](../IntentEngineCommandGuards.md), the
> per-feature behaviour in [`Features/`](Features/README.md), and the future
> features in §6 of the review — what would a from-scratch build look like?
>
> **It is not a proposal to rewrite.** §6 of this document is the part that
> matters operationally: every change here is reachable incrementally from the
> current code, and the ordering is the actual recommendation.

---

## 1. The headline

**The macro architecture survives.** Decision-in / plan-out, immutable state
generations, pure planners, claims + admission + lanes as separate concerns,
response barriers tied to effect phases — I would build all of that again, in
that shape, without hesitation. It is the reason the guard stack is
unbypassable, the reason 111 intents can be exposed to a language model safely,
and the reason a headless transport is a swap rather than a rewrite.

What I would change is **where the boundaries sit**. Almost every measured
performance problem and every future-feature blocker traces back to the same
root cause:

> **A correctness-critical distinction exists in the design, is documented in
> comments, and is not represented in the type system or the runtime — so it has
> to be maintained by hand at every call site.**

Four cases of that pattern account for most of the cost:

| # | The distinction | How it's enforced today | Cost |
|---|---|---|---|
| 1 | cheap read vs. expensive clone | a doc comment | 5.8–6.5 ms × 72 call sites |
| 2 | mutual exclusion vs. rate limiting | one `release()`, ~20 manual sites | claims held across a 4–6 s wait |
| 3 | durable record vs. telemetry | one SQLite table, `synchronous=FULL` | 348 µs/op, 98.6% of engine overhead |
| 4 | protocol vs. transport | one implementation | §6.1 and §6.2 both blocked on it |

A from-scratch build makes each of those a thing the compiler or the runtime
enforces. That is the whole rewrite, and it is worth more than any amount of
micro-optimisation.

---

## 2. What I would keep unchanged

Stating this first because it is the larger part of the answer.

**Decision-in / plan-out.** A `Policy` answers *"given this snapshot, what is
the single next thing to do?"* and returns a `Decision`. It cannot send, cannot
mutate, cannot block. The engine owns execution. This is the single best
decision in the codebase. It is why adding an LLM caller (§6.6) is a mapper over
two endpoints instead of a new safety story, and why 19 lanes across 16 features
can share one account without a tangle of feature-to-feature coupling.

**Immutable state generations with atomic pointer swap.** `generation.Load()`
returning an immutable state is what makes a read 74–114 ns and 0 B. Planning is
lock-free because of it. Keep exactly.

**Partition versioning as the staleness primitive.** Plans declare what they
read; the engine re-checks partition versions before dispatch. This is a better
design than optimistic-retry-on-everything and it is what makes guards able to
re-validate milliseconds before the wire.

**Claims, admission classes, and outbound lanes as three separate concerns.**
Mutual exclusion, fairness, and rate are genuinely three different problems.
Most systems collapse them into one queue and then cannot express "these two
things conflict but these two are merely both attacks."

**Response barriers (`wire` / `committed` / `wire-then-committed`).** This is
how "did the game actually accept it" is answerable at all, and it is what makes
`indeterminate` / `reconciliation_required` meaningful rather than a shrug.

**Per-frame clone-on-write in ingest.** 1.08 ms/frame is the *price* of the
immutable-generation design, and that design is what makes reads free. §3.5 of
the review already recommends leaving it alone. I agree — structural sharing per
domain is a large change for a modest win.

**SQLite for the operation store.** Right choice. It is used too strictly, not
wrongly chosen.

---

## 3. The four structural changes

### 3.1 Make the expensive read impossible to call by accident

**The finding that drives this:**

```go
func (store *Store) Snapshot() GameState     // 5.8–6.5 ms, 47,777 allocs, 14 MB
func (store *Store) ReadOnlyView() GameState // 74–114 ns, 0 B
```

`Server/State/Store.go:69` and `:79`. **They return the same type.** The only
thing distinguishing a 78,000× cost difference is a doc comment reading
*"Callers must not mutate the returned state."* There are **72 `Snapshot()` call
sites and 24 view call sites** in non-test server code.

This is not 46 individual mistakes. It is one type-design decision producing 46
predictable consequences. `PlanningView()` gets it right — it returns a distinct
`PlanningView` type — which is exactly why it is not the one showing up in the
profile.

**From scratch:**

```go
// The default. Read-only methods only. No way to mutate, no clone.
type ReadView interface {
    Castles() CastleIndex
    Movements() MovementIndex
    // ... accessors, all returning immutable or copied-scalar values
    Revision() uint64
}

func (store *Store) View() ReadView            // the ~96 call sites want this
func (store *Store) PlanningView() PlanningView // planners, carries partitions
func (store *Store) CloneForMutation() *GameState // rare, named to sting
```

Now a guard that only reads *cannot* pay for a clone, because the type it
receives has no mutable surface. The expensive path still exists — some callers
genuinely need a detached mutable copy — but it is opt-in, named, and greppable.

**Why this is the highest-value change:** it converts a permanent
code-review-discipline problem into a compile-time one. Every future feature
(rift scheduler, alliance observer, LLM read tools) adds read call sites; under
the current typing, each is a fresh opportunity to reach for the 6 ms version.

### 3.2 Separate mutual exclusion from rate limiting

**The finding that drives this:** in `Server/Intent/Engine.go`, the claim is
acquired at line 392 and released at 595 — and the entire step loop, including
`executeStep` → outbound router → **the 4–6 s `LaneAttackLaunch` spacing** —
happens inside that window. A castle claim is therefore held for seconds while
the operation does nothing but wait out a rate limit.

Two distinct needs are being served by one lock:

| Need | Real duration | Should be |
|---|---|---|
| *"nobody else touch this castle while I plan and act on it"* | ms | a claim |
| *"attacks leave no faster than one per 4–6 s"* | seconds | a rate reservation |

Compounding it: `release()` is called from **~20 sites**, one per early-exit
path. Every new failure branch is a chance to leak a claim.

**From scratch:**

```go
// Claims are scoped, not manually released. One exit path, always correct.
err := engine.claims.WithResources(ctx, plan.Resources, prio, func(claims Held) error {
    // planning-critical work only; short
    slot, err := engine.lanes.Reserve(ctx, plan.Lane, plan.ScheduledAt)
    if err != nil { return err }
    claims.DowngradeToDispatch()  // release conflicting exclusivity, keep identity
    return engine.dispatch(ctx, slot, plan)
})
```

Two properties fall out. Structured scoping removes the 20-site leak surface
entirely. And a lane reservation becomes a *time slot the dispatcher grants*
rather than a sleep inside a critical section — which is precisely what §6.3
needs, because a rift scheduler coordinating 20 instances is a slot-allocation
problem and cannot be expressed as "hold a lock and sleep."

**Add claim modes while doing it** (§4.1 of the review). `Claim{Key, Mode}` with
`Shared` / `Exclusive`. The `khan-lane` wildcard convention I added this cycle is
a workaround for their absence; with real modes, the reader lanes take `Shared`
on the castle and the protection intents take `Exclusive`, and the wildcard trick
disappears.

### 3.3 Two-tier persistence: durability where the invariant lives

**The finding that drives this:** `Engine.Submit` costs 4.8 µs with no store,
8.0 µs through the router, and **348 µs with the SQLite operation store** —
98.6% of engine overhead. `synchronous=FULL` + `SetMaxOpenConns(1)` means all
19 lanes serialise on one fsync'd connection at 10–15 writes per attack
operation.

But the guarantee that actually needs durability is narrow: **the pre-dispatch
record for effects that can duplicate a game action if we crash mid-send.** That
is write and launch effects — 76 of 111 intents. The other 35 (29 read,
6 external) cannot duplicate anything meaningful. And of the ~10–15 writes per
operation, exactly one is the crash-recovery record; the rest are phase
transitions that exist for the UI and the activity log.

**From scratch, two stores with different contracts:**

| | Intent journal | Observability store |
|---|---|---|
| Contains | pre-dispatch + post-observe, write/launch only | every phase transition, all effects |
| Durability | `synchronous=FULL`, fsync per record | WAL, `synchronous=NORMAL`, batched |
| Write path | synchronous, blocking, on the critical path | async, buffered, coalesced |
| On crash | authoritative for reconciliation | may lose the last second — fine |
| Reads | never on the hot path | separate connection, never queues behind writes |

Expected shape: ~348 µs stays on the one write that earns it, ~0 for read
intents, and post-dispatch phase writes stop blocking the wire.

This is the one change the review flags as needing **a deliberate durability
decision, not a mechanical edit** — and it stays that way. The question to answer
first is in §7 of the review: how much crash-recovery do we actually want?

### 3.4 Transport as an interface, protocol above it

**The finding that drives this:** `ChromiumTransport` observes frames via CDP
`Network.EventWebSocketCreated` but **sends by injecting JavaScript through
`Runtime.evaluate`**. That is the largest single source of both latency and
fragility in the send path, and §6.1 (headless, locked in) exists to remove it.

Today the protocol knowledge and the transport mechanism are entangled. From
scratch they are separated on day one, before there is only one implementation
to generalise from:

```
Policies / Intents          ← knows nothing about transport
        ↓
Codec: %xt%<ns>%<opcode>%<seq>%<json>%   ← framing, sequence, correlation
        ↓
type GameTransport interface {
    Send(ctx, frame Frame) error
    Frames() <-chan Frame
    Generation() uint64        // connection identity for barrier logic
}
        ↓
CDPTransport | DirectSocketTransport | RelayTransport
```

`DirectSocketTransport` is §6.1. `RelayTransport` is §6.2 — a hosted instance
holding the socket while a human plays alongside. Neither is a rewrite; both are
a constructor argument. The relay case in particular is *only* tractable if the
codec is transport-agnostic, because a relay has to re-frame and forward
sequence numbers it did not originate.

The open question in §6.7 of the review — does any outbound payload carry a
client-computed signature or nonce — is the gate on this, and it is answerable
cheaply from the captures already in `Data/Logs/channels/`. Settle it before
estimating anything.

---

## 4. Six refinements that unlock the future features

Smaller than the four above, but each removes a specific blocker.

### 4.1 Typed plan outcomes instead of `ErrPlanStale`

§4.2 of the review: `ErrPlanStale` conflates *"race, try again"* with *"this is
impossible now."* The `maximumStaleReplans = 3` cap I added this cycle is a
band-aid over that conflation — it bounds the damage without distinguishing the
cases, so an impossible plan still burns three full replans before failing.

```go
type PlanOutcome interface{ planOutcome() }
type Retry      struct{ Cause error }        // bounded, cheap
type Impossible struct{ Cause error }        // fail immediately, no replan
type Superseded struct{ By OperationID }     // another actor did it; not a failure
```

`Superseded` matters specifically for §6.3 and §6.6: when a central scheduler or
an LLM and a local policy both want the same action, "someone else already did
this" is a *success* for the caller, not an error, and today there is no way to
say so.

### 4.2 Land-at-T as an engine primitive

The rift work established that `riftReplayTiming` (`Server/App/AttackIntents.go`)
is already the land-at-T primitive — but it lives inside one intent's planner.
Every part of §6.3 is about landing attacks at a chosen unix instant across many
instances with different travel times.

From scratch this is an engine concept, not an intent detail:

```go
type ScheduledDispatch struct {
    LandAtUnixMs   int64
    TravelDuration time.Duration  // computable pre-send; units carry speed
    Lane           Lane
}
// engine derives sendAt = LandAt - Travel, and the lane reservation (§3.2)
// grants that slot or reports the conflict
```

With this plus §3.2's slot reservations, the rift scheduler becomes *"assign
land times and preset IDs to instances"* — a configuration and messaging
problem — rather than a new subsystem inside the engine.

### 4.3 Actor identity as a value type

§4.4 of the review: actor identity is an exact string match, so a typo is a
silent behaviour change. With local policies, a central scheduler (§6.3), and an
LLM (§6.6) all submitting intents, actor identity stops being cosmetic and
starts deciding precedence, rate budgets, and audit attribution. Make it a
registered type where an unknown actor fails at construction.

While there: **precedence between actors is currently undefined** — whoever
claims first wins. That is probably wrong once a scheduler says "launch at T"
and a user asks the LLM for something now. Worth an explicit policy.

### 4.4 Named intent argument types with generated schemas

Several planners decode their arguments inline with no named type. That blocks
three things at once:

- the LLM tool surface (§6.6) needs JSON Schema per intent — 111 of them
- `strict: true` tool use needs the same, to make malformed arguments impossible
  rather than a runtime decode failure
- client form generation currently duplicates by hand what the planner knows

From scratch: every intent declares `type Args struct` with tags, schemas are
generated from those types, and all three consumers read the generated artifact.
`ArgumentsExample` stays as documentation but stops being load-bearing.

Same treatment for config sections — typed sections with generated schemas means
the LLM's `config.update` surface and the client's settings UI derive from one
source.

### 4.5 Wake as typed events, with urgency on the event

Two small things. §3.3 of the review: the coordinator computes fingerprints
*before* the cheap early-out, so it pays for work it then discards. Reorder.

And urgency currently belongs to the *policy* (`UrgentWakeDomains()`, added this
cycle for the Khan rage lane). It more naturally belongs to the **event** — a
rage-bar-full transition is time-critical regardless of who is listening. With
typed domains instead of strings, the event carries its own urgency and any
future listener inherits the right latency without opting in.

### 4.6 Instance identity and a coordination-bus seam

§6.3 (rift), §6.4 (alliance observer), and §6.2 (relay) all need multiple
instances to agree on something. From scratch, define — but do not build — the
seam: instances have a stable identity, a clock discipline, and a
`CoordinationBus` interface with Discord as one implementation. The cost of
defining it now is nearly zero; the cost of retrofitting it across three
features later is not.

---

## 5. What I would explicitly not do

**Not a different language or runtime.** Go is right for this: the concurrency
model matches the problem, and nothing measured is bottlenecked on language
performance.

**Not event sourcing for game state.** The immutable-generation store already
gives the useful property (consistent point-in-time reads) at 74 ns. Full event
sourcing would add replay cost and complexity for a guarantee this app does not
need.

**Not a general-purpose plugin system.** 111 intents and 19 lanes registered in
one place is legible and greppable. Dynamic registration would buy extensibility
nobody has asked for and cost the ability to enumerate the whole action surface
statically — which is exactly what makes §6.6 tractable.

**Not micro-optimising the ingest path.** §3.5. Measured, understood, and the
cost of the design that makes everything else fast.

**Not splitting into services.** Single binary, in-process. §6.1 makes it
*deployable* headless; that is a different thing from decomposing it.

---

## 6. The part that actually matters: none of this needs a rewrite

Every change above is reachable incrementally from the current code. A rewrite
would discard 111 working intents, 19 tuned policy lanes, and a guard stack whose
edge cases were learned the expensive way — to arrive at the same architecture.

Ordered by value ÷ cost:

| # | Change | Value | Cost | Risk | Ref |
|---|---|---|---|---|---|
| 1 | `ReadView` type + convert read call sites | **Very high** | Medium (mechanical, 72 sites) | Low | §3.1 |
| 2 | Coordinator fingerprint reorder | Medium | **Very low** | Very low | §4.5 |
| 3 | Typed plan outcomes | Medium | Low | Low | §4.1 |
| 4 | Two-tier persistence | **High** | Medium | **Needs a durability decision** | §3.3 |
| 5 | Claim modes + scoped release | High | Medium-high | Medium | §3.2 |
| 6 | Lane reservation split from claims | High | High | Medium | §3.2 |
| 7 | Named arg types + generated schemas | High *if* §6.6 happens | Medium | Low | §4.4 |
| 8 | `GameTransport` interface | **High** (gates §6.1/§6.2) | Medium | Low | §3.4 |
| 9 | Land-at-T primitive | High *if* §6.3 happens | Medium | Medium | §4.2 |
| 10 | Actor value type | Low now, high later | Low | Low | §4.3 |

**Items 1 and 2 are worth doing regardless of what happens with §6.** Item 1 is
the largest measured win in the app and it is mechanical. Item 2 is nearly free.

**Item 8 is the one to sequence next** if §6.1 is going ahead, and it has a
prerequisite that costs almost nothing to resolve: answer §6.7's question about
client-computed payload values from the existing captures. That answer changes
the estimate for §6.1, §6.2, and item 8 simultaneously.

**Item 4 is blocked on a product decision, not an engineering one.** How much
crash-recovery do we want? Until that is answered, leave `synchronous=FULL`
alone — it is expensive and correct, which is the right way to be wrong.

---

## 7. The one thing I would tell a from-scratch builder

The architecture here is good because it separates *deciding* from *doing* and
never lets a decider reach the wire. Preserve that above everything.

But the recurring failure mode is subtler and worth naming: **a distinction that
lives only in a comment will be violated at scale.** `Snapshot()` vs
`ReadOnlyView()` returning the same type cost 72 call sites' worth of
opportunity. One `release()` covering both exclusion and rate cost a 4–6 s
critical section and 20 leak-prone exit paths. One table serving both durability
and telemetry cost 98.6% of engine overhead.

Each was a correct design idea that the code could not enforce. When the design
distinguishes two things, make the type system or the runtime distinguish them
too — or accept that in a year the code will not.

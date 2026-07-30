# Intent Engine — Game Command Dispatch, Context, and Guards

How a game command actually reaches the game, what context it needs, where that
context comes from, and every guard that stands between an automation deciding
something and a frame leaving the socket.

Written for someone who has never read this codebase. File references use
`path:line` and were accurate at the time of writing.

Per-feature behaviour lives in [`Docs/Features/`](Docs/Features/README.md).
Measured performance findings and open architecture questions live in
[`Docs/ArchitectureReview.md`](Docs/ArchitectureReview.md).

---

## 1. The short version

Nothing sends a raw command. Every outbound frame is a **step inside a plan**,
and a plan only executes after it has proven that the world it was planned
against is still the world it is about to act on.

The game is a stateful, single-session UI protocol. It has one focused castle,
one open attack dialog, one world-map context. Commands are not
self-describing — `cra` (launch attack) means "launch the attack currently
described by the dialog you opened", not "launch this attack". So the danger is
never a malformed command; it is a **correct command applied to the wrong
context**. Nearly every guard here exists to make that impossible.

The three ideas that carry most of the weight:

1. **Context is asked for, never assumed.** Focus, attack dialogs, saved
   presets, cooldowns and unit counts are established by sending a command and
   ingesting the game's own answer into state.
2. **Claims serialize what genuinely shares authority**, and nothing else.
   Two operations that touch the same castle's defense must not interleave; an
   event taunt and an outbound attack need not block each other.
3. **A plan is revalidated immediately before it dispatches.** If the world
   moved, the plan is stale and is rebuilt rather than sent.

---

## 2. How a command reaches the game

Six layers. A command that fails any of them never reaches the socket.

| # | Layer | Code | Responsibility |
|---|-------|------|----------------|
| 1 | Intent submission | `Server/Intent/Engine.go:237` `Submit` | Idempotency, priority, receipt lifecycle |
| 2 | Planning | `Definition.Planner` | Turn arguments + state into a `Plan` of steps |
| 3 | Step execution | `Server/Intent/Engine.go:901` `executeStep` | Resolvers, actions, command dependencies, response waits |
| 4 | Encoding | `Server/Protocol/Frame.go:86` `Encode` | Build `%xt%<namespace>%<opcode>%<seq>%<json>%` |
| 5 | Outbound routing | `Server/Outbound/Router.go` | Lanes, priority, pacing, dispatch gate, deadlines |
| 6 | Transport | `Server/Session/Controller.go:111` | Connection-generation check, websocket write, echo to ingest |

### 2.1 A plan is a list of steps

A `Step` (`Server/Intent/Types.go:77`) is exactly one of:

- **Action** — in-process Go function, no wire traffic. Used for guards and for
  recording outcomes (`khan.lane.guard`, `attack.cra.send.guard`).
- **Resolver** — deferred step that builds its concrete command at execution
  time from fresh state, rather than at planning time.
- **Command** — an opcode plus JSON payload, optionally awaiting a response.

`commandStep` (`Server/App/CommandContexts.go:14`) is the standard command
helper and defaults to a **10 s timeout** and **success code 0**.
`contextCommandStep` wraps it in `RebuildOnResume`, marking the step as
context-establishing: after any interruption it is re-sent rather than skipped,
because the context it established may no longer hold.

### 2.2 Encoding

`Encode` refuses a command whose namespace, opcode, sequence, or route contains
the `%` delimiter, refuses an empty opcode, and refuses a payload that is not
valid JSON. An empty payload becomes `{}`.

### 2.3 Lanes and pacing

The router assigns a lane by opcode (`Server/Outbound/Router.go:116`):

- `cra` → `LaneAttackLaunch`
- everything else → `LaneCommand`

Each lane has its own `nextAllowed` gate, set after each dispatch:

- `LaneCommand` — **25 ms** spacing (`Server/Session/Controller.go:129`)
- `LaneAttackLaunch` — **4–6 s**, from scheduler config
  (`Server/App/RuntimePolicy.go:18`)

Because they are separate lanes, the attack pacing delay never holds up ordinary
commands. Within a lane, ordering is by **aged priority** then FIFO.

### 2.4 Priority

`Server/Outbound/Priority.go:86` derives priority from the actor. Interactive
user work outranks everything; automations sit in a band:

| Actor | Priority |
|---|---|
| interactive (default) | 100 |
| `scheduler:*` | 95 |
| `automation:autoStation` | 90 |
| `automation:autoBird` | 80 |
| `automation:autoTCI` | 70 |
| `automation:autoHospital` | 50 |
| `automation:autoKhan` | 35 |
| `automation:*` (unrecognised) | 10 |

> **Sharp edge.** The lookup is an exact match on the actor suffix. A policy
> lane named `autoKhan:rage` produces actor `automation:autoKhan:rage`, which
> matches nothing and silently falls to background priority (10). Lanes avoid
> this by declaring `ActorID()` (`Server/Automation/Types.go:48`) so every lane
> of a feature reports as that feature.

Waiting work ages upward one point per second (`AgedPriority`), so low-priority
work cannot starve indefinitely.

---

## 3. The context a command needs, and where it comes from

### 3.1 Planning context

Planners receive a `PlanningContext` (`Server/Intent/Types.go:33`):

| Field | Source | Notes |
|---|---|---|
| `State` | `State.Store.PlanningView()` | One immutable generation, zero-copy. Treat as read-only. |
| `Partitions` | same generation | Per-capability version counters used for staleness checks |
| `ProtocolContext` | same generation | Focus epoch, focused castle, session + connection generation |
| `GameData` | `GameData.Store` | Official game catalogs (items, camps, troops) |
| `Language` | `GameData.LanguageStore` | Display names |

All four come from **one** generation, so a planner can never mix a castle from
one revision with a catalog from another.

State itself is built only from frames the game sent us. The ingest pipeline
decodes each frame, runs its reducer, and commits the result atomically
(`Server/Ingest/Pipeline.go:207` `CommitFrameGuarded`). Reducers report which
**domains** changed, which is what wakes automations.

### 3.2 Context-establishing commands

These are the commands whose only purpose is to put the game into a known state
so a later command means what we intend.

| Opcode | Step helper | Establishes |
|---|---|---|
| `jaa` | `castleFocusStep` (`CommandContexts.go:50`) | Focus a castle by map position |
| `jca` | `attackCastleContextStep` (`CommandContexts.go:76`) | Refocus an already-focused castle by id |
| `dfc` | `defenseContextStep` (`DefenseIntents.go:420`) | Defense editing context for one castle |
| `adi` | inside `craSetupContextSteps` | Attack dialog — target, limits, cooldowns |
| `gas` | inside `craSetupContextSteps` | Saved attack presets/formations |
| `gbl` | inside `craSetupContextSteps` | World-map context |
| `gam` | CRA dependency resolver | Fresh movement snapshot |
| `gie` | `generalSkillsContextSteps` | Commander general attack limits |
| `gnr` | `equipmentUpgradeContextStep` | Equipment upgrade menu |
| `kpi` | `kingdomTransportContextStep` | Kingdom transports |
| `sdi` | station route preview | Station route preview |

Two properties matter:

**They are conditional.** Context is only re-established when it is actually
stale, which is what keeps the chain cheap:

- `castleContextSteps` returns nothing when `castle.Focused` is already true.
- `attackCastleContextStep` sends the cheaper `jca` when the castle is focused
  and the full `jaa` when it is not.
- `generalSkillsContextSteps` sends `gie` only when the commander has a general
  **and** that general's data is older than 5 minutes.

**They are committed, not just sent.** Focus steps use
`ResponseBarrierCommitted`, meaning the step does not complete until the
response has been reduced into `GameState`. Without this, the next step would
plan against a state that does not yet know the focus moved.

### 3.3 Response barriers

`Server/Intent/Types.go:52` defines how long a step waits:

| Barrier | Waits until |
|---|---|
| `wire` | the response is decoded off the socket |
| `committed` | the response has been reduced into `GameState` |
| `wire-then-committed` | decoded, then commits before the plan continues |

`applyAutomaticResponseBarriers` (`Engine.go:1570`) applies these automatically
across runs of consecutive plain command steps, so a chain of context commands
pipelines on the wire but still fully commits before any state-dependent step.

---

## 4. The guard stack

In the order a command passes through them.

| # | Guard | Where | Prevents |
|---|---|---|---|
| 1 | Idempotency reservation | `Engine.go:845` | Re-running an operation id with different arguments |
| 2 | Priority resolution | `Outbound/Priority.go:76` | Background work outranking the user |
| 3 | Execution gate — before claims | `Engine.go:356` | Running while the bot lock is held |
| 4 | Admission | `Intent/Admission.go` | Concurrent attack launches across features |
| 5 | Claims / resources | `Intent/Claims.go` | Two operations sharing authority over one resource |
| 6 | Revalidation | `Engine.go:403` | Acting on a plan that aged while queued |
| 7 | Execution gate — before step | `Engine.go:527` | Continuing into a step after the lock engaged |
| 8 | Socket readiness + baseline | `Engine.go:1208`, `:1181` | Sending before the session has an authoritative baseline |
| 9 | Command dependencies | `Engine.go:1248` | Sending a command without its context chain |
| 10 | Dispatch permit | `Engine.go:481` | Dispatching into a world that moved |
| 11 | Outbound dispatch gate | `Controller.go:292` | Automation traffic during a lock or after reconnect |
| 12 | Response barrier + success codes | `Engine.go:1142` | Treating a rejected command as success |
| 13 | Stale replan bound | `Engine.go:603` | Unbounded retry re-running dependencies forever |
| 14 | Ingest commit validation | `Pipeline.go:207` | Committing a response from a stale session or focus |

### 4.1 Claims and resources

Plans declare `Claims` as strings; `Server/Intent/Resources.go` maps them onto
typed `ResourceKey`s with a scope (application / session / account / kingdom /
castle), a capability, a kind, and an id. Two operations conflict when any of
their keys overlap (`resourcesOverlap`, `Resources.go:315`), and `*` is a
wildcard in every dimension.

Claims are **exclusive** — there is no shared/read mode. Granularity is
therefore the whole design:

- `castle:<id>` expands to capability `*`, kind `*` — the **entire** castle. It
  collides with every other castle-scoped claim on that castle.
- `defense:<id>` is castle-scoped but capability `defense` only.
- `khan-lane:taunt` is account-scoped and collides only with `khan-lane:*`.

The wildcard is what lets a set of lanes run concurrently while still being
excludable as a group: each lane claims `khan-lane:<lane>`, and the protection
intents claim bare `khan-lane`, which wildcard-matches all of them at once.

Waiters are ordered by aged priority, then by deadline, then FIFO
(`claimWaiterBeforeAt`). A plan with an admission class gets a 5 s claim
timeout; a plan without one waits as long as its operation context allows.

### 4.2 Admission

A second, coarser gate in front of claims, for work that must not run
concurrently across features — currently attack launches
(`AdmissionAttackLaunch`). `normalizePlan` attaches it automatically to any plan
containing `cra`. Waiters age one weight point per 15 s and the manager consults
lane availability so an admitted operation is not granted a slot it would only
spend sleeping.

### 4.3 The dispatch permit

The last check before the **first** command of an attempt goes out
(`Engine.go:481`). It fails with `ErrPlanStale` if any of these changed since
planning:

- the declared state partitions, or — if none were declared — the raw state
  revision
- session generation, or connection generation
- bound game world or player id
- the official game-data catalog version
- the focused castle or focus epoch, **but only if the plan claims
  `castle-focus`** (`planUsesFocus`, `Engine.go:652`)

That last conditional is why claiming focus is not merely a lock: it also opts
the plan into focus-change detection.

### 4.4 Command dependencies

A step may declare `CommandDependencies` — commands that must succeed
*immediately* before it. The engine resolves them at execution time
(`executeCommandDependencies`, `Engine.go:1248`), and:

- For `cra` specifically, it first **waits out the attack-launch lane delay**,
  so the dependency chain runs immediately before the launch rather than 4–6 s
  ahead of it.
- The resolver returns a **route `Key`**. When the deferred resolver later
  produces the concrete command, the engine verifies the key still matches
  (`Engine.go:1270`), so a resolver cannot swap targets after its dependencies
  established context for a different one.

### 4.5 Effect phases and reconciliation

Every operation records a phase (`Types.go:168`): `accepted` → `planned` →
`dispatching` → `sent` / `awaiting_response` → `observed` → `completed`. The
`dispatching` phase is persisted **before** the write hits the socket, so a
crash mid-send is recoverable.

When an outcome is genuinely unknown — a send that may or may not have landed —
the operation is marked `indeterminate` with phase
`reconciliation_required` rather than failed. The distinction matters for
launch-effect intents, where a false "failed" would cause a duplicate attack.

Persistence is SQLite with `synchronous=FULL` on a single connection
(`OperationStore.go:42`). Measured cost is ~0.35 ms per operation — negligible
against network and pacing, and it buys crash-safe receipts.

---

## 5. Worked example: the attack chain

The canonical chain, and why each link exists. Steps come from `planKhanAttack`
(`Server/App/AutoKhanIntents.go:203`); the CRA dependencies come from
`resolveCRACommandDependencies` (`Server/App/CommandContexts.go:170`).

### 5.1 Plan steps

```
1. gie      Refresh commander general attack limits   (only if general data > 5 min old)
2. jaa|jca  Focus / refocus the attack source castle  (jaa if not focused, else jca)
3. action   Verify server daily attack limit          (only if a limit is configured)
4. resolver Build and launch the attack               (deferred; awaits cra)
5. action   Capture authoritative movement
```

### 5.2 What step 4 expands into

The resolver step does not send `cra` directly. Reaching it triggers the
dependency chain first:

```
   ── wait for the attack-launch lane (4–6 s) ──
4a. gam      Refresh commander movements      (only when a commander is involved; committed barrier)
4b. action   Close game UI
4c. gbl      Refresh world-map context
4d. adi      Refresh attack-dialog context    ← target, limits, cooldowns
4e. gas      Refresh saved attack presets
4f. action   Capture fresh tower capacity     (tower targets only)
4g. action   Verify authoritative CRA target  ← the send guard
   ── resolver now builds the concrete cra from fresh state ──
4h. cra      Launch
```

`adi` sits here, and only here, because the attack dialog is what the game uses
to compute the limits the `cra` will be validated against. It is a **dependency
of a launch**, not something to poll.

### 5.3 The CRA send guard

`guardCRASend` (`CommandContexts.go:243`) runs as the last dependency, reading
**live** state, and refuses to launch when:

| Check | Rejects when |
|---|---|
| Dialog freshness | `dialog.ObservedAt` is older than the guard's own timestamp — the `adi` did not actually land |
| Route identity | dialog source castle coordinates, kingdom, or target coordinates do not match the intended route |
| Target cooldown | tower / event-camp cooldown remaining, or storm target unavailable |
| Pending refresh | the target is awaiting a post-victory cooldown refresh |
| Map-side cooldown | the map observation still shows cooldown for this target type |
| Movement freshness | the movement snapshot predates the guard (commander launches only) |
| Commander | commander missing, unavailable, or already has an active movement |

Plus one guard before the chain even runs: if another feature has a prior attack
on the same tower target awaiting settlement, the dependency resolver rejects
immediately (`CommandContexts.go:200`).

### 5.4 Why the chain is ordered this way

- `gie` first, because attack limits feed the dialog's calculations.
- Focus before dialog, because the dialog is opened *from* the focused castle.
- `gam` before the dialog when a commander is involved, so commander
  availability is judged against a movement snapshot newer than the guard.
- Lane wait **before** the dependency chain, so the context is fresh at launch
  rather than stale by the pacing delay.
- Guard last, so it validates the state the `cra` will actually use.

---

## 6. Feature-level guards around the chain

The engine guarantees a command is correct *for the context it was planned in*.
It cannot know that launching an attack right now is a bad idea for product
reasons. That is the policy layer's job.

### 6.1 Coordinator gating

Before a policy is even asked for a decision (`Server/Automation/Coordinator.go:259`):
feature enabled → configuration/session fingerprint unchanged → session logged
in with a matching baseline → inside the configured weekly schedule → not inside
a safety pause from a previous failure.

After a decision, further protection: a repeated identical decision triggers a
30 s pause, and a chain of immediate re-runs is capped at 32
(`maxImmediatePolicyRuns`) before a forced pause. Policies are woken by declared
**state domains** and **configuration sections**, coalesced through a 250 ms
debounce — which a policy may opt out of per-domain via `UrgentWakeDomains()`
when the game-side window is short.

### 6.2 Auto Khan as the worked case

Auto Khan runs four independent lanes — attack chain, cooldown skip, taunt,
defense restock — that must not block each other, plus one guard that stops all
of them.

**Lane isolation** is expressed purely in claims:

| Lane | Claims |
|---|---|
| attack | `attack-context`, `attack-inventory:<castle>`, `khan-lane:attack`, `commander:<id>`, `khan-target:<k>:<x>:<y>`, plus `castle-focus` *only when the source castle is not the main castle* |
| cooldown skip | dungeon skip claim, `account-resources`, `khan-lane:cooldown` |
| taunt | `khan-lane:taunt` |
| defense restock | `castle-focus`, `defense:<castle>`, `khan-lane:defense` |
| open gate / point-limit stop / protection clear | `khan-protection`, `khan-lane`, plus castle and defense claims |

The protection intents claim bare `khan-lane`, which overlaps every
`khan-lane:<lane>`, so the safety path still excludes all four lanes at once
while the lanes stay free of each other. `castle-focus` appears only where focus
genuinely moves — a chain attack from the main castle leaves focus where the
rest of the loop already needs it.

**In-plan safety recheck.** Every Khan lane carries `khan.lane.guard`
(`AutoKhanIntents.go:503`) as a step immediately before its effect. It reads
live state via `application.State.Snapshot()` and refuses when: the main castle
is missing, a player attack is incoming, Auto Station is moving troops, the
gates are open, the protection lock is active, the Nomad point threshold is
reached, or the defense preset would put too many offensive units on the wall.
The taunt plan runs it before dispatch; the defense preset plan runs it
repeatedly, between each wall/moat/keep command.

This recheck — not the claim — is what actually enforces safety. Claims order
work; the guard decides whether the work is still a good idea.

---

## 7. Failure taxonomy

| Outcome | Meaning | Caller behaviour |
|---|---|---|
| `succeeded` | All steps completed | Optional immediate re-evaluation via `ReevaluateOnSuccess` |
| `failed` | Rejected before or during, no lasting effect | 30 s safety pause on that policy |
| `partially_succeeded` | Some steps landed, then failed | Needs reconciliation |
| `indeterminate` | A send may or may not have landed | Never retried blindly |
| `cancelled` | Context cancelled — lock, config change, session loss | Re-evaluated when the cause clears |
| `paused` | Yielded to the bot lock | Resumes from its checkpoint |

**`ErrPlanStale` is special.** It means "the world moved, rebuild and try
again", and the engine replans in place without failing the operation. Policies
opt into immediate re-evaluation on it via `ReevaluateOnStale`.

---

## 8. Known sharp edges

Honest notes for anyone changing this code.

1. **`ErrPlanStale` is overloaded.** Several genuinely durable conditions —
   a camp on cooldown, an attack preset short on items — are wrapped in
   `ErrPlanStale`, which means "retry immediately". A planner that cannot
   observe the condition then re-approves the same work, and each retry re-runs
   the plan's command dependencies, putting `gbl`/`adi`/`gas` back on the wire.
   One Khan attack operation reached **259 attempts in 65 seconds** this way.
   The engine now caps in-place stale replans at 3
   (`maximumStaleReplans`, `Engine.go:28`), which bounds the damage — but the
   right long-term fix is for durable conditions to be plain planning failures
   so the policy reroutes (to a time skip, or to pausing for inventory) instead
   of retrying at all.

2. **Claims have no shared mode.** Any guard expressed as a claim is exclusive,
   so a "readers observe, writers exclude" pattern has to be built out of
   wildcard naming, as `khan-lane` does.

3. **`castle:<id>` is a very large hammer.** It wildcards the whole castle. It
   is easy to add and it silently serializes unrelated features on that castle.
   Prefer the narrowest capability claim that covers what the plan touches.

4. **Actor names must match the priority table exactly.** Any new policy lane
   needs `ActorID()` or it runs at background priority.

5. **A planner must be pure.** It may not mutate state, which is why conditions
   only the wire can reveal cannot be resolved at planning time — the resolver
   and the send guard exist for exactly those.

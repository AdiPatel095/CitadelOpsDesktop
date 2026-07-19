# Concurrency architecture coverage assessment

Assessment date: 2026-07-15

## Decision

The correct near-term architecture is the augmented in-process modular monolith now implemented in this repository. A greenfield rewrite or worker-per-account system is not justified for the current single-active-account desktop product.

The implementation preserved the existing feature planners, protocol knowledge, command builders, response reducers, API compatibility, immutable state publication, attack admission, and one paced physical sender. It replaced the coordination boundaries that created the highest race and restart risks:

- exact string equality is now translated into typed, account-scoped, hierarchical resources with semantic parent/child and canonical alias overlap;
- production plans fail closed when resource declarations are empty, unknown, or unmapped;
- operations use a durable SQLite journal with request-fingerprint idempotency and explicit possible-send/reconciliation phases;
- account, session, connection, focus, and catalog lineage is captured and rechecked at observation commit and command dispatch;
- scheduled work has durable account-scoped operation identity, versions, cancellation, and stale-completion fencing;
- direct browser traffic pauses automation, while an operating-system profile/data-directory lease prevents a second CitadelOps process from sharing the authority;
- application services, ingest shutdown, game-data refresh, API snapshot delivery, and WebSocket ownership now have explicit concurrency guards;
- bounded priority aging prevents indefinite starvation in resource acquisition and outbound routing.

This is a deliberate augmentation, not a claim that all 152 register cases are closed. The state store still clones one account-wide aggregate; typed read-set/effect completeness is not generated for every feature; direct human commands cannot join an in-process transaction; and capability-specific reconciliation remains incomplete. Those are the remaining architecture program, with a separate multi-account worker design deferred until simultaneous account operation is a real product requirement.

The measured durable intent-to-transport fast path is approximately 0.191–0.202 ms on an Apple M4, leaving substantial headroom under the 25 ms uncontended objective. Whole-state mutation is approximately 0.747–0.809 ms with about 3.26 MB allocated and is now the first scaling boundary to monitor.

## What it means to account for every condition

No architecture can prevent every external ambiguity. The remote game, network, browser, operating system, filesystem, clock, and human player can change independently of CitadelOps.

Accounting for a risk means that every register row has all of the following:

1. A named owner and scope.
2. A prevention or serialization invariant where prevention is possible.
3. A detection and reconciliation path where prevention is impossible.
4. A bounded failure or degraded mode.
5. Telemetry that proves which path occurred.
6. A deterministic, race, fuzz, fault-injection, or operational test.

Before cutover, no row may remain Open or Audit. A row may close as:

- Prevented: the invalid interleaving cannot occur by construction.
- Reconciled: the external ambiguity can occur, but automatic replay is blocked and authoritative evidence resolves it.
- Constrained: the behavior is a deliberate physical or product limitation with a measured service objective.

This distinction is important. For example, a network failure after the browser may have sent a command cannot be made exactly-once by an in-memory lock. It can only be represented durably as possibly sent and reconciled before retry.

## Register score

The source register contains 152 numbered cases. Its status labels describe the current implementation, not the proposed design.

| Current status | Count | Share | Meaning |
|---|---:|---:|---|
| Covered | 48 | 32% | A current mechanism directly covers the case, subject to retained tests. |
| Partial | 75 | 49% | A useful safeguard exists, but an identity, durability, completeness, or proof gap remains. |
| Open | 0 | 0% | No register row is currently known to lack a general safeguard; `Partial` and `Audit` rows still prevent a blanket guarantee. |
| Audit | 23 | 15% | Correctness depends on a feature/protocol assumption that has not been mechanically proven. |
| Constraint | 6 | 4% | The behavior is intentionally serialized or bounded and needs an explicit service objective. |

There are 29 Critical cases:

| Critical status | Count | Share |
|---|---:|---:|
| Covered | 12 | 41% |
| Partial | 17 | 59% |
| Open | 0 | 0% |

No Critical row remains `Open`. The remaining Critical rows are `Partial` because external ambiguity, complete declaration proof, or capability-specific reconciliation cannot be closed by locking alone.

### Coverage by risk family

| Family | Covered | Partial | Open | Audit | Constraint | Total |
|---|---:|---:|---:|---:|---:|---:|
| Memory and Go concurrency | 3 | 7 | 0 | 2 | 0 | 12 |
| State, reducers, and ingest | 7 | 11 | 0 | 2 | 1 | 21 |
| Intent, claims, and admission | 8 | 12 | 0 | 4 | 2 | 26 |
| Outbound and correlation | 6 | 9 | 0 | 1 | 2 | 18 |
| Session and reconnect | 6 | 8 | 0 | 1 | 0 | 15 |
| Automation and scheduling | 9 | 7 | 0 | 1 | 1 | 18 |
| Persistence and restart | 2 | 8 | 0 | 2 | 0 | 12 |
| Time, identity, and protocol edges | 0 | 12 | 0 | 8 | 0 | 20 |
| API and UI | 7 | 1 | 0 | 2 | 0 | 10 |

## Augmented current architecture

### Control flow

~~~mermaid
flowchart LR
    Game["Remote game"] <--> Browser["Chromium game page"]
    Browser -->|"observed frames"| Controller["Session controller"]
    Controller --> Ingest["Decode and reducer pipeline"]
    Ingest --> Store["Bound account GameState plus partition versions"]

    UI["UI / CLI / API"] --> Engine["Intent engine"]
    Automation["Automation / scheduler / reports"] --> Engine
    Store --> Automation
    Store --> Engine
    Engine --> Durable["Durable operation and outcome journal"]
    Durable --> Claims["Typed hierarchical resources and attack admission"]
    Claims --> Permit["Account/session/focus/catalog dispatch permit"]
    Permit --> Router["Single paced outbound router with aging"]
    Router --> Browser

    Browser -.->|"human game sends"| Game
    Browser -->|"uncaused outbound detected"| Pause["Automation pause / invalidation"]
    Pause --> Engine
    Store --> Snapshot["JSON snapshot / JSONL history"]
    Store --> APIEvents["API snapshots and events"]
    Lease["OS profile/data-directory lease"] --> Browser
    Lease --> Durable
~~~

### What is already correct and worth retaining

| Existing mechanism | What it currently protects | What it does not prove |
|---|---|---|
| Atomic store generation | State, partition versions, and protocol context are observed from one generation. | The nested GameState references are immutable only by convention; mutations still clone the entire aggregate. |
| Single live ingest owner | Normal live frames commit in transport order; shutdown has one drain/discard owner. | Zero-generation compatibility observations and merged-source regression still require audit. |
| Observation and dispatch lineage | Account, session, connection, focus, causation, and catalog generations fence stale frames and commands. | Every newly decoded focus-derived opcode must still declare/prove its scope semantics. |
| Shared intent path | UI, automation, scheduler, reports, and CLI normally use the same validation and execution machinery. | Direct game-client traffic is detected but remains external to the transaction. |
| Plan revalidation | Waiting for resources or a changed dependency causes replanning; dispatch checks account/session/focus/catalog again. | Explicit read sets and configuration dependencies are not complete for every feature. |
| Typed resource manager | Parent/child and canonical aliases overlap by account scope; waiters receive bounded priority aging. | A nonempty declaration does not prove that every semantic side effect was declared. |
| Attack admission | Attack families share weighted, aged, paced admission. | Other scarce workflows rely only on ordinary claims and router bounds. |
| Outbound router | CitadelOps sends are physically serialized, paced, prioritized, bounded, aged, and connection-fenced. | It cannot prove whether a cancelled or timed-out browser evaluation sent the command. |
| Durable operations and response barriers | Watchers register before send; request fingerprints, send phases, explicit indeterminate outcomes, and committed responses survive restart. | Capability-specific reconciliation is incomplete for many ambiguous remote effects. |
| Account/profile authority | World/player binding prevents recovered-state merge; an OS lease prevents another process sharing the profile/data directory. | One runtime retains one active account aggregate; simultaneous account partitions are deferred. |
| Replay, streams, and telemetry | Protocol behavior can be examined offline; live activity is correlated with durable operations; overflow carries sequence/gap metadata and scoped resync. | The generated all-feature matrix, deterministic fault suite, and history/telemetry storage-health coverage remain incomplete. |

## Why the remaining boundaries do not close the whole register

### 1. State partitioning is currently metadata, not ownership

ScopeKey, PartitionKey, and partition versions are useful. However, Store.ApplyScoped still:

1. locks one account-wide writer mutex;
2. deep-clones the complete GameState;
3. lets a callback mutate any field;
4. advances metadata for declared partitions;
5. publishes one new global generation.

This makes planning reads fast—approximately 69–80 ns with no allocation—but it does not make buildings, queues, attacks, defense, or economy independently owned. A reducer can mutate an undeclared field, a new reference field can escape clone coverage, and all writes still pay the approximately 0.747–0.809 ms/3.26 MB global clone cost.

The target needs real capability stores or immutable capability values combined into an account generation. One accepted observation may update several capability values atomically, but a reducer must only receive write access to the partitions it owns.

### 2. Typed resources are enforceable, but declaration completeness is not inferred

Compatibility plans may still originate with legacy claim names, but plan finalization now maps them into typed account-scoped resource keys. The resource manager applies central semantic overlap for parents, children, and aliases such as wallet/currency, castle capabilities, leaders, targets, shops, responses, and schedules. Unknown or unmapped legacy claims fail production execution.

The remaining gap is completeness rather than overlap mechanics:

- a planner can declare one valid resource while omitting a second semantic effect;
- explicit versioned read sets are not complete across every feature;
- quantity reservations are not a general contract for wallet, troops, inventory, and capacity;
- the generated feature-by-resource matrix does not yet prove every declaration pair.

The next enforcement step is generated static coverage and reviewed effect contracts, not another resource-lock rewrite.

### 3. Planning and execution have durable outcomes but incomplete capability contracts

The current Step type is flexible, but it does not require:

- an idempotency class;
- concrete state/catalog/config/focus preconditions;
- a declared remote resource effect;
- authoritative completion evidence;
- compensation or reconciliation behavior;
- safe retry classification.

Receipts and durable phases now distinguish partial, indeterminate, reconciling, dispatching, sent, observed, reconciliation-required, and completed outcomes. Crash recovery never treats a possibly dispatched non-idempotent effect as safe to replay.

The remaining gap is per-capability evidence. Each effect still needs a reviewed idempotency class, authoritative completion predicate, and reconciliation query. A durable `reconciliation_required` record prevents a duplicate, but it cannot by itself determine whether the game applied a purchase, launch, queue mutation, or inventory spend.

### 4. The operation lifecycle is durable, but external effects are not transactional

Operation identity, request fingerprint, plan/effect phase, possible-send risk, and terminal outcome are written to SQLite in WAL mode with full synchronization. The hot in-memory receipt cache is bounded while durable idempotency evidence remains queryable.

A crash can still occur between:

- request acceptance and receipt storage;
- durable schedule status and intent submission;
- dispatch decision and browser send;
- browser send and response observation;
- response observation and state commit;
- state commit and receipt persistence.

The local journal makes these transitions explicit and restart-safe, but it cannot atomically commit with the remote game server. Dispatching or later phases therefore recover as reconciliation-required. That is the correct failure model; automatic capability reconciliation is the unfinished portion.

### 5. One process/account authority is enforced; human traffic remains external

The profile/data-directory lease prevents another CitadelOps process from sharing persistence or browser authority. Persisted state binds to world plus player, and an authoritative account transition resets old capability state instead of merging it.

The human game page can still send directly. Causation tagging detects that traffic and pauses automation, but it cannot retroactively acquire resources for the human command. Safe shared control therefore needs affected-capability reconciliation or an optional exclusive-control product mode.

The current runtime also keeps one active account aggregate. Switching accounts is safe but discards the prior active game-state snapshot. Simultaneous retained accounts require account-partition storage or worker extraction and remain intentionally deferred.

### 6. Focus is now a fenced protocol resource, with opcode coverage still auditable

Observations now carry account, session, connection, ingress, focus, causation, decoder, and catalog lineage. Commit validates that lineage before and after reduction; focus-authoritative baselines are handled explicitly; malformed multiple-focus state is normalized deterministically.

The remaining obligation is coverage: every newly introduced opcode must declare whether explicit wire identity, correlated operation scope, captured focus, or baseline authority owns its mutation. The envelope closes the timing race only when reducer registration describes the protocol semantics correctly.

- profile and game account;
- session generation;
- connection generation;
- ingress sequence;
- captured focus epoch and castle;
- correlation token or command causation where present.

The captured fields remain the required schema for any future transport adapter.

### 7. Lifecycle and safety-relevant event delivery have explicit ownership

Application, coordinator, report manager, scheduler, session controller, ingest queue, game-data refresh, API WebSocket reader/writer, and atomic snapshot/revision paths now have explicit ownership guards. Double start and queue shutdown no longer depend only on caller convention.

State, configuration, and operation publications now carry source sequence and an explicit gap marker when subscriber overflow coalesces a pending event. Configuration carries a full snapshot; state invalidations force an atomic snapshot refetch; operations expose recent durable receipts, send a reconnect snapshot, and trigger client refetch on a gap. Future safety-relevant streams must reuse this contract, and history/telemetry still need explicit storage health and backpressure semantics.

## Target architecture: account capability runtime

The resource scheduler, durable operation store, dispatch permit, observation envelope, profile/account fencing, and single effect dispatcher are now implemented within the existing packages. The diagram below remains the conditional end state for replacing the global cloned `GameState` with independently owned capability partitions and projections; it is not required merely to keep the current uncontended latency target.

### Core shape

~~~mermaid
flowchart TB
    Sources["Transport observations / API requests / policies / schedules / timers"] --> Envelope["Typed account envelope"]
    Envelope --> Sequencer["Account sequencer: short non-blocking turns"]

    subgraph Runtime["One in-process runtime per bound game account"]
        Sequencer --> CapStore["Immutable capability partitions"]
        Sequencer --> Operations["Durable operation and schedule store"]
        Sequencer --> Resources["Typed resource scheduler"]
        Resources --> Effects["Single account effect dispatcher"]
        Effects --> Guard["Dispatch permit: session, focus, versions, deadline"]
        Guard --> Transport["Chromium transport"]
        Transport -->|"send result / observed response"| Envelope
        CapStore --> Planner["Pure capability planners"]
        Planner --> Sequencer
        CapStore --> Projections["Versioned query projections"]
    end

    Projections --> Clients["UI / CLI / API"]
    Operations --> Clients
    Journal["Bounded causal journal"] <--> Sequencer
~~~

### One account sequencer, not one goroutine per castle

The remote game has one session, one live focus context, and one physical command stream per account. Those are account-wide consistency boundaries. A separate mutable actor per castle would turn cross-castle transfers, account inventory, commanders, focus, and attacks into distributed transactions inside one process.

Use one sequencing authority per account instead:

- every accepted observation and control event receives an account position;
- each turn is short, synchronous, and contains no network or disk wait beyond its chosen local transaction;
- a turn reads one immutable capability generation and publishes the next;
- external I/O is issued to the effect dispatcher and returns as a new event;
- operation continuation revalidates versions after every await boundary;
- reducers for different castles remain isolated by typed capability ownership even though their commits are sequenced.

This follows the useful part of turn-based actor execution without requiring an actor framework. Official Orleans documentation describes why single-threaded turns simplify state safety and also warns that interleaving across await points can observe changed state. CitadelOps should make those continuation/version checks explicit rather than enable implicit reentrancy.

### State hierarchy

The account generation should reference independently versioned immutable capability instances.

| Scope | Capability examples |
|---|---|
| Application | update metadata, official catalog registry, support policy |
| Profile/bootstrap | local profile ID, browser path, pre-login session history, profile lease |
| Session | namespace, session/connection generation, focus epoch, readiness, transport health |
| Account | identity, wallet/currencies, inventory namespaces, leaders, equipment, subscriptions |
| Kingdom | world-map partition, transport lanes, kingdom event instances, target indexes |
| Castle | identity, economy, buildings/layout, construction queue, equipped TCI, production, crafting, garrison, hospital, defense |
| Movement/target/event | stable movement, target, cooldown, event-run, attack-dialog, and launch instances |
| Local desired state | feature settings, commander assignments, presets, reserves, schedules |
| Execution | operation, step, lease, idempotency, correlation, reconciliation, and receipt records |

One observation can still update several partitions atomically. For example, a battle response might update a movement, source garrison, commander availability, target cooldown, event score, and report notice in one account commit.

### Observation envelope

Every transport observation should carry:

~~~text
ProfileID
GameAccountID or Unbound
WorldID
SessionGeneration
ConnectionGeneration
IngressSequence
ObservedAt and optional ServerTime
Direction
Opcode
ResponseToken
CapturedFocusEpoch
CapturedFocusedCastleID
CausationOperationID when known
DecoderVersion
CatalogVersion
~~~

Scope resolution order must be explicit:

1. stable identity in the wire payload;
2. correlated operation scope;
3. captured focus context;
4. current focus only for opcodes proven safe to interpret that way;
5. quarantine and request an authoritative refresh.

Current focus must never silently repair an older frame.

### Typed resource scheduler

A resource key should be a value, not a formatted string:

~~~text
ResourceKey {
    Account
    ScopeKind
    KingdomID
    CastleID
    Capability
    ResourceKind
    ResourceID
}
~~~

Every capability request declares four different sets:

| Set | Purpose |
|---|---|
| Read dependencies | Versioned state, catalog, configuration, time, and focus facts used to plan. |
| Write/effect resources | Remote or local resources that the operation may mutate. |
| Quantity reservations | Optional currency, inventory, troop, queue-slot, or capacity amounts reserved across competing work. |
| Physical lanes | Session focus, normal command, attack launch, shop dialog, or other protocol-wide serialization. |

The central overlap function must encode parent/child semantics. Examples:

- an account-wallet write overlaps a specific currency spend;
- a castle-garrison write overlaps every unit stack in that castle;
- a whole building-layout write overlaps every footprint mutation;
- a session-focus write overlaps every effect whose interpretation depends on focus;
- the same target uses event run, target type, object identity, kingdom, and coordinates;
- TCI CID, building instance ID, building/decor wodID, product PID, and castle AID are distinct key types.

Read dependencies normally do not block one another. If an observation advances one while an operation waits, the operation is invalidated and replanned. Inbound observations must never wait for an intent claim; the remote game is authoritative.

Begin with conservative parent resources. Narrow them only after conflict tests and protocol evidence prove independence. Quantity reservations are an optimization after typed exclusive resources are correct.

### Planning and dispatch

The safe fast path is:

1. Normalize the caller into a server-owned actor class and effective priority.
2. Resolve or create a durable idempotency record.
3. Read one immutable account/capability generation.
4. Run a pure capability planner.
5. Submit the plan to the account sequencer.
6. Validate all read versions and acquire typed resources.
7. Persist the operation, plan hash, resource set, and first effect phase.
8. Enqueue one effect with a dispatch deadline.
9. Immediately before physical send, obtain a short-lived dispatch permit that proves connection, namespace, focus, catalog/config dependencies, target identity, and reservations.
10. Transition durably to dispatching before calling the external transport.
11. Send through the single account dispatcher.
12. Correlate an observation, commit the state change, and transition the receipt using the same account order.

Planning may run concurrently from immutable views. Admission, resource acquisition, and mutation publication are short serialized turns. Network waiting never holds a Go mutex or blocks the account sequencer.

### Durable effect lifecycle

Use an explicit state machine:

~~~mermaid
stateDiagram-v2
    [*] --> Accepted
    Accepted --> Planned
    Planned --> Admitted
    Admitted --> Dispatching
    Dispatching --> SentUnconfirmed
    SentUnconfirmed --> ResponseObserved
    ResponseObserved --> StateCommitted
    StateCommitted --> Succeeded

    Accepted --> CancelledBeforeSend
    Planned --> CancelledBeforeSend
    Admitted --> CancelledBeforeSend
    Dispatching --> Indeterminate
    SentUnconfirmed --> Indeterminate
    Indeterminate --> Reconciling
    Reconciling --> Succeeded
    Reconciling --> Failed
    Reconciling --> NeedsAttention
    ResponseObserved --> FailedAfterEffect
    StateCommitted --> FailedAfterEffect
~~~

Important semantics:

- CancelledBeforeSend proves no remote effect was attempted.
- FailedBeforeSend proves the transport was not entered.
- Dispatching means a crash may have happened immediately before or after the external call.
- Indeterminate blocks automatic replay of a non-idempotent effect.
- FailedAfterEffect lists completed effects and remaining failed steps.
- Succeeded requires the operation’s declared completion evidence, not socket-send success.
- Reconciliation is capability-specific: refresh queue, movement, inventory, target, report, construction slots, or another authoritative projection.

A durable workflow engine does not create exactly-once external effects. Temporal’s own platform material makes the relevant lesson clear: activities can execute at least once and retry, so external operations still require idempotency or reconciliation. CitadelOps can implement the smaller lifecycle it needs locally rather than introduce a distributed workflow service.

### Exclusive control and direct browser traffic

The account runtime needs two independent fences:

1. An operating-system profile/data-directory lease prevents a second CitadelOps process from controlling the same profile or persistence store.
2. A direct-game-traffic detector distinguishes CitadelOps-tagged outbound commands from commands initiated by the human game client.

When direct state-changing traffic is detected:

- stop admitting new automation effects;
- allow an already possibly-sent effect to settle;
- invalidate focus/context-sensitive plans;
- refresh affected capabilities or the authoritative baseline;
- resume only after resource and focus state is reconciled.

An optional exclusive-control mode can block or discourage human interactions, but correctness must not assume that the browser UI can never send.

### Persistence authority

Use one application-owned transactional store for:

- profile/account bindings and leases;
- desired settings and revisions;
- capability checkpoints and versions;
- operations, steps, plan hashes, idempotency keys, and reconciliation state;
- schedules and schedule versions;
- projection cursors and event-stream positions;
- compact causal journal metadata;
- history indexes and stable deduplication IDs.

Keep large protocol captures in bounded files referenced by the database.

SQLite is a strong fit for a local desktop runtime because transactions can atomically commit related records and it supports concurrent readers with one writer. CitadelOps should still own one logical database writer so application ordering is explicit. If WAL is selected, pin a fixed SQLite release and test checkpoints. SQLite documents a WAL-reset corruption defect fixed in 3.51.3 and in the 3.44.6/3.50.7 backports as of this assessment.

The operation transition that establishes possible-send risk must commit before the external send. This adds durability to the latency budget; it is not optional if crash-safe retry is a requirement.

### API and stream model

Native capability APIs should expose:

- task-shaped immutable projection snapshots;
- scope and capability version or ETag;
- typed commands with server-issued or validated idempotency keys;
- operation receipts with explicit possible-send and reconciliation states;
- monotonically sequenced deltas;
- a gap marker that forces scoped refetch;
- server-owned actor and effective priority;
- explicit account, kingdom, castle, event, and target scope.

API v2 can remain a compatibility adapter with one global revision. It should obtain revision and snapshot from one atomic generation.

## Architecture controls mapped to all register families

The detailed scenarios remain in ConcurrencyRaceConditionsAndEdgeCases.md. This table maps every numbered family to the target controls that own it.

| Register range | Primary target controls | Required disposition |
|---|---|---|
| MEM-01–MEM-12 | Immutable capability ownership; no mutable references in public planning views; one lifecycle owner; no callback under core locks; race/leak tests | Prevent Go data races and duplicate service loops; test shutdown and clone/alias invariants. |
| STATE-01–STATE-21 | Account sequencer; complete observation envelope; focus/account fencing; typed reducers; real partition ownership; sequenced gap-aware events | Prevent stale/wrong-scope commits; quarantine ambiguous scope; constrain mutation throughput with measured backpressure. |
| INTENT-01–INTENT-26 | Typed hierarchical resources; mandatory read/write declarations; server-owned actors; durable idempotency; reservations; fairness and deadlines | Prevent local conflicts and duplicate admission; replan on external changes; reconcile post-effect failures. |
| OUT-01–OUT-18 | One effect dispatcher; physical lanes; send-phase state machine; token/sequence correlation; direct-traffic detection; profile lease | Serialize local sends; classify possible send; reconcile before retry; constrain queue/pacing latency. |
| SESSION-01–SESSION-15 | Profile/account lease; run IDs; session/connection/focus epochs; authoritative baseline gate; supervised lifecycle | Fence old runs/sockets/accounts and make reconnect/reload/shutdown ownership explicit. |
| AUTO-01–AUTO-18 | Durable operation and schedule versions; active operation linkage; config dependencies; single-start coordinator; idempotent report/history effects | Prevent duplicate local policy work; survive restart; make cancel/reschedule semantics explicit. |
| PERSIST-01–PERSIST-12 | Transactional operation store; single database writer; profile lock; account binding; stable record IDs; storage health gate | Prevent cross-process writers and revision regression; reconcile crash ambiguity; degrade safely on disk failure. |
| EDGE-01–EDGE-20 | Typed identifier namespaces; server-time offset; event-run identity; checked arithmetic; schema/catalog versions; bounded inputs; protocol fuzzing | Prevent type confusion/overflow; preserve unknownness; reconcile clock/schema/target ambiguity. |
| API-01–API-10 | Atomic projection snapshots; stream sequence/gaps; idempotency; server-owned actor class; context-owned goroutines; client version guards | Prevent mismatched snapshots, leaks, stale UI application, and caller-controlled scheduling privilege. |

The architecture accounts for the complete register only when the implementation generates a one-row-per-risk closure report linking each ID to code, tests, and telemetry. This family map is the design ownership map, not substitute evidence.

## Three implementation paths

### Path 1: augment the current engine in place

This is the path selected for the current implementation. The existing Store, Intent, Outbound, Session, Scheduling, and API packages were augmented directly:

- replace string claims with typed resources and overlap;
- make effect-resource declarations mandatory and begin the read-set migration;
- add durable operation/step/idempotency/schedule storage;
- add partial/indeterminate/reconciling receipt states;
- carry focus/account context in observations;
- enforce profile lease and direct-traffic pause;
- add lifecycle guards and sequence/gap resynchronization;
- map every production feature definition into the typed resource vocabulary.

Strengths:

- lowest short-term parity risk;
- reuses all current call paths;
- keeps the current microsecond in-process planning path;
- can close immediate critical defects quickly.

Costs:

- the generic Step model and global GameState remain the integration center during a long migration;
- every feature migration edits shared packages and compatibility behavior at once;
- compatibility claim strings coexist with typed resources, although production finalization rejects unknown mappings;
- persistence and operation semantics would be grafted onto APIs designed for memory-only receipts;
- it is easy to stop after “mostly migrated,” leaving two safety models indefinitely.

Verdict: selected and appropriate for the present product. Keep it while the single-account whole-state mutation cost remains within budget. Extract capability ownership only when measured scaling, proofability, or simultaneous-account requirements justify the migration cost.

### Path 2: build a new kernel beside the current runtime

Create a new in-process account runtime and capability contract, while retaining old behavior through adapters:

- old transport frames feed both legacy ingest and a shadow/new account sequencer;
- old API v2 remains served from the compatibility generation;
- new capability planners shadow legacy plans before gaining effect authority;
- one capability cluster at a time moves state, planning, policies, and queries;
- the new effect dispatcher becomes authoritative only after transcript and race parity;
- migrated operations are mirrored back into v2 receipts/state until the client moves.

Reuse:

- Protocol decoding/encoding and known wire shapes;
- Chromium and replay transport knowledge;
- official game-data parsers and catalogs;
- feature calculations, policies, and command builders after extracting typed ports;
- golden captures and current focused tests;
- UI behavior and v2 contracts during migration.

Replace:

- global mutable state ownership;
- string claims;
- optional planner dependencies;
- memory-only receipt and schedule authority;
- implicit focus attribution;
- scattered lifecycle ownership;
- full-state native API contracts.

Strengths:

- cleanest way to make the safety model mandatory rather than conventional;
- preserves one-process latency and desktop simplicity;
- supports real castle/kingdom capability partitioning;
- permits side-by-side replay and shadow-plan parity;
- avoids IPC and distributed-failure cost on the 25 ms fast path;
- leaves a clean account-runtime boundary that can later move to a worker.

Costs:

- temporarily runs compatibility and native models together;
- requires carefully defined bridges so two models do not both send;
- needs a parity manifest and vertical migration discipline;
- is a kernel rewrite even though it is not a full product rewrite.

Verdict: defer. Use this as the next state-ownership step if whole-state clone cost, reducer isolation, or multi-account retention becomes a demonstrated product limitation.

### Path 3: greenfield journal-first control plane and account workers

Build a new control-plane process with a durable journal/database and a supervised worker process per account. Each worker owns one account sequencer, capability state, resources, and browser transport.

Strengths:

- strongest crash and resource isolation between accounts;
- natural profile/account lease boundary;
- best foundation for many simultaneous accounts, remote workers, or untrusted extensions;
- durable replay and worker recovery are first-class.

Costs:

- IPC creates another queue, correlation, versioning, cancellation, and partial-failure layer;
- a central database writer and worker effects require durable acknowledgements and lease fencing;
- packaging, upgrades, diagnostics, and support become substantially more complex;
- the 25 ms target includes IPC and durable coordination;
- parity risk is highest because current synchronous boundaries become distributed.

Verdict: choose only when multi-account fault isolation, remote execution, or untrusted extension isolation is a committed near-term requirement. Do not choose it merely to solve races inside one account.

## Side-by-side decision

| Criterion | Pre-augmentation baseline | Path 1: augmented current | Path 2: new in-process kernel | Path 3: worker rebuild |
|---|---|---|---|---|
| Complete typed conflict model | No | Hierarchy implemented; declaration matrix pending | Designed in from registration | Designed in from worker contract |
| Real capability state ownership | No | Expensive retrofit | Yes | Yes |
| Durable operation ambiguity | No | Implemented; reconciliation coverage partial | Core invariant | Core invariant |
| Profile/account fencing | No | Implemented for one active account/profile | Core invariant | Natural worker lease |
| Exact behavior preservation | Existing behavior | Lowest immediate risk | Strong with shadow/parity bridge | Highest migration risk |
| Uncontended in-process latency | Excellent | Approximately 0.19–0.20 ms durable path | Excellent | IPC/durability overhead |
| All-features proofability | Low | Medium-high after matrix/stress completion | High | High, with more failure modes |
| Desktop operational simplicity | Existing | High | High | Low |
| Multi-account crash isolation | None | Safe account switch, no simultaneous retention | Logical isolation | Physical isolation |
| Long-term maintainability | Transitional | Good for current scale | Highest when capability extraction is justified | High only if worker scale is needed |
| Recommendation | Superseded | Adopt now | Conditional next step | Defer behind trigger |

## Conditional future capability boundary

If capability extraction is triggered, the new runtime should have a small shared kernel and vertical capabilities. These names are illustrative; they are not a request to duplicate the safety controls already implemented in the existing packages.

~~~text
Server/Runtime/
    AccountRuntime.go
    AccountSequencer.go
    ObservationEnvelope.go
    CapabilityStore.go
    ResourceKey.go
    ResourceScheduler.go
    Operation.go
    OperationStore.go
    EffectDispatcher.go
    CorrelationRegistry.go
    SessionLease.go
    ProjectionStream.go

Server/Capabilities/
    Construction/
    Economy/
    Production/
    Crafting/
    Garrison/
    Movement/
    Leaders/
    Equipment/
    Defense/
    Combat/
    Alliance/
    WorldMap/
    Events/
    Reports/
~~~

The names are illustrative. The dependency rules matter:

- Runtime cannot import a capability.
- A capability cannot access another capability’s mutable store.
- Cross-capability requests use typed read ports and resource keys.
- Only the account sequencer publishes capability changes.
- Only the effect dispatcher accesses the live send port.
- Only the persistence adapter writes runtime records.
- UI/API adapters receive projection DTOs, not internal state objects.
- Compatibility adapters are isolated and removable.

## Handling all features at the same time

When every feature is active:

1. Policies and user requests may evaluate concurrently against immutable views.
2. The account sequencer admits plans in short ordered turns.
3. Typed resources prevent conflicting wallet, inventory, castle, queue, garrison, leader, target, focus, and schedule effects.
4. Independent capabilities and different castles can remain planned or queued without sharing mutable memory.
5. One physical dispatcher sends one command at a time because there is one game socket.
6. Attack launch admission and required pacing remain separate from ordinary commands.
7. Inbound observations continue to commit while effects wait; they invalidate stale plans rather than waiting for locks.
8. Fairness/aging prevents continuous interactive or high-priority work from starving background work beyond a configured bound.
9. Direct human traffic pauses new effects; capability reconciliation must complete before a full shared-control guarantee is made.
10. Every operation remains observable, cancellable at safe boundaries, and classifiable after restart.

This design improves concurrency of planning, state reads, projections, and independent work. It does not make simultaneous physical WebSocket sends safe. The Nth queued effect cannot all meet a 25 ms intent-to-send target on one serialized transport.

## The 25 ms objective

Keep the objective exactly scoped:

Intent acceptance to completion of the first successful transport send for a connected, baseline-ready, correctly focused, uncontended account, excluding deliberate pacing and game-server response time.

The augmented runtime protects this objective by:

- remaining in process;
- using immutable O(1) planning snapshots;
- keeping sequencer turns short and free of network waits;
- indexing resource overlap rather than scanning active operations;
- resolving capability scope before admission;
- maintaining one warm transport and correlation registry;
- measuring queue, planning, durability, dispatch, browser evaluation, and transport separately.

There is one deliberate tension: a non-idempotent effect needs a durable pre-send transition. The selected SQLite WAL/full-sync path has now been benchmarked at approximately 0.0834–0.0839 ms for reservation plus dispatch transition and approximately 0.191–0.202 ms through the full fake-transport engine path on an Apple M4. This demonstrates headroom on that machine; supported-platform p95/p99 measurements and disk-degradation behavior remain release gates. Reads and effects proven idempotent may use a lighter path, but non-idempotent actions must not bypass durable possible-send recording merely to improve a benchmark.

Do not place IPC, a network broker, a distributed workflow service, or asynchronous safety projection on this fast path.

## Migration plan

Implementation status at the 2026-07-15 checkpoint:

| Phase | Status | Current disposition |
|---|---|---|
| 0. Safety inventory | Partial | The 152-case register exists and production resources are enumerable; generated read/effect and reducer mutation matrices remain. |
| 1. Immediate containment | Completed | Lifecycle, ingest ownership, profile lease, refresh serialization, atomic API snapshots, server-owned actors, direct-traffic pause, and explicit indeterminate outcomes are implemented. |
| 2. Walking skeleton | Completed in existing packages | Account/observation identity, typed resources, durable operations/schedules, dispatch permits, and one physical dispatcher are live without a parallel kernel. Independent capability state ownership remains conditional. |
| 3. Construction/TCI slice | Partial | Typed castle/focus/catalog/resource controls are live; quantity reservation and capability-specific ambiguous purchase/upgrade reconciliation remain. |
| 4. Conflict clusters | Partial | Production definitions map into the central typed vocabulary; generated completeness proof and all-feature stress are pending. |
| 5. API/persistence cutover | Partial | Durable operations, compatible contracts, and gap-aware state/configuration/operation streams are live; task-shaped projections and broader transactional desired-state/history storage remain. |
| 6. Worker extraction | Deferred | Trigger only for simultaneous account fault isolation, remote execution, quotas, or untrusted extensions. |

The detailed phase descriptions below remain the completion criteria for unfinished portions. They no longer imply that a second live kernel should be created before the existing augmentation is exhausted.

### Phase 0: freeze and generate the safety inventory

1. Freeze a parity manifest of every current intent, opcode, response, setting, policy, schedule, and user-visible outcome.
2. Generate the current intent-to-read/write-resource matrix.
3. Generate reducer-to-capability mutation coverage.
4. Label every effect idempotent, non-idempotent, launch, read, or external.
5. Link all 152 register IDs to proposed controls and tests.

Exit condition: no current effect or reducer is unowned.

### Phase 1: immediate containment in the current runtime

Implement independently useful fixes before the kernel cutover:

1. Add one application/service start owner and joined shutdown.
2. Give the ingest worker exclusive drain/discard ownership.
3. Add an OS profile/data-directory lease.
4. Serialize GameData.Manager refresh internally.
5. Return atomic state revision/snapshot envelopes.
6. Make API actor class and effective priority server-owned.
7. Add direct browser traffic detection and automation pause.
8. Add an Indeterminate receipt outcome and prohibit automatic replay after possible send.

Exit condition: the highest-risk lifecycle and bypass cases no longer depend only on convention.

### Phase 2: build the walking skeleton

1. Implement profile/account identity and observation envelopes.
2. Implement the account sequencer and immutable capability store.
3. Implement typed resource keys and overlap.
4. Implement durable operations, steps, idempotency, schedules, and reconciliation records.
5. Implement one effect dispatcher and correlation registry.
6. Feed replay observations through the new runtime without live send authority.

Exit condition: a read-only replay produces deterministic scoped projections and durable operation simulations.

### Phase 3: construction and TCI vertical slice

Port buildings, layout, construction queue, equipped TCI, construction inventory/shop, wallet/economy dependencies, focus, and response barriers together. This slice deliberately exercises:

- account, castle, and building scope;
- parent/child resource overlap;
- costs and quantity reservations;
- focus-derived responses;
- multi-step equip/upgrade/purchase flows;
- TCI identifier namespace rules;
- ambiguous purchase/upgrade recovery.

Run legacy and new planners in shadow against the same captures. Only one runtime has send authority.

Exit condition: golden commands, reducer transitions, receipts, race tests, restart tests, and live fast-path latency match the parity contract.

### Phase 4: port conflict clusters

Port related capabilities together so shared resources cannot remain split between legacy and native safety models:

1. production, crafting, hospital, economy, market, and resource logistics;
2. garrison, movements, stationing, troop transport, leaders, and equipment;
3. defense, attacks, targets, attack presets, and event families;
4. alliance, world map, reports, history, schedules, and automation.

During mixed mode, a bridge must conservatively make legacy claims overlap native resources. Never let the two schedulers independently believe they own the same wallet, focus, castle, commander, or transport.

### Phase 5: API and persistence cutover

1. Move native UI screens to capability projections and gap-aware streams.
2. Keep API v2 behind a compatibility adapter.
3. Migrate desired settings, schedules, receipts, and projection checkpoints transactionally.
4. Rebuild or refresh remote-authoritative game state rather than importing stale snapshots as truth.
5. Remove compatibility state and string claims only after all effect authority has moved.

Exit condition: no live effect path uses the legacy claim manager or global GameState as its safety authority.

### Phase 6: optional worker extraction

Extract AccountRuntime behind versioned IPC only if measured requirements justify it:

- multiple active accounts must survive one another’s crashes;
- remote/headless account nodes are funded;
- per-account CPU/memory quotas are required;
- trusted in-process modules are no longer an acceptable extension boundary.

## Verification gates

### Registration gate

Every capability operation must declare:

- scope resolver;
- read dependencies;
- write/effect resources;
- quantity reservations where applicable;
- physical lanes;
- actor classes allowed;
- idempotency class and key policy;
- completion evidence;
- retry/reconciliation policy;
- cancellation points;
- expected catalog/config/time dependencies.

Registration fails when a write operation omits any required declaration.

### Static conflict gate

Generate a feature-by-resource matrix. Any two write operations sharing an overlapping resource must be:

- serialized;
- mutually excluded by product eligibility;
- admitted by a shared scarce-workflow class; or
- accompanied by a reviewed proof that they are independent.

The matrix is reviewed whenever a capability changes.

### Deterministic concurrency gate

Run manual requests, all enabled policies, scheduler, reports, direct traffic, reconnect, and fake game frames under one deterministic clock. Explore ordering around:

- every plan/admit/dispatch/response/commit boundary;
- focus change and connection replacement;
- cancellation and automation pause;
- queue overflow and subscriber gaps;
- crash/restart at every durable effect phase;
- duplicate, late, missing, and malformed responses;
- target respawn/event rollover;
- catalog/config/time changes;
- disk-full and database-busy paths.

### Runtime gate

- Full Go race detector on the backend.
- Goroutine, watcher, lease, claim, and correlation leak checks.
- Protocol and reducer fuzzing.
- Golden transcript and replay parity.
- Fault injection around transport send and storage commit.
- Live intent-to-send p50/p95/p99 split by uncontended and contended paths.
- Soak test with every automation feature enabled.

### Cutover gate

Every one of the 152 register IDs has:

- Prevented, Reconciled, or Constrained disposition;
- implementation reference;
- test reference;
- telemetry reference;
- failure/degraded behavior;
- named owner.

Zero IDs remain Open or Audit.

## Research findings applied

- The Go memory model says concurrent access to mutable data must be serialized with channels, synchronization primitives, or atomics. This supports immutable capability generations and one explicit mutation authority rather than conventionally shared maps: [The Go Memory Model](https://go.dev/ref/mem).
- Orleans documents the value of single-threaded turns and the race risk introduced by request interleaving across awaits. This supports short account turns with external I/O returning as events, not holding mutable account state across network waits: [Orleans request scheduling](https://learn.microsoft.com/en-us/dotnet/orleans/grains/request-scheduling).
- Temporal demonstrates durable workflow recovery but also exposes retry and workflow-ID policies. Its activity model reinforces that durable orchestration does not remove the need for idempotency and reconciliation of external effects: [Temporal documentation](https://docs.temporal.io/) and [Temporal API semantics](https://api-docs.temporal.io/).
- SQLite documents atomic transactions, multiple concurrent readers with one writer, and WAL checkpoint behavior. This supports one local transactional authority while retaining immutable read projections: [SQLite transactions](https://www.sqlite.org/lang_transaction.html), [SQLite transactional guarantees](https://www.sqlite.org/transactional.html), and [SQLite WAL](https://sqlite.org/wal.html).
- Microsoft’s CQRS and event-sourcing guidance documents both the benefits of separate read projections and the added consistency/schema complexity. This supports capability query projections plus a selective causal journal, not full event sourcing of all game state: [CQRS pattern](https://learn.microsoft.com/en-us/azure/architecture/patterns/cqrs) and [Event Sourcing pattern](https://learn.microsoft.com/en-us/azure/architecture/patterns/event-sourcing).
- The transactional outbox pattern demonstrates committing business state and publication records together. CitadelOps should apply the local equivalent to operation transitions and projection/event cursors even though it does not need a cloud message broker: [Transactional Outbox pattern](https://learn.microsoft.com/en-us/azure/architecture/databases/guide/transactional-out-box-cosmos).

## Repository evidence

The following single reference block records the main code ranges used for this assessment.

~~~text
startLine: 28
endLine: 175
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/State/Store.go
purpose: atomic snapshot/revision, immutable planning view, normalized state generation, and global clone-and-publish mutation

startLine: 12
endLine: 388
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Intent/Resources.go
purpose: typed application/session/account/kingdom/castle/capability resources, compatibility mappings, normalization, and semantic overlap

startLine: 29
endLine: 316
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Intent/OperationStore.go
purpose: SQLite WAL operation reservation, request fingerprints, durable receipt phases, recovery, recent queries, and close ownership

startLine: 36
endLine: 181
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Intent/Claims.go
purpose: atomic typed-resource acquisition, semantic conflict checks, cancellation, and aged waiter ordering

startLine: 193
endLine: 510
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Intent/Engine.go
purpose: durable operation recovery, bounded hot receipts, planning, typed-resource acquisition, and dependency revalidation

startLine: 930
endLine: 990
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Intent/Engine.go
purpose: durable dispatching transition, account/session/connection/focus/catalog permit, transport send, and explicit ambiguous outcomes

startLine: 86
endLine: 290
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Ingest/Pipeline.go
purpose: observation envelope, account/session/connection/focus/catalog lineage capture, guarded commit, and stale-context rejection

startLine: 405
endLine: 505
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Session/Controller.go
purpose: single-owner bounded ingest queue, shutdown discard ownership, and direct-browser outbound detection

startLine: 470
endLine: 530
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Outbound/Router.go
purpose: bounded physical-send routing with priority aging, deadline/FIFO tie-breaking, and pacing

startLine: 64
endLine: 230
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/App/Application.go
purpose: one application lifecycle, profile lease and durable operation-store composition, startup and shutdown ownership

startLine: 36
endLine: 314
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Scheduling/Scheduler.go
purpose: account-scoped schedule versions, deterministic operation IDs, active cancellation, stale-completion fencing, and restart-safe durable reuse

startLine: 270
endLine: 490
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/API/Server.go
purpose: server-owned actor/priority, atomic snapshot/revision responses, and context-owned single-writer WebSocket delivery

startLine: 17
endLine: 70
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Runtime/ProfileLease.go
purpose: stable profile identity and operating-system data-directory execution lease

startLine: 50
endLine: 135
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/GameData/Updater.go
purpose: full refresh serialization and atomic immutable catalog publication

startLine: 210
endLine: 235
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Client/src/api/CitadelClient.ts
purpose: stable UI operation IDs, request fingerprinting, and identical in-flight submission coalescing
~~~

## Conclusion

The augmented architecture is the right control loop for the present single-active-account desktop product. It now has typed hierarchical conflict control, durable idempotency and possible-send recovery, account/session/focus/catalog fencing, lifecycle/process ownership, versioned schedules, direct-traffic pause, and a measured sub-millisecond durable fast path.

The best next change is evidence-driven completion, not another immediate rewrite:

1. generate the complete read/effect-resource matrix and close every omitted declaration;
2. add capability-specific completion and reconciliation for ambiguous remote effects;
3. add deterministic all-feature, crash, overload, and leak tests, retaining the implemented sequence/gap recovery under saturation;
4. measure whole-state clone/ingest pressure and extract capability ownership only if it becomes a real limit;
5. add account workers only if simultaneous multi-account fault isolation makes the additional failure model worthwhile.

This path continues closing the register without sacrificing the current feature set or placing unnecessary IPC and distributed-system overhead in the 25 ms uncontended fast path.

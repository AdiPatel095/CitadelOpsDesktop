# Concurrency, race-condition, and edge-case register

Review checkpoint: 2026-07-15

## Purpose

This document is the working concurrency threat model for CitadelOpsDesktop. It inventories both conventional Go data races and higher-level gameplay races that can occur when manual actions, scheduled operations, background services, and every automation feature are active together.

This is deliberately broader than a list of confirmed bugs. A row marked `Open`, `Partial`, or `Audit` means that the architecture must either close the case or prove why the behavior is safe before an all-features concurrency guarantee is made.

The remote game server remains authoritative. Local locking can prevent CitadelOps operations from conflicting with one another, but it cannot make a command transactional with unrelated browser activity, another CitadelOps process, a reconnect, or a server-side event. Those cases require correlation, freshness checks, idempotency classification, reconciliation, and explicit ambiguous-outcome handling.

## Status and severity legend

| Status | Meaning |
|---|---|
| `Covered` | A direct guard exists in the current implementation and has focused test coverage. |
| `Partial` | A guard exists, but important paths or failure modes remain. |
| `Open` | No general safeguard was found, or the current behavior can produce an unsafe or ambiguous outcome. |
| `Audit` | Correctness depends on game-protocol semantics or complete feature declarations that still need to be demonstrated. |
| `Constraint` | Intentional serialization, pacing, or availability behavior that must be preserved and measured. |

| Severity | Meaning |
|---|---|
| `Critical` | Can send a duplicate or wrong game effect, corrupt castle/account attribution, or spend/consume the wrong resource. |
| `High` | Can make a materially stale decision, lose an operation outcome, or block major functionality under ordinary concurrency. |
| `Medium` | Usually recoverable, but can cause stale UI, delayed automation, excessive retries, or operational instability. |
| `Low` | Defensive hardening, diagnostics, or an unlikely boundary condition. |

## Executive assessment

The augmented runtime now has a substantially stronger safety foundation:

- State mutations are serialized, clone before mutation, and publish state, partition versions, and protocol context as one atomic generation.
- Intent resources are typed, account-scoped, and hierarchical. Semantic parent/child and canonical alias overlap is evaluated centrally, and production registration rejects unmapped legacy claims.
- Intent resources are acquired as a complete set and held through the entire multi-step operation and acknowledged response commit.
- Plans are re-evaluated after claim/admission waits and after relevant state changes.
- Every dispatch is fenced by the bound account, session generation, connection generation, focus epoch, and catalog version.
- Attack launches use an admission class in addition to ordinary resource claims.
- Awaited response opcodes become claims automatically, reducing response cross-delivery.
- One outbound router performs one physical send at a time across command and attack lanes.
- Bounded priority aging prevents indefinite claim/router starvation without preempting an effect already in flight.
- Connection generations fence old commands and frames after socket replacement.
- Observations carry account/session/connection/focus/catalog lineage and are rejected when that lineage is stale.
- Durable operations reserve an ID plus request fingerprint and record possible-send phases before transport. A possibly dispatched non-idempotent effect is recovered as reconciliation-required, never replayed automatically.
- Scheduled operations have account-scoped deterministic IDs, versions, active cancellation, and restart-safe reuse of the durable operation record.
- The Chromium transport has a second physical send gate, fails closed on notice-stream overflow, and pauses automation after uncaused direct browser traffic.
- The data directory/profile has one operating-system lease, and recovered game state has an explicit world/player binding.

The largest remaining risks are:

1. Typed effect resources are mandatory, but explicit versioned read dependencies and effect-specific completion/reconciliation contracts are not yet mechanically complete for every feature.
2. Browser-originated commands are detected and pause automation, but they cannot participate transactionally in CitadelOps claims. Affected capabilities still need authoritative reconciliation before a stronger shared-control guarantee is made.
3. The durable journal prevents unsafe automatic replay, but many capabilities do not yet implement the authoritative query needed to resolve a reconciliation-required operation automatically.
4. State remains one cloned account aggregate. Planning reads are extremely fast, but sustained high-rate ingest still pays a whole-state clone, and only one active account snapshot is retained per runtime.
5. State, configuration, and operation streams now carry explicit sequence/gap metadata and scoped resynchronization, but the generated all-intent conflict matrix plus deterministic all-features fault suite remain incomplete.
6. A 25 ms intent-to-send objective applies to the connected, ready, uncontended first-send path. Game pacing, claim contention, focus changes, durability, and remote responses are separate latency classes.

## Implementation checkpoint — 2026-07-15

The selected implementation is a contained augmentation of the existing modular monolith. It preserves the current feature planners, protocol parsers, command builders, API surface, and single-process transport while replacing the unsafe coordination boundaries underneath them.

Implemented controls:

- typed hierarchical resources for application, session, account, kingdom, castle, capability, leader, target, shop, wallet, inventory, report, schedule, response, and transport scopes;
- atomic multi-resource acquisition, account-qualified identity, bounded priority aging, and production rejection of unknown legacy resource declarations;
- durable SQLite operation reservation, request-fingerprint idempotency, explicit dispatch/possible-send/outcome phases, crash recovery, and bounded in-memory receipt retention;
- account/world binding, profile/data-directory lease, observation lineage, authoritative account-transition reset, focus normalization, and dispatch-time account/session/connection/focus/catalog permits;
- one-owner bounded ingest shutdown, idempotent runtime service starts, serialized game-data refresh, server-owned actor/priority classes, and context-owned WebSocket delivery;
- versioned scheduled operations with deterministic account-scoped operation IDs, active cancellation, reschedule fencing, and indeterminate cancellation handling;
- stable UI operation IDs and coalescing of identical requests while they are in flight;
- gap-aware state/configuration/operation streams, recent durable operation snapshots on reconnect, and client-side scoped resynchronization;
- degraded persistence health plus fail-closed mutating execution when the state snapshot or durable operation journal cannot be written.

Verification completed at this checkpoint:

- the complete `go test ./Server/...` suite and `go vet ./Server/...` passed;
- the race detector passed across all concurrency-critical backend packages;
- the durable operation reservation/dispatch benchmark measured approximately 0.0834–0.0839 ms per operation on an Apple M4;
- the full durable engine path from intent submission through fake transport measured approximately 0.191–0.202 ms per operation;
- the immutable planning view measured approximately 69–80 ns with zero allocations;
- whole-state snapshot/apply remains approximately 0.739–0.747/0.747–0.809 ms and allocates about 3.26 MB per operation, making it the next scaling boundary rather than the intent fast path.

These measurements leave substantial headroom under the 25 ms uncontended target. They do not claim that the Nth operation in a simultaneous burst, an attack pacing interval, or a remote response can complete in 25 ms.

## Current concurrency model

| Actor | Concurrency behavior | Shared surfaces |
|---|---|---|
| Chromium/CDP callbacks | CDP callbacks enqueue socket notices; one notice worker processes the ordered queue. | Socket registry, active token, connection generation, frame channel. |
| Session controller | One controller loop receives status/frame events; one ingest worker commits queued observations. | Session state, ingest queue, outbound router. |
| State store | Any goroutine may request a mutation, but `writeMu` serializes clone/reduce/publish. Readers load an immutable generation. | Complete compatibility `GameState`, partition versions, protocol context. |
| Intent engine | Every submitted operation may plan concurrently; conflicting effects wait on typed resources. | Resource sets, admission slots, state generations, durable operations, sender, response watchers. |
| Automation coordinator | Policies are evaluated from one snapshot; at most one run per policy; different policies submit concurrently. | Configuration, state, intent engine. |
| Scheduler | Polls every 250 ms and starts a goroutine for each due operation. | Scheduled-operation state, intent engine. |
| Report manager | A single `Run` loop processes one next report operation at a time. | Report state, history store, intent engine. |
| Movement clock | Timer-driven state reconciliation runs alongside ingest. | Movements, commanders, Khan state. |
| API | HTTP handlers and WebSocket-submitted intents run concurrently. | Configuration, state snapshots, intent engine, event streams. |
| Persistence/history/telemetry | Background consumers snapshot, append, and flush independently. | Data-directory files and in-memory queues. |
| Game-data updater | Periodic and manual refresh paths can be requested independently. | Official-data cache files, current catalog pointer, hydrated state. |
| Human game client | The game itself can send commands without using the intent engine. | Every remote game resource and the global focus/context. |

## Required invariants

These are the properties the architecture must preserve regardless of feature count or scheduling order.

### Identity and scope

1. A frame, plan, command, response, receipt, and persisted record belongs to exactly one profile, session generation, connection generation, world account, and applicable kingdom/castle.
2. A browser profile logging into another account must never merge the new account with a recovered snapshot from the old account.
3. Castle IDs, kingdom IDs, building instance IDs, building definition IDs, construction-item CIDs, package IDs, unit IDs, leader IDs, movement IDs, report IDs, and event-run IDs remain distinct namespaces.
4. A focus-derived response updates a castle only when explicit wire identity, correlated operation context, or the valid focus epoch proves the target.
5. At most one castle is focused in a session generation. Zero focused castles is valid during bootstrap/reconnect; multiple focused castles is invalid.

### State and observation ordering

6. Readers observe either the complete old generation or the complete new generation, never a mixture.
7. Published immutable generations and game-data snapshots are never mutated by readers.
8. Accepted observations commit in causal wire order for a session connection.
9. An old-connection frame can never mutate the current generation.
10. A failed reducer cannot partially modify the published state.
11. A partition version advances for every capability value whose planning meaning changed, and does not advance for unrelated diagnostics when avoidable.
12. Freshness uses the observation time and lineage of the value being consumed, not merely the latest global state revision.

### Planning and effects

13. Every plan declares all state/catalog/focus inputs it read and every game resource its effects may consume or mutate.
14. Claims overlap whenever two effects cannot safely occur concurrently, including parent/child resource relationships.
15. Compound claims are acquired atomically and released only after all acknowledged response state is committed.
16. Waiting for claims, admission, focus, pacing, or a prerequisite forces revalidation of any dependency that may have changed.
17. A dynamic resolver cannot silently change target, castle, leader, resource, price, or route after its prerequisite checks.
18. A dry run is a preview, not a reservation; clients must not assume it guarantees later execution.
19. Partial multi-step success is represented explicitly. A later failure must not imply earlier remote effects were rolled back.
20. Operation IDs and idempotency keys have defined reuse semantics across retries, reconnects, restarts, and multiple clients.

### Transport and outcome

21. Only one CitadelOps physical send is active on a game socket at a time.
22. A response watcher is installed before its command is dispatched.
23. A response matches the operation by correlation token when supported and by the narrowest safe fallback otherwise.
24. Socket-send success means only that the send was accepted locally; the declared response/commit rule determines game success.
25. Cancellation, timeout, reset, or disconnect after a possible send is recorded as indeterminate unless a response or reconciliation proves the outcome.
26. A possibly sent non-idempotent effect is never automatically replayed.
27. Queue overflow fails explicitly and never silently drops a state-changing outbound command.

### Lifecycle and durability

28. Starting a runtime service twice cannot create duplicate coordinators, schedulers, report managers, persistence loops, or session controllers.
29. Shutdown has one owner for each queue and waits for workers or deliberately discards every remaining item with a recorded reason.
30. A crash between send, response, state commit, receipt update, and snapshot persistence has a defined recovery and reconciliation path.

## Master race and edge-case register

### Memory ownership and Go-level concurrency

| ID | Severity | Status | Scenario and consequence | Current protection | Required proof or change |
|---|---|---|---|---|---|
| MEM-01 | Critical | Partial | A planner/action mutates a map, slice, or pointer reached from `PlanningView`, corrupting the immutable generation and racing with readers. | Store mutations clone before modification; one known building preview mutation was removed. | Make immutable views harder to mutate, audit all planners/helpers, and add mutation-detection tests around every definition. |
| MEM-02 | Critical | Partial | A new nested reference field is added to `GameState` but omitted from `cloneGameState`; snapshots and future generations then share mutable aliases. | The current clone routine manually copies known maps, slices, and pointers. | Add a reflection/property test that mutating any clone-reachable reference cannot change the source; make clone coverage part of model changes. |
| MEM-03 | High | Audit | A caller mutates the `*GameData.Store` returned by `Current`, affecting concurrent planners using the same catalog snapshot. | Refresh builds a replacement and swaps the pointer under a mutex; callers conventionally treat stores as immutable. | Document and enforce catalog immutability or expose read-only interfaces/value projections. |
| MEM-04 | High | Partial | Raw JSON or request byte slices are retained and later mutated by a caller while an operation is planning/executing. | Several request/state paths copy `json.RawMessage`; outbound copies payload bytes. | Audit every registration, schedule, receipt, and state assignment for defensive copying. Add alias tests. |
| MEM-05 | High | Covered | `Application.Start` is called more than once, creating duplicate runtime service loops. | Application startup is process-idempotent and concurrent starts share one lifecycle transition. | Retain concurrent-start coverage whenever a new service loop is added. |
| MEM-06 | High | Covered | `Coordinator.Run` is started twice, allowing the same automation policy to execute twice concurrently. | The coordinator owns an atomic one-run guard. | Retain double-start and shutdown coverage. |
| MEM-07 | High | Partial | `Reports.Manager.Run` is started twice or a report side effect repeats across restart. | The manager owns a one-run guard and report/message resources serialize live work. | Add durable per-message history/share/archive idempotency; the process-local duplicate-run race is covered. |
| MEM-08 | Medium | Covered | Concurrent registry reads and startup registration race. | Intent and ingest registries use mutexes; normal mutation finishes during startup. | Keep registration startup-only and retain race tests if hot registration is ever introduced. |
| MEM-09 | Medium | Partial | A state mutation callback recursively calls `Store.Apply`, deadlocking on the non-reentrant write mutex. | Current reducers/actions generally mutate through one callback. | State the no-reentrancy rule, add a development assertion or architectural lint, and keep I/O/callbacks outside mutations. |
| MEM-10 | Medium | Audit | Lock-order inversion occurs between state, subscriber, session, transport, telemetry, or file locks during shutdown/error handling. | Most code copies data and releases locks before external calls. | Maintain a lock-order document and add shutdown/race stress tests; never invoke user/network callbacks while holding core locks. |
| MEM-11 | Medium | Partial | A subscriber is removed/closed while a publisher sends, producing a send-on-closed-channel panic. | State subscriptions remove without closing; configuration updates and unsubscribe share one mutex. | Preserve ownership rules and test concurrent publish/unsubscribe for every subscription implementation. |
| MEM-12 | Medium | Partial | Telemetry is closed while late ingest/intent goroutines continue recording. | Telemetry has memory/file/persistence mutexes and a closed flag. | Stress close versus record/flush and ensure late records fail safely without blocking shutdown. |

### State publication, reducers, and ingest ordering

| ID | Severity | Status | Scenario and consequence | Current protection | Required proof or change |
|---|---|---|---|---|---|
| STATE-01 | Critical | Covered | Readers see state from one revision and partition versions/protocol context from another. | All three are stored in one atomic `storeGeneration`. | Keep the atomic-generation invariant and its concurrency tests. |
| STATE-02 | Critical | Partial | Two committers reduce observations out of wire order. `writeMu` prevents simultaneous mutation but does not itself impose ingress-ID order. | The live controller has one ingest commit worker. | Make ordered commit authority explicit; reject/quarantine out-of-order ingress IDs or centralize all commits in one account loop. |
| STATE-03 | Critical | Covered | A focus-derived frame is attributed using a focus that changed after observation but before commit. | The observation envelope captures session, connection, focus epoch/castle, account, catalog, causation, and ingress identity; commit validates the lineage before and after reduction. | Retain focus-change-before-commit tests and register every newly focus-derived opcode. |
| STATE-04 | Critical | Covered | Recovered or malformed state contains multiple `Focused=true` castles. | Store normalization deterministically selects the smallest castle ID on load and preserves the valid current focus on malformed mutations. | Retain load/mutation normalization tests. |
| STATE-05 | Critical | Covered | A frame from a replaced socket mutates current state. | Frame acceptance and commit both compare connection generation; stale status updates are rejected. | Retain replacement/reconnect race tests. |
| STATE-06 | High | Partial | A frame with connection generation zero bypasses fencing and is accepted during/after reconnect. | Zero is allowed for compatibility/replay. | Restrict zero-generation observations to explicitly trusted offline/replay adapters; require nonzero generation in live Chromium mode. |
| STATE-07 | High | Covered | A reducer fails after changing its candidate and publishes a partial mutation. | Every mutation operates on a clone and publishes only after the callback succeeds. | Retain reducer failure tests. |
| STATE-08 | High | Partial | The transport/frame/ingest queues fill. Backpressure blocks status processing or the notice stream overflows, causing delayed reconnect fencing or lost observations. | Buffers are bounded; notice overflow invalidates the active socket and fails closed. | Expose queue depth/high-water marks, define maximum block time, and test overload recovery with an authoritative refresh. |
| STATE-09 | High | Covered | Controller shutdown closes or drains the ingest queue while multiple goroutines compete for queued frames. | A bounded single-owner ingest queue has explicit cancellation discard semantics, and shutdown joins its owner. | Retain stop/frame-close/status-close interleaving tests. |
| STATE-10 | High | Partial | A wire watcher receives a matching frame, but its buffer is already full and the actual response is dropped. | Watcher buffers hold one frame; response-token filtering narrows Chromium matches. | Guarantee one matching delivery per token, or use a per-operation correlation registry with explicit overflow errors. |
| STATE-11 | High | Partial | One ingress frame is delivered to multiple commit waiters; the first waiter removes the commit record and another sees “not tracked.” | Automatic response-opcode claims normally prevent overlapping watchers. | Enforce single owner per correlated ingress ID or make commit records reference-counted. Add adversarial watcher tests. |
| STATE-12 | High | Partial | An unsolicited/manual inbound frame with the expected opcode satisfies an uncorrelated watcher for a CitadelOps command. | Chromium correlation tokens are used when supported; response-opcode claims serialize local waiters. | Treat uncorrelated transports as reduced-safety mode; match route, payload identity, focus epoch, and send sequence where possible. |
| STATE-13 | High | Audit | Explicit payload scope and current focus disagree. A reducer chooses the wrong authority and mutates the wrong castle/kingdom. | Some reducers parse explicit IDs; others intentionally use focused castle. | Define precedence for every opcode: explicit wire identity, correlated operation scope, captured focus, or quarantine. Test conflicting inputs. |
| STATE-14 | Medium | Partial | Unknown or diagnostic frames advance global revision and force unrelated legacy intents to replan repeatedly. | Scoped partitions can avoid replan for definitions with read sets. | Separate observation/journal position from domain revisions; finish read-set migration. |
| STATE-15 | Medium | Partial | State subscriber buffers overflow. Coalescing preserves latest revision/domains but intermediate semantic transitions are lost. | State events are coalesced rather than silently dropped. | Require consumers to treat events as invalidations, never an event log; add gap-aware projection subscriptions for clients needing deltas. |
| STATE-16 | Medium | Covered | State, configuration, or intent subscribers overflow while the consumer is slow. | Published events carry monotonic source sequence and an explicit gap flag when a pending event is coalesced; configuration events carry a full snapshot, operations expose a recent durable snapshot, and the primary client performs scoped resynchronization. | Retain full-buffer/reconnect tests and apply the same contract to future safety-relevant streams. |
| STATE-17 | Medium | Partial | An older observation arrives late with a valid current generation and overwrites a newer capability value. | Live notice sequencing and ordered queueing reduce this risk. | Track per-capability observation position/time and reject regressions; do not rely only on global commit order for merged sources. |
| STATE-18 | Medium | Constraint | Full-state clone mutation takes about 0.8 ms and a very high inbound rate backs up commits even though planning readers remain nonblocking. | Single-writer clone-and-publish preserves correctness. | Measure ingest lag and eventually transact only affected capability partitions. |
| STATE-19 | Medium | Partial | `Wire` barrier cleanup deliberately ignores cancellation for up to ten seconds while committing acknowledged state, delaying shutdown and claim release. | This prevents an acknowledged response from being lost before claims release. | Keep the safety behavior, expose commit-cleanup latency, and bound shutdown expectations. |
| STATE-20 | Medium | Audit | Reducer domain/partition declarations are too narrow, so a plan skips revalidation after data it read changed. | Initial scoped partitions exist; building/TCI read sets have focused tests. | Add reducer-to-capability coverage tests and compare actual changed fields with declared partitions. |
| STATE-21 | Low | Partial | Revision or partition version reaches `uint64` maximum and stops behaving monotonically. | Session generation increment guards its maximum in one path. | Define overflow behavior consistently; fail closed or start a new durable epoch. |

### Intent planning, claims, admission, and execution

| ID | Severity | Status | Scenario and consequence | Current protection | Required proof or change |
|---|---|---|---|---|---|
| INTENT-01 | Critical | Covered | Resource equality fails to express hierarchy or aliases such as wallet/currency and account/castle capability relationships. | Typed account-scoped resource keys use one semantic overlap function for parents, children, and canonical aliases; focused tests cover cross-castle and synonymous resources. | Retain overlap symmetry/transitivity tests and extend the vocabulary only through central constructors. |
| INTENT-02 | Critical | Partial | A new planner omits a resource it spends/mutates. | Production plan finalization requires a nonempty typed resource set and rejects unknown or unmapped legacy declarations. | Generate the all-intent effect matrix and prove that each declared set is semantically complete; nonempty enforcement cannot infer an omitted second resource. |
| INTENT-03 | Critical | Partial | State changes after final plan revalidation but before or during a later step. Local claims block CitadelOps peers, not ingest, the human game client, or remote timers/events. | Dynamic resolvers re-read state; response barriers commit prerequisites. | Give each effect step explicit preconditions/version dependencies and fail or rebuild immediately before send. |
| INTENT-04 | Critical | Partial | A multi-step operation sends one or more successful effects, then a later step fails. The final receipt says failed although remote state changed. | Completed step checkpoints and captured exchanges retain some evidence. | Model `partially-succeeded`/`indeterminate`, list committed effects, and define compensation/reconciliation rather than rollback fiction. |
| INTENT-05 | Critical | Covered | The same operation ID is submitted again after completion or restart. | The SQLite operation store reserves operation ID plus canonical request fingerprint; exact reuse returns the durable operation while conflicting reuse is rejected. | Retain restart, concurrent reservation, and same-ID/different-request tests. |
| INTENT-06 | Critical | Partial | Two clients omit IDs or independently create distinct IDs for the same non-idempotent effect. | The primary UI creates a stable ID for every submission and coalesces identical in-flight actions; claims still serialize distinct IDs. | Add intent-specific semantic duplicate windows only where product semantics make repeated user actions unambiguously accidental; third-party callers must retain their IDs on retry. |
| INTENT-07 | High | Covered | A plan waits for a conflicting claim and then executes stale. | The engine replans after claim wait/admission transition or changed revision. | Retain targeted replan tests. |
| INTENT-08 | High | Partial | A definition declares a read set that misses a dependency, so an unrelated-looking partition change allows a stale plan to skip re-planning. | Read sets are currently limited to building/TCI and version-tested. | Add planner read tracing or explicit dependency review tests for every migrated definition. |
| INTENT-09 | High | Constraint | Definitions without read sets replan on any global revision, causing excess CPU and possible progress delay during noisy protocol traffic. | Conservative behavior is safe. | Migrate feature slices to complete scoped read sets before optimizing further. |
| INTENT-10 | High | Partial | A resolver changes route/target after command prerequisites ran. | Command dependency keys prevent a resolved opcode route key from changing. | Extend identity checks to castle, leader, inventory item, price, amount, and catalog version. |
| INTENT-11 | High | Partial | Cancellation occurs between steps. `ResumeOnce` skips completed local steps, while a possibly sent step may lack proof and be rebuilt/replayed. | Wire commits are flushed before yielding; resume policies distinguish rebuildable steps. | Classify every step as read/idempotent/non-idempotent and prohibit replay of ambiguous non-idempotent steps. |
| INTENT-12 | High | Covered | Attack features launch concurrently or starve one another indefinitely. | Shared attack admission, priorities, configured weights, aging, `attack-context`, commander/inventory claims, and pacing. | Retain admission/fairness tests and expose wait metrics per module. |
| INTENT-13 | High | Partial | Non-attack scarce workflows have no admission class, causing large numbers of valid operations to contend on claims/router and create head-of-line latency. | Ordinary claims and router bounds still preserve basic safety. | Add admission only for demonstrated scarce workflows; do not turn every feature into a global lane. |
| INTENT-14 | High | Covered | Continuous higher-priority work starves lower-priority claim waiters or outbound commands. | Claims and outbound routing apply the same bounded time-based priority aging while preserving deadline/FIFO tie-breaking. | Retain deterministic eventual-service tests and expose oldest-wait telemetry. |
| INTENT-15 | Medium | Constraint | A high-priority operation waits behind a low-priority operation already holding claims. Preemption would be unsafe mid-effect. | Claims are held to completion. | Accept bounded priority inversion, shorten operations safely, and report holder/wait duration. |
| INTENT-16 | High | Partial | Claims are held across a slow or lost response for up to the step timeout, blocking every related feature. | Timeouts and cancellation eventually release them. | Tune per-opcode timeouts from evidence and expose claim-holder step/age. |
| INTENT-17 | High | Partial | Admission is acquired before ordinary claims. An admitted operation cannot obtain claims and delays attack work. | A five-second admission-claim timeout releases admission and retries. | Test cyclic/high-load contention and report retry churn. |
| INTENT-18 | High | Audit | Two syntactically different claim names refer to the same wire/server resource, such as shop versus construction shop, resource versus currency balance, or target coordinates versus target object ID. | Shared response-opcode claims sometimes serialize these accidentally. | Build a canonical resource vocabulary and prohibit private synonyms. Do not rely on opcode claims as resource locks. |
| INTENT-19 | High | Covered | Game-data refresh swaps the catalog between planning and physical send. | Plans carry catalog version and every dispatch permit compares it with the current catalog; a mismatch triggers safe re-planning before the first or a later chained send. | Retain refresh-between-plan-and-send tests; configuration versions remain a separate incomplete dependency. |
| INTENT-20 | Medium | Covered | A dry run accidentally acquires resources or sends commands. | Dry run returns immediately after planning. | Clearly label previews as non-reservations in API/UI. |
| INTENT-21 | Medium | Partial | Global `ExpectedRevision` rejects an operation because an unrelated feature changed, or is accepted once and later dependencies change. | Explicit check prevents use of a known stale global snapshot. | Prefer per-resource expected versions/ETags for native capability requests. |
| INTENT-22 | Medium | Covered | The in-memory receipt map grows without eviction during a long-running desktop session. | With durable storage configured, the engine bounds its hot receipt cache to 10,000 entries while SQLite retains idempotency evidence and queryable outcomes. | Add database retention/archival policy only when measured history size requires it. |
| INTENT-23 | Medium | Audit | A local `Action` mutates state based on arguments captured before a remote command, while another observation changes the same state before the action runs. | Actions execute while operation claims remain held; state mutation itself is atomic. | Revalidate authoritative identifiers and amounts inside every post-command action. |
| INTENT-24 | Medium | Partial | The automation lock activates while a step is already sending or waiting for a response. | The gate pauses before claims and at safe step boundaries; it does not interrupt an unsafe in-flight effect. | Preserve safe-boundary semantics and make UI say “pausing” until the current effect settles. |
| INTENT-25 | Medium | Audit | Claim keys omit account/world scope. In a future multi-account engine, identical castle/leader IDs could block one another or, if managers are shared incorrectly, conflict. | Current engine/controller is effectively one account session. | Typed claims must include `WorldAccountKey` before multi-account execution. |
| INTENT-26 | Low | Audit | Operation ID generation collides across processes with the same millisecond and counter sequence. | Timestamp plus process-local atomic counter makes collision unlikely in one process. | Use UUID/ULID plus durable idempotency namespace for cross-process/multi-node use. |

### Outbound routing, response correlation, and ambiguous effects

| ID | Severity | Status | Scenario and consequence | Current protection | Required proof or change |
|---|---|---|---|---|---|
| OUT-01 | Critical | Covered | Two CitadelOps goroutines write the game WebSocket simultaneously and interleave context/effect commands. | One router goroutine dispatches sequentially across both lanes; Chromium has a second send gate. | Retain physical-send serialization tests. |
| OUT-02 | Critical | Partial | The human game client sends directly on the same socket while CitadelOps operates. Direct game traffic bypasses claims, admission, pacing, and router ordering. | CitadelOps propagates causation IDs through both transports; uncaused outbound game traffic pauses/yields background work until a quiet interval. | Add capability-aware authoritative reconciliation or an optional exclusive-control mode before promising safe shared manual/automation control. |
| OUT-03 | Critical | Partial | Context cancellation, timeout, router reset, or socket replacement happens after the browser may have sent the payload. Caller receives failure while the remote effect succeeds. | Chromium marks send errors indeterminate and invalidates/reloads the socket. | Return an explicit indeterminate outcome and reconcile before any retry. |
| OUT-04 | Critical | Partial | Router `Reset` completes an in-flight command with `ErrReset` while its send function may still return after having transmitted. | Command context is cancelled and duplicate completion is suppressed. | Track send phase (`queued`, `dispatching`, `possibly-sent`, `acknowledged`) and never treat reset as proof of no effect. |
| OUT-05 | Critical | Partial | Browser evaluation times out after JavaScript executed `send`, or CDP loses the evaluation result. | Active socket is invalidated and session reloads. | Classify as indeterminate and use operation-specific reconciliation. |
| OUT-06 | High | Covered | Response arrives before watcher registration. | Watchers are registered before sender dispatch. | Retain fast-response tests. |
| OUT-07 | High | Partial | A response token is not echoed, is duplicated, or attaches to the wrong browser frame. | Token and expected opcode list are passed through injected send logic. | Fault-test token loss/duplication and fall back only when route/focus/send sequence proves safety. |
| OUT-08 | High | Partial | A late response from a timed-out command arrives and is interpreted as a new operation’s response. | Unique response tokens prevent this in correlated Chromium mode; opcode claims serialize local uncorrelated waits. | Quarantine expired tokens and test late-frame delivery around retry/reconnect. |
| OUT-09 | High | Covered | The server returns an unsuccessful code but the operation is marked successful. | Steps declare success codes and response reduction errors fail the step. | Require explicit success policy for every write/launch effect. |
| OUT-10 | High | Partial | A command declares no awaited response and is marked successful after local send even though the server rejects it. | Most game mutations use response steps; socket-send success is documented as non-authoritative. | Inventory every effect and require outcome evidence or explicit `fire-and-reconcile` classification. |
| OUT-11 | High | Constraint | Simultaneous feature bursts queue behind one physical sender and 25 ms normal-command pacing. The Nth command cannot meet a 25 ms latency objective. | Queue/transport/intent latency telemetry distinguishes fast path and contention. | Define 25 ms only for connected, focused, uncontended first send; publish separate contention SLOs. |
| OUT-12 | High | Covered | Outbound queues grow without bound. | Each lane has a bounded queue and rejects overflow. | Expose queue size, rejection counts, oldest age, and per-actor backlog. |
| OUT-13 | High | Covered | Continuous interactive/high-priority commands starve automation commands until their context deadlines expire. | The router applies bounded time-based priority aging in addition to priority, deadline, and FIFO ordering. | Retain eventual-service tests and surface oldest queued age. |
| OUT-14 | Medium | Constraint | Normal commands can dispatch during the attack lane’s 4–6 second delay. | Pacing is per lane while physical sends are global. | Preserve if protocol evidence shows only CRA-to-CRA spacing is required; otherwise introduce a shared cooldown class. |
| OUT-15 | Medium | Audit | Pacing is measured from dispatch start, not confirmed server response. Slow sends can change effective spacing. | Physical sends remain serialized. | Define whether the game requires send-start, send-complete, or response-based pacing per command class. |
| OUT-16 | Medium | Partial | Deadline expires just after physical send but before response. The operation reports timeout although the effect may complete. | Response waiting and session checks are bounded. | Treat post-send deadline expiry as indeterminate for non-idempotent effects. |
| OUT-17 | Medium | Partial | Oversized/invalid outbound payload spends queue time or exceeds browser evaluation limits. | Router validates protocol decode and rejects empty payload; browser evaluation has a timeout. | Add effect-specific payload limits before enqueue and telemetry for encode/evaluation size. |
| OUT-18 | Medium | Covered | Two CitadelOps processes/controllers target the same game socket/profile. | Application startup acquires an operating-system lease for the stable profile/data directory before execution authority is created. | Retain contention/release tests; independent profiles remain intentionally independent authorities. |

### Session, browser, focus, and reconnect lifecycle

| ID | Severity | Status | Scenario and consequence | Current protection | Required proof or change |
|---|---|---|---|---|---|
| SESSION-01 | Critical | Partial | `Start`, `Stop`, and another `Start` overlap. An older start failure/run can clear or cancel the newer controller state. | Intent-level `session` claim serializes normal UI operations; transport generation rejects superseded Chromium starts. | Add controller-owned lifecycle generation and compare-before-clear/cancel. Stress concurrent direct calls. |
| SESSION-02 | Critical | Partial | An old controller run observes a shared channel closure after a new run starts and cancels the new run’s `cancel` function. | Ordinary stop/start timing and transport generation reduce exposure. | Bind every run goroutine to an immutable run ID and only mutate matching controller state. |
| SESSION-03 | Critical | Covered | An older WebSocket becomes active after a newer socket. | Socket ordinal, sequence, generation checks, and the physical send gate protect activation. | Retain socket replacement tests. |
| SESSION-04 | Critical | Covered | Reconnect leaves a stale focused castle and automation sends against it. | Protocol context clears focused castle and advances focus epoch on session/connection change; baseline gating blocks sends. | Require all focus-dependent reducers/plans to consume protocol context, not just compatibility flags. |
| SESSION-05 | Critical | Partial | The same browser profile logs into a different world/account while recovered state still contains the prior account’s capability data. | State persists a stable world/player binding. An authoritative baseline mismatch atomically clears prior account capability data while preserving runtime lineage; stale transition frames are rejected. | A single runtime safely quarantines/resets between accounts, but retaining and concurrently operating multiple account snapshots requires an account-partition bank or worker per account. |
| SESSION-06 | Critical | Covered | Two application processes use the same Chromium profile/data directory. | A stable profile identity and operating-system data-directory lease prevent a second process from starting against the same authority. | Retain stale-process/release/platform tests. |
| SESSION-07 | High | Covered | Automation runs after socket login but before the current authoritative GBD baseline. | Session baseline generation gates coordinator evaluation and intent sends. | Retain ready/baseline transition tests. |
| SESSION-08 | High | Covered | Socket-notice queue overflows and frames are silently lost. | Overflow clears socket state and marks the session error. | Automatically refresh authoritative baseline after recovery and expose overflow count. |
| SESSION-09 | High | Partial | Frame channel or ingest queue backpressure blocks the socket notice worker/controller long enough to delay status changes and reconnect handling. | Bounded queues and generation checks prevent many stale commits. | Add nonblocking status priority or separate ordered control/data envelopes; monitor lag. |
| SESSION-10 | High | Partial | Status updates have equal/older wall-clock timestamps and are rejected or accepted incorrectly after a clock adjustment. | Connection generation is the primary fence; timestamps break ties/order within a generation. | Use monotonic run-local sequence numbers, not wall clock, for status ordering. |
| SESSION-11 | High | Partial | Execution context is destroyed or active socket changes while a command waits for the send gate. | Send re-reads active context/token/generation after acquiring the gate. | Retain destroy/activate/send interleaving tests. |
| SESSION-12 | High | Covered | Login cooldown reload fires after a newer connection/session exists. | Reload checks transport generation and connection generation. | Retain timer/reconnect tests. |
| SESSION-13 | Medium | Audit | Network WebSocket events and injected socket notices disagree about readiness/closure. | Injected active-token state and ordered status guards reduce stale updates. | Define one authoritative socket lifecycle source and use the other only as diagnostics. |
| SESSION-14 | Medium | Partial | Namespace changes between planning and command encoding. | Namespace is read immediately before encoding and session generation is fenced. | Include namespace/protocol version in session dependency and reject unsupported transitions. |
| SESSION-15 | Medium | Partial | Browser crashes during an app update, state snapshot write, or telemetry flush. | Atomic replacement protects several files; runtime context cancels services. | Add crash-recovery integration tests across each effect phase. |

### Automation, scheduling, configuration, and service loops

| ID | Severity | Status | Scenario and consequence | Current protection | Required proof or change |
|---|---|---|---|---|---|
| AUTO-01 | High | Covered | One policy submits the same decision again while its previous intent is running. | Per-policy `running` state and repeated-decision safety pause. | Retain continuous-chain tests. |
| AUTO-02 | High | Covered | Different policies evaluate the same snapshot and submit conflicting requests. | Intent claims/admission and re-planning arbitrate after submission. | Complete typed claims/read sets so this remains true for every feature pair. |
| AUTO-03 | High | Partial | Relevant state/config event is dropped because a subscriber buffer fills, so a policy sleeps too long. | Coordinator also ticks every two seconds and compares current fingerprints/revisions. | Expose missed/coalesced event metrics and keep polling recovery. |
| AUTO-04 | High | Covered | Policy is disabled or materially reconfigured while its intent is running. | Coordinator cancels disallowed runs; intent pauses/cancels at safe boundaries. | Document that an already sent effect cannot be undone. |
| AUTO-05 | High | Partial | Configuration changes after policy evaluation but before effect send. | Relevant changes cancel runs; execution claims and resolvers revalidate state, not all configuration inputs. | Include configuration section versions in operation dependencies and recheck before send. |
| AUTO-06 | Critical | Covered | A scheduled non-idempotent operation sends successfully, then the process crashes before schedule completion persistence. | Every schedule version has a deterministic account-scoped operation ID and reuses the durable operation record; recovered possible-send phases require reconciliation and are never automatically replayed. | Retain restart tests at each durable phase and add capability reconciliation implementations. |
| AUTO-07 | High | Covered | A scheduled operation is cancelled while its submitted intent is running. | The scheduler retains the active intent ID and cancellation context, calls engine cancellation, and records cancelled-before-send versus reconciliation-required after possible send. | Retain cancellation tests at queued, dispatching, and indeterminate boundaries. |
| AUTO-08 | High | Covered | A scheduled ID is rescheduled with new arguments/time while the old execution is already running. | Rescheduling increments a durable version, cancels the prior operation, and prevents the old version’s completion from overwriting the replacement. | Retain reschedule-during-run and stale-completion tests. |
| AUTO-09 | High | Partial | Process clock jumps forward/backward around `ExecuteAt`, event end, cooldown, or weekly schedule boundaries. Work may fire early, late, twice after restart, or not at all. | Times are normalized to UTC; timers/tickers drive reevaluation. | Persist last-fired schedule version, use monotonic time in-process, define DST/time-zone behavior, and test clock jumps. |
| AUTO-10 | High | Covered | Client-controlled `Actor`/priority labels make scheduled/background work appear interactive. | HTTP/WebSocket requests are normalized to the server-owned UI actor and interactive priority; scheduler submissions use a server-owned background actor. | Retain entry-point tests and apply the same normalization to future transports. |
| AUTO-11 | Medium | Partial | Configuration clients update without expected revision/value and overwrite each other (last writer wins). | Store serializes writes and supports revision/value conditions. | Require conditional writes in UI/API for edits based on a prior view. |
| AUTO-12 | Medium | Covered | App update check and install run concurrently. | AppUpdate manager serializes both through an operation mutex. | Retain operation serialization tests. |
| AUTO-13 | High | Covered | Periodic and manual game-data refresh run concurrently and publish competing cache/catalog snapshots. | The game-data manager serializes the complete refresh operation in addition to atomic catalog publication. | Retain concurrent refresh tests and cancellation behavior. |
| AUTO-14 | Medium | Partial | Policy immediately chains follow-up work after success while the authoritative state response is delayed or incomplete. | Decisions can request reevaluation and intents use response barriers. | Require named completion evidence/freshness for every chain transition. |
| AUTO-15 | Medium | Partial | Report fetch/archive repeats after history append succeeds but state status update fails/crashes, creating duplicate JSONL records or shares. | Report manager processes serially and message claims serialize intents. | Add durable report-message idempotency and deduplicate history by message/report ID. |
| AUTO-16 | Medium | Constraint | One slow report fetch blocks other report processing because manager intentionally processes one next item at a time. | Serial manager avoids report-context collisions. | Measure backlog and safely parallelize only with message-scoped claims/correlation. |
| AUTO-17 | Medium | Partial | State snapshot persistence is debounced for two seconds; a crash loses recent local-only scheduler/automation/workflow state even if remote effects occurred. | Shutdown flushes and writes use atomic rename. | Move execution records to synchronous/durable transactions or reconcile recovered state before work resumes. |
| AUTO-18 | Low | Audit | Timer reset races cause a stale timer event to trigger an early persistence/history/policy pass. | Several loops stop/drain timers; persistence/history debounce code varies. | Standardize timer stop/drain/reset helpers and test event-versus-timer selection. |

### Feature-level conflict inventory

The following table records the important feature intersections. `Covered` means current claims generally overlap; it does not remove the need for typed claim/read-set verification.

| Shared resource or context | Features that can collide | Status | Notes and required action |
|---|---|---|---|
| Global castle focus | Buildings, TCI, production, hospital, defense, attacks, scans, stationing, kingdom troop transport, construction shop | `Partial` | Most focus-dependent intents declare `castle-focus`, which safely serializes even different castles. Audit every focus-derived opcode and make focus a typed session claim. |
| One castle’s general state | Building, defense, production, garrison, attack, station, resource transfer | `Covered`/`Broad` | Most declare `castle:<id>`. This is safe but can serialize independent capabilities. Narrow only after typed write sets prove independence. |
| Account wallet/currencies | Shops, crafting rental/skip, production, hospital, building expansion, TCI purchase/upgrade, dungeon/kingdom time skips, equipment upgrades | `Partial` | Typed wallet, currency, inventory, and shop aliases now overlap centrally. Complete per-feature read/quantity declarations and send-time reserve checks remain to be proven. |
| Castle resource balance | Building construction/upgrade/expansion, production, market shipment, crafting, food balancing | `Partial` | Castle claims often serialize these, but the architecture should claim exact castle resource/reserve keys and include projected consumption. |
| Construction queue | Construct, upgrade, demolish, finish-free, time skip, AutoTCI host changes | `Covered`/`Broad` | Building helper claims layout/construction for the castle. Verify TCI host mutation and building mutations remain overlapping after focus claim is narrowed. |
| Building footprint/layout | Place, move, store, demolish, expand, preset decoration | `Partial` | Building intents claim layout plus position/instance. Decoration preset uses broader layout context; generate footprint overlap tests including rotations and fixed layers. |
| Construction-item inventory | TCI purchase, equip, upgrade/renew, shop refresh | `Partial` | Typed focus, castle capability, inventory, shop, wallet, and response resources overlap centrally. Explicit quantity/read dependencies and purchase reconciliation remain. |
| Construction-item identity | CID, building OID, package PID, decor/building wodID | `Open` as maintenance risk | Preserve separate types/catalogs. Never resolve a TCI CID with building/decor data; verify BCID replacement and level from construction item catalog. |
| Production line capacity | AutoRecruit, AutoTool, manual enqueue, alliance help, hospital | `Partial` | Production uses castle and line claims; line claim lacks castle in its string but castle claim disambiguates. Move to `ProductionLineKey(CastleKey, line)`. |
| Crafting building/slots | AutoCrafting, manual start/skip/rent, resource overflow logistics | `Partial` | Building and account-resource claims cover many paths; add slot-level write claims and recipe/catalog dependencies. |
| Hospital wounded units/queue | AutoHospital, manual heal/discard, incoming battle updates | `Partial` | Local intents serialize through castle/focus/hospital; server battle frames can change wounded counts mid-operation. Revalidate exact unit/amount immediately before send. |
| Source troops | Attacks, stationing, kingdom troop shipping, AutoBird/AutoStation, defense setup | `Partial` | Source castle claims usually overlap local effects. Add typed garrison/unit-stack claims and reconcile direct browser or battle changes. |
| Commander availability | Every attack module, equipment changes, movement completion | `Covered`/`Partial` | CRA builders claim commander plus `leader:commander:<id>`; equipment uses the same leader key. Server/direct-browser movement can still change availability after revalidation. |
| Castellan/equipment/defense | Equipment reconfiguration, defense presets, AutoKhan defense | `Audit` | Determine which defense operations depend on castellan loadout and whether leader/equipment claims must overlap castle defense claims. |
| Attack launch lane | Rift, tower, invasion, nomad, RBC trial, Khan, Storm, future attacks | `Covered` | Shared attack admission and automatic `attack-context` for CRA. Preserve module weights, aging, pacing, and commander/target claims. |
| Attack dialog/presets | All CRA launchers and direct human attacks | `Partial` | CitadelOps commands rebuild and verify context before CRA; human game commands bypass claims and can replace the dialog. Detect direct outbound activity. |
| Same map target | Tower, Nomad/Samurai, invasion, Khan, Storm, scans, cooldown skips | `Partial` | Target claims exist in attack helpers but formats vary by feature. Canonicalize `TargetKey` using kingdom, type, object ID when available, and coordinates. |
| Target cooldown | Attack launch, minute skip, post-battle report/map refresh | `Partial` | Pending-cooldown guards exist for several features. Test late reports, loss versus victory, target respawn/reuse, and event-camp ID changes. |
| Map kingdom | Map query/scans and target selection in multiple features | `Partial` | Scans use `map:<kingdom>`; attacks often use target claims instead. Read versions must detect map row replacement without unnecessarily blocking all attacks. |
| Event difficulty | Nomad/invasion selectors and attack policies | `Partial` | Global `event-difficulty` claim may serialize different event families. Split by event run after protocol proof. |
| Event score/end time | Nomad, invasion, Khan, Storm policies and purchases | `Audit` | Require event-run identity and freshness; guard exact boundary, delayed frames, and event rollover with reused IDs. |
| Resource transport lane | Manual/automatic market and kingdom resource shipments | `Covered`/`Broad` | `resource-transport` globally serializes. Determine whether same-kingdom barrows and cross-kingdom lanes can safely separate. |
| Resource versus troop kingdom transport | Food/crafting logistics versus AutoBird/troop transfers | `Audit` | They use different global claims but share target kingdom keys in some combinations. Confirm server permits independent resource and troop pending lanes. |
| Time-skip inventory | Building, dungeon, resource transport, troop transport | `Partial` | Legacy aliases map into typed wallet/inventory scope, preventing local conflicting spends. Complete stack identity, amount reservations, and authoritative post-spend reconciliation remain. |
| Alliance directory/holding | Alliance refresh/inspect, AutoBird, AutoStation, station route | `Partial` | Refresh and station use different claims. State-replan handles changes before execution, but holding deletion/move during route setup needs a send-time guard. |
| Equipment inventory/item | Equip, unequip, swap, reconfigure, gem actions, upgrade, sell | `Covered`/`Broad` | `game:equipment` serializes all current equipment work. Later narrow to item and leader claims only after inventory atomicity is understood. |
| Shop offer quote/confirm | Manual offer purchase and confirmation, other shop purchases | `Open` | Quote and confirm are separate intents; the shared claim is released between them. Another operation or offer expiry can invalidate the quoted price. Persist a server quote token/version/expiry and bind confirmation to it. |
| Shop stock/history | Package purchases, TCI shop, Mercenary Post, event shops | `Partial` | Several claim namespaces and opcodes overlap. Canonicalize shop instance/table/offer/stock keys and revalidate live stock at send time. |
| Reports/message capture | Report manager and manual fetch/share | `Covered`/`Partial` | Message-scoped/report claims serialize local fetches; archive/history effects need durable idempotency across crash/restart. |
| Rift launch/template | Template rename/delete, schedule/cancel/replay, commander and source castle | `Partial` | Launch and scheduled-operation claims cover local edits. Version templates and bind scheduled replay to the captured template revision. |
| Scheduled operation ID | Schedule, reschedule, cancel, execute, restart recovery | `Covered` | Account-scoped schedule versions have deterministic durable operation IDs, active cancellation, stale-completion fencing, and no-replay recovery. |
| Game-data/catalog version | Every validation, cost calculation, capacity check, hydration | `Covered`/`Partial` | Refresh is serialized and dispatch is fenced by catalog version. Complete catalog read declarations for every planner are still an audit item. |
| Bot/automation lock | Automation, scheduler, report manager, background tasks, manual API | `Partial` | Actor/priority classes are server-owned and direct browser traffic pauses new automation. Capability reconciliation is still required before robust mixed manual/automatic operation. |

### Persistence, restart, filesystem, and multi-process cases

| ID | Severity | Status | Scenario and consequence | Current protection | Required proof or change |
|---|---|---|---|---|---|
| PERSIST-01 | Critical | Partial | Crash after remote effect but before receipt/state snapshot creates an ambiguous operation. | SQLite WAL records accepted/planned/dispatching/sent/awaiting/observed/reconciliation/completed phases with full synchronization; recovery blocks replay of any possibly dispatched non-idempotent effect. | Implement authoritative capability-specific reconciliation so more indeterminate records can resolve automatically rather than require attention. |
| PERSIST-02 | Critical | Partial | Recovered snapshot belongs to another world/account. | Persisted world/player binding is verified against authoritative GBD; mismatch atomically resets prior account capability data before binding the new account. | This prevents merging but does not retain isolated snapshots for simultaneous multi-account operation. |
| PERSIST-03 | Critical | Covered | Two processes write the same state/config/history/log/cache files. | Application startup owns one operating-system lease for the profile/data directory. | Retain cross-process and abnormal-release tests on supported platforms. |
| PERSIST-04 | High | Covered | Crash during state/config replacement leaves a partially written destination. | Temp file, sync, close, and rename are used. | Also sync the containing directory where required for crash durability. |
| PERSIST-05 | High | Partial | Configuration replacement fallback removes the destination before retrying rename; a second failure leaves no settings file. | Error is returned and temp cleanup occurs. | Use platform-safe atomic replace/backup and recovery journal. |
| PERSIST-06 | High | Partial | State persistence captures revision N while later changes occur, then completes after a newer save and replaces it with older state. | Application-level `statePersistenceMu` serializes current saves. | Ensure every alternative snapshot writer uses the same authority; optionally reject revision regression at write. |
| PERSIST-07 | High | Partial | History append succeeds but process crashes before associated state marks the report/sample complete, causing duplicates. | JSONL append is mutexed and synced. | Store stable record IDs and deduplicate on append/import. |
| PERSIST-08 | Medium | Partial | Crash leaves a truncated JSONL line. | Readers skip malformed entries and retain prior lines. | Record/checksum sequence and explicitly report truncation instead of silently ignoring all malformed records. |
| PERSIST-09 | Medium | Partial | Disk full or permission loss causes snapshot/config/history/telemetry divergence while automation continues. | Operation-journal writes are synchronous and fail before send when unavailable; state snapshot failures are retried, exposed as degraded health, and fail mutating execution closed until recovery. Configuration writes fail without publication. | Extend explicit health/backpressure to history and telemetry, define storage budgets, and fault-test read-only, full, and transient database/filesystem recovery. |
| PERSIST-10 | Medium | Partial | Official-data refreshes race on identical cache destinations or load a partially replaced file. | Downloads use unique temps and rename. | Serialize refresh; validate checksum/schema before publication; retain last known-good snapshot. |
| PERSIST-11 | Medium | Audit | Legacy migration runs twice or concurrently and duplicates schedules/reports/presets. | Several migration paths keep markers and use atomic files. | Prove idempotency with repeated/concurrent migration tests and stable source hashes. |
| PERSIST-12 | Low | Audit | Filesystem rename/sync semantics differ on Windows/macOS/Linux. | Current implementation uses standard temp+rename patterns. | Test supported platforms and document durability guarantees. |

### Time, arithmetic, identity, and protocol boundaries

| ID | Severity | Status | Scenario and consequence | Required handling |
|---|---|---|---|---|
| EDGE-01 | High | Partial | Local clock is ahead/behind the game server, making cooldown, arrival, event-end, and quote freshness calculations wrong. | Track server-time offset where observable, include safety margins, and reconcile authoritative rows before irreversible effects. |
| EDGE-02 | High | Partial | Wall clock jumps backward/forward while timers or schedule comparisons are active. | Use monotonic durations in-process, persist wall-clock targets plus last-fired version, and test NTP/manual changes. |
| EDGE-03 | High | Audit | Weekly schedules cross DST gaps/folds or the user changes timezone. | Persist timezone ID and define one/two/no execution behavior for ambiguous/nonexistent local times. |
| EDGE-04 | High | Partial | An operation is valid at planning time but crosses an event-end/cooldown boundary while waiting for claims/pacing. | Revalidate time-sensitive predicates immediately before send and include `NotBefore`/deadline in admission. |
| EDGE-05 | High | Audit | Target/object IDs are reused after respawn or event rollover; coordinates alone match a different object. | Use event-run ID + type + object ID + coordinates + observation lineage. |
| EDGE-06 | High | Audit | Sentinel values such as `-1`, `0`, omitted, or null have different meanings across commands. | Keep command-specific typed payloads and validate every sentinel against captures; never normalize globally. |
| EDGE-07 | Critical | Partial | Construction-item CID is interpreted as a building/decor wodID, or `gbc.CID` is interpreted as an item instead of castle AID. | Existing types/comments/catalog helpers distinguish them. Add type-level compile barriers and wire-shape tests. |
| EDGE-08 | High | Partial | TCI upgrade changes CID via BCID while an operation/policy still references the prior definition ID. | Treat full CI snapshot as authoritative and re-resolve variant/level after every upgrade. |
| EDGE-09 | High | Partial | Remaining seconds reaches zero between validation and send, or missing remaining time is treated as zero. | Preserve pointer/knownness, subtract elapsed observation age carefully, and revalidate at the boundary. |
| EDGE-10 | High | Partial | Resource/unit amount addition overflows `int64`, or float-backed resource balances lose integer precision near large values. | Continue explicit overflow checks; define integer wire quantities and conservative comparisons. |
| EDGE-11 | Medium | Partial | Duplicate unit/resource/item rows are double-counted or last-write-wins inconsistently. | Normalize and merge only where protocol semantics allow; reject duplicates in user requests when ambiguity matters. |
| EDGE-12 | High | Partial | A castle is deleted/captured/renamed/moved or changes kingdom while an operation references it. | Resolve by stable castle ID, then revalidate ownership, kingdom, coordinates, and capability freshness before send. |
| EDGE-13 | High | Partial | Commander/general/castellan disappears, becomes unavailable, or is assigned elsewhere between context refresh and effect. | Typed leader claim plus send-time roster/movement verification. |
| EDGE-14 | High | Audit | Game returns duplicate, partial, reordered, or unsolicited response fragments. | Reducers must be idempotent/merge-aware; correlation and completion predicates must require all necessary fragments. |
| EDGE-15 | High | Partial | Successful response is decoded but state reduction fails due to new/unknown schema. | Operation fails and protocol observation records the reducer error. Reconcile before retrying the possibly successful effect. |
| EDGE-16 | Medium | Audit | Catalog refresh removes/changes a definition while existing state contains it. | Preserve unknown observed IDs, version plans by catalog, and fail validation without deleting observed game truth. |
| EDGE-17 | Medium | Partial | Empty/nil collections after recovery are mistaken for authoritative observed emptiness. | Track `ObservedAt`/freshness separately from collection length and normalize maps without fabricating observation. |
| EDGE-18 | Medium | Audit | Maximum queue sizes, response sizes, map rows, reports, or state snapshot size cause memory/CPU spikes. | Bound inputs/queues, stream large logs, expose resource limits, and load-test worst supported account. |
| EDGE-19 | Low | Audit | Unicode/case/whitespace differences create distinct claim, actor, section, or stable keys. | Canonicalize identifiers at typed constructors; avoid user text in resource identity. |
| EDGE-20 | Low | Audit | Revision/generation/timestamp equality at exact boundaries selects nondeterministically. | Define tie-breakers using sequence/ID and test equality, not only before/after cases. |

### API and UI concurrency cases

| ID | Severity | Status | Scenario and consequence | Current protection | Required proof or change |
|---|---|---|---|---|---|
| API-01 | High | Covered | API reads revision and snapshot from different state generations. | `SnapshotWithRevision` returns both from one immutable generation and API responses use that atomic pair. | Retain concurrent mutation/query tests. |
| API-02 | High | Covered | WebSocket state/operation/config events overflow subscriber buffers. | Each source marks its sequence and gap; state/config snapshots and the recent durable operation endpoint provide scoped resync, and the client automatically refetches on gaps and reconnect. | Retain overload/reconnect tests and reject future source-sequence regression. |
| API-03 | High | Covered | Client sends arbitrary `Actor` and requested priority, influencing scheduling. | API entry points replace caller labels with a server-owned UI actor and effective interactive priority. | Retain entry-point tests and introduce authenticated classes before exposing privileged callers. |
| API-04 | High | Partial | UI double-click/retry sends two operations without a stable idempotency key. | The client creates a stable action ID for every submission and coalesces identical in-flight requests; the server durably enforces same-ID request fingerprints. | Persist/reuse the action ID across page reload/network retry flows and add semantic duplicate windows only for intents where repeat is never meaningful. |
| API-05 | Medium | Covered | Intent response goroutine blocks publishing after its WebSocket handler exits or its response queue fills. | Asynchronous publications select on connection context; synchronous handler-owned results write in the single writer loop rather than feeding its own queue. | Retain disconnect/backpressure leak tests. |
| API-06 | Medium | Covered | WebSocket reader goroutine blocks writing to a full incoming channel after the handler stops consuming. | Reader delivery is context-aware and owned by the connection lifecycle. | Retain shutdown/backpressure leak tests. |
| API-07 | Medium | Covered | Concurrent goroutines write directly to one API WebSocket. | The handler loop owns all writes, including pings and JSON responses. | Preserve single writer if event processing is parallelized. |
| API-08 | Medium | Audit | Two client fetches complete out of order and the older response overwrites a newer screen state. | Server snapshots carry revisions. | Client reducers must compare scope/projection revision before applying responses. |
| API-09 | Medium | Covered | Operation event is missed and UI shows `running` forever. | Operation overflow carries a gap marker; recent receipts are queryable from the durable store, sent as a reconnect snapshot, and automatically merged by the client. | Retain terminal-event overflow and reconnect tests. |
| API-10 | Low | Audit | User navigates between castles while a request for the prior viewed castle is in flight. | Backend intent scope is explicit in many requests. | UI state must separate viewed castle from live focused castle and discard mismatched response scope. |

## Required typed resource model

Free-form strings should be replaced by typed keys with a central overlap rule. A useful starting vocabulary is:

```text
SessionKey(profile, sessionGeneration, connectionGeneration)
SessionFocusKey(session)
WorldAccountKey(world, player)
AccountWalletKey(account)
InventoryStackKey(account, namespace, definitionID)
LeaderKey(account, leaderKind, leaderID)
EquipmentInstanceKey(account, itemKind, instanceID)
KingdomKey(account, kingdomID)
KingdomTransportLaneKey(account, targetKingdomID, cargoKind)
CastleKey(account, kingdomID, castleID)
CastleResourceKey(castle, resourceID)
CastleGarrisonKey(castle)
UnitStackKey(castle, unitID)
BuildingLayoutKey(castle)
BuildingFootprintKey(castle, cells)
BuildingInstanceKey(castle, buildingInstanceID)
BuildingQueueKey(castle)
ConstructionSlotKey(castle, buildingInstanceID, slot)
ProductionLineKey(castle, lineID)
CraftingSlotKey(castle, buildingInstanceID, slotType, slot)
DefenseKey(castle, section)
MovementKey(account, movementID)
MapPartitionKey(kingdom)
TargetKey(kingdom, type, objectID?, x, y, eventRun?)
ShopKey(account, shopKind, tableID?, castle?, kingdom?)
ShopOfferKey(shop, offerID/productID/slotID)
ReportMessageKey(account, messageID)
ScheduledOperationKey(account, scheduledID, version)
```

Overlap must be semantic, not only equality. Examples:

- `AccountWalletKey(A)` overlaps every currency/resource stack spent from that wallet.
- `CastleKey(C)` should not automatically block every child capability forever, but an explicitly declared whole-castle write must overlap all child writes.
- `CastleGarrisonKey(C)` overlaps `UnitStackKey(C, any)`.
- `BuildingLayoutKey(C)` overlaps every footprint mutation in that castle; two disjoint footprint claims may run only if focus/protocol context also allows it.
- `SessionFocusKey(S)` overlaps every effect whose response interpretation or command context depends on live focus.
- `TargetKey` equality must account for event-run/object reuse, not coordinates alone.

Every intent definition should declare:

```text
read dependencies: versioned capability/catalog/config/focus keys
write/effect claims: typed resources with overlap semantics
admission class: only when a scarce paced workflow exists
idempotency class: read, idempotent write, non-idempotent write, launch, external
completion evidence: send, wire response, committed response, projection predicate, reconciliation
retry policy: safe, conditional, or never automatically
```

## All-features stress and verification plan

### Static coverage checks

1. Enumerate every registered intent definition at test time.
2. Require nonempty typed read dependencies for planners that read state/catalog/configuration.
3. Require typed write/effect claims for every command/action effect.
4. Require every focus-derived opcode to declare the session-focus dependency/claim.
5. Require every non-idempotent effect to declare completion evidence and ambiguous-outcome reconciliation.
6. Generate a feature-by-resource matrix and review new/changed rows in code review.
7. Detect broad/narrow keys that should overlap and private string aliases.
8. Verify every reducer’s declared capability partitions include all semantically changed fields.
9. Verify clone independence whenever `GameState` gains a reference field.
10. Verify server-owned actor/priority classification for every API/scheduler entry point.

### Deterministic simulation scenarios

Run all enabled policies, manual intent submissions, scheduler, report manager, movement clock, and fake game frames under a deterministic clock and fake transport. Vary ordering at every boundary:

- state changes immediately before and after planning;
- claim acquisition immediate versus delayed;
- admission acquired, claim timeout, release, and retry;
- bot lock before claims, before each step, during send, and during response wait;
- cancellation queued, dispatching, possibly sent, response decoded, and response committing;
- reconnect before send, during browser evaluation, after send, and before response commit;
- focus change from CitadelOps and direct browser traffic;
- response before send returns, duplicate response, late response, wrong token, missing token, wrong opcode, and reducer failure;
- queue full, notice overflow, ingest backlog, subscriber overflow, disk full, and telemetry close;
- schedule cancel/reschedule while queued/running and crash at each execution phase;
- catalog refresh before planning, between plan and send, and between response and verification;
- clock jump, DST boundary, event rollover, target respawn, and exact zero-remaining boundary.

### Feature-pair scenarios

At minimum, execute these pairs simultaneously with the same and different castles/kingdoms:

1. Build/upgrade/demolish/skip versus AutoTCI equip/upgrade/purchase.
2. Building spend versus crafting spend versus shop purchase versus every time-skip feature.
3. Recruit/tools/hospital versus building and resource logistics in the same castle.
4. Attack versus attack for every pair of Rift/tower/invasion/nomad/RBC/Khan/Storm.
5. Attack versus equipment change for the same commander and another commander.
6. Attack versus station/troop transfer using the same source troops.
7. Defense update/preset versus AutoKhan protection/open-gate/tool replenishment.
8. Resource market/kingdom shipments versus FoodBalance/Crafting logistics.
9. Resource versus troop kingdom transport to the same and different kingdoms.
10. Map scan/target lock/cooldown skip/attack against the same target and respawned target.
11. Report manager fetch/archive/share versus manual report operations.
12. Rift schedule/replay/cancel/template rename/delete at the fire boundary.
13. Manual API intent versus matching automation policy.
14. Direct game-client command versus every focus-dependent automation class.
15. Game-data refresh versus every planner that calculates cost/capacity/eligibility.

### Invariants to assert during stress

- No two active operations hold overlapping typed claims.
- No two CRA launches occupy attack admission simultaneously.
- No two physical CitadelOps sends overlap.
- No current-generation state is mutated by an old-generation frame.
- No focus-derived reducer commits without a valid identity/focus proof.
- No commander or unit stack is allocated twice from the same observed availability.
- No currency/inventory stack is spent below its configured reserve by concurrent operations.
- No scheduled operation version sends more than once without explicit reconciliation approval.
- No response is delivered to two operations or to a later retry.
- No terminal receipt claims full success when a required state commit failed.
- No state/config/projection revision moves backward at persistence or client application boundaries.
- Every goroutine and claim/admission/watcher/commit record is released after cancellation/shutdown.

### Tooling

- Run the full backend under `go test -race` periodically, not only selected packages.
- Fuzz protocol decode/reducer inputs and response ordering.
- Use randomized scheduler testing with a deterministic seed printed on failure.
- Add leak detection for goroutines, response watchers, wire commit records, claims, and admission holders.
- Add fault injection around send, file sync/rename, reducer commit, event publication, and process restart.
- Keep production-wire golden captures for focus, purchases, TCI, attacks, movement, reports, and event rollover.

## Observability required to diagnose concurrency

### Per operation

- operation ID and idempotency key;
- actor class assigned by the server;
- intent and feature module;
- account/kingdom/castle/target scope;
- planned state, partition, catalog, config, session, connection, and focus versions;
- typed claims requested, wait start/end, holder on contention, and release reason;
- admission class, weight, wait, and granted order;
- every step phase and idempotency class;
- outbound queue/lane position, queue time, dispatch time, and pacing wait;
- response token, expected opcodes, send sequence, ingress ID, and commit revision;
- final outcome: succeeded, partially succeeded, failed-before-send, cancelled-before-send, or indeterminate-after-send;
- reconciliation decision and evidence.

### Runtime gauges and counters

- active/queued intents by actor, feature, priority, and scope;
- claim waiters, oldest wait, and top contended typed resources;
- admission waiters and per-module service rate;
- outbound queue depth/high-water/rejections and oldest age by lane;
- intent-to-dispatch and intent-to-transport p50/p95/p99 split into fast path and contended path;
- transport evaluation duration/timeouts/indeterminate sends;
- frame, notice, ingest, state-event, config-event, operation-event, and API-response queue depths;
- observed-to-committed ingest lag and reducer duration/error count;
- stale-generation frame/command/status rejection counts;
- response-token mismatches, late responses, watcher overflow, and commit cleanup timeout;
- current session/connection/focus generation and baseline readiness;
- persistence last-success revision/time/error and durable-operation lag;
- goroutine count and active watcher/commit/claim/admission counts.

## Prioritized hardening backlog

### P0: required before claiming all-feature safety

1. Generate and review the all-intent read/effect-resource conflict matrix so nonempty typed declarations are proven complete, not merely present.
2. Classify every remote effect as read, idempotent, non-idempotent, launch, or external, with explicit completion evidence and capability-specific reconciliation.
3. Add deterministic all-features simulation, restart fault injection, stream-overflow tests, and leak checks around every send/outcome boundary.
4. Reconcile capabilities after detected direct browser traffic, or provide an optional exclusive-control mode with explicit product semantics.
5. Finish configuration-version dependencies and complete explicit capability read sets for every state-dependent planner.

### P1: complete capability isolation

1. Replace whole-account clone mutations with independently owned immutable capability transactions after reducer parity is proven.
2. Add per-capability observation positions and reject stale merged-source regressions without relying only on global ingress order.
3. Add durable report/history/share/archive idempotency and extend explicit storage health/backpressure to history and telemetry.
4. If simultaneous accounts become a product requirement, retain an isolated capability/schedule/history partition per `WorldAccountKey` or move each account runtime behind a worker lease.
5. Add quantity reservations for currency, inventory, troops, queue slots, and capacity only after exclusive typed resources and authoritative reconciliation are complete.

### P2: improve throughput without weakening safety

1. Narrow broad castle/focus/account resources using measured conflict data and protocol evidence.
2. Split query/projection streams by capability and scope where measurements show full-state refetch pressure.
3. Tune response timeouts, queue bounds, aging caps, and pacing from live telemetry.
4. Add database retention/checkpoint policy after measuring long-running operation-history size and write latency.

## Definition of “all features can run together safely”

The claim is supportable only when all of the following are true:

- Every registered intent has reviewed typed read dependencies, effect claims, idempotency class, and completion evidence.
- The generated conflict matrix contains no unexplained feature pair sharing a remote resource without overlapping claims.
- Direct browser activity and multiple-process control have defined exclusion/reconciliation behavior.
- Reconnect, cancellation, timeout, crash, and restart tests demonstrate no automatic duplicate non-idempotent effect.
- Focus-derived frames cannot update a castle without valid scope proof.
- The full race detector, deterministic stress suite, protocol replay suite, and leak checks pass.
- Queue, claim, admission, ingest, and persistence saturation are explicit health states rather than silent loss.
- The 25 ms p95 target is met for the defined uncontended fast path, while contended latency and deliberate attack pacing meet separate documented SLOs.

Until then, the accurate statement is: the runtime prevents the principal known local races and refuses unsafe automatic replay, but declaration completeness, external-browser reconciliation, and every all-feature interleaving have not yet been mechanically proven.

## Primary code surfaces for future audits

- `Server/State/Store.go` and `Server/State/Partitions.go`
- `Server/Ingest/Pipeline.go`, `Server/Ingest/ScopedPartitions.go`, and focused-castle reducers
- `Server/Intent/Engine.go`, `Server/Intent/Claims.go`, `Server/Intent/Resources.go`, `Server/Intent/OperationStore.go`, `Server/Intent/Admission.go`, and `Server/Intent/Types.go`
- `Server/Outbound/Router.go` and `Server/Outbound/Priority.go`
- `Server/Session/Controller.go` and `Server/Session/ChromiumTransport.go`
- `Server/Automation/Coordinator.go` and every policy definition
- `Server/Scheduling/Scheduler.go`
- `Server/Reports/Manager.go`
- `Server/Configuration/Store.go`
- `Server/GameData/Updater.go`
- `Server/API/Server.go`
- `Server/App/*Intents.go`, especially shared claim/context helpers
- `Server/State/Persistence.go`, `Server/History/Store.go`, and `Server/Telemetry/Store.go`

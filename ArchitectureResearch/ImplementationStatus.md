# Concurrency-safe backend implementation status

Implementation checkpoint: 2026-07-15

## Outcome

CitadelOps now uses a concurrency-safe augmentation of the existing in-process modular monolith. The implementation preserves current feature eligibility, planners, command payloads, response reducers, response-commit barriers, attack admission, API v2 compatibility, and one physical game transport. It changes the execution boundaries underneath those features so simultaneous automation and UI work is serialized by semantic game resources rather than incidental goroutine timing or exact string equality.

The architecture is appropriate for the current single-active-account desktop scenario. A full greenfield rewrite or worker-per-account topology would add parity, IPC, packaging, and failure-mode cost without improving the present fast path. Independent capability stores remain a later extraction option if measured state-clone pressure or simultaneous-account requirements justify them.

## Implemented controls

### State, account, and observation lineage

- The store publishes state, partition versions, protocol context, and revision as one immutable atomic generation.
- `SnapshotWithRevision` prevents API envelopes from pairing a revision with a different state generation.
- Application, session, account, kingdom, and castle capability scopes are explicit.
- Persisted state has a stable world/player binding. An authoritative baseline for another player or world atomically clears prior account capability data before binding the replacement, preventing cross-account merge.
- Recovered or malformed multiple-focus state is normalized deterministically.
- Observations carry profile, world/player, session, connection, focus, catalog, decoder, causation, time, and ingress lineage.
- Commit rejects stale account, session, connection, or focus context before and after reduction.
- The live ingest path has one bounded queue owner and explicit discard/join behavior during cancellation.

### Intent resources, planning, and fairness

- Plans use typed account-scoped `ResourceKey` values for application, session, account, kingdom, castle, capability, leader, target, shop, wallet, inventory, response, report, schedule, and transport resources.
- One semantic overlap function handles parent/child relationships and canonical aliases such as account wallet versus a currency, castle versus a child capability, and leader/equipment aliases.
- Complete resource sets are acquired atomically and held through acknowledged response commit.
- Production plan finalization rejects empty, unknown, or unmapped legacy effect-resource declarations.
- Plans are revalidated after resource/admission waits and dependency changes.
- Every dispatch rechecks the bound world/player, session generation, connection generation, focus epoch/castle, and catalog version.
- Catalog refresh between planning and any send in a command chain causes safe re-planning.
- Claims and outbound routing use bounded priority aging, deadline, and FIFO ordering so continuous high-priority work cannot starve background work indefinitely.

### Durable operations and ambiguous remote effects

- A local SQLite operation store runs in WAL mode with full synchronization and one logical connection.
- Reservation binds an operation ID to a canonical request fingerprint. Exact retry reuses the durable record; same ID with different intent/arguments is rejected.
- Durable phases include accepted, planned, dispatching, sent, awaiting response, observed, reconciliation required, and completed.
- Receipt outcomes include partial, indeterminate, and reconciling states in addition to ordinary terminal outcomes.
- The possible-send transition is persisted before the external transport call.
- Restart recovery never automatically replays a possibly dispatched non-idempotent effect.
- The engine bounds its in-memory hot receipt cache to 10,000 entries while the durable store retains idempotency evidence.
- The application owns operation-store open, recovery attachment, and close lifecycle.

### Scheduling, services, transport, and API

- Scheduled operations have a durable version, world/player binding, updated time, and deterministic account/version-scoped operation ID.
- Cancellation propagates to the active intent and distinguishes cancelled-before-send from reconciliation-required-after-possible-send.
- Rescheduling increments the version, cancels the prior version, and prevents stale completion from overwriting the replacement.
- Recovered/running schedules reuse their prior durable operation ID, preventing restart replay.
- Application, automation coordinator, scheduler, report manager, and session controller have one-run lifecycle guards.
- Periodic and manual game-data refresh are serialized inside the manager.
- A stable profile identity and operating-system data-directory lease prevent two CitadelOps processes from sharing one browser/persistence authority.
- CitadelOps causation IDs propagate through Chromium and dedicated transports. Uncaused outbound browser traffic pauses/yields background automation until the configured quiet period.
- HTTP and WebSocket callers cannot self-assign actor/priority privilege; entry points normalize them to server-owned classes.
- WebSocket reader and asynchronous response paths observe connection context, while the handler remains the single network writer.
- State, configuration, and operation events carry source sequence plus an explicit gap flag when overflow coalesces pending delivery.
- Configuration carries a full replacement snapshot; state gaps trigger an atomic state refetch; operation gaps/reconnects use a recent durable receipt snapshot and automatic client merge.
- State snapshot write failures retry, degrade health, and fail mutating intents closed until recovery. Operation journal failures are synchronous and prevent an unrecorded effect from reaching transport.
- The TypeScript client generates a stable operation ID for every submission and coalesces an identical request while it is in flight.

## Performance result

The latency objective is intent acceptance to completion of the first successful transport send for an already connected, baseline-ready, correctly focused, uncontended account. Deliberate game pacing, resource contention, focus prerequisites, browser/network/server latency, and response waiting are separate latency classes.

Benchmarks used an in-process fake transport on an Apple M4:

| Path | Measured time |
|---|---:|
| Durable operation reservation plus dispatch transition | 0.0834–0.0839 ms/op |
| Full durable engine intent through fake transport | 0.191–0.202 ms/op |
| Immutable state planning view | 69–80 ns/op, 0 allocations |
| Defensive whole-state snapshot | 0.739–0.747 ms/op, approximately 3.26 MB allocated |
| Whole-state apply/clone/publish | 0.747–0.809 ms/op, approximately 3.26 MB allocated |

The durable engine path has substantial headroom under the 25 ms uncontended target. This does not mean every operation in a simultaneous burst can send within 25 ms: there is deliberately one physical game sender, and the Nth conflicting or paced operation must wait.

The next local scaling boundary is whole-state clone allocation under sustained ingest, not intent planning or durable operation admission.

## Verification completed

The complete backend package suite passes:

```text
go test ./Server/...
go vet ./Server/...
```

Focused functional tests also cover state, ingest, intent, outbound, scheduling, session, automation, reports, API, runtime, application composition, and game data.

The race detector passes across the concurrency-critical backend packages:

```text
go test -race ./Server/State ./Server/Configuration ./Server/Ingest ./Server/Intent ./Server/Outbound ./Server/Scheduling ./Server/Session ./Server/Automation ./Server/Reports ./Server/API ./Server/App ./Server/Runtime ./Server/GameData
```

Focused latency benchmarks cover the SQLite operation store, durable engine path, immutable planning view, and whole-state compatibility paths.

The current dirty frontend tree has unrelated TypeScript errors outside this migration. `npx tsc --noEmit -p tsconfig.app.json` does not report an error in `Client/src/api/CitadelClient.ts` or in the account/schedule/operation contract additions, but it is not a repository-wide passing frontend validation.

## Remaining limits

The implementation closes the principal known in-process races and prevents unsafe automatic replay, but it does not prove that every possible feature/protocol interleaving is solved.

1. Generate the all-intent read/effect-resource matrix. Mandatory nonempty typed resources cannot infer that a planner omitted a second semantic resource.
2. Complete explicit versioned read dependencies and configuration-version checks for every state-dependent feature.
3. Give every remote effect a reviewed idempotency class, completion predicate, and authoritative capability-specific reconciliation query.
4. Add deterministic all-features simulation, restart fault injection, overload tests, protocol fuzzing, and goroutine/watcher/resource leak checks, retaining sequence/gap recovery under saturation.
5. Reconcile affected state after direct browser traffic, or add an optional exclusive-control mode. Pausing automation cannot make a human command transactional with local claims.
6. Extend explicit persistence health, budgets, and backpressure from state/operation safety stores to history and telemetry.
7. Move from one cloned `GameState` to independently owned immutable capability values only if ingest/heap measurements justify the migration.
8. Add an account-partition bank or supervised account workers only if simultaneous retained accounts become a committed product use case. The current implementation safely switches accounts but intentionally retains one active account aggregate.

## Accurate safety statement

The runtime now prevents the principal known local races, semantically serializes conflicting feature effects, fences stale account/session/focus/catalog context, survives operation/schedule restart without automatically duplicating a possibly sent effect, and meets the defined uncontended latency objective in local benchmarks.

The stronger statement—“all features and all external interleavings are mechanically proven safe”—must wait for the generated declaration matrix, capability reconciliation coverage, and deterministic all-feature fault suite described above.

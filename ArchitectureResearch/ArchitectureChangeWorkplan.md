# Architecture change workplan

## Outcome

An effective architectural change should rebuild CitadelOps around scoped capability state and a smaller intent execution kernel while using the current application as a behavioral oracle. “From scratch” should describe the new code boundaries, not a big-bang loss of evidence or an unverified replacement.

The work has five coupled outcomes:

1. Replace the global `GameState` integration point with independently versioned capability instances scoped to account, kingdom, castle, target, event run, or session.
2. Replace the generic intent planning context with typed capability requests, explicit read sets, typed claims, and observation-backed outcomes.
3. Keep one account-ordered control loop and explicit focus protocol context so partitioning does not create unsafe game-command concurrency.
4. Replace the public internal-state dump and full-state client refetch with versioned task projections, scoped subscriptions, and an API v2 adapter.
5. Port each feature as a complete vertical slice—protocol, state, operations, automation, settings, API, UI, persistence, and parity evidence—rather than rewriting all parsers, then all state, then all UI.

The safest delivery shape is a **strangler inside the same desktop application**: the old and new read paths can run side by side, but exactly one path is permitted to send live game commands.

## Non-negotiable constraints

### Behavior and safety

- Preserve the current ordering, focus transitions, pacing, admission rules, response correlation, commit barriers, cancellation, and receipt meaning for every reachable effect.
- Do not treat socket write success as game success.
- Do not automatically retry a possibly sent purchase, attack, upgrade, or other non-idempotent effect after an ambiguous disconnect.
- Preserve atomic visibility when one accepted observation updates multiple feature partitions.
- Fence every observation, effect, and response by profile, session, connection generation, and bound game account.
- Keep replay and shadow planning effect-free.

### Game identity and data

- World, account, kingdom, castle, building definition, building instance, construction-item CID, package, resource, unit, leader, equipment instance, movement, report, and target identities remain typed and non-interchangeable.
- Focus-derived responses may update a castle only through explicit wire identity, correlated operation scope, or a captured valid focus epoch.
- Official in-game JSON remains under the official-data tree. Product settings, overrides, presets, schedules, state, receipts, and history live in the per-instance application data directory.
- `CITADEL_DATA_DIR` remains supported and wins over default-location discovery.

### Product scope

- Reachable current behavior is mandatory parity.
- Server-supported, legacy-only, incomplete, and future behavior is classified before porting; module registration does not make a dormant feature live.
- Desired documents survive missing/stale game observations and castle retirement.
- The initial topology remains a modular monolith. Partitioning is logical ownership, not a service-per-feature or worker-per-castle design.

## Current-to-target change map

| Current area | Present responsibility | Target responsibility | Migration treatment |
|---|---|---|---|
| `Server/State` | One shared model, clone/normalize, global revision, event domains | Scope keys, capability stores, account read views, atomic change sets, compatibility composition | Introduce beside old store; feed both from typed facts; retire feature fields slice by slice |
| `Server/Ingest` | Decode and directly mutate the global clone | Observation envelope, scope resolution, typed fact decode, atomic fact routing | Split transport normalization from feature decoders; compare old/new reductions in replay |
| `Server/Intent` | Global state planning context, string claims, generic raw steps, correlation/receipts | Domain-agnostic execution kernel with typed dependencies/claims/effects and durable receipts | Preserve engine semantics, narrow the surface; adapt old definitions until each capability is ported |
| `Server/App` | Composition plus thousands of lines of feature planners/payloads/guards/actions | Thin composition root and compatibility adapters | Move one vertical feature at a time into capability packages; prohibit new domain rules here |
| `Server/Automation` | Policies receive broad snapshots and domain-name notifications | Scoped policy instances consuming declared read projections and submitting typed requests | Resolve global settings to instances; shadow decisions against current policies |
| `Server/API` | Routes plus direct `GameState` response and broad invalidations | Authenticated query/command gateway, versioned DTOs, scoped subscription stream, v2 adapter | Add v3 without breaking v2; migrate owned clients before removing adapter |
| `Client/src/api` | Hand-mirrored global contracts/context, full-state polling/refetch | Generated transport client plus feature query caches/subscriptions | Introduce route by route; keep legacy provider only for unported views |
| `Client/src` features | Local view switch, app-owned modals, mixed persistent/presentation behavior | Router, scope shell, feature modules, operation center, local presentation state | Preserve journeys; move durable policy behavior only through explicit product decisions |
| `Server/Configuration` | Versioned sections plus legacy file migration | Capability-owned schemas with declared profile/account/castle/policy scope | Import current documents unchanged first, then migrate independently with receipts |
| persistence/history | JSON, JSONL, snapshots, files, captures, logs | Transactional metadata/documents/snapshots/receipts plus bounded capture files | Copy-based, idempotent, checksummed migration with original retained for rollback |
| `Server/GameData` | Runtime official catalogs and projections | Immutable catalog snapshots selected by version | Retain; expose narrow catalog ports and record catalog version in plans/projections |
| `Server/Session` | Browser/transport lifecycle and connection state | Profile/session bootstrap, transport port, generation fencing, account binding | Port early; retain Chromium and replay adapters behind the same observation/effect ports |
| telemetry | Raw observations and operational logs | Causal tracing, capability health, bounded/redacted capture references | Separate diagnostic positions from domain versions; enforce retention and export policy |

## Proposed implementation shape

The exact package names should be proven with a walking skeleton. The important rule is direction of dependency: capabilities depend on small runtime contracts, while the runtime does not import capability-specific types.

An illustrative structure is:

```text
Server/
  Runtime/
    Identity/
    Observation/
    Scope/
    Store/
    Execution/
    Operations/
    Scheduling/
    Catalog/
  Capabilities/
    Account/
    Castles/
    Construction/
    Production/
    Crafting/
    Economy/
    Movement/
    Equipment/
    Defense/
    Combat/
    Alliance/
    Reports/
    Events/
      Towers/
      Invasion/
      Nomad/
      Khan/
      Storm/
      Rift/
      Berimond/
  Adapters/
    Chromium/
    Replay/
    HTTP/
    Storage/
    OfficialData/
  Compatibility/
    V2/
    LegacyConfiguration/
```

Each capability may contain small consumer-defined contracts rather than one mandatory framework interface:

```text
Construction/
  Facts.go
  Decoder.go
  State.go
  Reducer.go
  Queries.go
  Requests.go
  Planner.go
  Outcomes.go
  Claims.go
  Policy.go
  Configuration.go
  Migrations.go
  Contracts.go
  Parity_test.go
```

New hand-written Go basenames must follow the repository’s PascalCase rule. The names above are illustrative and comply with it.

### Dependency rules

- A capability may import runtime contracts, shared typed identities, and catalog read ports.
- A capability may request another capability through a narrow query/request port defined by the consumer.
- A capability may not import another capability’s persistence record or reducer state.
- Event orchestrators compose normal feature requests; they do not duplicate construction, logistics, defense, or combat logic.
- Protocol decoders publish typed facts. They do not call UI, automation, or database adapters directly.
- HTTP DTOs are public contracts and may differ from internal state records.
- The composition root registers modules and adapters but contains no game-specific planner logic.

## State architecture implementation

### 1. Establish scope keys before moving fields

Introduce and validate:

- `ProfileID`, `SessionID`, and `ConnectionGeneration`;
- `WorldKey` and `WorldAccountKey`;
- `KingdomKey` and `CastleKey`;
- typed leader, movement, event-run, target, report, and operation keys;
- canonical string/binary encodings for persistence, URLs, logs, and claim ordering.

Every composite key must have a round-trip test and reject missing or cross-account components. Avoid a generic stringly `ScopeID` inside domain code; generic `ScopeRef` belongs only at routing and contract boundaries.

### 2. Add observation context and account sequencing

Every inbound/outbound record receives an immutable envelope before feature decode:

```text
ObservationEnvelope
  ObservationID
  ProfileID
  SessionID
  ConnectionGeneration
  IngressSequence
  ReceivedAt
  Direction
  Route / namespace / opcode / response code
  Bound WorldAccountKey, if authoritative
  Focused CastleKey + FocusEpoch, if valid
  Correlated OperationID + expected scope, if present
  CatalogVersion
  Secure payload reference or decoded transport record
```

One account-loop authority assigns causal observation positions and commits accepted change sets. CPU-heavy decoding may run outside the loop only if the result is returned to the correct position and generation before commit.

### 3. Resolve scope explicitly

Build one scope-resolution service with auditable precedence:

1. validated identity carried in the wire payload;
2. declared response scope of the correlated operation;
3. captured focus context from the same session generation and focus epoch;
4. unresolved/degraded result.

Do not let individual reducers invent different fallback behavior. A wire identity that conflicts with correlation is an anomaly and cannot silently update either candidate scope.

### 4. Decode typed facts

Decoders should turn protocol details into capability-owned facts, such as:

```text
CastleDiscovered
CastleSnapshotObserved
ConstructionLayoutObserved
ConstructionSlotsObserved
ProductionQueueObserved
CastleResourcesObserved
MovementSnapshotObserved
EquipmentInventoryObserved
MapViewportObserved
```

A combined `jaa` or initial `gbd` can yield multiple facts. Facts retain observation lineage and authoritative/partial semantics. A decoder error must preserve the current all-or-nothing acceptance rule until the protocol owner deliberately classifies a subrecord as independently tolerable.

### 5. Commit atomic account change sets

Stage all mutations caused by one accepted observation:

```text
AccountChangeSet
  account observation position
  fact references
  old/new partition versions
  capability snapshot mutations
  operation transitions satisfied by this commit
  projection invalidations/deltas
  health/freshness changes
  legacy v2 revision transition
```

Commit crash-critical snapshot and receipt changes in one short transaction. Publish notifications only after commit. Subscribers cannot observe half of a multi-capability frame.

### 6. Separate version domains

- Observation position advances for every accepted observation.
- Partition state version advances only for a material domain change.
- Projection version/ETag advances only when the public query result changes.
- API v2 compatibility revision follows the current broad rules exactly.
- Desired-document versions advance independently of observed state.
- Catalog version is an immutable dependency, not a state revision.

Native requests declare the partition/document/catalog/focus versions they actually read. HTTP query mutations may use ETag/`If-Match` where it matches the resource semantics; RFC 9110 supports conditional state changes to avoid applying a mutation to a changed representation.

### 7. Build immutable read views

`AccountReadView` captures references to immutable partition snapshots at one account observation position. It avoids cloning the entire account while giving a planner a consistent multi-partition view. A read view includes freshness and source lineage, not only values.

The first implementation can use copy-on-write partition values under one account loop. Do not begin with fine-grained locks throughout the domain; the ordered account loop is easier to reason about and closer to current semantics.

## Intent engine refactor

### Preserve the mechanism, remove domain ambiguity

The current intent path already provides valuable semantics. The change is not “delete the engine”; it is to turn it into a smaller execution kernel and move feature planning into typed capabilities.

### Request envelope

An internal request should carry:

```text
RequestEnvelope
  RequestID
  Actor: user | policy | schedule | CLI | tool
  AccountKey
  Capability
  ScopeRef
  RequestType + SchemaVersion
  TypedPayload
  SubmittedAt / deadline
  IdempotencyKey, only where semantics support one
  ParentOperationID, optional
  Approval/preview context, when required
```

JSON is decoded at the adapter boundary. Domain planners receive typed request structs, not arbitrary maps or raw wire payloads.

### Planning contract

Planning returns either a domain rejection or a typed plan:

```text
Plan
  Summary and risk class
  ReadDependencies[]
  Claims[]
  AdmissionClass and priority
  Effects[]
  CompletionRules[]
  ReconciliationRule
  RedactedPreview
```

The planner is effect-free. It may request a prerequisite refresh as a child operation, but it does not send a game command while planning.

### Dependency set

Replace one global `ExpectedRevision` with explicit dependencies:

```text
PartitionDependency(capability, scope, version, minimum freshness)
DocumentDependency(document ID, version)
CatalogDependency(snapshot version)
SessionDependency(session generation, focus epoch)
ProjectionDependency(only if a planner legitimately reads a projection)
```

Immediately before the first irreversible effect—and where a long plan requires it—the kernel revalidates these dependencies. Unrelated castle or report changes do not invalidate a construction plan.

### Typed claims

Claims need stable kind, account scope, resource identity, mode, and canonical ordering:

```text
Claim
  AccountKey
  Kind
  ScopeKey
  ResourceKey
  Mode: shared | exclusive
```

Examples include session focus, castle layout footprint, construction queue, building instance, production line, resource balance, kingdom transport lane, commander, equipment instance, attack admission lane, target, defense setup, and shop offer. Claims prevent competing effects; versions prevent plans based on changed information. Both are required.

The kernel acquires compound claims in deterministic order to avoid deadlock. Capabilities may calculate claims but cannot create private lock managers.

### Effects

Use a small closed set of mediated mechanisms:

- send a structured game command on a named lane;
- request a protocol refresh/context transition;
- wait for a timer or scheduled wake-up;
- persist a local desired-document change;
- publish a notification;
- invoke a registered local application adapter such as update/install where appropriate.

Capabilities own command serialization for their protocol. They do not receive a raw socket or Chromium handle.

### Correlation and completion

Each command attempt records operation, session generation, send sequence, expected response route/code, expected scope/focus epoch, and timeout. A capability completion rule can require:

- matching response observed;
- matching response committed to named partitions;
- later authoritative fact satisfying a predicate;
- best-effort completion where current behavior truly has no acknowledgement;
- reconciliation after ambiguity.

Receipts should distinguish `timed-out` from `indeterminate`. The latter means the effect may have happened and must not be repeated until a safe read resolves it.

### Compatibility bridge

During migration, legacy intent definitions can be hosted through adapters:

```text
v2 request
  -> validate legacy global revision
  -> map to current capability-version vector
  -> invoke old definition or new typed request, depending on ownership
  -> emit legacy-shaped receipt/result
```

This permits feature-by-feature porting. It must not allow old and new planners to send the same live request.

## Automation and scheduling refactor

### Policy instances

Convert every enabled policy into explicit instances based on its natural scope:

| Policy | Recommended instance scope |
|---|---|
| Recruit | Castle/production line |
| Tool production | Castle/production line |
| Hospital | Castle |
| AutoTCI | Castle, reading account construction inventory |
| AutoSceat/crafting | Castle/building |
| AutoBird | Protected castle or configured account threat set |
| AutoStation | Protected/source castle plus station target rules |
| AutoBeriWorld | Berimond run/castle with source castle |
| AutoFoodBalance | Account, coordinating configured castles |
| AutoTowers | Source castle |
| AutoInvasion | Event run/source castle |
| AutoNomad | Event run/source castle |
| AutoKhan | Event run/source castle plus protected-castle workflow |
| AutoStorm | Storm event run/Storm castle |

Global settings remain account templates. A resolver materializes effective values for each instance without rewriting the user document. This preserves current global/per-castle semantics while making status granular.

### Evaluation triggers

Policies declare:

- partition and document dependencies;
- which changes should trigger reevaluation;
- minimum freshness;
- periodic fallback interval;
- schedule/enablement gates;
- maximum one-request-per-evaluation behavior where current policy relies on serial observation;
- shared admission class and priority.

The runtime coalesces repeated triggers and prevents evaluation storms. A policy decision records `acted`, `waiting`, `blocked`, `complete`, or `disabled`, with the exact dependency/reason and next check.

### Scheduler

Persist typed scheduled requests rather than raw unversioned protocol-shaped JSON. Preserve time zone, weekly slots, feature schedules, attack delays/priorities, cancellation, and restart behavior. A migration adapter can retain original payload bytes for audit while storing the typed form.

## API and client refactor

### API v3 shape

Create task-oriented resources with explicit account and scope:

```text
GET  /api/v3/accounts/{account}/overview
GET  /api/v3/accounts/{account}/capabilities
GET  /api/v3/accounts/{account}/castles/{castle}/construction
GET  /api/v3/accounts/{account}/castles/{castle}/production
GET  /api/v3/accounts/{account}/kingdoms/{kingdom}/map
GET  /api/v3/accounts/{account}/movements
GET  /api/v3/accounts/{account}/operations
POST /api/v3/accounts/{account}/requests/{requestType}
```

Every query includes schema, scope, projection version/ETag, freshness, health, observation position, and relevant operation links. Large histories/maps are paginated or cursor-based.

### Event stream

Subscribe to named projections/scopes. A message carries subscription, scope, projection version, sequence, and either a typed delta or invalidation. On a gap, refetch only that projection. Backpressure is bounded and observable; state-changing inbound traffic is never coupled to an unbounded UI queue.

### API v2 adapter

Compose the existing `GameState` shape from capability queries and preserve the broad compatibility revision. The adapter is permitted to do expensive composition because it is temporary. Freeze examples of current `/api/v2/state`, intent arguments/results, operation events, errors, omission/null behavior, and WebSocket invalidation before implementation.

### Frontend shell

Introduce a router and scope-aware application shell owning only:

- profile/session/account selection;
- viewed kingdom/castle selection;
- live focus indicator;
- global alerts/notifications;
- operation drawer/center;
- command palette and accessibility/theme;
- local authentication/bootstrap.

Each route owns its query hooks and presentation state. Generated API types stop at the transport layer; feature view models remain hand-written. The legacy global context stays mounted only around unported screens and disappears last.

### Behavioral boundaries

- The UI never emits raw game protocol.
- An optimistic UI may show a pending receipt but cannot present destructive game state as committed.
- Modal open/selection/filter state remains client-local.
- Presets, policies, schedules, targets, and other durable desired documents live server-side with versions.
- Client-owned background effects remain unchanged until explicitly promoted to a durable policy.

## Persistence and migration

### Proposed records

A relational application store can begin with:

```text
profiles
sessions
account_bindings
world_accounts
kingdoms
castles
capability_snapshots
desired_documents
policy_instances
schedules
operations
operation_transitions
journal_records
projection_cursors
report_records
history_indexes
capture_segments
migration_receipts
```

Capability payloads may start as schema-versioned JSON documents. Normalize only collections that need indexing, joins, retention, or partial updates, such as operations, reports, history, map targets, and capture metadata.

### Data-directory cutover

1. Resolve explicit `CITADEL_DATA_DIR` first.
2. Discover the existing per-instance/executable-adjacent data root.
3. Create a new versioned destination without mutating the original.
4. Copy and import settings, presets, schedules, browser/profile references, meaningful last-known snapshots, histories, report/capture indexes, and local overrides.
5. Record counts, hashes, schema versions, unknown records, and invariant failures.
6. Build new projections and compare representative totals/samples.
7. Switch using one marker only after integrity gates pass.
8. Retain the original and a rollback marker until the user confirms the new runtime.

Migration is idempotent: rerunning it against the same source yields the same logical records and no duplicate schedules, reports, presets, or operations.

### What not to import as fresh truth

- Old last-known state becomes `imported`/`stale`, not a live authoritative baseline.
- Old in-flight operations become historical/indeterminate records unless there is enough evidence to reconcile them; they never resume sends automatically.
- Unknown configuration sections are preserved for audit/export and reported, not silently discarded.
- Raw official data remains a catalog cache and is not copied into product configuration tables.

## Capability migration template

Every feature port should produce the same artifact set:

1. **Parity inventory:** current routes, intents, opcodes, reducers, settings/defaults, policies, storage, UI journeys, errors, warnings, and incomplete surfaces.
2. **Scope design:** state owner, key, freshness, identity namespaces, cross-partition reads, focus dependency, and atomic commit groups.
3. **Protocol fixtures:** sanitized baseline, deltas, outgoing commands, success/failure/timeout/ambiguous cases, optional/missing fields, and catalog versions.
4. **Typed facts and reducer:** deterministic replay output and explicit unknown/partial semantics.
5. **Query contract:** task DTO, schema/version, pagination, freshness, health, and operation links.
6. **Typed requests/planners:** validation, preview, dependencies, claims, effects, completion evidence, and reconciliation.
7. **Policy/configuration:** schema, defaults, migration, scope resolution, triggers, schedules, and explanations.
8. **Persistence:** snapshot/document schema, migrations, retention, and import fixture.
9. **Frontend route:** scoped loading/stale/error/empty/operation states and parity journey.
10. **Cutover evidence:** replay comparison, shadow-plan comparison, one-sender assertion, API compatibility, migration result, and rollback check.

The feature is not complete merely because its state parses or its UI renders.

## Recommended port order

### Phase 0: freeze the behavioral target

Create the parity manifest before writing replacement feature code. Inventory every registered/reachable intent, reducer opcode, automation, settings section/default/migration, UI route/modal, persistence format, and CLI/API workflow. Label each as reachable, server-only/incomplete, legacy-only, or future.

Required evidence:

- sanitized operation captures;
- representative full/baseline and partial frames;
- deterministic clocks/settings/catalogs;
- current projections/receipts after each meaningful observation;
- UI journey outcomes and warnings;
- current performance and data-size measurements.

Exit gate: every effect and automation has an owner, risk class, and at least one acceptance scenario.

### Phase 1: identity, session, account loop, and read-only walking skeleton

Implement profile/session/account binding, generation fencing, observation envelopes, deterministic clock, replay adapter, scope keys, account loop, transactional store, authenticated local API, and a minimal account/castle directory projection.

Exit gate: a replay baseline binds to the correct account, discovers kingdoms/castles, publishes a scoped query/update, persists it, restarts as last-known, and sends no effect.

### Phase 2: focus context and execution kernel

Implement typed requests, dependencies, claims, admission, lanes/pacing, operation receipts, correlation, committed completion barriers, cancellation, focus epochs, and ambiguity handling. Port one reversible focus/read operation and one small mutation.

Exit gate: reconnect, old-generation response, timeout, cancellation, full queue, and ambiguous-send scenarios match the defined behavior; no duplicate effect occurs.

### Phase 3: construction and TCI vertical slice

Port castle construction because it exercises nearly every important boundary:

- castle scoping and focus-derived responses;
- combined atomic snapshots;
- building definition versus instance IDs;
- TCI CID namespace rules;
- account inventory plus castle slots;
- resources, catalog, shop, purchase, upgrade;
- desired building/decoration/TCI documents;
- long multi-step plans and authoritative verification.

Exit gate: current construction/TCI requests and AutoTCI decisions match golden/shadow evidence, including `gbc`/`sbp`/`rpc`/`ubc` semantics and disconnect ambiguity.

### Phase 4: production, crafting, economy, and logistics

Port recruitment, tools, hospital, crafting, Sceat/resource workflows, castle economy, food balance, market shipping, and kingdom resource/troop transport. This proves repeated castle instances and controlled cross-castle orchestration.

Exit gate: global/per-castle configuration resolves identically; source/destination/reserve selection matches; one-in-flight lanes and authoritative resource updates are preserved.

### Phase 5: leaders, equipment, garrison, and movements

Port leader roster, equipment/gems/loadouts, optimizer, garrisons, movement ledger, station/recall, Bird/Station, and Berimond transfer. This proves account-shared inventories and movements with castle indexes and compound claims.

Exit gate: no commander/item/troop can be double-reserved, optimizer inputs are versioned, movement lifecycle is authoritative, and current cleanup lifecycle remains intentionally classified.

### Phase 6: defense, combat, presets, spy, and Rift

Port defense and temporary protection workflows, attack context/presets, attack planning/admission, spying, reports required for completion, and Rift templates/launches. This establishes the reusable combat boundary before event policies.

Exit gate: manual attack/defense/spy/Rift journeys and pacing match, temporary defense restoration is durable/reconciled, and local versus game presets remain distinct.

### Phase 7: map, alliance, intelligence, and event families

Port kingdom map and target identity, alliance inspection/refresh/help, then Towers, Invasion, Nomad/Samurai, Khan, and Storm in increasing orchestration complexity. Event modules reuse normal combat, movement, defense, economy, construction, and shop requests.

Exit gate: each event has run/source/target scope, per-instance status, equivalent target selection and safety policy, and no duplicated domain implementation.

### Phase 8: reports, analytics, support, and complete UI cutover

Port normalized report capture, battle/player/alliance history projections, retention/redaction, support export, diagnostics, settings, updates, patch notes, and remaining screens. Add the full operation center and capability matrix. Remove the legacy global React provider after the last view migrates.

Exit gate: all mandatory UI journeys use native scoped queries, analytics totals are reconciled, support bundles are redacted, and v2 remains the only legacy composition path.

### Phase 9: data cutover and hardening

Make the new runtime the only live sender, complete copy-based data migration, enforce local authentication and storage/retention policies, test abrupt restart and database recovery, compare performance budgets, and retain bounded replay/legacy comparison tooling.

Exit gate: every mandatory manifest row is accepted; migration and rollback work on representative copies; no old sender can activate.

### Phase 10: optional architecture escalation

Only after measured triggers:

- deepen journal/reprojection infrastructure if protocol forensics and analytics justify its schema cost;
- extract account runtimes into supervised worker processes if simultaneous accounts need crash/resource isolation;
- add remote/mobile/tool adapters only with explicit identity and authorization;
- add a subprocess or WASI extension host only after a real ecosystem and permission model exist.

## Verification strategy

Although documentation work does not require executing the product test suite, the architecture implementation requires stronger evidence than ordinary unit tests because parity includes external side effects.

### Golden transcript comparison

For each effect family, compare:

- initial scope/state/catalog/settings;
- request and actor;
- admission and rejection reason;
- required focus/context transitions;
- semantically decoded outgoing records and timing boundaries;
- response matching and state commit;
- final query projection and receipt.

Compare bytes where omission/order is meaningful; otherwise compare typed protocol meaning. Include failure, timeout, reconnect, stale input, and ambiguous send cases.

### Reducer replay comparison

Feed captures through old and new paths with the same deterministic clock/catalog. Compare after every meaningful observation. Normalize only documented representation differences; do not hide scope or null/unknown mismatches behind a broad final-state comparison.

### Shadow planning

In live or replay shadow mode:

- old runtime is the sole sender;
- new planners see mirrored accepted facts and equivalent request triggers;
- proposed claims, commands, pacing, and completion rules are recorded;
- differences are classified as bug, intentional change, missing observation, or baseline ambiguity.

Enforce the sole-sender rule structurally through effect-port wiring, not a Boolean every planner must remember.

### Policy decision comparison

For each automation, record a trace of effective settings, declared input versions/freshness, candidates considered, exclusions, selected request, or no-action reason. Compare current and new policy decisions across representative states and time boundaries.

### Contract and UI journeys

- Freeze v2 API examples and error/revision behavior.
- Validate generated v3 clients against public schemas.
- Exercise start/offline/reconnect, scope navigation, manual operations, policy configuration, schedules, cancellations, errors, stale data, and indeterminate recovery.
- Measure that an update to one castle/capability does not refetch or rerender unrelated feature payloads.

### Migration and persistence

- Use anonymized fixtures for every known settings/preset/schedule/history/state version.
- Verify counts, stable IDs, hashes, unknown fields/records, defaults, and idempotency.
- Kill the process during snapshot/receipt/migration transactions and verify recovery.
- Verify no live effect runs from imported stale state.
- Exercise backup, rollback marker, database integrity, capture retention, and secure export.

### Performance budgets

Measure with production-like captures:

- observation-to-critical-partition commit latency;
- observation-to-subscribed-projection/UI latency;
- CPU/allocation cost versus full-state clone;
- maximum account-loop and outbound queue depths;
- persistence checkpoint age;
- projection payload bytes and client rerenders;
- startup/restart time with realistic reports/history;
- multiple account-loop fairness, even before simultaneous live accounts ship.

Budgets should come from measured current and target workloads, not arbitrary numbers chosen before capture analysis.

## Definition of done for one capability

A capability is migrated only when all applicable statements are true:

- Its current behavior classification is approved.
- It owns typed identities, facts, state, freshness, and scope.
- Its combined observations commit atomically with all affected partitions.
- Its queries are public DTOs independent of storage records.
- Its requests are typed and list complete dependencies and claims.
- Its effects use the kernel and have correlated completion/reconciliation rules.
- Its policies use the same requests as manual actors and report decisions.
- Its configuration schema/default/migration and scope are explicit.
- Its snapshots/documents/receipts survive restart and schema migration.
- Its API v2 compatibility output remains accepted where required.
- Its frontend route handles fresh/stale/unknown/empty/degraded/offline/active-operation states.
- Golden replay, shadow plan/policy, migration, and user-journey evidence pass.
- The legacy implementation for that feature cannot send once cut over.
- Rollback does not lose or reinterpret user desired documents.

## Major risks and mitigations

| Risk | Consequence | Mitigation/evidence |
|---|---|---|
| Wrong focus-derived scope | State from one castle corrupts another | Captured focus epoch, operation correlation, explicit precedence, anomaly quarantine |
| Split atomic frame | UI/planner sees impossible mixed snapshot | Staged account change set and post-commit publish |
| Claims too broad | Architecture retains unnecessary blocking | Derive claims from parity cases; inspect wait metrics; narrow only with safety evidence |
| Claims too narrow | Duplicate purchases, leaders, troops, lanes, or focus races | Compound resource inventory; adversarial concurrency scenarios; deterministic acquisition |
| Versions too broad | Unrelated changes still reject plans | Per-capability-instance versions and declared read sets |
| Versions too narrow | Planner misses a dependency change | Planner trace plus dependency coverage review; stale-plan tests |
| Global settings change meaning when instantiated | Different castles/actions become enabled | Effective-settings snapshots and old/new policy decision comparison |
| Desired and observed state merge | Presets/targets vanish or appear authoritative | Separate stores, schemas, freshness, and migration paths |
| Event module duplicates core features | New vertical silos and inconsistent safety | Event orchestrators submit normal typed requests; shared kernel only for mechanisms |
| Dual execution during migration | Duplicate live game effect | Single effect port/sender lease; shadow path physically cannot send |
| Imported snapshot treated as current | Automation acts on stale data after restart | Imported freshness class; baseline gate on all live effects |
| API v2 behavior weakens | Existing clients accept/reject different operations | Dedicated global compatibility revision and frozen contract fixtures |
| Persistence rewrite loses data | Presets/history/settings lost or duplicated | Copy-based idempotent migration, receipts/hashes, rollback marker |
| Raw diagnostics expose secrets | Account/session/privacy compromise | Bounded capture store, redaction, previewed export, restrictive permissions |
| Over-generalized framework | Feature work becomes harder than current code | Prove abstractions in several distinct vertical slices; generalize only repeated mechanisms |
| Premature distributed topology | IPC, partial failure, deployment burden | Keep logical partitions in one process until measured worker triggers |

## Architectural decision gates

The implementation team should stop and make an explicit decision at these points:

### Gate A: parity baseline

Are active-tree behavior, legacy expectations, incomplete surfaces, and future intent classified well enough that “same functionality” is testable?

### Gate B: walking-skeleton viability

Can one replay produce scoped state/query/UI updates with no global clone and with transactional restart behavior?

### Gate C: kernel sufficiency

Can construction/TCI, equipment, movement/combat, and a report workflow use the kernel without adding feature-specific branches to it?

### Gate D: state granularity

Do measured invalidation, claim contention, transaction size, and policy read sets support the proposed partition boundaries? Split or merge partitions based on consistency and change patterns, not package aesthetics.

### Gate E: UI/product behavior

Which incomplete or client-lifecycle features are preserved as-is, promoted, redesigned, or retired? Record these as product changes separate from parity.

### Gate F: cutover readiness

Do replay, shadow, migration, UI journey, security, recovery, and performance evidence cover every mandatory parity row, and is only one live sender possible?

### Gate G: topology escalation

Is there measured multi-account crash/resource isolation or remote-worker demand sufficient to justify process workers? If not, retain the modular monolith.

## Concrete first implementation backlog

The first engineering increment should be intentionally narrow:

1. Freeze a sanitized `gbd` baseline and two `jaa` focus/castle snapshots for at least two castles/kingdom contexts.
2. Define canonical profile, session, world account, kingdom, castle, observation, operation, and capability-instance keys.
3. Implement generation-fenced observation envelopes and focus epochs in a new read-only account loop.
4. Decode castle-directory facts and one castle construction snapshot into new partitions without changing the old reducer.
5. Persist those snapshots and freshness metadata transactionally.
6. Expose `accounts`, `castle directory`, and one `castle construction overview` v3 query with ETags.
7. Add a small routed client shell showing viewed castle, live focus, and capability status for that construction view.
8. Replay the same capture through old/new paths and report field/scope/version differences.
9. Add a typed, effect-free preview of one reversible construction refresh/focus request.
10. Only after that read path is stable, wire the execution kernel to one live operation under a sole-sender adapter.

This increment tests the hardest foundational assumption—correct scoped state under changing castle focus—before the team invests in every feature module or a generalized framework.

## Documentation and governance artifacts

Keep these living artifacts beside implementation:

- parity manifest and decision status for every feature;
- opcode/fact ownership registry;
- capability/scope/partition catalog;
- typed claim registry and acquisition rules;
- public API schema and compatibility matrix;
- settings/document schema registry and migration fixtures;
- operation risk/idempotency/reconciliation catalog;
- data classification, retention, and support-export policy;
- architecture decision records for changed boundaries;
- replay/shadow discrepancy ledger;
- cutover and rollback runbook.

These are not generic process paperwork. They are the durable replacement for behavior that is currently implicit across reducers, planners, settings components, captures, and logs.

## Repository evidence index

The following reference block records the current-code areas that this workplan changes. Line numbers describe the 2026-07-15 working tree.

```text
startLine: 11
endLine: 240
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/State/Store.go
purpose: global clone transaction, revision, snapshot, normalization, and state events to replace incrementally

startLine: 37
endLine: 942
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/State/Models.go
purpose: current global state families to assign to scoped capability owners

startLine: 101
endLine: 250
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Ingest/Pipeline.go
purpose: current ordered observation, reducer transaction, protocol observation, and response completion pipeline

startLine: 5
endLine: 134
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Ingest/CoreReducers.go
purpose: opcode ownership inventory for typed fact routing

startLine: 23
endLine: 111
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Intent/Types.go
purpose: global expected revision, global planning context, string claims, generic plan/step/barrier types

startLine: 179
endLine: 830
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Intent/Engine.go
purpose: current safety semantics to retain in the smaller execution kernel

startLine: 151
endLine: 203
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/App/Application.go
purpose: current automation composition and runtime startup

startLine: 323
endLine: 420
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/App/Application.go
purpose: current application/session/configuration/catalog/update/scheduling actions and definitions

startLine: 58
endLine: 90
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/API/Server.go
purpose: current route surface

startLine: 172
endLine: 178
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/API/Server.go
purpose: current direct internal GameState serialization

startLine: 311
endLine: 419
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/API/Server.go
purpose: current full snapshot and revision/domain invalidation event behavior

startLine: 32
endLine: 169
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Client/src/api/ApiContext.tsx
purpose: global client state and broad refetch/polling behavior to retire route by route

startLine: 36
endLine: 176
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Client/src/App.tsx
purpose: current local screen selection, top-level feature modal state, and UI-lifecycle cleanup

startLine: 22
endLine: 31
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Configuration/LegacyMigration.go
purpose: existing configuration imports that need fixtures and scope-preserving migration
```

## Conclusion

The architectural change is feasible without changing current game functionality, but only if state partitioning, focus semantics, intent dependencies, feature ownership, and parity evidence are developed together. Splitting structs alone would move complexity without resolving it; rewriting the engine alone would leave a global state and UI contract; rebuilding screens alone would hide the same backend coupling.

The first decisive milestone is a read-only, multi-castle scoped walking skeleton that proves correct focus attribution and narrow UI updates. Construction/TCI should then be the first full vertical slice because it exercises the hardest identity, state, catalog, cross-partition, shop, desired-state, and outcome rules. Once that slice and the execution kernel are sound, the remaining capabilities can be ported in dependency order with explicit feature impact and verifiable parity.

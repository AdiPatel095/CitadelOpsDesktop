# Scoped state architecture

## Conclusion

CitadelOps should replace its single `GameState` with **capability partitions addressed by explicit game scope**. The scope hierarchy is profile/session → world account → kingdom → castle, with additional typed identities for leaders, movements, reports, event runs, and map targets.

This is a logical and transactional decomposition inside the recommended modular monolith. It does not imply one database, process, service, or goroutine per field. The goal is to give each state change one clear owner, version only what actually changed, and make every feature declare the exact scopes it reads and mutates.

The essential model is:

```text
CapabilityInstance = CapabilityKind + ScopeKey

Construction + CastleKey
Defense      + CastleKey
WorldMap     + KingdomKey
Equipment    + AccountKey
TowerRun     + CastleKey + EventKey
```

That decomposition is consistent with the architectural principle that an aggregate contains only data that must remain consistent together. Microsoft’s current [tactical DDD guidance](https://learn.microsoft.com/en-us/azure/architecture/microservices/model/tactical-domain-driven-design) describes an aggregate as a consistency boundary and advises keeping independently changing data in separate aggregates. CitadelOps should use that principle inside one process, not turn every aggregate into a microservice.

## What partitioning solves

Today, one protocol observation can:

- deep-clone the entire account state;
- increment one global revision even when only protocol diagnostics changed;
- invalidate a plan for an unrelated castle or feature;
- persist and serialize a structure containing every feature;
- notify the React client, which then refetches the entire state;
- make all global-context consumers eligible to render.

Partitioning changes this to:

```text
observation position advances
        |
        v
scope is resolved -> typed facts -> atomic change set
                                   ├── Construction[Castle A] v42
                                   ├── Economy[Castle A] v18
                                   └── SessionContext v9

only affected projections and policies are notified
```

An equipment update no longer invalidates construction in an unrelated castle. A production update in one castle no longer republishes every movement, report, event, and alliance record. A protocol-only observation advances diagnostics but not domain versions.

## Partitioning does not remove wire serialization

Many game responses contain an explicit castle or kingdom ID, but several important responses are interpreted using the currently focused castle. Production, focused units, construction items, building mutations, defense fragments, and resource responses can depend on session context.

Therefore:

- state can be independently stored and versioned by castle;
- queries and automation can independently observe those partitions;
- different accounts can progress independently;
- but focus-changing commands within one game session must remain serialized through a shared session-focus claim.

Partitioning improves isolation and precision. It must not invent command concurrency that the game protocol cannot safely support.

## Identity and scope model

### Bootstrap identity

The game account is unknown before the login baseline. CitadelOps needs a stable local identity before it can create an account partition:

```text
ProfileID
  -> SessionID + ConnectionGeneration
  -> authoritative login baseline
  -> WorldAccountKey
```

`ProfileID` identifies the configured browser profile and survives restarts. `SessionID` identifies one runtime attempt. `ConnectionGeneration` fences old sockets and responses. The authoritative baseline supplies the player identity and world/server identity needed to bind the runtime to a `WorldAccountKey`.

A profile that logs into a different game account must bind to a different account partition. State, desired settings, histories, captures, and operations must never be merged merely because the same browser profile was reused.

### Typed keys

The internal model should use typed composite keys:

```text
WorldKey
  stable server/realm identity

WorldAccountKey
  WorldKey + PlayerID

KingdomKey
  WorldAccountKey + KingdomID

CastleKey
  KingdomKey + CastleID

LeaderKey
  WorldAccountKey + LeaderKind + LeaderID

MovementKey
  WorldAccountKey + MovementID

TargetKey
  KingdomKey + target type + object ID when present + coordinates

EventRunKey
  WorldAccountKey + event kind + event ID/run ID
```

The current server URL can help discover `WorldKey`, but the final key should use the most stable world/realm identifier available from the authenticated protocol. A numeric player ID alone is insufficient because the same value can exist on different worlds.

`CastleID`, `KingdomID`, construction-item CID, building definition ID, building instance ID, package ID, and leader IDs remain distinct types. Composite keys add scope; they do not collapse the underlying wire namespaces.

### Public scope reference

API and event contracts can use a discriminated scope union:

```text
ScopeRef =
  { kind: "application" }
  { kind: "profile", profileId }
  { kind: "session", profileId, sessionId, generation }
  { kind: "account", worldId, playerId }
  { kind: "kingdom", worldId, playerId, kingdomId }
  { kind: "castle", worldId, playerId, kingdomId, castleId }
  { kind: "target", worldId, playerId, kingdomId, targetType, objectId?, x, y }
```

Domain code should still accept typed keys rather than switching on a generic scope everywhere. `ScopeRef` is a contract and routing representation, not a replacement for domain types.

## Four categories currently mixed inside `GameState`

### 1. Observed game state

Examples include castles, resources, units, defense, buildings, production, equipment, movements, alliance, map observations, event scores, and transport state. These belong in capability partitions with observation lineage and freshness.

### 2. Local desired state

Examples include CitadelOps attack/defense/decoration presets, captured building targets, Rift templates, schedules, commander assignments, automation settings, reserves, and policy targets. These are user documents, not observed game truth. They require their own schema versions and should survive even when the game reports no corresponding object.

### 3. Execution state

Claims, admission slots, queued operations, correlation tokens, retries, schedules currently being dispatched, automation runtime status, and receipts belong to the execution runtime and operation store. They should not be serialized as part of a game-state projection.

### 4. Diagnostics

Protocol opcode counts, raw frames, reducer errors, traces, and capture cursors belong to telemetry and the selective journal. An unknown frame should not version every domain feature.

Keeping these categories separate is as important as splitting state by castle.

## Proposed partition catalog

| Current state family | Target owner | Natural scope | Important notes |
|---|---|---|---|
| Schema/global revision/update time | Account runtime and compatibility adapter | Account/session | Replace with observation position, partition versions, and v2 legacy revision. |
| Catalog/language version | Catalog service | Application snapshot | Operations record the catalog version used; catalog is not mutable account state. |
| Session | Session runtime | Profile/session generation | Includes connection, baseline, browser, retry/cooldown, and protocol namespace. |
| `Castle.Focused`, attack dialog, command context | Session protocol context | Session plus referenced castle/target | Focus is one live-session context, not a property independently owned by every castle. |
| Player identity and metrics | Account profile | World account | Level, might, glory, gallantry, VIP, achievements, Hall of Legends. |
| Player resources/currencies | Account wallet | World account | Keep separate from castle resource balances. |
| Castle identity | Castle directory | World account with `CastleKey` records | Name, kingdom, slot type, coordinates, ownership, discovery/retirement. |
| Castle resources and food | Castle economy | Castle | Amounts, rates, capacity, freshness, projected food risk. |
| Market castle rows and caravan | Logistics | Castle rows plus account caravan | Same-kingdom shipping reads source/destination castle economy and market capability. |
| Stationed/traveling/hospital units | Castle garrison | Castle | Derived totals should be projections; hospital actions also depend on production/inventory context. |
| Defense | Castle defense | Castle | Wall, keep, moat, tools, open gate, castellan link, freshness. |
| Buildings/layout/expansions/queue | Castle construction | Castle | One consistency boundary because collision, queue, placement, upgrade, and target rules change together. |
| Equipped construction items | Castle construction | Castle + building instance | CID namespace remains construction-specific. |
| Construction-item inventory | Construction inventory | Account | Shared across castles; TCI equip/purchase reads both account inventory and castle construction. |
| Construction shop offers | Construction commerce | Castle/kingdom context | Offers are live and scoped to the castle/kingdom used to open the shop. |
| Recruitment/tool production | Castle production | Castle + production line | Queueable definitions and command session context must be preserved. |
| Crafting queues/research | Castle crafting | Castle/building | Account query may aggregate all crafting buildings. |
| Commanders/generals/castellans | Leader roster | Account with castle links | Availability derives partly from movements; castellan assignment references a castle. |
| Equipment/gems | Equipment inventory/loadouts | Account + leader | Equipment mutation often atomically changes inventory and one or more leaders. |
| Generic inventory rows | Typed inventory facts routed to owners | Account, sometimes castle context | Do not recreate one untyped inventory bucket; construction, equipment, time skips, tools, and event items retain namespaces. |
| Movements and movement snapshot | Movement ledger | Account with source/target indexes | Cross-castle by nature; index by castle, kingdom, commander, direction, and target. |
| Stationing operations | Stationing workflow | Account + source/target castles | Execution/workflow state linked to movements, not raw castle state. |
| Resource/troop kingdom transport | Kingdom logistics | Source account/castle + target kingdom | Unlocks and one-in-flight restrictions are kingdom-scoped. |
| Subscriptions | Account entitlements | Account | Read by multiple capabilities through a narrow entitlement projection. |
| Alliance and inspected alliances | Alliance directory | World account/world alliance | Holdings carry kingdom/castle coordinates; inspection does not mutate castle ownership state. |
| Map observations | World map | Kingdom/target | Partition by kingdom; target keys own freshness and event/tower metadata. |
| Tower cooldowns and queues | Tower capability | Target cooldown + source castle queue | A battle acknowledgement is not a successful cooldown; report and later map observation complete it. |
| Invasion state | Invasion capability | Event run + source castle | Difficulty/score may be account-event scoped; scans and active targets are source-castle scoped. |
| Nomad/Samurai state | Nomad capability | Event run + source castle + target | Locked target, cooldown, RBC trial, score and movements have different identities but one event workflow owner. |
| Khan state | Khan capability | Event run + source/main castles | Includes attack run, taunts, point protection, defense/open-gate workflow, and movements. |
| Storm state | Storm capability | Storm kingdom + Storm castle | Construction, logistics, map targets, shop context, attack presets, and Aquamarine policy are orchestrated dependencies. |
| Berimond capacity | Beri capability | Event castle/run | Keep parsed source, consumed capacity, and transfer operation together. |
| Rift launches/templates | Rift desired-state capability | Account, referencing kingdom/castle/commander | Captured templates are durable local documents, not canonical game state. |
| Game-saved attack presets | Combat context | Account/session | Distinguish these from richer CitadelOps `attacks.presets` configuration documents. |
| Scalable event scores/shop routes | Event directory | Account + event run | Shared read model for event-specific policies; event modules own their run state. |
| Scheduled operations | Scheduler/operation store | Account + requested feature scope | Store typed request plus scope and schema, not raw protocol. |
| Automation statuses | Policy runtime | Policy instance + scope | A policy may have one account instance or several castle instances. |
| Report notices/captures | Report inbox/capture runtime | Account/session | Transient multi-part capture is separate from normalized durable report history. |
| Protocol observations | Telemetry/journal | Session/opcode | Does not belong in domain state or capability versioning. |

## Observation routing

### Observation context

Before domain decoding, every message receives an immutable context:

```text
ObservationContext
  ProfileID
  SessionID
  ConnectionGeneration
  IngressSequence
  ReceivedAt
  Direction
  Namespace / opcode / route / response code
  Bound WorldAccountKey, if known
  Focused CastleKey + FocusEpoch, if known
  Correlated OperationID and expected scope, if known
  CatalogVersion
```

The context must be captured in account-loop order. An asynchronous decoder must not look up “whatever castle is focused now” after a later focus transition has already occurred.

### Scope-resolution precedence

For a fact affecting a castle or target:

1. Use explicit, validated wire identifiers when the payload contains them.
2. Use the correlated operation’s declared response scope when the protocol response omits identity.
3. Use the captured focus context only when the same session generation and focus epoch remain valid for that observation.
4. If scope remains ambiguous or conflicts, record an unresolved observation and degrade the dependent capability; never guess and mutate a castle.

An explicit wire ID that contradicts operation correlation is a protocol anomaly. It should fail the completion barrier and produce diagnostics rather than silently selecting either value.

### Typed facts

Decoders produce facts with resolved keys:

```text
CastleSnapshotObserved(CastleKey, ...)
CastleProductionObserved(CastleKey, LineID, ...)
ConstructionSlotsObserved(CastleKey, BuildingInstanceID, ...)
AccountEquipmentObserved(WorldAccountKey, ...)
MovementSnapshotObserved(WorldAccountKey, ...)
MapViewportObserved(KingdomKey, Bounds, Targets, ...)
```

A fact describes domain meaning and source lineage. It does not expose the raw transport shape to capability state.

### One observation, several facts

Initial `gbd` and castle `jaa` snapshots update several domains. Combined response reducers can update both feature state and resource balances. The replacement must stage all facts from one observation and commit an **atomic account change set**:

```text
AccountChangeSet
  ObservationPosition
  Facts[]
  PartitionMutations[]
  ChangedPartitionVersions[]
  ProjectionInvalidations[]
  CapabilityHealthChanges[]
```

No subscriber should observe a new castle layout with the old construction queue when both came from one accepted snapshot. If current behavior rejects the entire frame after a sub-decoder error, the compatibility path must retain that all-or-nothing result until a deliberate protocol policy changes it.

## Focus as an explicit protocol capability

Create a `SessionProtocolContext` containing:

- current session and connection generation;
- authoritative-baseline generation;
- focused `CastleKey`;
- monotonically increasing `FocusEpoch`;
- time and observation position of the focus acknowledgement;
- current attack dialog/context and production session context;
- correlation entries for in-flight focus-dependent operations.

Every operation that can change or depend on focus claims `session/focus`. A focus command succeeds only after the focus response is committed and the expected castle becomes the current context. Subsequent focused commands include the expected focus epoch in their dependency set.

The UI’s **viewed castle** remains presentation state and can change offline without mutating live focus. The UI should display both when they differ:

```text
Viewing: Winter Outpost (last observed 8 minutes ago)
Live game focus: Main Castle
```

This makes existing offline behavior clearer and prevents state partitioning from conflating navigation with a game effect.

## Versions, read views, and freshness

### Four useful counters

1. **Observation position** advances for every accepted account observation, including unknown/protocol-only messages.
2. **Partition version** advances only when one capability instance materially changes.
3. **Projection version/ETag** advances when a public query representation changes.
4. **Legacy v2 revision** advances according to the current broad commit rules for compatibility.

The v2 adapter continues to expose one global revision and exact broad stale-request behavior. Native capability requests use a dependency set such as:

```text
Construction[Castle A] = 42
Economy[Castle A] = 18
ConstructionInventory[Account] = 7
SessionProtocolContext = generation 11, focus epoch 23
Catalog = Items-2026-07-15
```

For HTTP clients, a projection can expose its version as an `ETag`, and state-changing requests can use `If-Match` where appropriate. [RFC 9110](https://www.rfc-editor.org/rfc/rfc9110) defines `If-Match` specifically for conditional requests that prevent applying a mutation when the selected representation has changed.

### Consistent read views

A planner that reads multiple partitions needs an immutable `AccountReadView` captured at one observation position. Each entry carries its partition version. The account loop makes creation of that view atomic without cloning every partition.

Final validation compares only the declared dependencies. Claims prevent conflicting concurrent effects; versions detect that planning inputs changed. Neither mechanism replaces the other.

### Freshness metadata

Each observed partition records:

- state/schema version;
- last observation position and time;
- session/connection generation;
- source opcode/fact type;
- authoritative snapshot versus partial delta/imported data;
- catalog version used for derived fields;
- refresh-in-flight or refresh-failed state;
- expiry policy where the feature has one.

“Missing,” “unknown,” “empty,” “stale,” and “unsupported” must remain distinct.

## Capability-instance status

To provide “some idea for all features,” maintain a queryable directory of capability instances:

```text
CapabilityInstanceStatus
  Capability
  ScopeRef
  Support: supported | not-applicable | not-discovered
  Health: healthy | degraded | blocked
  Freshness: never-observed | imported | stale | refreshing | fresh
  StateVersion
  ProjectionVersion
  LastObservedAt
  LastObservationPosition
  MissingDependencies[]
  ActiveOperationIDs[]
  AutomationInstances[]
  RecommendedAction
```

Capabilities register which scopes they support. Discovery of a castle creates applicable capability instances; kingdom and slot type determine which are supported. Removing or losing access to a castle retires the live instances but does not immediately delete history or desired settings.

An account overview can compose a matrix without owning the underlying state:

| Capability | Main castle | Outpost | Storm castle | Account |
|---|---|---|---|---|
| Construction | Fresh | Stale | Fresh | — |
| Defense | Fresh | Never observed | Fresh | — |
| Equipment | — | — | — | Fresh |
| Towers | Running | Disabled | Not applicable | Healthy |
| Storm | — | — | Running | Healthy |

## Cross-partition operations

### TCI equip

Reads:

- construction state for the target castle/building;
- account construction-item inventory;
- official construction-item catalog;
- session focus context.

Claims:

- target castle/building/slot;
- account TCI inventory;
- session focus.

The response commits both the castle’s equipped slots and any account resource/inventory changes before success.

### Same-kingdom resource shipment

Reads and claims source castle economy/market, destination castle economy, account/kingdom logistics, and relevant resources. It does not lock equipment, reports, or unrelated castles.

### Attack launch

Reads source-castle garrison, account leader/equipment/inventory, movement ledger, session attack context, target map/event state, presets, and catalog. Claims the source attack inventory, selected commanders, target, attack admission class, and session focus/context. A movement acknowledgement and committed movement state remain required according to the existing operation.

### Auto Storm

Auto Storm is an orchestrator, not one giant aggregate. Its policy read view composes:

- Storm event/run and map state;
- Storm castle construction, economy, garrison, and defense;
- account construction inventory/shop context;
- kingdom logistics and donor-castle state;
- leader/movement availability;
- attack and decoration desired-state documents;
- scheduler, reserves, and policy status.

It then submits typed Construction, Logistics, Decoration, Shop, or Attack requests. Those capabilities remain the only owners of their mutations.

## Query and subscription model

The new API should expose task-shaped resources rather than an internal state tree:

```text
GET /api/v3/accounts/{account}/capabilities
GET /api/v3/accounts/{account}/overview
GET /api/v3/accounts/{account}/kingdoms/{kingdom}/map
GET /api/v3/accounts/{account}/castles/{castle}/construction
GET /api/v3/accounts/{account}/castles/{castle}/economy
GET /api/v3/accounts/{account}/castles/{castle}/production
GET /api/v3/accounts/{account}/castles/{castle}/defense
GET /api/v3/accounts/{account}/movements
GET /api/v3/accounts/{account}/equipment
GET /api/v3/accounts/{account}/operations
```

The event stream publishes projection deltas or invalidations with scope and version. A lost sequence causes a refetch of that projection only. The API v2 adapter composes the existing `GameState` shape and existing global revision until all current clients migrate.

## Persistence shape

A practical first implementation can persist one versioned document per capability instance rather than fully normalize every game field:

```text
scope_directory
  profile/account/kingdom/castle identities and bindings

capability_snapshots
  account key
  capability kind
  scope kind + canonical scope key
  schema version
  state version
  observation position/generation/time
  catalog version
  payload

desired_documents
  account/capability/scope/document ID/schema/version/payload

operations + operation_transitions
journal_records + capture_segments
history/report projections
```

An account change set writes all affected snapshot rows, receipt transitions, and its durable observation cursor in one short transaction where crash consistency is required. Large raw captures remain separate bounded files.

Capability documents allow independent migration and replacement. Highly queried histories or map targets can use normalized/indexed projections without forcing every capability into the same persistence model.

## Invariants for the scoped store

- No live account partition exists without a recorded profile/session-to-world-account binding.
- No focused response mutates a castle unless scope was explicit or resolved from a valid correlation/focus epoch.
- One observation’s accepted multi-part state changes are atomically visible.
- Only materially changed partitions increment native versions.
- A native plan lists all partitions, catalog versions, and session context it depends on.
- Cross-partition commands acquire typed, account-scoped claims in deterministic order.
- Unknown observations affect diagnostics, not domain versions.
- Imported legacy state is marked imported/last-known, never presented as a fresh live baseline.
- Desired documents are never erased because observed game state is missing.
- The v2 compatibility view retains the current revision and response semantics until retired explicitly.

## Repository evidence index

The following reference block records the principal current-code ranges used in this scoped-state analysis. Line numbers describe the 2026-07-15 working tree.

```text
startLine: 9
endLine: 30
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/State/Models.go
purpose: distinct wire identity types

startLine: 37
endLine: 540
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/State/Models.go
purpose: session, player, castle, construction, production, crafting, leaders, movements, inventory, market, transport, and alliance state

startLine: 592
endLine: 902
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/State/Models.go
purpose: Rift, map, events, attack context, reports, command context, and automation state

startLine: 904
endLine: 942
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/State/Models.go
purpose: monolithic GameState aggregation

startLine: 11
endLine: 240
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/State/Store.go
purpose: global revision, full-state snapshots/deep cloning, and state events

startLine: 17
endLine: 173
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Ingest/AccountReducers.go
purpose: multi-domain initial baseline reduction and atomic baseline generation

startLine: 147
endLine: 315
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Ingest/CastleReducers.go
purpose: focus transitions and multi-domain castle/focused response routing

startLine: 609
endLine: 620
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Ingest/CastleReducers.go
purpose: current focused-castle lookup

startLine: 5
endLine: 134
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Ingest/CoreReducers.go
purpose: inbound/outbound opcode-to-reducer registry

startLine: 101
endLine: 250
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Ingest/Pipeline.go
purpose: ordered observation, global transaction, reducer commit, and response completion

startLine: 23
endLine: 111
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Intent/Types.go
purpose: global planning context, global expected revision, claims, steps, barriers, and admission

startLine: 179
endLine: 830
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Intent/Engine.go
purpose: revalidation, claims, execution, correlation, committed barriers, and receipts

startLine: 1407
endLine: 1419
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/App/BuildingIntents.go
purpose: current castle/building/position claim construction

startLine: 52
endLine: 75
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/App/CommandContexts.go
purpose: JAA/JCA focus and attack-context behavior

startLine: 10
endLine: 54
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/App/Parity_test.go
purpose: existing feature/intent/automation parity manifest

startLine: 58
endLine: 90
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/API/Server.go
purpose: current API surface

startLine: 172
endLine: 178
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/API/Server.go
purpose: direct internal GameState serialization

startLine: 311
endLine: 419
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/API/Server.go
purpose: full initial snapshot and revision/domain event stream

startLine: 32
endLine: 169
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Client/src/api/ApiContext.tsx
purpose: global client state, full-state invalidation/refetch, and global polling

startLine: 523
endLine: 535
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/App/Application.go
purpose: global configuration defaults for scheduler and automation features

startLine: 22
endLine: 31
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Configuration/LegacyMigration.go
purpose: existing legacy configuration-to-section migration map
```

## Result

The scoped model provides precise state ownership without weakening the current execution semantics. It also creates the missing product-level overview: every supported capability can report where it applies, how fresh it is, whether it is healthy, which operation or automation is active, and what observation is needed next.

# Feature-by-feature architectural impact

## Executive finding

The proposed architecture does not require removing a current feature. It changes **where each feature owns truth, how it declares dependencies, and how users select and understand its scope**.

Most manual actions can retain the same game behavior and wire commands. The largest product-visible changes would be beneficial clarifications:

- every view and setting identifies its account, kingdom, and castle scope;
- the castle being viewed is distinct from the castle currently focused in the live game session;
- every feature shows whether its data is fresh, stale, never observed, unsupported, or degraded;
- operations show their declared scope, dependencies, claims, progress, and observed outcome;
- automation is displayed as policy instances attached to the scopes they actually manage;
- cross-castle features show all participating castles rather than hiding them in one global state object;
- a broken parser or stale event capability does not make unrelated features appear invalid;
- the client subscribes to task-shaped projections instead of refetching all account state.

The intended preservation boundary is:

```text
same user request + same observed inputs + same settings
  -> same eligibility decision
  -> same required focus transitions
  -> same semantically meaningful wire commands and pacing
  -> same correlated completion rule
  -> equivalent user-visible result
```

The architecture can improve explanations, navigation, freshness, isolation, and recovery without changing game effects. Any deliberate gameplay-policy change—different target selection, new retries, server-side equipment cleanup, broader automation, or altered purchases—must be separately approved and cannot be smuggled into the rewrite.

## How to read the matrix

Four kinds of state recur throughout the product:

| Kind | Meaning | Example |
|---|---|---|
| Observed | Facts learned from the game and valid only with lineage/freshness | Castle queue, movement, equipment inventory, event score |
| Desired | Durable CitadelOps documents describing what the user wants | Building target, attack preset, schedule, reserves |
| Execution | Temporary control-loop state | Claim, queued operation, focus transition, response correlation |
| Diagnostic | Evidence about transport and interpretation | Raw frame, reducer error, capability degradation |

The target scope column names the **natural owner**, not the only data a feature may read. A castle construction request can read account inventory and session focus while still mutating the construction partition for one castle.

## Scope and capability summary

| Product area | Primary target scopes | Cross-scope dependencies | Main visible change |
|---|---|---|---|
| Session/browser | Profile and session generation | Bound world account | Explicit profile-to-account binding and connection health |
| Account/player | World account | Catalog and event runs | Account overview independent of castle focus |
| Castle directory/focus | Account directory; session focus | Kingdom and castle identity | Viewed castle separated from live focus |
| Buildings/layout | Castle | Account inventory, catalog, session focus | Per-castle freshness, queue, target, and operation status |
| Construction items | Account inventory plus castle/building slots | Shop context, wallet, focus | CID-safe inventory and per-slot explanations |
| Production/hospital | Castle and production line | Inventory, VIP/catalog, alliance help | One status per castle/line instead of global automation state |
| Crafting/Sceat | Castle/building | Account wallet, kingdom logistics | Explicit source, destination, queue, and reserve dependencies |
| Economy/logistics | Castle; kingdom workflow; account wallet | Market/caravan, catalog | Cross-castle transfer plan and food-risk overview |
| Garrison/movements | Castle garrison; account movement ledger | Leaders, targets, kingdoms | Source/target indexes and authoritative lifecycle |
| Leaders/equipment | Account inventory and leader | Castle assignments, movements | Loadout readiness and conflicts visible across castles |
| Defense | Castle | Castellan/loadout, shop, focus | Per-castle defense health and protected recovery workflow |
| Attacks/spy/Rift | Source castle, target, account presets/templates | Garrison, leaders, movement, map, focus | A complete operation view rather than modal-local context |
| Alliance/map/intelligence | Alliance/world and kingdom/target | Reports, events, castles | Scoped observation freshness and provenance |
| Towers | Source castle plus target/cooldown | Map, leaders, movements, presets | Per-castle queues and proof of victory/cooldown |
| Invasion | Event run plus source castle | Map, score, leaders, presets | Event status separated from source-castle execution |
| Nomad/Samurai | Event run, source castle, locked target | Cooldowns, skips, movement, score | Explicit target state machine and reserve accounting |
| Khan | Event run, source/main castle | Attack, defense, map, score, movements | Protection workflow visible as composed operations |
| Storm | Storm kingdom/castle and event run | Construction, logistics, combat, shop | One orchestrator dashboard backed by normal capabilities |
| Reports/analytics | Account history and indexed entities | Reports, players, alliances, targets | Durable provenance, filters, and replayable calculations |
| Scheduler/automation | Policy instance plus declared scopes | Capability read views and operation store | Scope-aware status, reasons, next check, and blocking dependency |
| Catalog/update/support | Application snapshot/profile | Capability catalog version lineage | Version provenance and bounded support exports |

## 1. Session, browser, startup, and offline mode

### Current behavior

CitadelOps selects or launches a Chromium browser, owns a profile/session, observes the game socket, tracks connection and baseline generations, can use replay transport, and exposes session start/stop/select-browser intents. Session fields currently live beside all game domains in `GameState`.

### Target ownership

- `ProfileDirectory[ProfileID]` owns configured browser profile and local preferences.
- `SessionRuntime[ProfileID, SessionID]` owns transport, connection generation, baseline generation, namespace, retry/cooldown, and socket health.
- `AccountBinding[ProfileID, SessionID]` records the authoritative binding to `WorldAccountKey`.
- `SessionProtocolContext` owns live focus and other context needed to interpret context-dependent responses.
- Last-known capability snapshots remain account-scoped and are never promoted to fresh session state on restart.

### Internal change

Startup no longer constructs one anonymous state tree and later fills in player identity. It first has a profile/session scope, then creates or attaches to the correct account runtime after the authoritative baseline identifies the world and player. Old connection generations are fenced from every fact and response barrier.

### User-visible effect

The session screen can say which profile is starting, which account it resolved to, whether the view is live or last-known, which baseline is missing, and why operations are blocked. Offline browsing remains possible, while all live effects stay disabled until a current authoritative baseline is committed.

### Parity hazards

- Reusing one profile for a different world/account must not merge data.
- A reconnect must not allow a delayed old-socket response to satisfy a new operation.
- Replay sessions must never acquire a live sender accidentally.
- Headless/detached modes must retain the same browser selection and lifecycle behavior.

## 2. Account profile, currencies, entitlements, achievements, and Hall of Legends

### Current behavior

Player identity, alliance identity, level, might, glory, gallantry, VIP, achievements, legend skills, account resources, currencies, and subscriptions are stored in the global account state. Hall of Legends has refresh, purchase, and reset operations.

### Target ownership

- `AccountProfile[WorldAccountKey]`: identity and durable player metrics.
- `AccountWallet[WorldAccountKey]`: currencies and truly account-wide resources.
- `AccountEntitlements[WorldAccountKey]`: subscriptions and feature unlock facts.
- `Achievements[WorldAccountKey]`: observed progress.
- `LegendSkills[WorldAccountKey]`: observed tree plus skill purchase/reset operations.

These are separate capability partitions because currency changes should not version achievements, and a legend refresh should not invalidate equipment planning unless the planner explicitly reads a derived effect.

### Feature impact

The account overview becomes usable without selecting or focusing a castle. Currency and subscription dependencies can be injected as narrow read projections into construction, crafting, events, or shop planners. Hall of Legends can expose its own freshness and operation history rather than borrowing the validity of all account state.

### Parity hazards

- Do not misclassify castle resources as account wallet balances.
- Preserve the exact refresh and purchase response barriers.
- Derived catalog effects must record the catalog version used.

## 3. Castle directory, kingdom membership, selection, and live focus

### Current behavior

Every `CastleState` contains identity, kingdom, slot, name, coordinates, a `Focused` flag, and most feature state. A castle snapshot flips focus across the castle map. The React UI has a castle focus switcher, while multiple reducers infer scope from the currently focused castle.

### Target ownership

- `CastleDirectory[WorldAccountKey]` owns discovered castle records and stable `CastleKey`s.
- `KingdomDirectory[WorldAccountKey]` owns known kingdoms and applicability metadata.
- `SessionProtocolContext[SessionID]` owns exactly one live focused castle and `FocusEpoch`.
- The client router owns the viewed account/kingdom/castle selection.

### Internal change

`Focused` is no longer duplicated as a Boolean inside every castle aggregate. An operation that needs game focus acquires a typed session-focus claim, focuses the required castle if necessary, observes the acknowledgement, and then plans/sends dependent commands against that focus epoch.

### User-visible effect

Navigation no longer causes an implicit live command unless a workflow actually needs focus. The header can display both states when they differ:

```text
Viewing: Everwinter Outpost — last observed 8m ago
Live focus: Main Castle — session generation 14
```

The castle directory can show a capability matrix for every castle, including slot-specific applicability such as Storm, Berimond, or kingdom transport.

### Parity hazards

- Do not assume every response includes a castle ID.
- Never resolve a focused response by looking up focus asynchronously after a later transition.
- Castle retirement must preserve history and local desired documents.
- A castle ID is only unique inside its account/world scope.

## 4. Buildings, layout, expansion, queue, decorations, and desired targets

### Current behavior

The building surface includes refresh, expansion, expansion gifts, construction, placement, movement, upgrade, free finish, time skip, store, demolition, decoration presets, layout collision rules, construction queues, and captured/desired building targets. Current implementation spreads parsing, state, planners, guards, intent steps, catalog reads, configuration, API projections, and UI across technical packages.

### Target ownership

- `Construction[CastleKey]` owns observed layout, expansions, building instances, build queue, and construction-slot state.
- `ConstructionTargets[CastleKey, DocumentID]` owns desired building/level/layout documents with schema/version and provenance.
- `DecorationPresets[WorldAccountKey, DocumentID]` owns reusable local presets; application records identify the destination castle.
- Official building and decoration definitions remain immutable `CatalogSnapshot` data.

### Operation dependencies and claims

A building operation declares the target castle, building instance or proposed footprint, relevant construction queue, account/castle resources, catalog version, and session focus. Typed claims replace ad hoc strings:

```text
SessionFocus(account)
ConstructionQueue(castle)
BuildingInstance(castle, instance)
LayoutFootprint(castle, cells)
ResourceBalance(castle, resource IDs)
```

The exact current command sequences and verification steps remain capability-owned. Placement, demolition, upgrade, expansion, gift collection, free completion, and time-skip completion still require their present authoritative evidence.

### User-visible effect

The castle screen becomes a projection of one selected `Construction[CastleKey]`, not a slice of a global object. It can show observed layout, desired target, drift, queue occupancy, stale dependencies, and current reconciliation operation together. Applying a decoration preset states its source document and destination castle explicitly.

### Parity hazards

- Building definition ID, building instance ID, decoration definition ID, and construction-item CID are distinct namespaces.
- Layout changes and queues from one combined castle snapshot must become atomically visible.
- Desired targets must not be overwritten when live state is absent or stale.
- A reconstruction engine must not silently make destructive choices that current behavior leaves manual.

## 5. Construction items, inventory, upgrades, and trivial shop purchases

### Current behavior

Construction-item slots are observed per building, while inventory and purchase data are account/shop related. Operations include inventory refresh, shop open, equip, upgrade, and purchase. The current repository correctly distinguishes TCI construction-item IDs from building/decor IDs and has special wire semantics for `rpc`, `ubc`, `gbc`, and `sbp`.

### Target ownership

- `Construction[CastleKey]` owns equipped slots by building instance.
- `ConstructionInventory[WorldAccountKey]` owns unequipped construction-item definitions/counts.
- `ConstructionCommerce[CastleKey or KingdomKey, ShopSessionID]` owns observed offers and their freshness.
- `ConstructionItemGoals[CastleKey, DocumentID]` owns AutoTCI desired configuration.
- The official construction-items and packages catalogs remain immutable, versioned inputs.

### Cross-partition behavior

An equip or upgrade reads account inventory, target castle/building slots, catalog definition/level, wallet/resources, and session focus. A purchase also reads the scoped live shop offer. The response change set can update both account inventory and castle slots before the operation succeeds.

### User-visible effect

The TCI picker can explain: item CID and catalog level, owned quantity, which castle/building slot is targeted, current equipped item, shop observation age, cost/reserve, and why the action is unavailable. AutoTCI becomes one goal/status per castle with an account-inventory contention view.

### Hard parity constraints

- `CID` in construction slots/settings/catalog is a construction-item definition ID and must never be interpreted through building or decoration catalogs.
- `RS` is remaining seconds; internal JSON remains `remainingSec`, optional where the wire omits it.
- `CID` in the outgoing `gbc` payload is the castle instance ID, not a construction-item ID.
- `ubc` may return `BCID` plus a full `CI` snapshot; both must commit atomically.
- Trivial shop PID/amount mapping comes from official package data plus the per-instance override in the application data directory.

## 6. Recruitment, tool production, hospital, and alliance help

### Current behavior

Recruit and tool policies inspect production queues and configuration, enqueue units/tools, and can request alliance help. Hospital operations heal or discard units. Settings support global item lists and per-castle behavior, schedules, queue slots, intervals, and related catalog/VIP constraints.

### Target ownership

- `Production[CastleKey, ProductionLineID]` owns observed active/queued items and queue capabilities.
- `Hospital[CastleKey]` owns wounded-unit queues and healing actions.
- `ProductionPolicy[PolicyID, CastleKey]` owns per-castle evaluation status and cursor.
- Reusable global defaults remain account policy templates; their resolved instances are castle-scoped.
- `AllianceHelp[WorldAccountKey]` owns help-request state/cadence when observable.

### Internal change

The policies no longer receive an entire `GameState`. Each evaluation declares the castle production lines, units/inventory, account entitlements/VIP, catalog, schedule, and operation status it needs. Global mode expands to explicit per-castle policy instances at configuration resolution time.

### User-visible effect

Automation can display a row per castle and production line: desired item/amount, queue state, next evaluation, schedule gate, missing building/unlock, pending alliance-help operation, and last outcome. One stale castle does not pause production elsewhere unless a shared account claim or user policy requires it.

### Parity hazards

- Preserve queue-slot and amount calculations, including catalog/VIP effects.
- Preserve the ordering between enqueue and alliance-help request.
- Hospital and production may share wire/focus context even though their domain states are separate.
- Changing global mode into per-castle instances must not change which castles are enabled.

## 7. Crafting, queue rental, skips, and Sceat/resource automation

### Current behavior

Crafting refresh/start/rent/skip operations combine castle crafting buildings, queue definitions, account currencies, recipes, time skips, and resource logistics. AutoSceat/resource behavior can ship resources from another castle or kingdom before continuing a recipe.

### Target ownership

- `Crafting[CastleKey, BuildingInstanceID]` owns observed queues and slot rentals.
- `CraftingPlans[CastleKey, BuildingInstanceID]` owns desired recipe sequences and cursors.
- `CraftingPolicy[PolicyID, CastleKey, BuildingInstanceID]` owns evaluation status.
- Economy, wallet, inventory, and logistics remain separate dependencies.

### Feature impact

The policy becomes an explicit saga/orchestrator over ordinary operations:

```text
evaluate recipe
  -> enough local resources: start crafting
  -> same kingdom source available: market shipment
  -> kingdom import available: kingdom shipment
  -> queue unavailable and allowed: rent slot
  -> active duration and reserve allow: skip
  -> otherwise: blocked with named dependency
```

This workflow state is not stored in the crafting aggregate as if it were game truth. Each child operation remains independently receipted and observed before reevaluation.

### User-visible effect

The crafting view can show the recipe cursor, source of required materials, shipment/skip reserve, queue rental, and the exact blocker. Multiple crafting buildings can be viewed and scheduled independently.

### Parity hazards

- Preserve source selection and reserve calculations.
- Do not treat a sent shipment as available resources before the authoritative state changes.
- Keep recipe definition IDs and resource/currency IDs typed.
- Do not retry rent, purchase, or skip operations blindly after an indeterminate disconnect.

## 8. Castle economy, food balance, market shipping, and kingdom logistics

### Current behavior

Castles carry resource balances/rates/capacities and food state. Market rows and caravan capacity support same-kingdom shipments. Kingdom transport supports cross-kingdom resources and troops with unlocks, time skips, and one-in-flight constraints. AutoFoodBalance chooses sources and destinations using safety hours and reserves.

### Target ownership

- `CastleEconomy[CastleKey]`: balances, rates, capacities, projected depletion.
- `CastleMarket[CastleKey]`: carriage capacity and current shipment context.
- `AccountCaravan[WorldAccountKey]`: shared caravan state where the game models it globally.
- `KingdomLogistics[WorldAccountKey, KingdomID]`: unlocks, active transfer, skips, destination rules.
- `FoodBalancePolicy[WorldAccountKey]`: account orchestrator over a configured set of source/destination castles.

### Internal change

Food balance is intentionally cross-castle, but it reads immutable economy summaries rather than owning or copying every castle. It chooses a proposal, submits one typed shipment, waits for the corresponding resource/logistics observations, and reevaluates. Claims name the source resource balance, destination capacity, market/caravan or kingdom lane, and any shared coin reserve.

### User-visible effect

An economy overview can group castles by kingdom and show food runway, safe source reserve, eligible route, shipment capacity, transfer in flight, and last observation. The user sees why a destination was selected and which shared lane prevents another transfer.

### Parity hazards

- Account currencies, castle resources, and special kingdom resources must not share an untyped balance map.
- Same-kingdom market shipping and cross-kingdom transport have different protocol/state rules.
- Source and destination balances may be observed at different times; the policy needs an explicit maximum-staleness rule.
- Preserve the one-in-flight and time-skip semantics before parallelizing evaluations.

## 9. Garrison, movements, stationing, recall, Bird, and Berimond transfer

### Current behavior

Castle state holds stationed/traveling/hospital units. The account has movement snapshots and stationing operations. Users can refresh movements, station troops, recall movements, and transfer troops between kingdoms. AutoBird and AutoStation inspect alliance/movement threats. AutoBeriWorld refreshes capacity and transfers troops.

### Target ownership

- `Garrison[CastleKey]` owns observed units at that castle by status.
- `MovementLedger[WorldAccountKey]` owns movements and indexes them by source castle, destination castle/target, kingdom, leader, direction, and status.
- `StationingWorkflow[OperationID]` owns an in-progress station/recall causal chain.
- `ThreatPolicy[PolicyID, ProtectedCastleKey]` owns Bird/Station evaluation status.
- `BeriCapacity[EventRunKey, CastleKey]` owns observed event capacity and consumption.

### Feature impact

Movements remain account-scoped because they connect castles and targets. They are not duplicated as mutable children of each castle. Castle and event pages consume indexed movement projections. Bird/Station policies declare protected castles and read only relevant incoming/outgoing movement and alliance facts. Berimond transfer composes source garrison, destination capacity, kingdom lane, and movement state.

### User-visible effect

The movement screen gains reliable filters and operation lineage. A castle shows troops physically present separately from troops traveling to or from it. Automation explains which threat, movement, capacity, or refresh is driving its decision.

### Parity hazards

- Movement acknowledgement is not always final arrival or final state.
- Preserve commander and troop reservation across concurrent attack/station operations.
- Do not count the same traveling units in both a source garrison and movement availability projection.
- Berimond capacity refresh and transfer confirmation remain distinct steps.

## 10. Leaders, equipment, gems, optimizer, and cleanup

### Current behavior

The product observes commanders, generals, castellans, equipment, gems, and loadouts. It supports refresh, equip/unequip, gem equip/unequip, swap, reconfigure, upgrade, sell, optimizer projections, and a client-side periodic equipment cleanup behavior.

### Target ownership

- `LeaderRoster[WorldAccountKey]` owns leader identities, availability, type, and castle assignments.
- `EquipmentInventory[WorldAccountKey]` owns item/gem instances.
- `LeaderLoadout[LeaderKey]` owns observed equipment and gems for one leader.
- `EquipmentOptimizer` is a pure query/planning service over an immutable account read view and catalog version.
- `EquipmentCleanupSettings` is desired state; any automated cleanup runtime must be a separately approved account policy.

### Cross-scope behavior

Availability derives from the movement ledger and live operations, while castellan assignment references a castle. Equipment mutations may atomically change inventory and one or more loadouts. Claims are instance-specific, such as an equipment item, gem, source/target leader, and wallet/resource cost.

### User-visible effect

Equipment can show whether an item is owned, equipped, reserved by a pending operation, recommended by an optimizer result, or unavailable because the leader is moving. Optimizer output records the input partition versions and catalog version so a stale recommendation can be identified before apply.

### Deliberate product decision

Moving periodic cleanup from a mounted React hook to a durable server policy would improve detached operation, but it can cause effects when the UI is closed. That is a behavior change, not automatic parity work. The rewrite should first preserve the current reachability/lifecycle, then promote it only with explicit user-facing enablement, schedules, receipts, and safeguards.

### Parity hazards

- Equipment definition ID and equipment instance ID are distinct; the same applies to gems.
- A swap/reconfigure may touch multiple leaders and inventory atomically.
- Never sell or upgrade from an optimizer recommendation whose declared dependencies changed.
- Preserve current cleanup filters, timing, and UI-lifecycle semantics until a product decision changes them.

## 11. Castle defense, presets, open gate, and Khan protection

### Current behavior

Defense state includes wall, keep, moat, tools, open gate, and castellan-related information. Operations refresh defense and update wall/keep/moat, apply a defense preset, replenish tools, and perform Khan-specific open-gate/point-limit protection and recovery.

### Target ownership

- `CastleDefense[CastleKey]` owns observed defense and open-gate state.
- `DefensePresets[WorldAccountKey, DocumentID]` owns local desired documents.
- `LeaderLoadout[CastellanKey]` remains equipment-owned and is referenced by defense.
- `DefenseRecoveryWorkflow[OperationID]` owns multi-step temporary changes and restoration.

### Feature impact

Applying a preset becomes a composed request with explicit wall/keep/moat child effects and an authoritative completion projection. Khan protection can temporarily acquire the castle-defense claim, save the required restoration inputs, apply protection, verify it, and later restore using the same operation lineage.

### User-visible effect

Each castle gets a defense health card: last refresh, castellan, setup drift from a selected preset, tool sufficiency, open-gate state, active protection workflow, and restoration status. A blocked restoration is visible as a high-severity operation, not hidden in event state.

### Parity hazards

- A response fragment can depend on live castle focus.
- Preserve the exact wall/keep/moat ordering and verification barriers.
- Never infer successful defense or tool purchase from command send alone.
- Protection workflows must survive restart or explicitly reconcile indeterminate state before another change.

## 12. Attacks, game presets, CitadelOps presets, spying, and Rift

### Current behavior

Attack setup uses source castle troops/tools, leaders, movement capacity, target context, attack dialog, game-saved presets, CitadelOps configuration presets, travel boosts, and event rules. Spy launch/fetch/share and Rift maiden/replay/template operations add their own workflows.

### Target ownership

- `CombatContext[SessionID]` owns live attack dialog and game-saved preset observations.
- `AttackPresets[WorldAccountKey, DocumentID]` owns richer local CitadelOps preset documents.
- `AttackOperation[OperationID]` owns one planned launch and causal outcome.
- `SpyReports` and `BattleReports` own normalized report results.
- `RiftTemplates[WorldAccountKey, DocumentID]` owns captured local templates and communication settings.
- `RiftRun[EventRunKey]` owns launches/status where observed.

### Internal change

Attack planners receive a consistent read view containing source garrison, selected leader/loadout, movement ledger, target observation, attack contexts/presets, inventory/resources, catalog, and session focus. They return a typed plan with explicit dependencies and claims. The execution kernel handles account ordering and response correlation; the combat capability owns eligibility, payload meaning, and success evidence.

### User-visible effect

An attack is a durable operation page rather than only modal state: source castle, target, preset version, formation summary, reserved commanders/troops/tools, focus transition, sent commands, movement acknowledgement, and final status. Rift replays identify the captured template/version and source scope.

### Parity hazards

- Keep game-saved `AttackPresets` distinct from local `attacks.presets`; similarly distinguish local Rift templates from observed game state.
- A successful send or generic acknowledgement is not necessarily an observed launch.
- Preserve attack pacing/admission priorities shared across Towers, Storm, Rift, and manual attacks.
- A template captured under one catalog/protocol version may require validation before replay.

## 13. Alliance, world map, player tracking, and target intelligence

### Current behavior

The app refreshes and inspects alliances, queries map areas, stores observed targets and alliance holdings, tracks players/alliances, and presents alliance targets. Map records also feed event automations.

### Target ownership

- `AllianceDirectory[WorldKey or WorldAccountKey]` owns the current alliance relationship and inspected alliance snapshots with provenance.
- `WorldMap[KingdomKey]` owns target observations indexed by coordinate, object ID/type, owner, event type, and observation time.
- `PlayerIntelligence[WorldKey, PlayerID]` and `AllianceIntelligence[WorldKey, AllianceID]` own normalized historical projections.
- `TargetLists[WorldAccountKey, DocumentID]` owns user-curated local lists.

### Feature impact

Map query results no longer expand a single account aggregate indefinitely. Each observation carries viewport, kingdom, time, source operation, and expiry policy. Event modules reference target keys and may maintain event-specific derived status, but they do not own a duplicate mutable map.

### User-visible effect

Player Tracker and Alliance Targets can expose observation provenance and freshness. Kingdom selection becomes explicit. A target row can link to reports, movements, event workflow, and operations without making those records children of one map object.

### Parity hazards

- Coordinate alone may be insufficient across kingdoms/worlds; use composite target keys.
- Map absence in a later partial viewport is not proof that an object vanished.
- Alliance inspection must not mutate the player’s own alliance membership incorrectly.
- Retention and privacy policies are required before expanding historical intelligence.

## 14. Towers

### Current behavior

Tower automation scans map context, captures a batch/queue, refreshes target context, selects a target, launches an attack, tracks cooldowns, and uses later map/report evidence. Configuration is per castle with shared intervals and attack priority.

### Target ownership

- `TowerTarget[TargetKey]` owns observed level/cooldown metadata.
- `TowerQueue[PolicyID, CastleKey]` owns the policy’s selected target sequence and scan lineage.
- `TowerPolicy[PolicyID, CastleKey]` owns status, schedule, next evaluation, and metrics.
- Combat, movements, map, leaders, and presets remain dependencies.

### Feature impact

One policy instance exists for each configured source castle. It can run independently when its own map/garrison data is fresh, subject to shared attack admission, commander, and focus claims. Queue changes are desired/runtime state, not edits to the kingdom map aggregate.

### User-visible effect

The Towers view can show per-castle scan age, queue, current target, cooldown proof, commanders, attack admission position, and last victory/refresh evidence. A stale map in one kingdom does not label all Towers state stale.

### Parity hazards

- Battle acknowledgement alone does not prove the tower cooldown advanced.
- Preserve batch capture and target-consumption ordering.
- Shared commanders and attack pacing prevent unsafe parallel launches.
- A target observation must be tied to the correct kingdom and scan.

## 15. Invasion

### Current behavior

Auto Invasion selects an active event/difficulty, observes score and remaining time, scans around a source castle, chooses eligible invasion castles, validates commanders/preset/currency, and attacks until a score or time boundary.

### Target ownership

- `InvasionRun[EventRunKey]` owns event identity, difficulty, score target/progress, and time boundary.
- `InvasionScan[EventRunKey, CastleKey]` owns scan bounds, lineage, and eligible target projection.
- `InvasionPolicy[EventRunKey, CastleKey]` owns execution status.

### Feature impact

Account/event facts such as event score are separated from the castle-scoped scan and attack source. Difficulty selection is an event-run operation; map scan and attack are source-castle operations. Shared combat primitives remain reusable.

### User-visible effect

The event page shows event status and target score once, with one or more source-castle execution cards. Each card explains map age, eligible targets, preset, commanders, fortification currency, and attack result.

### Parity hazards

- Preserve Foreign Lords/Bloodcrow difficulty mapping and eligibility.
- Do not consume a target until the same evidence current behavior requires.
- The event remaining-time guard must use authoritative time/freshness.
- Difficulty/score are not necessarily castle-scoped even when attacks are.

## 16. Nomad, Samurai, and RBC trial behavior

### Current behavior

Auto Nomad selects event difficulty, scans four camps around a source castle, levels/selects/locks a camp, manages cooldowns and time-skip reserves, chains attacks, tracks authoritative arrival/victory order, and has an opportunistic RBC trial workflow.

### Target ownership

- `NomadRun[EventRunKey]` owns active event, difficulty, score, target, and time boundary.
- `NomadCamp[TargetKey]` owns the latest observed camp/cooldown state.
- `NomadSelection[EventRunKey, CastleKey]` owns scan set, leveling progress, locked target, and lineage.
- `NomadPolicy[EventRunKey, CastleKey]` owns the policy state machine.
- `RbcTrialRun[RunID, CastleKey, TargetKey]` is a distinct workflow, not an incidental flag in map state.

### Feature impact

The current complex policy becomes easier to audit because each transition names its observation and operation evidence. It still composes ordinary map query, attack, cooldown-skip, movement, commander, inventory, score, and difficulty operations.

### User-visible effect

The event card can show the four scanned camps, maximum/actual victory count, locked camp, cooldown, reserved skips, chain arrivals, score, next transition, and RBC trial separately. This exposes why the policy is waiting instead of presenting only a global automation status.

### Parity hazards

- Preserve the exact camp-count, level/weakest selection, locking, and chain semantics.
- A camp refreshed from the wrong scan/run must not satisfy a transition.
- Time-skip reserve and ruby/coin safeguards require account-inventory claims.
- Arrival order and confirmed victory evidence are causal, not merely timestamp comparisons.

## 17. Khan

### Current behavior

Auto Khan combines event score, map/camp attacks, commander assignment, cooldown skips, defense refresh/tool replenishment, open gate, offensive-unit threshold, point-limit protection, and restoration/soft-lock behavior.

### Target ownership

- `KhanRun[EventRunKey]` owns event and score/protection state.
- `KhanPolicy[EventRunKey, SourceCastleKey]` owns attack progression.
- `KhanProtectionWorkflow[EventRunKey, ProtectedCastleKey]` owns temporary defense/open-gate changes and restoration status.
- Combat, CastleDefense, Garrison, Map, Movement, and inventory remain separate owners.

### Feature impact

Khan is modeled as an orchestrator with two explicit lanes—attack and protection—coordinated through typed claims. It does not absorb defense or attack internals. A soft lock becomes a visible admission/policy condition with a reason and recovery action.

### User-visible effect

Users see source castle, protected castle, point/offensive-unit thresholds, active protection, tool reserve, open gate, attack target, and restoration operation. High-risk temporary state is visible even after restart.

### Parity hazards

- Protection must not race manual defense changes or another event policy.
- Preserve which castle is protected versus which castle launches attacks.
- Replenishment purchases and time skips are non-idempotent.
- Restoration must reconcile actual live defense before applying saved desired values.

## 18. Storm

### Current behavior

Auto Storm is the broadest current policy. It can reconcile Storm construction, apply a decoration preset, transport resources/troops from donor castles, build/upgrade harbor, scan and attack forts/islands, retain defense units, and spend Aquamarine at the event shop under reserves and ordering preferences.

### Target ownership

- `StormRun[EventRunKey, KingdomKey]` owns event identity and Storm-wide status.
- `StormPolicy[EventRunKey, StormCastleKey]` owns orchestration status and settings resolution.
- Storm castle construction/economy/garrison/defense remain ordinary castle capability instances.
- Donor castles remain normal economy/garrison instances.
- `StormTargets[EventRunKey, KingdomKey]` derives from scoped map observations.
- `StormShop[EventRunKey, CastleKey]` owns observed offers/stock; purchase goals remain desired documents.

### Feature impact

Storm becomes the strongest proof of the architecture. It composes normal capabilities rather than creating a parallel “Storm version” of buildings, logistics, attacks, inventory, and shops:

```text
Storm policy evaluates an immutable scoped read view
  -> submits one Construction, Logistics, Decoration, Attack, or Shop request
  -> child capability executes and commits authoritative outcome
  -> Storm policy reevaluates from changed partitions
```

### User-visible effect

The Storm dashboard shows phases and dependencies: castle readiness, harbor target, resource deficits and donors, troop imports, decoration target, map scan, forts/islands, Aquamarine reserve, shop goals, active child operation, and next action. Users can drill into the same construction or movement view used for normal castles.

### Parity hazards

- Storm kingdom/castle identity must be explicit; special resources must not leak into generic account balances.
- Donor selection and reserves span several castles and need a consistent read view.
- Only one child effect should be admitted where current policy expects serial reevaluation.
- Shop stock and target scans are time-sensitive observations.
- “More parallel because state is partitioned” would be unsafe where focus, commanders, attack pacing, transport lanes, or reserves are shared.

## 19. Reports, battle statistics, player tracking, and history

### Current behavior

The app fetches/shares spy reports, fetches battle summary/details, captures notices and multi-part reports, stores history, and presents battle statistics, player tracking, and alliance target views. Persistence is split across JSON/JSONL/files and live state.

### Target ownership

- `ReportInbox[SessionID]` owns transient notices and capture assembly.
- `ReportStore[WorldAccountKey, ReportID]` owns normalized durable reports with source lineage.
- `BattleHistory`, `PlayerHistory`, and `AllianceHistory` are indexed read projections, rebuildable from normalized reports/observations where possible.
- Raw capture segments live in bounded diagnostic storage with retention/redaction metadata.

### Feature impact

Reports stop being mixed transient/global state. Capture assembly is generation-fenced; normalization commits a durable record; analytics projections update independently and can be recomputed under a named calculation version. A report can link to the originating target, movement, operation, and catalog version without copying those objects.

### User-visible effect

History becomes filterable by account, kingdom, castle, target, player, alliance, event, and time. A statistic can expose its calculation version and source coverage. Missing details or stale intelligence are explicit.

### Parity hazards

- Do not mark a report complete until all required parts are captured.
- Historical reprocessing must never emit live commands.
- Raw frames may contain sensitive account/session/player data and need redaction/retention.
- Existing exported formats and identifiers require a compatibility decision.

## 20. Scheduler, automation coordinator, commander assignments, and operation center

### Current behavior

The automation coordinator evaluates global policies from state-domain notifications. Configuration includes enabled flags, weekly schedules, attack priorities, delays, commander-feature assignments, global/per-castle settings, and feature-specific documents. Scheduled operations and automation statuses also live in or beside global state.

### Target ownership

- `PolicyDefinition` describes a feature policy and supported scope type.
- `PolicyConfiguration[WorldAccountKey, PolicyID]` stores schema-versioned account defaults.
- `PolicyInstance[PolicyID, ScopeKey]` stores resolved enablement, schedule, status, next check, dependencies, and metrics.
- `CommanderAssignments[WorldAccountKey]` is desired state with references to policy instances.
- `ScheduledRequest[WorldAccountKey, ScheduleID]` stores a typed request, scope, schema, time zone, and admission class.
- `OperationStore[WorldAccountKey]` owns durable receipts and transitions.

### Intent-engine impact

The generic engine becomes a smaller execution kernel. It owns:

- account-ordered admission and bounded queues;
- typed claim acquisition in deterministic order;
- pacing and attack-priority classes;
- cancellation and generation fencing;
- step dispatch through declared effect ports;
- response/observation correlation;
- durable receipts and indeterminate outcomes.

It does **not** own game feature planning semantics. A capability planner declares a typed request schema, read-set, plan, claims, and completion evidence. Policies call the same typed requests as the UI.

### User-visible effect

The Automation view becomes a scope-aware control center rather than a list of mostly global toggles. Each instance shows enabled/scheduled/paused state, current scope, last decision, exact blocker, next check, active operation, last receipt, and freshness requirements. The operation center can filter by capability, account, kingdom, castle, event, policy, status, and risk.

### Parity hazards

- Configuration migration must preserve global versus per-castle resolution and existing defaults.
- Attack priorities and delays are shared account admission rules, not settings copied into each event module.
- Commander assignments must reserve actual leaders against movements and concurrent policies.
- The rewrite must not awaken dormant policies merely because their modules register.
- Scheduled raw JSON should migrate to versioned typed requests without changing due-time/time-zone behavior.

## 21. Shops, packages, currencies, and purchases

### Current behavior

Construction and Storm include shop operations; package/catalog data supplies definitions and mappings. Purchases can consume account or event currencies, have live stock/context, and may be non-idempotent.

### Target ownership

- Each domain owns its typed commerce session, such as `ConstructionCommerce` or `StormShop`.
- `AccountWallet` and relevant castle/event resource capabilities own balances.
- Official package definitions live in the catalog snapshot.
- `PurchaseOperation` records offer identity, quantity, price snapshot, reserve decision, send/correlation, and authoritative result.

### Feature impact

There should not be one generic shop aggregate that erases domain semantics. Shared purchase execution primitives can exist, but construction CID mapping, Storm Aquamarine stock, and future event stores retain their own typed offer schemas and validation.

### User-visible effect

A purchase confirmation/receipt can state which live offer was observed, its age, price and balance before planning, reserve applied, and resulting observation. An indeterminate purchase after disconnect is visible and reconciled before retry.

### Parity hazards

- Preserve domain-specific ID namespaces and wire payload fields.
- Never retry a non-idempotent purchase merely because no response arrived.
- Offer stock/context can expire even if the catalog definition remains valid.
- Optional local overrides must remain in the instance data directory, never in official `Server/Data` mirrors.

## 22. Official game data, application updates, settings, diagnostics, and support

### Current behavior

The application discovers and downloads official game data, maintains cache/digest/version metadata and offline fallback, exposes update check/install, persists configuration sections, records telemetry/protocol observations, supports replay, and provides settings/patch-notes/support surfaces.

### Target ownership

- `CatalogManager[Application]` owns immutable, versioned game-data snapshots and activation.
- `ConfigurationStore[Profile or WorldAccountKey]` owns schema-versioned desired documents; each schema declares its scope.
- `ApplicationUpdate[Application]` owns release check/install state.
- `Telemetry[Session/Account]` owns bounded metrics/traces and capability degradation records.
- `CaptureStore[Session]` owns bounded raw evidence and redaction metadata.
- `SupportExport[Profile]` assembles a user-previewed, redacted manifest from selected records.

### Feature impact

Operations and derived projections record which catalog snapshot they used. A catalog refresh activates a coherent snapshot and invalidates only derived views whose definitions changed. Configuration validation is capability-owned and can migrate one document independently. Diagnostics refer to scopes and causal IDs without becoming domain state.

### User-visible effect

Settings can distinguish application, profile, account, kingdom, castle, and policy-instance documents. A protocol-resilience dashboard can show which capabilities are degraded, the first failing observation, last good version, and safe refresh action. Support export previews sensitive categories and redaction before writing a bundle.

### Parity hazards

- Preserve runtime official-data download, digest, cache, localization, and offline behavior.
- `Server/Data` and shipped copies remain official game JSON only; Citadel config/overrides stay in the per-instance data directory.
- A catalog activation during an operation must not change that operation’s meanings midway.
- Routine logs and exports must not expose raw tokens, sessions, or private payloads by default.

## Cross-feature product changes

### Scope navigator

The primary navigation should carry a routeable scope:

```text
/accounts/{account}/overview
/accounts/{account}/castles/{castle}/construction
/accounts/{account}/kingdoms/{kingdom}/map
/accounts/{account}/events/{eventRun}/storm
/accounts/{account}/operations/{operation}
```

Changing the route changes the viewed scope. It does not necessarily issue a live castle-focus command.

### Capability coverage matrix

Every discovered castle and kingdom receives registered capability-instance statuses. The overview answers:

- Does this feature apply here?
- Has it ever been observed?
- How fresh is it?
- Is it healthy, degraded, or blocked?
- What operation or automation is active?
- What observation or action would restore it?

This is the practical mechanism for having “some idea for all features” without loading every feature payload.

### Operation center

Manual, scheduled, and automated requests use one receipt model. A receipt should expose:

- request and schema version;
- initiating user/policy/schedule;
- account/kingdom/castle/target scopes;
- planning read-set and versions;
- claims and admission class;
- steps sent and correlated evidence;
- outcome: succeeded, rejected, cancelled, failed, or indeterminate;
- child/parent operation links;
- safe recovery or refresh recommendation.

### Desired-state documents

Building targets, presets, Rift templates, automation settings, schedules, reserves, and commander assignments are durable product data. They must be independently exportable, migratable, and recoverable. They are not deleted because a castle is temporarily missing, a parser breaks, or a live baseline is unavailable.

### Native queries and compatibility

The native client consumes narrow capability queries and scoped invalidations. API v2 continues to compose the existing `GameState` and global revision while parity clients remain. The compatibility layer is deliberately allowed to be less efficient; it must not dictate the new internal model.

## What must remain unchanged versus what may improve

| Concern | Preserve exactly during parity | Safe architectural/product improvement |
|---|---|---|
| Protocol | Payload meaning, ordering, required focus, pacing, correlation, completion evidence | Typed facts, scope routing, decoder isolation, better diagnostics |
| Manual operations | Eligibility and game effects | Better explanation, durable receipts, deep links, scoped refresh |
| Automation | Current enablement, selection rules, reserves, timing, and effects | Per-scope status, explicit dependencies, isolation, restart visibility |
| Presets/settings | Values, identifiers, migration, application behavior | Versioned documents, scope labels, export/import, validation |
| State | Material observed meaning and atomic multi-domain commits | Partition versions, freshness, narrow queries, capability health |
| UI | Reachable workflows and actions | Router, scope navigator, task projections, operation center |
| Persistence | User data and recoverable history | Transactional metadata, independent schemas, bounded captures |
| Security | Existing local usability | Per-launch local credential, origin/host policy, redacted diagnostics |

## Features needing an explicit product decision before migration

The following cannot be answered by architecture alone:

1. Whether client-lifecycle equipment cleanup becomes a durable server automation.
2. Whether currently registered but unreachable/incomplete intents should become first-class UI features.
3. Whether multiple live accounts are a launch requirement or only an extraction seam.
4. Whether remote/mobile access is in scope; it requires a distinct security mode.
5. Which raw captures and historical intelligence are retained, for how long, and how users delete/export them.
6. Whether desired-state reconciliation may make destructive changes automatically or only propose them.
7. Whether an event policy may use multiple source castles concurrently once typed claims exist.
8. Which legacy configuration/export/API contracts must remain externally compatible and for how long.

Until decided, the migration should preserve current reachability and behavior, record dormant capabilities in the parity catalog, and avoid silently enabling new effects.

## Repository evidence index

The following reference block records the principal current-code ranges used for this impact analysis. Line numbers describe the 2026-07-15 working tree.

```text
startLine: 37
endLine: 942
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/State/Models.go
purpose: current session, account, castle, capability, event, report, automation, and global state families

startLine: 5
endLine: 134
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Ingest/CoreReducers.go
purpose: current inbound and outbound protocol reducer surface

startLine: 17
endLine: 173
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Ingest/AccountReducers.go
purpose: multi-domain authoritative account baseline

startLine: 147
endLine: 315
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Ingest/CastleReducers.go
purpose: castle snapshot, focus, buildings, queues, TCI, units, food, player, and alliance coupling

startLine: 23
endLine: 111
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Intent/Types.go
purpose: current global planning context, request revision, claims, plan, step, and completion barrier model

startLine: 179
endLine: 830
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Intent/Engine.go
purpose: current intent admission, revalidation, dispatch, correlation, commit, and receipt behavior

startLine: 151
endLine: 166
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/App/Application.go
purpose: registered automation policies

startLine: 323
endLine: 420
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/App/Application.go
purpose: application/session/configuration/catalog/update/scheduling intent actions and definitions

startLine: 523
endLine: 535
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/App/Application.go
purpose: scheduler, automation, event, commander-assignment, food-balance, and Rift defaults

startLine: 10
endLine: 54
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/App/Parity_test.go
purpose: existing legacy intent and automation parity manifest

startLine: 22
endLine: 31
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Configuration/LegacyMigration.go
purpose: legacy scheduler, production, hospital, Sceat, TCI, Bird, Station, Berimond, and Rift configuration migration

startLine: 36
endLine: 176
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Client/src/App.tsx
purpose: current top-level screen selection, modal ownership, and client-lifecycle equipment cleanup

startLine: 32
endLine: 169
filePath: /Users/nebulabot/Desktop/CitadelOpsDesktop/Client/src/api/ApiContext.tsx
purpose: current global state context, full-state invalidation/refetch, and polling
```

## Conclusion

The target architecture changes features primarily by making their true scope and dependencies explicit. Castle-local features become independently fresh and operable; kingdom features gain correct target/map boundaries; account-wide inventories, leaders, movements, and currencies remain shared; session focus remains serialized; and event automations become orchestrators over normal capabilities.

That is a meaningful product improvement without changing the game-facing behavior. Users gain a coherent answer to “what does CitadelOps know, for which castle or kingdom, how fresh is it, what is it doing, and why?”—the information that the current global state and UI cannot reliably provide.

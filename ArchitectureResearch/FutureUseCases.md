# Future use cases and quality scenarios

## Why future use cases matter here

Architecture should not be justified by a vague claim that CitadelOps may “scale.” Different futures exert different pressure:

- more game features demand modularity and protocol resilience;
- more historical analysis demands a journal and indexed projections;
- more concurrent accounts demand partitioning and perhaps process isolation;
- remote control demands identity and authorization;
- extensions demand a stable capability boundary and possibly a sandbox.

This document distinguishes observed product behavior, repository-stated direction, strong adjacent opportunities, and speculative hypotheses. Only the first category is a parity requirement.

## Category 1: current behavior that a rewrite must preserve

### Browser-backed account control

The app owns a Chromium profile/session, observes the game’s WebSocket traffic, sends commands through the game page, distinguishes dashboard and game connectivity, and remains useful against last-known data while disconnected.

Architectural pressure: replaceable transport, explicit session generation, authoritative login baseline, offline projections, and secrets/profile lifecycle.

### Interactive operations with safety guarantees

Users can initiate construction, layout, shop, equipment, movement, defense, event, report, configuration, and lifecycle operations. Destructive or stale-sensitive paths validate current state, claim resources, apply pacing, correlate responses, and expose receipts/cancellation.

Architectural pressure: one typed command plane, fresh-state planning, claims, idempotency policy, outcome barriers, and task-shaped UI feedback.

### Server-owned automation and scheduling

The product runs policies for recruitment, tools, hospital, TCI, crafting resources, Bird, Station, Beri, food balance, Towers, Invasion, Nomad/Samurai, Khan, and Storm, plus reusable weekly schedules and commander assignments. One equipment-cleanup policy currently remains UI-owned and should become a normal server capability if parity includes headless behavior.

Architectural pressure: deterministic clocks, wake-up scheduling, policy explanation, shared intent safety, resource arbitration, and configuration migrations.

### Presets and captured templates

Attack and defense presets, decoration/castle layouts, Rift formations, automation presets, and target states turn live game structures into reusable local desired state.

Architectural pressure: versioned user documents distinct from observed game state, compatibility validation, previews, and reversible schema migration.

### Intelligence, history, and reporting

The app tracks alliance targets, spy and battle reports, movements, player/troop/currency history, automation/event state, and diagnostics. Some UI surfaces are incomplete or unreachable, but the underlying product direction is clearly broader than command execution.

Architectural pressure: indexed history, read projections, retention, freshness, report parser versioning, and privacy-aware export.

### Runtime official catalogs

The app fetches, caches, localizes, and projects official game data and uses it to validate IDs, resolve levels, plan commands, and render the UI.

Architectural pressure: immutable catalog versions, atomic refresh, offline fallback, bounded cache, and recording which version informed an operation.

### Replay and support diagnostics

Recorded traffic can be streamed through the ingest path. Raw protocol observations, telemetry, history, and operation receipts support reverse engineering and issue diagnosis.

Architectural pressure: deterministic time, safe no-effect replay, decoder versioning, bounded capture storage, redaction/encryption, and causal correlation.

## Category 2: direction already present in code or repository intent

These are not fully implemented parity requirements, but the existing architecture or adapters intentionally point toward them.

### Headless and detached operation

The executable already has headless/browser flags, the CLI uses the API, and the architecture describes a detached API surface. A secure headless daemon could keep automation running without the main dashboard open.

Needed design:

- separate UI lifecycle from account-runtime lifecycle;
- local service identity and explicit startup policy;
- task-oriented query/command contracts;
- notification and approval adapters;
- platform-appropriate data and secret locations.

### Alternative game transports

The current session port supports Chromium and replay; repository intent mentions a future direct WebSocket adapter. A direct adapter could reduce browser overhead but would inherit authentication, protocol, anti-abuse, and compatibility responsibility.

Needed design: keep transport assembly, authentication, protocol decoding, and capability facts separate. A direct adapter must pass exactly the same conformance suite before it can replace browser-backed behavior.

### Tool and LLM adapters

Stable named operations and dry runs are a natural foundation for constrained automation tools or an LLM assistant. This should expose high-level typed capability requests, previews, receipts, and approvals—not raw opcodes or an unrestricted intent console.

Needed design:

- scoped capabilities and per-operation authorization;
- schema-generated tools with bounded arguments;
- mandatory preview/approval classes for destructive actions;
- clear provenance, audit, quotas, and revocation;
- prompt-independent server validation.

### A first-class replay/protocol workbench

Replay currently supports development. It could become a product-facing capture inspector that compares decoder versions, visualizes state evolution, explains unknown opcodes, and produces redacted support bundles.

Needed design: selective journal, capture indexes, decoder/catalog version metadata, deterministic projections, secure export, and explicit no-effects mode.

## Category 3: strong adjacent product opportunities

These opportunities follow from existing data and workflows, but should be validated with users before they drive expensive topology choices.

### Task-oriented operations center

Receipts already exist but the client does not present a general operation center. A unified surface could show active, blocked, scheduled, failed, cancelled, ambiguous, and completed work; explain claims and dependencies; and offer safe retry or refresh actions.

Architectural pressure: durable receipts, stable operation links, typed error/recovery codes, causal explanations, and query subscriptions.

### Desired-state reconciliation

Construction/layout targets, defense presets, equipment targets, commander assignments, and schedules all hint at a higher-level workflow: declare a desired state, preview the gap, approve a plan, and reconcile incrementally.

Architectural pressure: local desired-state documents, dependency graphs, explainable planners, partial progress, resource budgets, pause/cancel, and authoritative refresh. This should be implemented per capability before attempting a universal workflow language.

### Rich indexed analytics

Current JSONL history and large client-side aggregation can evolve into searchable battle/player/resource/event/automation analytics, comparisons over time, and “what changed?” explanations.

Architectural pressure: indexed projections, retention tiers, source/version lineage, background recomputation, and export. Option B becomes more attractive if historical reprojection is core rather than incidental.

### Protocol-change resilience dashboard

Unknown/failed messages, reducer changes, stale projections, command outcome rates, and catalog drift can be surfaced as health indicators. This reduces silent breakage when the game changes.

Architectural pressure: normalized telemetry, compatibility probes, capture sampling, failure budgets, decoder versioning, and safe degradation by capability.

### Native desktop shell and deep linking

The current UI is a SPA served into a controlled Chromium window, has no URL router, and centralizes modal state. A native shell or purpose-built webview can add platform notifications, secure secret integration, file export, single-instance/deep links, and more deliberate navigation while retaining the same API.

Architectural pressure: UI-independent application contracts, explicit local authentication, routeable feature modules, operation links, and narrow shell bridges.

### Capability-level degradation

If a game update breaks one event parser, unrelated construction, equipment, and history features should continue. Users should see that the affected capability is stale or disabled and why.

Architectural pressure: capability-owned health/freshness, dependency graph, circuit breakers around effects, and no global “state valid” boolean.

## Category 4: plausible but uncommitted hypotheses

The architecture should avoid blocking these futures, but the product should not pay their full cost today.

### Concurrent multi-account control

One user may want multiple accounts or servers active simultaneously, with isolated browser profiles and schedules.

Design now: make the pre-login `ProfileID` and post-baseline `GameAccountID` first-class identities with an explicit binding, remove singleton session/state assumptions, scope claims and storage, and define per-account quotas.

Defer now: worker processes, fleet scheduling, and cross-account commands until there is a measured concurrency requirement.

### Remote or mobile monitoring and approval

A user may want status, alerts, and approval of queued high-impact actions from another device.

Design now: versioned query/command contracts and durable receipts.

Defer now: Internet exposure. A remote mode needs explicit authentication, authorization, TLS, revocation, rate limits, secure enrollment, and threat modeling; changing the bind address is not a product design.

### Cross-account resource and commander orchestration

A future fleet view might schedule work across accounts, balance resource budgets, or avoid simultaneous event conflicts.

Design now: account-scoped commands and observable capacity.

Defer now: a global optimizer. It introduces fairness, partial failure, policy conflicts, and potentially consequential automated behavior.

### Alliance collaboration

Users could share redacted intelligence, targets, reports, or coordination plans with trusted alliance members.

Design now: provenance and exportable typed reports.

Defer now: a hosted collaboration service, account linking, access control, moderation, data residency, and conflict resolution.

### Third-party capability ecosystem

Advanced users may eventually want custom reports, read-only projections, policies, or game-event modules.

Design now: capability descriptors, schemas, narrow ports, and explicit permission classes.

Defer now: dynamic plugin loading. Trusted compiled-in modules are simpler. If a real ecosystem emerges, use versioned subprocess RPC for trusted extensions or evaluate a capability-oriented WASI sandbox for untrusted code. Native Go plugins are not an appropriate portable boundary.

### Simulation and what-if planning

Captured observations and official catalogs could drive offline combat/resource/building simulations, policy comparisons, or automation dry runs.

Design now: deterministic clock, effect-free planner, catalog version, and journal lineage.

Defer now: claiming predictive accuracy. Remote server rules and hidden state may make simulations advisory rather than authoritative.

## Quality-attribute scenarios

These scenarios turn broad goals into architecture tests. They follow the scenario-oriented evaluation principle in the SEI’s [ATAM material](https://www.sei.cmu.edu/library/architecture-tradeoff-analysis-method-collection/).

### Q1. Wire parity for a destructive action

- **Stimulus:** A user or automation requests an existing purchase, upgrade, layout, equipment, defense, or attack action.
- **Environment:** Live connected account with the same observed state and settings as the baseline system.
- **Response:** The replacement makes the same admission decision, emits semantically identical wire commands in the same required order/pacing, applies the same response interpretation, and exposes an equivalent receipt.
- **Measure:** No unexplained command, state, or outcome difference across golden transcripts; destructive commands require an approved parity fixture.

### Q2. Game protocol adds or changes a message

- **Stimulus:** A capture contains an unknown opcode, extra field, missing optional field, or changed response payload.
- **Environment:** Normal live or replay operation.
- **Response:** The transport remains healthy, the observation is retained within policy, only dependent capabilities become stale/degraded, and a new decoder can be added and replayed without altering unrelated modules.
- **Measure:** No process crash or unrelated state corruption; change is localized to the owning capability and contract fixtures.

### Q3. High-frequency protocol traffic

- **Stimulus:** A burst reaches or exceeds observed peak frame rate while several UI projections are subscribed.
- **Environment:** Typical desktop hardware and a large last-known account state.
- **Response:** Per-account ordering is preserved, queues remain bounded, safety-critical input is not silently lost, UI updates only affected projections, and persistence checkpoints make progress during sustained traffic.
- **Measure:** Define p95 observation-to-projection latency, maximum memory/queue size, checkpoint age, and UI frame budget from captured production-like traces before implementation.

### Q4. Disconnect during a non-idempotent effect

- **Stimulus:** Chromium/socket disconnects after a purchase or attack may have been sent but before its correlated outcome is committed.
- **Environment:** Live operation.
- **Response:** The old generation is fenced, the receipt becomes indeterminate or awaits authoritative reconciliation, and the effect is not blindly retried.
- **Measure:** No duplicate non-idempotent action; recovery explanation links send attempt, disconnect, and later refresh.

### Q5. Add a new event automation

- **Stimulus:** A developer adds protocol state, settings, UI, planning, and a policy for a new event family.
- **Environment:** Normal development.
- **Response:** Most changes stay inside one capability module plus generated contract registration; shared kernel changes are unnecessary unless a genuinely new execution primitive appears.
- **Measure:** No modification to a global state model, clone function, generic API contract file, or unrelated capability.

### Q6. Offline and restart recovery

- **Stimulus:** The app restarts without network or the game session remains disconnected.
- **Environment:** A valid prior local state and catalog cache exists.
- **Response:** Last-known projections load with explicit freshness, local desired state and schedules remain available, no live effects run without a current authoritative baseline, and later reconnect reconciles safely.
- **Measure:** No loss of user documents; no command before baseline; integrity and migration checks pass.

### Q7. Local hostile caller

- **Stimulus:** Another local process or unrelated web page attempts to call the CitadelOps API or open its event stream.
- **Environment:** Default local desktop mode.
- **Response:** Strict host/origin policy and a per-launch credential reject the caller; secrets and raw frames do not appear in routine logs.
- **Measure:** Mutation and sensitive-query attempts fail without a valid session credential; security events are safely recorded.

### Q8. Remote mode

- **Stimulus:** An authenticated user monitors or approves work from another device.
- **Environment:** Explicitly enabled remote mode.
- **Response:** Encrypted transport, durable identity, account- and operation-scoped authorization, rate limits, revocation, and audit apply. Local expert/debug operations are not automatically exposed.
- **Measure:** Threat-model acceptance criteria and authorization tests pass before any non-loopback listener is enabled.

### Q9. Historical reprocessing

- **Stimulus:** A decoder or report calculation changes.
- **Environment:** Existing captures/journal records and catalog versions.
- **Response:** A shadow reprojection runs without live effects, records code/schema/catalog lineage, compares outputs, and switches only after validation.
- **Measure:** Repeat runs are deterministic for a fixed version; discrepancies are attributable to specific observations.

### Q10. Account isolation

- **Stimulus:** One account has a browser crash, malformed payload loop, or resource spike.
- **Environment:** Multiple active accounts, if that product mode is enabled.
- **Response:** Other accounts retain ordering and service; the failed account is fenced and recovered within its restart budget.
- **Measure:** Option A must at least isolate account loops and quotas; requiring process-level survival triggers Option C.

### Q11. Sensitive support export

- **Stimulus:** A user generates a support bundle.
- **Environment:** Captures may include login/session/private player data.
- **Response:** The export previews included categories, applies versioned redaction, excludes secrets by default, and records a manifest/hash without modifying source data.
- **Measure:** Known secret fixtures never appear in default bundles; retention and deletion are testable.

### Q12. UI workflow growth

- **Stimulus:** A new task-oriented workflow uses construction, inventory, and operation status.
- **Environment:** Desktop UI and possibly a future mobile client.
- **Response:** The UI composes versioned query projections and submits typed requests without receiving internal storage structs or mounting global polling providers.
- **Measure:** Only subscribed projections update; the workflow has a stable deep link and explicit loading/error/operation states.

## Architectural priority map

| Do now in the foundation | Keep an inexpensive seam | Defer until validated |
|---|---|---|
| Exact behavior/parity contract | Direct game WebSocket adapter | Worker process per account |
| Account and capability partitioning | Remote/mobile adapter | Public Internet service |
| One guarded request/effect path | Tool/LLM schema adapter | Cross-account optimizer |
| Versioned query/command contracts | Rich replay workbench | Alliance collaboration backend |
| Offline projections and catalog versions | Read-only extension subscriptions | Untrusted plugin marketplace |
| Selective causal journal | Additional analytics projections | Universal workflow DSL |
| Authenticated local endpoint | Native shell bridge | Universal event sourcing |
| Bounded/redacted diagnostics | Process-extractable runtime contract | Microservices by capability |

## Triggers that should change the decision

### Promote toward Option B when

- replay, automation explanation, or historical analytics becomes a top-three user workflow;
- support routinely needs to reconstruct why actions occurred;
- decoder changes require multiple durable reprojections per release;
- users require audit retention beyond bounded diagnostic captures.

### Promote toward Option C when

- two or more concurrent accounts become a supported, routinely tested workflow and one account’s failure must not affect another;
- account runtimes must execute on separate hosts;
- per-account memory/CPU limits are contractual;
- independently deployable or untrusted extensions become committed requirements.

Until those triggers are real, capability partitioning inside one process provides most of the option value with much less cost.

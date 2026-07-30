# Decision, parity strategy, and greenfield delivery

## Decision

Build the replacement as a **capability-oriented modular monolith** with:

- one profile-scoped bootstrap that becomes an ordered runtime per authenticated game account;
- a small shared execution kernel;
- vertical capability ownership;
- typed/versioned query and request contracts;
- per-capability state versions and task-shaped projections;
- transactional local persistence;
- a bounded selective journal for observations, decisions, effects, and outcomes;
- transport, UI, CLI, replay, storage, clock, catalog, and secret adapters;
- an authenticated local control surface;
- contracts that can later move an account runtime behind IPC.

Do not start with full event sourcing, feature microservices, per-account worker processes, a universal workflow DSL, or dynamically loaded Go plugins.

This choice preserves the current synchronous safety model while removing the global state/API/UI coupling that creates most of the present complexity.

## Weighted comparison

Scores use a 1–5 scale, where 5 is strongest. Weights reflect the stated goal: exact functionality and a better product are more important than speculative scale. The arithmetic is a decision aid, not objective truth.

| Criterion | Weight | A. Modular monolith | B. Journal-first | C. Supervised workers |
|---|---:|---:|---:|---:|
| Functional parity and operation safety | 25% | 5 | 4 | 4 |
| Change isolation and protocol evolution | 20% | 5 | 5 | 4 |
| Desktop simplicity and performance | 15% | 5 | 3 | 2 |
| Replay, explanation, and analytics | 15% | 3 | 5 | 4 |
| Multi-account fault isolation | 10% | 3 | 3 | 5 |
| Secure remote/extension path | 10% | 4 | 4 | 5 |
| Delivery and migration risk | 5% | 5 | 2 | 1 |
| **Weighted result** | **100%** | **4.40** | **4.00** | **3.75** |

### Sensitivity

- If replay/audit becomes more than about one-third of the decision and permanent event-schema work is acceptable, Option B can win.
- If multi-account isolation plus remote workers becomes more than about one-quarter of the decision, Option C can win.
- If the target remains a local desktop product and parity is non-negotiable, Option A remains clearly favored.

The most robust choice is therefore A with journal-ready seams, not A with no history.

## Architectural decision record

### D1. Decompose by capability

Construction, equipment, movement, combat/commanders, alliance/intelligence, reports/analytics, Rift, and event families own their protocol facts, state, planners, policies, queries, configuration schema, migrations, and frontend feature surface.

Why: these decisions change together. The current layer-oriented spread makes one feature touch global state, ingest, application planners, automation, API contracts, and UI.

Constraint: capabilities communicate through typed facts, requests, and read ports—not another capability’s storage model.

### D2. Keep a small execution kernel

Centralize account sequencing, session generation, capability versions, claims, admission, command lanes, pacing, deadlines, correlation, cancellation, and receipts.

Why: these are safety mechanisms that must behave consistently for manual, automated, scheduled, CLI, and future tool actors.

Constraint: the kernel contains no construction, equipment, event, or catalog-specific rule.

### D3. Treat game traffic as observations

The remote game remains authoritative. An inbound message is evidence, an outbound command is an attempted effect, and a correlated committed observation establishes the outcome.

Why: captures are partial, sessions begin after prior state changes, messages can be snapshots or deltas, and a send acknowledgement does not prove the game applied an action.

Constraint: the local journal is authoritative only for CitadelOps observations and actions, not for the entire game.

### D4. Partition state and versions

Maintain account-scoped, independently versioned capability stores rather than a single cloned `GameState` and global revision.

Why: this removes unrelated conflicts, clone cost, persistence starvation, and API amplification while retaining stale-plan protection.

Constraint: a plan records every capability version it depends on and is revalidated immediately before a destructive effect.

### D5. Separate public contracts from internal models

Expose task-shaped query DTOs, typed requests, receipt DTOs, and subscription deltas. Generate TypeScript transport clients from a reviewed contract. Provide an API v2 compatibility adapter until existing clients move to the replacement API.

Why: serializing internal state makes both backend refactoring and frontend performance unnecessarily expensive.

Constraint: a public schema has an owner, semantic version policy, compatibility tests, and deprecation path.

### D6. Use selective journaling and transactional projections

Persist settings, desired state, schedules, receipts, capability snapshots, journal metadata, and history indexes transactionally. Store large captures separately under bounded retention.

Why: replay and explanation are valuable, but event-sourcing every preference or catalog creates avoidable migration burden.

Constraint: safety-critical projection and receipt changes commit synchronously; analytics may lag with visible cursors.

### D7. Make bootstrap and account scope explicit from day one

Before login, every session and capture is scoped to a stable local `ProfileID` plus a session/connection generation. After an authoritative login baseline reveals `GameAccountID`, the runtime records a fenced binding and every query, request, receipt, setting, claim, and account history uses that account scope, even if the first release allows one active account.

Why: removing singleton assumptions later is expensive and multi-account is a plausible future.

Constraint: no implicit “current global account” below the UI shell/application facade. Reusing a browser profile for another account starts or selects a separate partition; it never merges identities automatically.

### D8. Secure local and remote modes separately

Default to literal loopback binding, a random or user-scoped endpoint, per-launch high-entropy credential, strict host/origin checks, narrow CORS, request limits, and safe logs. Remote mode remains disabled until it has real identity, authorization, encryption, enrollment, revocation, and audit.

Why: network location alone is not caller identity, and the existing API includes high-impact operations and sensitive data.

Constraint: expert/debug operations require an explicit elevated capability and are not automatically part of a remote API.

### D9. Compile first-party modules; defer plugin runtime

Register first-party capabilities at compile time. If an extension ecosystem becomes real, use a versioned subprocess or evaluate WASI with explicit host capabilities.

Why: Go shared-library plugins have portability and coupling limitations and do not provide an adequate sandbox.

Constraint: an extension cannot obtain raw transport, secrets, filesystem, or arbitrary effect authority by default.

### D10. Align the UI by workflow and capability

Use URL routing/deep links, per-feature query hooks, explicit loading/error/stale/operation states, and a general operation center. Keep presentation state local and move persistent policies into server capability schemas.

Why: the current global context, provider polling, app-owned modals, and giant view components amplify unrelated changes and make background behavior hard to see.

Constraint: the UI never constructs raw game protocol or treats optimistic state as authoritative for a destructive operation.

### D11. Preserve API v2’s global revision in the adapter

The new core uses account positions and per-capability versions. The v2 adapter retains a separate monotonically increasing compatibility revision under the old commit rules and associates each emitted revision with the current capability-version vector.

Why: existing v2 callers submit one `expectedRevision`, receive one global revision, and treat any intervening state commit as stale. Silently weakening that check would not be exact compatibility.

Constraint: a v2 request succeeds revision admission only when its `expectedRevision` equals the adapter’s current compatibility revision, then plans against the mapped current capability versions. Native v3 requests declare only the capability versions they actually depend on.

## What “exactly the same” must mean

A rewrite cannot preserve an undefined target. The active tree contains both implemented behavior and incomplete/unreachable surfaces, while the repository also points to a frozen 1.3.8 reference and a 2.0 architectural intent. Before implementation, create a **parity manifest** with four labels:

| Label | Meaning | Rewrite treatment |
|---|---|---|
| Reachable current behavior | A user can execute it in the active app | Mandatory semantic parity |
| Server-supported but unmounted/incomplete UI | Backend or component exists but is not a complete current workflow | Preserve data/operation compatibility; product decision before mounting |
| Legacy reference behavior | Present in the frozen 1.3.8 product but absent/incomplete in the active tree | Explicit restore, retire, or defer decision |
| Stated future intent | Mentioned in architecture/comments but not implemented | Seam only unless promoted to a requirement |

The manifest should list every:

- inbound/outbound opcode and supported payload variant;
- named intent/request, argument schema, actor, priority, claims, response code, timeout, and receipt behavior;
- automation policy, trigger, setting/default, scheduling rule, and safety gate;
- state/query field and its unknown/stale/merge semantics;
- configuration section and migration;
- user route, modal, preview, warning, error, and offline behavior;
- history/capture/report format;
- browser, update, CLI, API, replay, and data-cache workflow.

This ledger is the rewrite backlog and acceptance record. It prevents “architecture cleanup” from silently deleting obscure but relied-upon behavior.

## Parity harness

### 1. Golden transport transcripts

For each operation family, preserve sanitized captures containing:

- minimum initial authoritative baseline;
- relevant prior state/catalog version;
- submitted request and settings;
- exact outbound wire records and timing constraints;
- inbound responses and later snapshots;
- normalized state/query changes;
- final receipt and user-visible result.

Compare wire bytes where formatting/token order is contractually significant; otherwise compare decoded semantic commands plus omission/null behavior. Identifier namespaces require typed assertions.

### 2. Replay conformance

Feed the same capture and deterministic clock through old and new observation paths. Compare capability projections after every meaningful observation, not only final state. Record intentional differences in the parity manifest.

Replay mode disables all live effects. A separate planner shadow mode can record the commands it would send.

### 3. Shadow planning

In a live session, the old implementation remains the only sender. The new planner receives mirrored safe observations and requests and records proposed commands/claims/outcomes for comparison. Never allow both systems to send the same operation.

### 4. Contract snapshots

Freeze API v2 examples and client journeys. Test the compatibility adapter against them while the new API uses capability DTOs. Generate and validate TypeScript clients from the new schema.

### 5. Migration fixtures

Maintain anonymized examples of every known settings, state, preset, schedule, history, and capture version. Migration runs against a copy, verifies counts/IDs/hashes and invariant checks, and records a receipt. The old data remains recoverable until the user confirms cutover.

### 6. Protocol fuzzing

Use Go’s built-in coverage-guided fuzzing on frame assembly, decoder boundaries, tolerant field handling, normalization, and redaction. Fuzzing supplements known captures; it does not decide valid game semantics.

### 7. UI journey acceptance

Record task-level outcomes rather than pixel identity:

- start/stop/reconnect and offline browsing;
- castle focus rules;
- preset create/capture/preview/apply;
- equipment preview/apply/sell warnings;
- construction and TCI namespace-sensitive actions;
- defense fresh-read/no-auto-buy behavior;
- attack admission, commander eligibility, and pacing;
- each automation configure/enable/trigger/disable path;
- schedule and operation cancellation;
- report/history/analytics retrieval;
- diagnostics and redacted support export.

### 8. Performance and resilience budgets

Establish budgets from real captures before coding targets. At minimum measure:

- observation-to-critical-projection latency;
- observation-to-subscribed-UI latency;
- queue depth and memory under bursts;
- persistence checkpoint age during sustained traffic;
- startup/recovery time with realistic history;
- API payload and rerender counts per workflow;
- reconnect behavior with in-flight idempotent and non-idempotent effects.

## Greenfield delivery sequence

The system can be implemented from scratch without a big-bang behavioral cutover. Build a new core beside the existing app and use the existing implementations and captures as an oracle.

### Stage 0: freeze evidence

- Snapshot/tag the exact active behavior being used as baseline; a dirty worktree is not a durable specification.
- Complete the parity manifest and scrubbed capture library.
- Record current configuration/data samples and reachable UI journeys.
- Define performance, retention, and security baselines.
- Resolve whether incomplete surfaces such as the operation center, player-history backfill, building planning UI, standalone reports, currency/resource views, Beri settings, and UI-owned equipment cleanup are parity, restoration, or future scope.

Exit gate: every supported mutation and automation has an owner and at least one observable acceptance scenario.

### Stage 1: walking skeleton

- Create the new composition root, account runtime, deterministic clock, transport port, replay adapter, contract pipeline, SQLite unit of work, capture store, and authenticated local API.
- Implement identity/session baseline plus one thin read-only capability end to end.
- Deliver snapshot/delta subscriptions to one route in a capability-oriented UI shell.

Exit gate: a capture produces a versioned query projection and UI update without a global state clone or full-world refetch.

### Stage 2: execution kernel

- Implement typed requests, planners, claims, admission, lanes, pacing, generation fencing, correlation, committed-response barriers, cancellation, and receipts.
- Port one representative reversible operation and one ambiguous/non-idempotent operation.
- Establish shadow planning and operation causality.

Exit gate: golden transcript and disconnect scenarios match the baseline, including no duplicate effect.

### Stage 3: representative vertical capabilities

Port deliberately different slices before scaling the pattern:

1. **Construction/TCI** to prove typed identifiers, catalog integration, desired targets, and purchase/upgrade behavior.
2. **Equipment** to prove optimizer preview/apply, inventory freshness, irreversible warnings, and command pacing.
3. **Movement/attack** to prove high-frequency state, commander claims, attack admission, lanes, and schedules.
4. **Reports/history** to prove multi-message capture, parser versioning, indexed projection, and retention.

Refine the kernel only for execution concepts shared by at least two capabilities. Do not add genericity in anticipation.

Exit gate: the module template works without feature logic leaking into the composition root or global contracts.

### Stage 4: capability-by-capability parity

- Port remaining capabilities with their state, protocol, requests, automation, configuration, queries, UI, migrations, and parity fixtures together.
- Move UI-owned policies into the account runtime where headless parity requires it.
- Maintain API v2 through the compatibility adapter.
- Track parity manifest status and intentional changes openly.

Exit gate: every mandatory row in the parity manifest passes replay, operation, migration, and journey acceptance.

### Stage 5: data migration and dual-read validation

- Import settings, browser selection/profile references, presets, schedules, last-known state where meaningful, histories, and capture indexes into a new versioned data directory.
- Build new analytics/read projections in shadow and compare totals/samples.
- Back up the original data and provide a rollback marker.
- Never reinterpret official game JSON as Citadel-specific configuration; keep official caches and instance data separate.

Exit gate: migration is idempotent, integrity-checked, and reversible on representative user data.

### Stage 6: cutover and hardening

- Make the new runtime the only live sender.
- Keep legacy read/replay comparison available for a bounded diagnostic period.
- Enforce local authentication, retention, redaction, database backup/checkpoint policy, queue limits, and capability health.
- Remove the v2 compatibility adapter only after all owned consumers migrate.

Exit gate: parity, resilience, security, performance, and recovery scenarios meet their budgets with no legacy sender enabled.

### Stage 7: optional escalation

Only after product evidence:

- deepen the journal and reprojection tooling if Option B triggers are met;
- extract the account runtime into supervised workers if Option C triggers are met;
- add remote/mobile/tool adapters with explicit authorization;
- introduce an extension host with scoped capabilities.

## Data and filesystem direction

A clean-slate data layout should separate application-owned data from official cache and large captures:

```text
ApplicationSupport/CitadelOps/
  Database/CitadelOps.db
  Captures/<ProfileID>/<SessionID>/*.capture.zst
  OfficialData/<Version>/...
  Browser/<ProfileID>/<Browser>/Profile/...
  Backups/...
  SupportBundles/...
```

`CITADEL_DATA_DIR` remains a supported override. Use platform application-support directories by default rather than placing mutable data next to an installed binary. Apply restrictive permissions, and put secrets in platform secret storage rather than the general database or config export.

Changing the default location is an intentional compatibility change, not an invisible replacement of the old path. On first run, the new application checks the explicit `CITADEL_DATA_DIR` first, then discovers the legacy repository/executable-adjacent data location, offers or performs a recorded copy-based migration, verifies integrity before switching, and preserves the original plus a rollback marker until cutover is confirmed. It must not split one active instance across old and new roots.

If SQLite WAL is selected, pin a release containing SQLite’s documented 2026 WAL-reset fix, use one application-owned writer, set an explicit checkpoint policy, test abrupt shutdown, and never place the live database on a network filesystem.

## Security baseline

Before feature parity is considered complete:

- bind only literal loopback addresses by default;
- use a high-entropy per-launch browser/API credential and rotate it on restart;
- validate `Host`, `Origin`, content type, schema, message size, and rate;
- protect WebSocket handshake and every privileged message;
- separate read, operate, configure, debug, browser-control, update, and secret capabilities;
- require re-approval or elevated capability for irreversible/high-impact operations where product policy demands it;
- redact login/session tokens and private payloads from routine logs;
- offer bounded capture retention and secure deletion/export controls;
- keep remote mode off unless its separate threat model is accepted.

## Risk register

| Risk | Consequence | Mitigation |
|---|---|---|
| Hidden protocol behavior is missed | Silent parity loss or unsafe effects | Parity manifest, golden captures, old/new shadow planning, typed ID assertions |
| Capability boundaries become cosmetic | New global state/kernel growth | Import rules, public API budget, composition-root checks, capability ownership review |
| Selective journal grows into universal event sourcing | Migration/privacy burden | Written inclusion criteria, retention classes, no events for secrets/catalog/preferences by default |
| Async projections inform safety decisions | Stale destructive plans | Synchronous critical projections, version dependency set, final revalidation |
| “Local” API is exposed | Account takeover or sensitive data loss | Authenticated local mode, strict bind/origin, explicit remote mode |
| Raw captures leak secrets | Privacy/security incident | Redaction/encryption, restrictive permissions, bounded retention, safe support export |
| Rewrite changes UI and core simultaneously without oracle | Scope explosion and unverifiable parity | API v2 adapter, representative vertical slices, task-level journey baseline |
| SQLite becomes an unexamined bottleneck | Checkpoint lag or lock contention | One writer, short transactions, metrics, realistic capture benchmarks, separate large files |
| Multi-account assumptions leak into globals | Expensive later redesign | First-class `ProfileID`/`GameAccountID` binding and per-account loops/paths/claims from stage 1 |
| Worker topology is adopted prematurely | IPC/operations cost overwhelms product work | Trigger-based Option C decision; keep extraction seam only |
| Direct WebSocket adapter bypasses proven browser semantics | Authentication/protocol breakage | Same transport conformance and parity harness; browser adapter remains fallback |
| An ambiguous effect is automatically retried | Duplicate purchase/attack | Explicit idempotency class and `indeterminate` receipt with authoritative refresh |

## Decisions deliberately left open

The research does not justify choosing these yet:

- native-shell framework versus controlled browser/webview;
- specific SQLite Go driver and migration library;
- OpenAPI-only versus OpenAPI plus AsyncAPI;
- protobuf/gRPC versus another generated IPC if workers are eventually extracted;
- exact journal fact retention versus recomputation from raw observations;
- one broad Events capability versus separate capability per event family;
- remote/mobile product and its identity provider;
- WASI extension runtime.

Each can be decided with a narrow spike after the stable capability and execution boundaries exist.

## Final rationale

CitadelOps already has the seed of the right product architecture: all actors request guarded operations against an observed, remotely authoritative game. Its next architecture should make that control loop smaller and more explicit, not replace it with fashionable distribution.

The recommended design improves usability and engineering speed by letting each capability own a complete user-facing slice, reducing data amplification, exposing stable task contracts, and making every automated effect explainable. It preserves future paths to replay-heavy CQRS and isolated account workers without paying those costs before the product requires them.

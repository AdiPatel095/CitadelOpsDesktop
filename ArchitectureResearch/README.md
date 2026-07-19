# CitadelOps architecture research

Research snapshot: 2026-07-15

Implementation progress and measured latency are recorded in [ImplementationStatus.md](ImplementationStatus.md).

This dossier answers two connected questions: which architecture would preserve CitadelOps behavior if rebuilt from a clean slate, and how far the current runtime can be safely augmented before that rewrite is justified.

The greenfield answer is a **capability-oriented modular monolith with a small execution kernel** and selective observation/decision/outcome journaling. The implementation decision for the current product is narrower: retain the existing modular monolith and feature behavior, but install the typed resource, durable operation, account/focus fencing, lifecycle, scheduling, and transport controls in place. Keep capability-store extraction and process boundaries available for measured future triggers rather than paying their migration cost now.

## Recommendation in one page

The most useful abstraction is not “parser,” “automation,” “API,” or “desktop UI.” It is an **account capability runtime**:

```text
observation -> capability reducer -> capability state/query
                                      |
request ----> capability planner ----> guarded effect -> observed outcome
```

A capability is a cohesive product area such as construction, equipment, troop movement, alliance intelligence, or an event family. It owns its protocol interpretation, state, queries, commands, automation policy, persistence projection, and frontend contract. A small shared runtime owns only cross-cutting execution rules: account ordering, revisions, resource claims, pacing, correlation, cancellation, receipts, transport, time, and diagnostics.

That boundary preserves the strongest idea already present in CitadelOps—interactive actions and automations both travel through one intent/execution path—without retaining the current giant state object, full-state client refresh, layer-oriented domain scattering, or overly generic intent machinery.

The three greenfield options considered are:

1. **Capability-oriented modular monolith.** One Go process, one deployable desktop application, explicit ports and vertical capability modules. This has the lowest parity and operational risk and is the recommendation.
2. **Journal-first core with CQRS projections.** An append-only observation/decision/outcome journal feeds capability projections. This is strongest for replay, protocol forensics, explainable automation, and analytics, but carries event-versioning and consistency costs.
3. **Control plane with supervised account workers.** A local control plane starts an isolated worker per configured profile/session, then leases it to the observed game account using versioned IPC. This is strongest for multi-account fault isolation, remote workers, and extension isolation, but has the highest implementation and operational cost.

These are solution packages, not mutually exclusive buzzwords. The recommended product uses option 1 as its topology, adopts a narrow part of option 2 for replay and audit, and keeps contracts extractable into option 3 if concrete scaling triggers appear.

## Decisions that matter most

- Preserve wire behavior, ordering, admission, correlation, and response-commit semantics before improving internals.
- Scope pre-login work to a stable local profile identity, then bind it to the game account revealed by the authoritative baseline; never merge account partitions implicitly.
- Partition mutable state by account and capability. Do not clone or publish an entire world state for every frame.
- Treat received game traffic as **observations**, local actions as **intents**, and only a correlated response or later authoritative observation as an **outcome**.
- Keep state reduction synchronous and ordered within an account. Analytics projections may be asynchronous; safety-critical command planning may not depend on lagging projections.
- Publish versioned query and command contracts. Do not expose internal storage models as the HTTP API.
- Preserve API v2’s single global revision in its compatibility adapter while native capability contracts use narrower version dependencies.
- Generate TypeScript clients and event contracts from those public schemas instead of manually mirroring a large Go state structure.
- Use transactional local storage for configuration, projections, operation receipts, and compact journal metadata. Keep large captures in bounded, compressed files referenced by the database.
- Make local endpoints authenticated and origin-restricted. Treat remote operation as a separate security mode with identity, authorization, and encryption.
- Keep extensions compiled in until there is a real ecosystem. Do not use Go shared-library plugins as the product extension boundary.

## Reading order

1. [ImplementationStatus.md](ImplementationStatus.md) records the implemented augmentation, verification, latency, and remaining limits.
2. [CurrentState.md](CurrentState.md) explains what the application does today, its main data flows, strengths, and architectural pressure points.
3. [TargetAbstraction.md](TargetAbstraction.md) defines the proposed stable abstraction and the boundaries common to all three solutions.
4. [ScopedStateArchitecture.md](ScopedStateArchitecture.md) breaks the state into account, kingdom, castle, session, target, and event capability instances, including focus routing, versions, freshness, and cross-partition operations.
5. [FeatureImpactMatrix.md](FeatureImpactMatrix.md) maps the complete current feature and automation surface to those scopes and explains internal changes, visible product effects, and parity hazards.
6. [FutureUseCases.md](FutureUseCases.md) distinguishes current behavior, stated direction, and plausible future hypotheses, then turns them into quality scenarios.
7. [ArchitectureOptions.md](ArchitectureOptions.md) develops the three greenfield solutions and compares their consequences.
8. [DecisionAndMigration.md](DecisionAndMigration.md) records the recommendation, behavior-parity strategy, delivery sequence, risks, and escalation triggers.
9. [ArchitectureChangeWorkplan.md](ArchitectureChangeWorkplan.md) turns the decision into a concrete state/intent/API/client/data migration plan, capability port order, evidence gates, and first implementation backlog.
10. [ConcurrencyRaceConditionsAndEdgeCases.md](ConcurrencyRaceConditionsAndEdgeCases.md) inventories the Go, gameplay, lifecycle, persistence, protocol, and all-feature concurrency risks.
11. [ConcurrencyArchitectureAssessment.md](ConcurrencyArchitectureAssessment.md) scores the current runtime against that register and compares in-place augmentation, an in-process kernel replacement, and a supervised-worker rebuild.
12. [ResearchSources.md](ResearchSources.md) lists the external evidence and how it affected the conclusion.

## Evidence and limits

The repository review covered the active `codex/version-2.0.0` working tree, including substantial uncommitted work, rather than treating the checked-in `Architecture.md` as proof that every intended boundary is implemented. Findings therefore distinguish:

- behavior visible in the current code;
- design intent stated in the repository;
- future uses inferred during this study.

The design documents are architecture research rather than proof of implementation. [ImplementationStatus.md](ImplementationStatus.md), the source code, and the cited verification commands are the authoritative implementation checkpoint; names and package sketches elsewhere remain options until code and tests adopt them.

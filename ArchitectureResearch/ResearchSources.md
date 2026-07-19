# Research sources

Research was performed on 2026-07-15. Technical conclusions favor original papers, standards, official documentation, and primary project documentation. Product-specific conclusions come from the current CitadelOps working tree.

## Architecture and modularity

### Information hiding

D. L. Parnas, [“On the Criteria To Be Used in Decomposing Systems into Modules”](https://doi.org/10.1145/361598.361623), argues that a system’s changeability depends on the decomposition criterion, not merely on drawing module boxes. This supports grouping CitadelOps by domain decisions likely to change—construction rules, equipment interpretation, event mechanics—rather than by technical layers that force each feature to span the entire repository.

### Ports and adapters

Alistair Cockburn’s original [Hexagonal Architecture](https://alistair.cockburn.us/hexagonal-architecture) describes application conversations as ports with replaceable adapters for UI, tests, databases, and external systems. It supports treating Chromium/CDP, replay, HTTP, CLI, storage, and clocks as adapters around one application core.

The Go team’s [module layout guidance](https://go.dev/doc/modules/layout) documents `internal` packages as a mechanism for keeping implementation APIs private while sharing a core across multiple commands. This makes a modular monolith enforceable in the language rather than dependent on diagrams alone.

### Aggregate and consistency boundaries

Microsoft’s official [Tactical Domain-Driven Design guidance](https://learn.microsoft.com/en-us/azure/architecture/microservices/model/tactical-domain-driven-design) defines aggregates as transactional consistency boundaries, recommends keeping them small, and advises referencing independently changing aggregates by identity. This supports independently versioning CitadelOps capability instances by castle, kingdom, account, or event while committing all facts from one accepted protocol observation as an atomic account change set. The guidance is applied inside a modular monolith; it is not evidence that each partition should be a microservice.

### Architecture evaluation

The Software Engineering Institute’s [Architecture Tradeoff Analysis Method collection](https://www.sei.cmu.edu/library/architecture-tradeoff-analysis-method-collection/) emphasizes evaluating an architecture through concrete quality-attribute scenarios and tradeoffs. The decision matrix in this dossier is therefore paired with scenarios and trigger conditions rather than presented as universally correct.

[ISO/IEC 25010:2023](https://www.iso.org/standard/78176.html) supplies a broad product-quality model. The study narrows that model to the characteristics that materially differentiate the options: functional suitability, reliability, performance efficiency, maintainability, security, and portability.

## State, commands, and history

Microsoft’s official [CQRS pattern](https://learn.microsoft.com/en-us/azure/architecture/patterns/cqrs) explains separate command and query models, purpose-built read DTOs, and the resulting complexity and consistency tradeoffs. It supports separating capability commands from UI projections, but also argues against CQRS where ordinary transactional state is sufficient.

Microsoft’s official [Event Sourcing pattern](https://learn.microsoft.com/en-us/azure/architecture/patterns/event-sourcing) documents immutable append-only history, replay, audit, projections, snapshots, schema evolution, ordering, and eventual-consistency costs. Its cautions are why this dossier recommends a **selective journal**, not event sourcing for every preference, catalog, or state field.

The CNCF [CloudEvents specification](https://github.com/cloudevents/spec/blob/main/cloudevents/spec.md) and [CloudEvents primer](https://github.com/cloudevents/spec/blob/main/cloudevents/primer.md) offer a small model for unique event identity, source, type, schema, time, duplicate detection, and extension metadata. CitadelOps can borrow these envelope properties without adopting a cloud event platform.

## Transport, process boundaries, and flow control

[RFC 6455](https://www.rfc-editor.org/rfc/rfc6455) distinguishes WebSocket messages from frames and specifies fragmentation, ordering, control frames, and closing behavior. It supports separating transport assembly from game-protocol decoding. The rule against silently dropping state-changing input on queue overflow is this study’s safety inference from CitadelOps parity requirements, not a rule stated by RFC 6455.

The official [gRPC core concepts](https://grpc.io/docs/what-is-grpc/core-concepts/) cover IDL-generated services, unary and streaming calls, and stream ordering. The official guidance for [deadlines](https://grpc.io/docs/guides/deadlines/), [cancellation](https://grpc.io/docs/guides/cancellation/), [health checking](https://grpc.io/docs/guides/health-checking/), and [flow control](https://grpc.io/docs/guides/flow-control/) identifies the extra semantics required if a future account runtime crosses a process boundary.

Chromium’s [multi-process architecture](https://www.chromium.org/developers/design-documents/multi-process-architecture/) is a primary precedent for using process separation to contain crashes and restrict resource access while accepting IPC and memory overhead. It supports the supervised-worker option when isolation is worth that cost.

The Erlang/OTP [supervision principles](https://www.erlang.org/doc/system/design_principles.html) distinguish workers from supervisors responsible for starting, stopping, and monitoring them. This is used as a design pattern for a future Go control plane, not as a proposal to rewrite CitadelOps in Erlang.

## Persistence

SQLite’s [application file format guidance](https://sqlite.org/appfileformat.html) describes the portability, transactional behavior, and schema flexibility that make SQLite suitable for a local desktop product.

SQLite’s [write-ahead logging documentation](https://sqlite.org/wal.html) documents concurrent readers with one writer, same-host constraints, and checkpointing. The page also records the 2026 WAL-reset defect and fixed releases. Any implementation using WAL must pin a SQLite release containing that fix and verify the current upstream security guidance at implementation time.

## API contracts and observability

The [OpenAPI 3.2 specification](https://spec.openapis.org/oas/latest.html) provides a language-neutral HTTP API description suitable for schema review and client generation. It supports replacing manually duplicated Go/TypeScript contracts with explicit versioned public DTOs.

The [AsyncAPI 3.1 specification](https://www.asyncapi.com/docs/reference/specification/latest) provides a contract format for asynchronous messages and channels. It is an option for documenting the UI event stream; adopting it is useful only if generated validation or tooling justifies the additional artifact.

[RFC 9110, HTTP Semantics](https://www.rfc-editor.org/rfc/rfc9110) defines validators and conditional requests, including `If-Match` for preventing a state-changing method from being applied when the selected representation has changed. This supports projection ETags and conditional desired-document/query mutations at the HTTP boundary; internal plans still need explicit multi-partition dependency sets rather than one HTTP validator.

OpenTelemetry’s [signals model](https://opentelemetry.io/docs/concepts/signals/) and [context propagation guidance](https://opentelemetry.io/docs/concepts/context-propagation/) support tracing the causal chain from observation through reducer, decision, command, and outcome. Exporters should remain opt-in because this desktop application can handle sensitive account traffic.

The Go project’s [fuzzing guidance](https://go.dev/doc/security/fuzz/) describes coverage-guided fuzzing for edge cases and security weaknesses. It supports fuzzing protocol decoders and normalization boundaries in the parity harness.

## Security and extension boundaries

OWASP’s [WebSocket Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/WebSocket_Security_Cheat_Sheet.html) recommends explicit origin allowlists, handshake authentication, per-message authorization and validation, rate and size limits, lifecycle logging, and avoiding sensitive payload logging. These controls apply even when the primary UI is local.

[RFC 8252](https://www.rfc-editor.org/rfc/rfc8252) is OAuth-specific, but its loopback discussion is still useful: bind literal loopback addresses and do not assume that possession of a loopback route proves application identity. This informs the recommended per-launch local credential and strict host/origin checks.

NIST [SP 800-207, Zero Trust Architecture](https://csrc.nist.gov/pubs/sp/800/207/final) rejects implicit trust based solely on network location. This is most relevant if CitadelOps exposes a remote or LAN mode; that mode needs explicit identity and authorization rather than a broader bind address.

Go’s official [`plugin` package documentation](https://pkg.go.dev/plugin) lists platform gaps, exact toolchain/dependency coupling, initialization and race-detector limitations, and the risk of loading dangerous code. It explicitly notes that IPC can be a better fit. CitadelOps should not use Go shared libraries as its long-term extension contract.

Microsoft’s [named-pipe security and access-rights guidance](https://learn.microsoft.com/en-us/windows/win32/ipc/named-pipe-security-and-access-rights) warns that default security descriptors can grant broader access than intended. A Windows worker implementation would need explicit user-scoped access control.

[WASI.dev](https://wasi.dev/) describes a capability-oriented model with no ambient authority by default. It is a possible future sandbox for genuinely untrusted extensions, but its toolchain maturity and the lack of a current extension requirement make it an escalation option rather than a foundation.

## How sources changed the recommendation

- The information-hiding and hexagonal sources reinforced a capability-oriented modular monolith rather than another layer-oriented rewrite.
- Tactical DDD guidance clarified that castle/capability partitions should follow consistency and change boundaries, not become services or arbitrary struct fragments.
- CQRS and event-sourcing sources made a journal attractive for replay and explanation, while their explicit complexity warnings ruled out universal event sourcing.
- WebSocket, gRPC, and supervision sources exposed the ordering, flow-control, and partial-failure work hidden inside a worker architecture.
- Security sources turned “local API” from an assumed trust boundary into an explicit authenticated product mode.
- The Go plugin guidance ruled out native shared-library plugins as a portable architecture.
- SQLite’s current WAL guidance added a concrete dependency-pinning and checkpointing requirement, rather than a generic “use SQLite” recommendation.
- HTTP conditional-request semantics supplied a standard boundary for projection/document concurrency while leaving cross-partition planning to the execution kernel.

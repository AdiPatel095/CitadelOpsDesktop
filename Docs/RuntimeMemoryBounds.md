# Runtime memory bounds (CIT-52 C)

Contract: [CIT-52](https://linear.app/adipatelorg095/issue/CIT-52), Daniel's
`CIT Doc Vault/Engineering/Plans/CIT-52 Runtime memory bounds.md`, part C,
and `CIT Doc Vault/Application Map/Runtime ingest, persistence and memory bounds.md`
(AD-W2-8).

Six dispatch guard reads use immutable `State.ReadOnlyView()` generations instead
of deep-copying `Snapshot()`. Each read stays at its existing dispatch boundary;
guards and their helpers must never mutate the view.

| State | Retention |
| --- | --- |
| Alliance directory | Keep the observed alliance, the player's alliance and the own alliance. Other entries must have been observed within seven days of frame time; keep at most 64, ordered newest first by `(ObservedAt, ID)`. |
| Rift deletion tombstones | Remove entries older than 24 hours on a changed launch capture (frame time) or template deletion (UTC wall clock). The exact 24-hour boundary is retained. Recent tombstones still block captures observed at or before deletion. |
| Shared Storm scan windows | Remove keys outside the current plan once their completion is older than twice the two-hour refresh interval (four hours). Remove the corresponding SQLite rows in the existing persistence transaction; failed batches re-queue unless a newer update already exists. Current plan keys and the exact four-hour boundary are retained. |

Event scores are intentionally **not pruned**. `ByEvent`, `RankingByEvent` and
`ActivityByEvent` use recurring event IDs. `EnsureEventActivity` resets activity
for each new occurrence; `LaunchIDs` is capped at 12,000 and `PendingAttacks` at
512 (`Server/State/EventScores.go`).

Scheduled operations, including finished `rift:` entries, are intentionally
**not pruned**. An entry is reused with an increasing `Version`, which is part of
its idempotent operation ID (`Server/Scheduling/Scheduler.go`). Removing an entry
would restart it at version 1 and collide with an existing journal operation,
silently skipping the new run.

The documented budget for CIT-52 A–C is **at most 64 MiB of post-GC live heap per
runtime above its own idle baseline**, under saturated receipt history (at least
10,000 stored receipts) and a map-scan burst. Part C implements the guard and
retention contributions. This budget requires the other parts' receipt and queue
bounds; local structural tests do not establish live compliance. After an
authorized deployment, Miles measures process-wide `postGcLiveHeap` and runtime
count through `/orchestrator/v1/diagnostics` at idle, saturated receipts and a
Storm or Fortress scan. That live-ops measurement is separate from PR approval.

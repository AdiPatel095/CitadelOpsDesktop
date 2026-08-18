# CitadelOps 2.3 Tenant Architecture

CitadelOps 2.3 runs the desktop and hosted product from the same code. The
desktop is a process group with one account. CitadelOpsTenant is a bounded
process group with multiple account runtimes, one game websocket per account,
and one authenticated frontend shard per account.

The design has two separate rules:

1. The logical model follows official game concepts and wire identities.
2. The physical model follows tenant cost and isolation requirements.

This lets feature code keep reading recognizable game state while the runtime
stores large or frequently changing collections in compact partitions.

## Process and account boundary

```text
CitadelOps process
  shared official GameData manager
  shared immutable reducer registry, update manager, and cloud clients
  shared WorldMapStore, partitioned by world and kingdom
    objective player-castle and Storm facts
    anonymous cooperative Storm scan coverage
  account supervisor
    account A: session + websocket + state + intents + automation + API
    account B: session + websocket + state + intents + automation + API
    ...bounded by maxAccounts
```

`Server/Accounts.Supervisor` owns account cardinality and lifecycle. Every
account gets a separate `App.Application`, `State.Store`, game session,
reducer pipeline, intent engine, durable operations database, report database,
configuration, and API server. Official item/language data is decoded once per
process. The immutable protocol reducer registry, update manager, and cloud
clients are also constructed once, so opcode tables, update polling, and HTTP
connection pools are not multiplied by the account count. Objective map facts
may be shared only between accounts bound to the same canonical world and only
for kingdoms that account has unlocked.

## Ownership model

The physical representation is shared-first, but the admission rule is
fail-closed. A value is shared only when every projected field is an objective
official fact with the same meaning for every eligible viewer. Everything else
is account-private and keyed by the supervisor's internal account identity.
That identity is never stored in a shared generation.

| Scope | Stored once | Kept per account |
| --- | --- | --- |
| Process | official item and language definitions; immutable reducer registry; update manager; immutable report and World Intelligence HTTP clients | none of these services contains account payload, queue, or retry state |
| Same world and unlocked kingdom | allowlisted player-castle identity; complete public Storm island/fort observations; anonymous Storm scan-window freshness | account ID and scan contributor; locked kingdoms receive no fact, revision, delta, or query result |
| Account | none | player/session identity, castles, resources, inventory, movements, reports, settings, automation progress, operations, cooldown decisions, targeted verification, action history, and Storm suppression |

The runtime intentionally does not share ordinary kingdom towers, Nomad,
Samurai, Khan, invasion, Berimond, or Rift observations. Their visibility,
progression, cooldown, spawn, or actionability can be account-relative. A
future optimization must split out and prove an objective subprojection before
any part of those records can enter shared state.

The hosted router never selects an account from the URL alone. Authentication
first resolves an account identity from a bearer secret or signed session; the
resolved identity must exactly match `/accounts/{id}`. A mismatch is returned
as not found. Cookies are HTTP-only, same-site, and scoped to one account path.
The frontend derives both HTTP and websocket endpoints from that same shard
prefix.

## Logical state

`GameState` remains the compatibility and feature-facing aggregate. Its public
fields use official concepts such as player, castle, commander, movement,
inventory, map observation, event, and report. Static definitions, localized
names, costs, effects, and images remain in shared `GameData`; state retains
only runtime facts and definition IDs.

An official field is retained only when a current reducer, planner, automation,
API projection, or recovery path reads it. Unknown wire fields remain
available to telemetry but are not automatically promoted into durable state.
Adding a new use case consists of adding the smallest typed official
projection and registering its reducer/component; it does not require storing
the complete official payload.

Official GBC purchase counters are account-private and indexed by their exact
castle and kingdom context. Luna, Auto Buyer, and construction-shop readers
retain independent snapshots instead of competing for one process-global
"last response" slot. Luna refreshes its snapshot only after a purchase flow
or when five minutes have elapsed; Auto Buyer evaluates no faster than every
30 minutes and refreshes counters no faster than hourly.

Map storage is explicitly allowlisted by official object type in
`Server/State/MapProjection.go`. Unknown types fail closed. Each retained type
keeps only the fields consumed for that type and has a bounded retention age.

## Physical state

Each account has one serialized writer. Readers consume immutable generations;
they never observe a partially applied frame. A mutation declares its writable
components before it runs. The store shallow-copies the top-level generation,
then copies only the component, keyed entry, or 1-of-256 shard that the reducer
actually changes. The committed generation reuses that one prepared state
allocation rather than copying it again.

Large keyed domains currently use tenant-native copy-on-write storage:

- account and shared-world map coordinates: official feature kind first, then
  1-of-256 coordinate shards inside that kind;
- movements: 256 shards keyed by movement ID;
- Storm target suppression: sparse account-private shards, with each positive
  observation stored once in the same-world map generation;
- tower cooldowns: 256 shards;
- tower queues: per castle;
- reports: 256 shards keyed by message ID;
- event scores/activity/ranking: 256 shards keyed by event ID;
- castles and inventory: selective keyed/part mutation;
- attack analytics: selective slice mutation.

The public `GameState` shape is materialized only at compatibility boundaries
that genuinely need it. Hot backend readers use lookup/range accessors and do
not rebuild whole maps. Adding another high-cardinality domain should follow
the same pattern: retain the official logical type, add a hidden immutable
generation, expose lookup/range/set/delete operations, emit keyed changes, and
partition persistence by the same identity.

Partition versions use fixed immutable slots for the standard account/session
capabilities and a copy-on-write fallback for uncommon kingdom, castle, or
custom scopes. This preserves intent dependency validation without rebuilding
or formatting the entire partition table on each revision.

Targeting applies to every state domain, not only Storm. Official envelopes
that contain several logical projections run as independently owned reducer
steps, so a castle response cannot mark Player, Inventory, Market, or another
unchanged component dirty merely because those values arrived under the same
opcode. Keyed mutation logs identify exact map coordinates, castle parts,
inventory collections, movements, reports, event IDs, cooldowns, and queues;
bounded equality checks remove unchanged small projections from the remaining
conservative multi-component write sets.

Automation uses an indexed domain-to-policy wake table. A policy evaluates
only after one of its declared domains changes, its relevant configuration or
session changes, or its own deliberate deadline expires. Disabled and
state-gated policies are event-only rather than being rescanned by the
two-second scheduler tick. Expensive feature reads use an exact identity or
official feature partition and occur during that feature's evaluation or
immediately before its intent. The intentional broad-read exceptions are an
official account baseline/game refresh, bounded maintenance retention, and the
single cooperative Storm coverage cycle described below.

## Shared world map

The process-wide `WorldMapStore` contains only allowlisted objective facts.
Player-castle sharing requires a positive owner and retains only its public
identity projection. Storm islands and forts retain their complete public GAA
projection once per world and kingdom: coordinate, type, level, object/island
identity, owner, victory count, cooldown observation, and observation time.
The account store holds only a sparse negative overlay for targets that this
account consumed or rejected; one account's action never removes another
account's target or mutates the shared fact.

Same-world account generations reference the same immutable map generation
plus their private overlay. Different worlds never share a generation, and a
same-world account cannot query or receive changes for a kingdom it has not
unlocked. Inaccessible shared changes silently advance only its hidden shared
pointer, producing no account revision or frontend event. Shared facts persist
in a WAL-backed SQLite store using grouped dirty-coordinate commits.

## Cooperative Storm sensing

Storm sensing is a process-level core policy rather than work owned by the
private Auto Storm toggle. Every connected account that has a Storm castle can
contribute read-only map queries; its private setting still exclusively
controls whether it attacks, skips cooldowns, ships resources, or spends
anything.

The current known Storm area is divided into 25 bounded windows. A stable,
anonymous roster assigns windows by deterministic modulo partition. With four
capable tenants in a process sized for six, the four active tenants receive
6, 6, 6, and 7 disjoint windows, so the map is covered once every two hours
instead of four times. Process capacity is irrelevant; only currently capable
participants divide the work.

Each assignment is a two-minute expiring lease. A participant first registers
a heartbeat, the roster settles briefly, and then only its exact windows are
sent as GAA queries. Successful completion marks those windows fresh and
removes stale Storm facts only when they were omitted from those exact
authoritative windows. A failed or stopped account releases its leases;
otherwise they expire and another participant receives the work. Window
freshness is persisted, while participant IDs, lease ownership, and contributor
identity exist only in process memory and are never published or written to
SQLite.

Coverage geometry and tile keys are cached in the immutable shared-world
generation. Each returned tile updates only its Storm-kind coordinate shards
and emits a backend progress domain that wakes no automation policy. The
dashboard does not rebuild the complete Storm projection for intermediate
tiles. Completion emits one `storm-scan` event, refreshes the client aggregate
once, and lets private policies evaluate the completed shared view once.

Accounts can act on useful shared observations before every window completes.
A targeted refresh and all private preconditions still run for the acting
account before a command is sent. If there is no process shared-map service,
the desktop-compatible legacy full scan remains a bounded fallback; normal
desktop N=1 and tenant compositions use the same coordinator.

## Frame and revision flow

```text
account websocket frame
  -> decode and assign ingress ID
  -> validate captured account/session/focus context
  -> run the registered reducer with an explicit component write set
  -> copy only dirty physical partitions
  -> commit one immutable account generation and partition versions
  -> publish a sparse component/key delta
  -> satisfy exact-frame waiters
  -> group dirty durable partitions for persistence
```

Successful frames without a retained projection still satisfy exact response
waiters and can advance ephemeral focus context. They do not create a state
revision merely to record that a frame existed. Only the small opcode set in
`Server/State/ProtocolProjection.go` is retained as durable protocol
observation state.

## Frontend transport

The initial account connection receives one projected snapshot. Subsequent
`state.changed` events carry sparse component patches, including keyed changes
for map coordinates, castles, inventory entries, movements, and event scores.
Backend-only partitioned domains are omitted or reduced to compact dashboard
aggregates. The React store applies contiguous deltas to its account-local view
and performs a full resync only when the stream reports or reveals a gap; it
does not issue a full-state HTTP request after every revision.

Backend-only collections and fields are removed by the client projection. The
frontend can never subscribe to another account's state stream because its
HTTP handler, websocket hub, authentication identity, route shard, and
application instance are all bound at the same supervisor boundary.

## Persistence

Account state uses a component manifest under `State/Components`. Mutations are
group-committed on a fixed two-second window. Only dirty components are encoded;
partitioned domains rewrite only dirty IDs, castle parts, inventory parts, or
physical shards. A legacy `State/GameState.json` remains a supported migration
source, but normal runtime persistence never clones and serializes the full
aggregate.

The first component save writes one complete manifest so recovery is atomic.
Later manifests can reference unchanged component files from earlier
revisions. Files are written with atomic replacement and restrictive account
data permissions.

## Live hosted reconciliation

Desktop startup remains unchanged when neither `--hosted` nor
`--tenant-config` is present. This is the ordinary N=1 composition and keeps
the existing profile root, browser behavior, localhost API, and dashboard URL.

`--hosted` selects an initially empty N=0 worker cell. The process reads the
orchestrator bearer secret from `CITADEL_TENANT_ORCHESTRATOR_TOKEN` (or the
explicit environment-variable name), creates no root account API, and exposes
only these private control routes:

```text
GET    /orchestrator/v1/status
GET    /orchestrator/v1/events
POST   /orchestrator/v1/reconcile
PUT    /orchestrator/v1/runtimes/{runtimeId}/dashboard-grant
PUT    /orchestrator/v1/runtimes/{runtimeId}/login
DELETE /orchestrator/v1/runtimes/{runtimeId}/login
POST   /orchestrator/v1/runtimes/{runtimeId}/reconnect
```

`POST …/reconnect` forces a fresh game connection for a running runtime now —
the Account Center "Reconnect" action — bypassing a scheduled retry, cooldown
wait, or login park; it answers `202` or `409 reconnect_refused` with a stable
reason (`login_parked`, `reconnect_scheduled`, `transport_unavailable`,
`session_start_failed`) and never touches the runtime otherwise.

`PUT …/login` installs the game username, password, server code, and language
the backend holds for the account into one exact placement (epoch-fenced,
no-store, decoded once into the runtime's protected saved-login file and never
echoed or logged); the desired session restarts so the credential takes effect
immediately. `DELETE …/login` scrubs every saved login for that runtime from
the cell, also after the runtime was drained. Neither the reconcile body nor
any status or event payload ever carries a credential.

Reconciliation submits the complete desired runtime set with a monotonically
increasing desired revision. Every assignment carries an opaque tenant ID, an
independently monotonic placement epoch, a bounded placement lease, and the
desired game-session state. A stale revision or epoch fails closed.

Runtimes are long-lived: an account runtime exists from the first reconcile
that names it until a reconcile omits it (or the cell process stops), i.e. for
as long as the user keeps the hosted account enabled in the backend's desired
state. `startSession: false` parks only the game session (suspend) and keeps
the runtime, its data, and its dashboard shard. The placement lease bounds the
credential-bearing side channels of a placement, not the runtime: while it is
current the runtime may publish private metrics under its grant; once it
lapses the publisher stops spending the grant and reports
`waiting-for-placement`, dashboard grants keep their own expiry, and status
reports `placementLease: "lapsed"` until the next reconcile renews it. A lapsed
lease never stops the game session, drains the runtime, or interrupts a
sibling. Adding, renewing, or draining one assignment never reconstructs the
supervisor or another `App.Application`.

While a runtime is enabled its game session is meant to stay connected. Every
interruption is followed by a reconnect after the user's relog delay
(`session.reconnect.relogDelaySec`, Settings → Relog delay, 1 minute to 24
hours, default 5 minutes — also the "downtime" a user grants their own client
before the runtime takes the session back). The only exceptions are login
outcomes that retrying cannot fix: `LOGIN_COOLDOWN_ACTIVE` waits out the
game's cooldown plus the relog delay; a temporary suspension (`IS_BANNED` with
a remaining time) reports `suspended` and resumes automatically when it ends
plus the relog delay; an unknown login code retries with the relog delay
doubling per repeat, capped at one hour; invalid credentials, a wrong server
selection, a permanent suspension, or a deactivated account park the session
in `error` until the saved login or server selection changes (a changed login
restarts it at once, an unchanged one is refused instead of being spent again).
Stopping the account (omitting it from the desired set) shuts the runtime
down and frees its slot; re-enabling it starts a fresh runtime on the same
profile and installs the stored credential; `startSession: false` parks only
the session and keeps the runtime and its dashboard shard.

Each assignment also carries `onDisconnect`. `hold` is the behaviour above —
the runtime owns the wait, as the desktop always does. `release`, the hosted
default when the field is omitted, makes the runtime's lifetime follow the
game connection instead: a plain drop gets a short
immediate retry window (three attempts five seconds apart), and any wait
beyond that — the relog delay, a login cooldown, a suspension, or a login that
needs the user — is reported as session state `released` with the earliest
sensible `retryAt` (`cooldownUntil` / `suspendedUntil`, never sooner than the
relog delay; no `retryAt` when only a changed login can help), after which the
runtime stops its own loop and refuses an unforced `Start` before that time.
The control plane persists the wait, drains the runtime so its slot serves
another account, and creates a fresh runtime when the time elapses, when the
user presses Reconnect, or when a rotated credential arrives. A 5-minute
cooldown therefore costs 5 minutes of no runtime, and a 24-hour suspension
saves 24 hours of hosting. Both policies expose the same `session.reconnect`
intent behind the Command Center's Reconnect button, which retries at once
regardless of the timer.

The dashboard is an asynchronous control panel over that long-lived runtime,
not part of it. Every command enters as an intent that the runtime accepts and
then executes under its own lifetime: `POST /accounts/{id}/api/v2/intents/{name}`
answers `202` with the accepted receipt, progress and completion stream as
`operation.changed`, and a closed tab, sleeping laptop, dropped connection, or
gateway timeout never cancels a running operation (only an explicit cancel or
the runtime stopping does). Observation is push-based and non-blocking, so a
slow or absent dashboard cannot stall ingest, automation, or publishing.

Because a lapse no longer self-fences the worker, relocation is a control-plane
responsibility: reconcile the runtime out of the old cell (or send
`startSession: false`) and confirm the response before placing the same game
account elsewhere. If the old cell is unreachable, its runtime keeps its
session until it can be reached again; the game's single-session rule and the
login cooldown bound the overlap, and the first reconcile after reconnection
drains it.

The first runtime dynamically joins the process-owned GameData manager,
immutable reducer registry, update manager, cloud HTTP clients, and allowlisted
WorldMapStore. Every later runtime receives those same safe shared service
pointers while constructing a separate state store, session transport, intent
engine, automation coordinator, configuration, operations database, report
database, persistence workers, API server, and profile lease under
`Data/Accounts/{runtime-id}`.

Dashboard connection tokens are a separate contract from the orchestrator
secret. The backend installs a short-lived grant for one exact runtime and
placement epoch. The worker retains only its SHA-256 digest. The browser posts
the grant in a no-store login body, never a URL, and receives a signed,
HTTP-only, SameSite cookie scoped to `/accounts/{runtime-id}/`. Rotating the
grant increments that runtime's session generation and invalidates old
cookies. A token for runtime A cannot authenticate runtime B, enumerate it, or
enter the orchestrator routes. The React client derives HTTP, SSE, and
WebSocket URLs from the authenticated account shard.

Private metrics use a third credential class. Dynamic hosted mode enables the
publisher only when `--tenant-private-metrics-url` or
`CITADEL_PRIVATE_METRICS_URL` supplies an internal CitadelOpsBackend ingest
endpoint. Every reconciled runtime must then carry a unique, short-lived
`privateMetrics` grant whose expiry is no later than its placement lease. The
grant is held only in memory, sent only as the outbound bearer header, omitted
from the sample body and all status/event payloads, and cannot be reused by a
sibling runtime or as a dashboard grant. Static canaries and the default N=1
desktop remain non-publishing unless they later receive an explicit equivalent
identity contract.

The runtime never receives database credentials. Its account-owned publisher
waits for an authoritative binding (`account UID + canonical world + player
ID`), a logged-in/socket-ready session, matching generation and baseline, and
the current connection generation. It then sends one merged private sample:

- complete My Stats values, including zeroes, troops by unit, and currencies;
- bounded 24-hour, 7-day, 30-day, and all-history Feature Stats rollups;
- current private event scores and activity totals; and
- a separate allowlisted public-candidate projection for backend provenance
  resolution.

Uploads are idempotent and carry the exact tenant, runtime, cell, desired
revision, placement epoch, and lease. The publisher classifies every backend
answer before retrying:

- transient failures (network errors, timeouts, `408`, `429`, `5xx`) replay
  the identical pending sample under the same idempotency key, spaced by the
  publish interval doubled per failure, capped at eight intervals, jittered,
  and never shorter than a `Retry-After` hint;
- durable rejections (any other `4xx`, for example a stale epoch or an invalid
  body) drop that sample so the next cadence publishes a fresh one; a rejected
  sample can never block the runtime until the next reconcile;
- a refused grant (`401`/`403`) drops the pending sample, reports
  `grant-rejected`, and holds the credential for the longest backoff so a
  revoked grant is not hammered; a placement rotation clears the hold at once.

State changes only pull the first publication forward; they never bypass a
retry backoff or the steady cadence. Rotating the placement discards pending
work from the old epoch and continues on the regular cadence under the new
grant instead of bursting an extra upload from every runtime in the cell. One
attempt (sample build plus upload) is bounded by a timeout.

Each runtime keeps a compact in-memory projection of its attacker report
history keyed to the analytics store's process-local write generation, so a
steady-state sample rolls its feature windows from memory and re-reads the
store only after a report was actually written. CitadelOpsBackend may upsert
the merged private current row and append sample history, then promote only
allowed public fields into World Intelligence. World Intelligence remains
backend-owned and continues operating when no account runtime is present.

Under the same placement grant the runtime also publishes **dashboard
checkpoints** to `--tenant-dashboard-checkpoint-url` (which requires the
metrics endpoint): the `state.snapshot` client projection, the configuration
snapshot, the last hundred operation receipts, the sanitized session
situation, and the bound identity, gzip-compressed and idempotent by
checkpoint ID. Cadence is every five minutes while the revision changes,
promptly (two-second debounce) after a session transition, and once more —
synchronously and bounded — before the runtime is drained. The backend keeps
the latest checkpoint per account so the frontend can render the dashboard
while no runtime exists. Status reports `checkpointState`, `checkpointAt`, and
`checkpointRevision`. `--tenant-dashboard-origins` allowlists the frontend's
browser origin(s) for live access to account shards and the tenant login
(CORS with credentials, preflights answered); same-host requests always work
and every other origin is refused.

Existing local My Stats history and report analytics remain a safety fallback
until backend ingestion and tenant-authorized read APIs are implemented and
verified. Enabling the publisher alone does not delete local data or make the
backend copy authoritative.

The control status/event payload is deliberately sanitized: runtime and tenant
locators, placement epoch/lease and whether the lease is active or lapsed,
capacity, game-data readiness, session state, login/socket readiness,
generation counters, the current login obstacle (`cooldownUntil`, `retryAt`,
and `loginFailure: {code, class, fatal, observedAt, suspendedUntil?}`), the
disconnect policy in force, whether a saved login is installed and for which
server, private-metrics state/last successful publish time, and dashboard
checkpoint state/time/revision only. It contains no dashboard
grant, orchestrator secret, password, raw game traffic, resources, inventory,
reports, configuration, player name, or other account payload.

## Static canary manifest

Tenant manifests contain metadata and environment-variable names, never
secrets:

```json
{
  "version": 1,
  "maxAccounts": 8,
  "sessionKeyEnv": "CITADEL_TENANT_SESSION_KEY",
  "accounts": [
    {
      "id": "account-a",
      "tokenEnv": "CITADEL_ACCOUNT_A_TOKEN",
      "startSession": true
    }
  ]
}
```

Run with `--tenant-config <path>` for a startup-only local/canary composition;
it is intentionally separate from dynamic `--hosted` reconciliation. Hosted accounts are background-only and use
their own protected saved game login under
`Data/Accounts/{account-id}`. `maxAccounts` is a process-group safety boundary,
not a promise that every workload fits the same group. Production sizing must
be based on replay/live CPU, allocation, heap, and websocket measurements; a
heavy account can be moved to another bounded group without changing its data
or API shape.

## Extension checklist

When adding official game state:

1. Prove a current consumer and model only its official identity and fields.
2. Assign it to the narrowest existing component or add one component.
3. Register every reducer with an explicit write set.
4. Use keyed or sharded copy-on-write storage when cardinality or update rate
   can grow; never deep-clone the account aggregate.
5. Expose bounded lookup/range methods for backend readers.
6. Emit a keyed client delta and persist the same dirty partition.
7. Decide explicitly whether every projected field is account-private,
   same-world objective, or process-global static data. Default to private;
   mixed records must be split into a shared fact and a private overlay.
8. Add isolation, immutable-generation, delta, persistence-recovery, and
   allocation benchmarks before enabling it for hosted accounts.
9. For shared sensing, persist only anonymous coverage, use expiring leases,
   reassign stopped participants, and prove disjoint complete coverage in
   tests. Never use private automation settings to decide whether the process
   can collect a public read-only fact.

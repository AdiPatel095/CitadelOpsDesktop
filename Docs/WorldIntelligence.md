# World Intelligence

World Intelligence is CitadelOps' own shared, world-scoped dataset. It does not call, scrape, or import GGE-Tracker. Designated owned CitadelOps accounts read the same public GGE `hgh` leaderboards used by GGE-Tracker, contribute bounded observations to the unified CitadelOps backend, and every desktop reads the merged dataset through the local CitadelOps API.

## Data flow

1. A designated collector uses the ordinary CitadelOps intent engine to read six public might brackets (`LT=6`, `LID=1..6`) and the public weekly-loot board (`LT=2`, `LID=1`). Pages are requested ten ranks at a time through the account's existing authenticated game socket.
2. The scanner validates every response, stops at the server-reported row count, caps each category at 50,000 players, and discards the entire partial scan if a page fails. Player details embedded in the leaderboard rows supply public identity, alliance, metrics, and holdings without a world-map sweep.
3. It normalizes the world host and creates deterministic SHA-256 batches for one 15-minute capture bucket. Players, alliances, and holdings are split at the cloud's existing per-batch limits.
4. A local SQLite outbox at `Runtime/WorldIntelligence.sqlite` durably queues each batch. Each installation has a random local ID and secret; there is no player credential in the cloud authorization header.
5. The uploader registers that installation, sends idempotent batches, and retries transient failures with bounded backoff.
6. The Cloud Run service appends 15-minute player and alliance observations in PostgreSQL, records holding history only when a public holding changes, and maintains merged current snapshots for search, rankings, and profiles.
7. The World Intel desktop view and Alliance Targets query the cloud through CitadelOps' local HTTP API.

World Intelligence reads are always enabled and have no consent switch or feature gate. Collection is operationally isolated through hidden profile assignment fields: `collectorPlayerId`, `collectorSlot`, and `collectorSlots`. With two owned accounts, slots `0/2` and `1/2` alternate on global 15-minute boundaries, so each account scans every 30 minutes while the shared dataset receives one scan every 15 minutes. With one owned account, slot `0/1` scans every 15 minutes. Unassigned installations send no leaderboard traffic.

## Privacy boundary

Uploaded fields are limited to public world identity and observations:

- player ID, name, alliance membership, level, legend level, might, glory, honor, and weekly loot;
- alliance ID, name, member count, and total might;
- publicly visible alliance holdings: owner, castle ID, kingdom, coordinates, and slot type;
- public event-alliance ranking, score, member count, and fame values;
- observation timestamps and a normalized world host.

The collector does not serialize account UID, login/session credentials, resources, currencies, troops, inventory, commanders, equipment, movements, reports, raw frames, chat, protection state, or automation configuration. It stores only decoded public leaderboard fields. Unassigned profiles are read-only by default, while collector assignment is bound to the expected logged-in player ID.

## Cloud API

The service exposes:

- `GET /api/world-intel/health`
- `POST /api/world-intel/v1/installations`
- `POST /api/world-intel/v1/observations` with `Authorization: CitadelInstall <installation-id>.<secret>`
- `GET /api/world-intel/v1/search?worldId=&q=&type=&limit=`
- `GET /api/world-intel/v1/players/{id}?worldId=&limit=`
- `GET /api/world-intel/v1/alliances/{id}?worldId=&limit=`
- `GET /api/world-intel/v1/rankings/{players|alliances}?worldId=&metric=&limit=`
- `GET /api/world-intel/v1/coverage?worldId=`

Observation requests are capped at 4 MiB, reject unknown JSON fields, enforce per-entity limits and a six-month time window, and verify the deterministic payload digest. Inserts are idempotent by batch ID. Current projections reject older observations from overwriting fresher metrics.

## Deployment

World Intelligence leaderboard ingestion ships in CitadelOpsBackend 1.3.11 and runs inside the existing `live-backend-cloud` Cloud Run service. It reuses that service's Cloud SQL connection and public `citadelops.app` domain; there is no second backend service, database, deployment file, or database secret.

Backend startup creates the additive `world_intel_*` tables and indexes in a PostgreSQL transaction. If the schema cannot be initialized, the new Cloud Run revision fails closed and the previous healthy revision continues serving. The normal CitadelOpsBackend `main` promotion and Google Cloud Build trigger deploy the API together with the report and licensing routes.

The desktop production default is `https://citadelops.app/api/world-intel/v1`. For development, set `CITADEL_WORLD_INTEL_URL` to another compatible base URL.

Reads and installation registration are public but use the backend's per-IP rate limiter; only ingestion requires an installation secret. Keep the shared database private, rotate its credentials, enable backups and point-in-time recovery, and apply retention and abuse-review policy before broad enrollment.

Installation authentication prevents anonymous batch writes but does not prove that a public observation is truthful. Treat the initial dataset as community-observed rather than authoritative; before broad public enrollment, add abuse review and contributor reputation or multi-install corroboration so one registered installation cannot poison rankings.

## Coverage behavior

The UI shows entity counts, total observations, first/last observation time, collector-slot status, scan progress, queue depth, and per-row freshness. Rankings use the freshest merged observation, profiles retain append-only history, and holdings come from the most recent completed leaderboard scan. Sparse coverage is expected only until the first complete owned-account scan reaches the cloud.

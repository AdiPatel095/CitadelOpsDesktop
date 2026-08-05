# World Intelligence

World Intelligence is CitadelOps' own shared, world-scoped dataset. It does not call, scrape, or import GGE-Tracker. Desktop installations contribute bounded public observations to the unified CitadelOps backend, and desktop queries read the merged dataset back through the local CitadelOps API.

## Data flow

1. The desktop collector observes already-decoded public game state.
2. It normalizes the world host, removes private state, and creates a deterministic SHA-256 batch every 15-minute capture bucket or when public data changes.
3. A local SQLite outbox at `Runtime/WorldIntelligence.sqlite` durably queues the batch. Each installation has a random local ID and secret; there is no player credential in the cloud authorization header.
4. The uploader registers that installation, sends idempotent batches, and retries transient failures with bounded backoff.
5. The Cloud Run service appends immutable observations in PostgreSQL and maintains merged current player and alliance rows for search, rankings, and profiles.
6. The World Intel desktop view and Alliance Targets query the cloud through CitadelOps' local HTTP API.

## Privacy boundary

Uploaded fields are limited to public world identity and observations:

- player ID, name, alliance membership, level, legend level, might, and glory;
- alliance ID, name, member count, and total might;
- publicly visible alliance holdings: owner, castle ID, kingdom, coordinates, and slot type;
- public event-alliance ranking, score, member count, and fame values;
- observation timestamps and a normalized world host.

The collector does not serialize account UID, login/session credentials, resources, currencies, troops, inventory, commanders, equipment, movements, reports, raw frames, chat, protection state, or automation configuration. Collector tests marshal the final payload and guard this exclusion list. World Intelligence and public contribution have separate desktop toggles; both default off for 2.1.0, and contribution can be disabled later without deleting the local installation identity.

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

World Intelligence ships in CitadelOpsBackend 1.3.10 and runs inside the existing `live-backend-cloud` Cloud Run service. It reuses that service's Cloud SQL connection and public `citadelops.app` domain; there is no second backend service, database, deployment file, or database secret.

Backend startup creates the additive `world_intel_*` tables and indexes in a PostgreSQL transaction. If the schema cannot be initialized, the new Cloud Run revision fails closed and the previous healthy revision continues serving. The normal CitadelOpsBackend `main` promotion and Google Cloud Build trigger deploy the API together with the report and licensing routes.

The desktop production default is `https://citadelops.app/api/world-intel/v1`. For development, set `CITADEL_WORLD_INTEL_URL` to another compatible base URL.

Reads and installation registration are public but use the backend's per-IP rate limiter; only ingestion requires an installation secret. Keep the shared database private, rotate its credentials, enable backups and point-in-time recovery, and apply retention and abuse-review policy before broad enrollment.

Installation authentication prevents anonymous batch writes but does not prove that a public observation is truthful. Treat the initial dataset as community-observed rather than authoritative; before broad public enrollment, add abuse review and contributor reputation or multi-install corroboration so one registered installation cannot poison rankings.

## Coverage behavior

The UI treats sparse data as expected: it shows entity counts, total observations, first/last observation time, and per-row freshness. Rankings use the freshest merged observation, profiles retain append-only history, and alliance rosters/holdings come from the latest complete alliance observation. Until enough desktops encounter a player or alliance, search and rankings can be incomplete without being considered an error.

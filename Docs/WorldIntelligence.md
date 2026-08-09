# World Intelligence

World Intelligence preserves world-player history and now collects versioned ranking, event, and reward catalogs from the official Goodgame Studios CDN. The catalog source is the same `items_v{version}.json` document used by CitadelOps' official game-data subsystem:

`https://empire-html5.goodgamestudios.com/default/items/items_v{version}.json`

The catalog is public, global, and independent of a logged-in game session. Existing player, alliance, holding, and ranking observations remain queryable as historical data, but the scheduled collector no longer issues game-socket leaderboard commands.

## Catalog collection

1. CitadelOps downloads and verifies the current official item document through its existing game-data manager.
2. A designated World Intelligence profile reads the verified in-memory collections. It does not read a game socket, require a synchronized baseline, or call a third-party tracker.
3. Each supported collection is compacted and independently identified by SHA-256. Identity excludes collector and capture time, so identical CDN content deduplicates across profiles.
4. A separate SQLite outbox at `Runtime/WorldIntelligence.sqlite` durably queues each dataset snapshot and retries uploads with bounded backoff.
5. The backend stores immutable dataset versions in PostgreSQL and records which installations contributed each version.
6. Every desktop can list all collected datasets and render the latest rows plus version history through its local API.

The allowlist includes official collections for:

- Storm alliance and player thresholds (`islandrewardranks`, `islandPlayerRewards`);
- leagues, highscores, might and alliance-fame ranks, temporary-server ranks, alliance battleground ranks, and leaderboard reward brackets;
- gacha events, pull limits, spin costs, lucky-wheel reward sets, and sale-day wheel reward sets;
- event definitions, point-event scoring quests, collector-event scoring, and their reward sets;
- season, promotion, donation, Daimyo, activity, and reward-bag definitions.

CitadelOps also emits `rankingReferencedRewards`, a deterministic subset of the official `rewards` collection containing reward records referenced by the selected ranking and event datasets. This makes reward IDs usable without uploading unrelated reward definitions.

## What the official document does and does not contain

The document contains static public definitions: rank thresholds, event and league IDs, scoring requirements, gacha pull bounds and costs, reward brackets, and reward contents. It does not contain current player names, current Storm scores, current glory or gallantry, or a player's spins in an active gacha event. The UI labels these rows as official catalog data and does not present them as live player observations.

## Collector assignments

Reads are always available. Scheduled writes are controlled by the hidden `world-intelligence` settings:

- James Holden: `collectorPlayerId=17756610`, slot `0/2`;
- Adolphus Murtry: `collectorPlayerId=17334928`, slot `1/2`;
- Amos Burton: unassigned with `collectorPlayerId=0` and `collectorSlots=0`.

The assignment is profile-local and no longer depends on the currently logged-in player or socket state. Unassigned profiles do not enqueue or upload catalog snapshots.

## Privacy boundary

Catalog uploads contain only official CDN data and provenance:

- official item version, URL, and document digest;
- dataset key, label, category, fields, rows, row count, and dataset digest;
- capture time and the configured collector player ID.

They do not contain login credentials, session tokens, raw game frames, account UID, resources, troops, inventory, commanders, equipment, movements, reports, chat, or automation settings. Installation authentication uses a random local installation ID and secret.

## Cloud API

Catalog endpoints:

- `POST /api/world-intel/v1/catalog-snapshots` with `Authorization: CitadelInstall <installation-id>.<secret>`;
- `GET /api/world-intel/v1/catalog-datasets`;
- `GET /api/world-intel/v1/catalog-datasets/{key}?historyLimit=`.

The existing historical endpoints remain available:

- `GET /api/world-intel/v1/search?worldId=&q=&type=&limit=`;
- `GET /api/world-intel/v1/players/{id}?worldId=&limit=`;
- `GET /api/world-intel/v1/alliances/{id}?worldId=&limit=`;
- `GET /api/world-intel/v1/ranking-metrics/{players|alliances}?worldId=`;
- `GET /api/world-intel/v1/rankings/{players|alliances}?worldId=&metric=&limit=`;
- `GET /api/world-intel/v1/coverage?worldId=`.

Catalog requests reject unknown JSON fields, accept only the official GGS item-CDN host, enforce bounded rows and bytes, verify source and dataset SHA-256 digests, and validate the deterministic snapshot ID. Backend inserts are additive and idempotent.

## Deployment

The API runs inside the existing CitadelOps backend and uses its existing Cloud SQL connection. Schema startup creates the additive `world_intel_catalog_*` tables and indexes transactionally. Deployment uses the normal repository promotion flow: feature branch to `develop`, then approved `develop` to `main`; the Git-triggered Cloud Build deploys the backend. No direct GCP mutation is required.

The desktop production endpoint remains `https://citadelops.app/api/world-intel/v1`. Set `CITADEL_WORLD_INTEL_URL` only for a compatible development backend.

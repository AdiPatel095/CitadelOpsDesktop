# World Intelligence

World Intelligence is a read-only cloud view inside CitadelOps Desktop. The desktop queries shared public rankings, search results, player and alliance profiles, historical graphs, personal records, coverage, and official definition references. It does not collect or upload World Intelligence data.

## Desktop boundary

The interactive desktop runtime has no World Intelligence scheduler, collector assignment, GGE ranking scanner, socket command reader, local World Intelligence SQLite store, installation credential, observation outbox, catalog outbox, or uploader. Opening the view cannot add game traffic or collector memory/storage pressure to the desktop process.

The desktop exposes only unauthenticated read requests to the configured World Intelligence backend. It derives the current world from the connected account so the shared directory opens on the correct server.

## Update subscription model

Desktop clients and hosted tenants keep one lightweight server-sent event subscription per active world. That stream broadcasts a cumulative revision vector for coverage, rankings, profiles, and event runs. The backend polls each active world's revision once and fans the same update out to every subscriber on that service instance, with a 30-second heartbeat.

The currently selected event leaderboard has its own subscription. A new subscriber always receives every stored row in one complete current leaderboard snapshot as its base; the subscription is not subject to the bounded REST query limit. The backend caches that base once per active leaderboard, recomputes it once when the world's event-run revision changes, and broadcasts only row upserts and removals thereafter. A slow subscriber that may have missed a delta receives a new complete snapshot before continuing, so deltas are never applied to an uncertain base.

On an event-run revision, the client refreshes the lightweight run directory but does not fan out full requests for historical runs. If either stream is temporarily unavailable, the desktop retains the last usable data and falls back to a bounded 30-second coverage, directory, and reference-ranking check plus a REST refresh only when the selected leaderboard is stale.

Coverage is deliberately lower priority than the selected leaderboard. Desktop requests delay it until the event directory has had a chance to render and cancel it after five seconds, preventing an older backend's expensive coverage query from consuming the full general request timeout or delaying the primary ranking view.

## Dedicated collector boundary

Collection belongs in separate lightweight executables running only the accounts assigned to public-data collection. Those executables should own their connection lifecycle, collector credentials, scheduling, retry policy, transient response handling, normalization, batching, and uploads. They must not reuse an interactive desktop profile or credentials.

The collector executables are a separate implementation and deployment boundary. Removing the desktop collector does not itself create or provision those processes.

## Full player-level Storm collection

The target Storm dataset is player-level across the participating map, not only the alliance leaderboard:

1. Fully enumerate the public Storm alliance leaderboard with `hgh`, list type `13`.
2. Read each returned alliance roster with `ain`.
3. Read each roster member's Island Kingdom performance with `gpe`, using event ID `102`.
4. Keep players with current Storm participation and upload their public profile plus the event metrics.

The player event payload exposes cargo points as `AMT` and the following `PST` metric IDs:

- `15`: Aquamarine collected;
- `16`: Aquamarine from resource islands;
- `17`: Aquamarine from Storm fortresses;
- `18`: Aquamarine from PvP;
- `19`: Aquamarine spent on cargo ships;
- `20`: Aquamarine lost to players;
- `21`: first-points timestamp.

The backend read model retains source, event ID, metric ID, observation time, and validity window. That lets the desktop rank current values while preserving historical graphs and records.

## Other public rankings

Standalone collectors may also populate player might, weekly loot, glory, honor, alliance might, and other public highscore categories. The UI discovers populated ranking metrics from the backend instead of hard-coding a collector schedule into the desktop.

## Official definition references

Static definition datasets come from the official Goodgame Studios item document:

`https://empire-html5.goodgamestudios.com/default/items/items_v{version}.json`

These definitions contain rank thresholds, event IDs, scoring requirements, reward brackets, and reward contents. They do not contain current player names, ranks, Storm scores, or active event progress. Dedicated collectors may snapshot and upload the selected official datasets; the desktop only reads the resulting catalog.

## Privacy boundary

The shared service accepts only public player/alliance observations and official catalog data. Collector implementations must never upload login credentials, session tokens, raw frames, private resources, troops, inventory, commanders, equipment, movements, reports, chat, or automation settings.

## Desktop cloud API

The desktop uses these read endpoints:

- `GET /api/world-intel/v1/search?worldId=&q=&type=&limit=`;
- `GET /api/world-intel/v1/players/{id}?worldId=&limit=`;
- `GET /api/world-intel/v1/alliances/{id}?worldId=&limit=`;
- `GET /api/world-intel/v1/event-runs?worldId=&eventKey=&limit=`;
- `GET /api/world-intel/v1/event-runs/{occurrenceId}/rankings?worldId=&listType=&leagueId=&limit=`;
- `GET /api/world-intel/v1/event-runs/{occurrenceId}/subscribe?worldId=&listType=&leagueId=` (uncapped complete base followed by row deltas);
- `GET /api/world-intel/v1/players/{id}/event-scores?worldId=&eventKey=&occurrenceId=&limit=`;
- `GET /api/world-intel/v1/ranking-metrics/{players|alliances}?worldId=`;
- `GET /api/world-intel/v1/rankings/{players|alliances}?worldId=&metric=&limit=`;
- `GET /api/world-intel/v1/coverage?worldId=`;
- `GET /api/world-intel/v1/subscribe?worldId=` (server-sent revision stream);
- `GET /api/world-intel/v1/catalog-datasets`;
- `GET /api/world-intel/v1/catalog-datasets/{key}?historyLimit=`.

The production endpoint remains `https://citadelops.app/api/world-intel/v1`; `CITADEL_WORLD_INTEL_URL` selects a compatible development backend.

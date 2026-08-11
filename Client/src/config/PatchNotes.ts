/**
 * In-app changelog. Append a new release at the top when you ship a version.
 */
export type PatchNoteKind = 'added' | 'fixed' | 'removed' | 'changed' | 'security' | 'deprecated';

export interface PatchNoteItem {
  kind: PatchNoteKind;
  text: string;
}

/** Badge label shown in the UI for each kind */
export const PATCH_NOTE_KIND_LABEL: Record<PatchNoteKind, string> = {
  added: 'Added',
  fixed: 'Fixed',
  removed: 'Removed',
  changed: 'Changed',
  security: 'Security',
  deprecated: 'Deprecated',
};

export interface PatchNotesRelease {
  version: string;
  /** Short subtitle for the release card */
  subtitle?: string;
  /** Optional ISO date string for display */
  date?: string;
  /** Changelog lines, each tagged for badge + color */
  items: PatchNoteItem[];
}

export const PATCH_NOTES_RELEASES: PatchNotesRelease[] = [
  {
    version: '2.2.0',
    subtitle: 'Guarded Auto Buyer controls and unified event-aware World Intelligence',
    date: '2026-08-11',
    items: [
      { kind: 'added', text: 'Auto Buyer can purchase explicitly selected stock-limited items from supported merchants and active event shops, maintain fixed-price specialists at a user-selected floor of at least 14 days, and keep food or ruby feasts above a selected remaining-time floor' },
      { kind: 'changed', text: 'Auto Buyer shop items are now selected through a focused shop dropdown with currency filtering, substring search, official localized package and price names, and a per-reset purchase limit that can be lower than the available stock' },
      { kind: 'security', text: 'Every automatic purchase rechecks its official catalog entry, live stock or timer state, event availability, balances, reserves, ruby ceilings, and configured purchase limit immediately before spending, then verifies the resulting server counter or timer' },
      { kind: 'changed', text: 'Timed account offers remain disabled for unattended purchase until offer-item matching and the server-quoted ruby confirmation can be enforced end to end' },
      { kind: 'added', text: 'World Intelligence now consumes the backend event-run and event-score history contract, including occurrence identity, rank-only observations, score units, public event titles, and level-league metadata' },
      { kind: 'changed', text: 'World rankings now use one event-aware table with permanent Name, Might, Honor, and Alliance columns plus Rank and Score for the selected event; every header is sortable and player or alliance search matches any substring' },
      { kind: 'changed', text: 'Event selection uses human-readable public titles without exposing board IDs, adds Storm Islands rankings, and shows a level-league selector only for events that need one while defaulting to the viewing player\'s current league' },
      { kind: 'removed', text: 'The official definition-catalog browser and separate regular-versus-event ranking tables were removed from World Intelligence in favor of the unified public leaderboard' },
      { kind: 'changed', text: 'Player dossiers now place localized public titles and relevant profile facts in the header, remove raw player IDs and redundant profile or observation-history sections, and add chartable Storm metrics beside Might, Loot, Glory, and Honor' },
      { kind: 'fixed', text: 'Alliance dossiers are restored as full graph-driven views with Combined Might and Members history, 24-hour through all-time ranges, public event activity, holdings, and the observed roster' },
    ],
  },
  {
    version: '2.1.3',
    subtitle: 'Official World Intel catalogs, selectable public datasets, and clearer historical dossiers',
    date: '2026-08-09',
    items: [
      { kind: 'added', text: 'World Intelligence now collects versioned Storm thresholds, ranking and league definitions, gacha pull rules, event scoring, reward brackets, and referenced reward definitions from the same official Goodgame Studios CDN used for CitadelOps game data' },
      { kind: 'added', text: 'The World Intelligence table includes a searchable public-dataset selector and renders every collected official category with source version, row coverage, contributor count, and stored version history' },
      { kind: 'changed', text: 'Only the configured Adolphus Murtry and James Holden profiles contribute official catalog snapshots; Amos Burton remains a read-only viewer, and collection no longer requires a connected or baseline-synchronized game session' },
      { kind: 'removed', text: 'World Intelligence no longer sends game-socket leaderboard commands or builds scheduled observations from local account state' },
      { kind: 'fixed', text: 'Player and alliance dossiers retain a visible back button and full-page navigation while preserved historical observations remain available below the official catalog view' },
    ],
  },
  {
    version: '2.1.1',
    subtitle: 'Reliable Berimond construction, schedule-driven recruiting, corrected tool limits, and Background login recovery',
    date: '2026-08-06',
    items: [
      { kind: 'fixed', text: 'Auto Beri construction now preserves the live Berimond event context, so the built-in complete camp target can continue upgrading every camp and tent family to its official maximum WoD while respecting the selected Stable level' },
      { kind: 'fixed', text: 'Auto Recruit now resolves the active schedule option every time a queue slot opens, enqueues one scheduled stack, and reevaluates before filling another slot instead of falling back to the static configured unit' },
      { kind: 'fixed', text: 'Attack preset tool limits now keep PvE at 30 / 40 / 30, use 40 / 50 / 40 for legendary PvP, and apply the logged-in player\'s official Hall tool bonus only to the flanks, allowing 50 / 50 / 50 when unlocked' },
      { kind: 'fixed', text: 'Background mode now validates and explicitly re-enables an existing protected saved login when selected, offers an in-place recovery action when an earlier Full-mode page close incorrectly disabled it, and reconciles a fresh authoritative baseline when login and bootstrap frames arrive nearly simultaneously' },
    ],
  },
  {
    version: '2.1.0',
    subtitle: 'Community-built world intelligence, cloud rankings, and public player and alliance history',
    date: '2026-08-05',
    items: [
      { kind: 'added', text: 'World Intelligence adds cloud-backed player and alliance search, rankings, profiles, metric history, roster views, holdings, dataset coverage, and freshness indicators without relying on GGE-Tracker' },
      { kind: 'added', text: 'Desktops can contribute bounded public observations through a durable private outbox; account credentials, resources, currencies, troops, inventory, movements, reports, raw game traffic, and protection details are never included' },
      { kind: 'added', text: 'An explicitly opted-in Experimental Battle Prediction Calculator can save low-confidence pre-impact estimates and completed forward-test trials for outgoing PvP attacks you launch; it never launches attacks and clearly warns that its automatic spy missions use game resources' },
      { kind: 'added', text: 'Auto Beri now starts from a built-in complete camp target, resolves every camp and tent family to the current official maximum WoD, and lets each user choose Stable level 1 through 5' },
      { kind: 'changed', text: 'Alliance Targets now uses CitadelOps World Intelligence as its shared discovery source while retaining live in-game inspection for protection status' },
      { kind: 'changed', text: 'World Intelligence cloud access and public-data contribution are visible, independently configurable, and disabled by default until the desktop user explicitly enables them' },
    ],
  },
  {
    version: '2.0.2',
    subtitle: 'Background operation, typed automation, bounded Rift runs, readable diagnostics, and safer event workflows',
    date: '2026-08-05',
    items: [
      { kind: 'added', text: 'Game Connection settings now offer both Full application and Background-only modes. Full mode keeps the complete playable game tab, while Background mode connects directly with substantially lower processor and memory use after one successful Full-mode sign-in' },
      { kind: 'added', text: 'Background-only operation persists the selected startup mode, validates saved non-secret connection metadata, reconnects directly to the game server, and preserves the game\'s application heartbeat without opening Chromium' },
      { kind: 'added', text: 'Rift Maiden probes accept an exact goal from 1 through 9,999, continue across commander-return rounds until that many launches are confirmed, show live progress, and can be cancelled without retracting attacks already sent' },
      { kind: 'added', text: 'Auto Beri can capture durable functional, layout, or exact camp blueprints and build from confirmed returned loot without importing resources from another kingdom' },
      { kind: 'added', text: 'Auto Beri has an optional authoritative Gallantry-booster gate covering transfers, armorer purchases, camp setup, attacks, tools, and construction while preserving the separate booster setting' },
      { kind: 'changed', text: 'Alliance Help is now a core live-session behavior instead of a feature toggle: CitadelOps answers actionable Help All requests even when every automation feature is off while preserving session readiness, deduplication, retry, and Bot Lock safeguards' },
      { kind: 'changed', text: 'Rift Maiden launch, Berimond attack/build/tool lanes, Alliance Help, Storm construction, and other migrated workflows now declare typed resources and scoped read dependencies in the current intent engine' },
      { kind: 'changed', text: 'Auto Food treats the Storm castle as a storage invariant: it keeps Food and Mead at observed capacity, never waits on troop consumption there, prioritizes Mead when both are low, and permits safe residual top-offs' },
      { kind: 'changed', text: 'Auto Invasion, Auto Nomad, and Auto Advisor use the server-advertised event currencies and difficulties, preserving Bot Lock and failing closed when a requested live option is unavailable' },
      { kind: 'changed', text: 'Operation receipts, scoped resource claims, read sets, state persistence, movement timing, and production reconciliation now retain the context required for safe retries and multi-step automation' },
      { kind: 'changed', text: 'Castle, equipment, item, building, troop, tool, currency, effect, and other game identifiers are translated to user-facing names in intent failures, automation status, and activity messages whenever official labels are available' },
      { kind: 'changed', text: 'Existing battle-report history is imported into the current SQLite report store with durable deduplication and compatibility handling' },
      { kind: 'fixed', text: 'Rift Maiden launches use the main castle and current Rift destination consistently, and exact-count runs no longer stop after only the commanders available in the first round' },
      { kind: 'fixed', text: 'Berimond attack admission, target refresh, tool preparation, booster gating, returned-loot construction, and lane isolation no longer block unrelated work or dispatch against stale context' },
      { kind: 'fixed', text: 'Storm food balancing no longer counts or waits for Storm troop consumption and no longer leaves small safe Food or Mead deficits below the ordinary minimum-shipment threshold' },
      { kind: 'fixed', text: 'Session banners, headers, runtime state, and settings now distinguish the configured connection mode from the active mode and clearly show when a restart is required' },
      { kind: 'fixed', text: 'Windows desktop launches now open operation and report databases correctly when the data directory is on a drive-letter or OneDrive path instead of exiting with an invalid SQLite URI authority error' },
    ],
  },
  {
    version: '2.0.1',
    subtitle: 'Material 3 Expressive interface, safer automation recovery, refined logistics, and bounded telemetry',
    date: '2026-08-03',
    items: [
      { kind: 'added', text: 'Material 3 Expressive visual system across the complete dashboard, with new light and dark color themes, tonal surfaces, expressive shapes, adaptive navigation, typography, and motion' },
      { kind: 'added', text: 'PvE and PvP attack-preset types with visible lane tool limits, Hall of Legends tool-bonus support, builder clamping, and authoritative send-time enforcement' },
      { kind: 'added', text: 'App-native rolling 48-hour telemetry retention with three-hour channel-log rotation, safe legacy-log compaction, and rotated-history support for tails and launch counters' },
      { kind: 'added', text: 'Auto Beri armorer restocking for configured wooden tools plus optional MS1 through MS7 troop-transfer skips, with each purchase and skip confirmed before continuing' },
      { kind: 'added', text: 'Auto Storm can explicitly unlock a selected official prebuilt Storm castle and runs Aquamarine shop goals in an independent lane from combat and castle maintenance' },
      { kind: 'changed', text: 'Auto Food Balance uses one-way, storage-aware routing, excludes Berimond from ordinary food logistics, tries alternate safe donors, and resolves selected market travel boosts from each castle’s placed Stable, Faction Stable, or Harbor' },
      { kind: 'changed', text: 'Auto Storm now separates combat, shop, and build decisions; revalidates targets, resources, troop imports, construction state, and time-skip inventory before each committed action' },
      { kind: 'changed', text: 'Auto Bird uses fresh per-castle troop inventory, cycle-scoped claims, and explicit stale-tracking cleanup so one castle’s recovery does not block unrelated automation' },
      { kind: 'changed', text: 'Auto Nomad keeps separate Nomad and Samurai attack presets while preserving legacy preset settings during migration' },
      { kind: 'changed', text: 'Automation retry, cooldown, focus, transport, and stale-plan handling is scoped to the affected lane or castle instead of globally delaying unrelated work' },
      { kind: 'changed', text: 'Expected troop and equipment shortages are shown as warnings rather than fatal application errors while genuine command failures remain errors' },
      { kind: 'fixed', text: 'Prevented the Commander view from crashing when older or partially hydrated equipment records contain null effects' },
      { kind: 'fixed', text: 'Auto Beri now refreshes unavailable targets, holds attack context through launch verification, keeps troop transfers focus-neutral, and recovers from stalled attack admission with bounded retries' },
      { kind: 'fixed', text: 'Berimond troop transfers reject Mead and Beef units, reconcile arrivals and consumed skips, and stop a skip chain when the game does not confirm a timer reduction' },
      { kind: 'fixed', text: 'Auto Storm settings remain compatible with older runtime previews, and the frontend no longer crashes when newer troop-history fields are absent' },
      { kind: 'fixed', text: 'Castle, movement, transport, attack analytics, and automation state updates now invalidate only their true dependencies, reducing stale decisions and unrelated policy wakeups' },
      { kind: 'removed', text: 'Removed the remaining glass-morphism styling, translucent blur surfaces, and glow-based status treatment from the dashboard' },
    ],
  },
  {
    version: '2.0.0',
    subtitle: 'Canonical automation, event operations, resource logistics, Storm blueprints, and desktop reliability',
    date: '2026-07-27',
    items: [
      { kind: 'added', text: 'Unified Automation workspace with authoritative live status, guarded operations, schedules, duration controls, and per-feature settings' },
      { kind: 'added', text: 'Auto Towers with per-castle target maps, cooldown tracking, commander selection, travel boosts, and independent nearest-target attacks' },
      { kind: 'added', text: 'Auto Invasion for Foreign Lords and Bloodcrows with difficulty selection, score goals, target strengthening, presets, and guarded spending' },
      { kind: 'added', text: 'Auto Nomad and Samurai chains plus one-run Auto Advisor support with event-specific presets, camp locking, cooldown skips, and score targets' },
      { kind: 'added', text: 'Auto Khan attack, cooldown, rage, taunt, defense, tool-restock, and protection lanes that can run concurrently without blocking one another' },
      { kind: 'added', text: 'Auto Storm fort and resource-island operations, troop transfers, Aquamarine goals, durable castle blueprints, and construction or destruction time-skip controls' },
      { kind: 'added', text: 'Auto Station threat response with explicit castle reserves, temporary troop evacuation, automatic recall, and optional open-gate fallback' },
      { kind: 'added', text: 'Auto Sceat Resources with research-aware per-castle crafting plans, overflow handling, independent logistics, and kingdom storage buffers' },
      { kind: 'added', text: 'Auto Food Balance for Food, Honey, Mead, and Beef reserves using market barrows first and optional cross-kingdom transport with time skips' },
      { kind: 'added', text: 'Dedicated Attack Presets with courtyard support troops, Sceat support tools, safe CRA-string import and sharing, plus Defense Presets and Feature Stats workspaces' },
      { kind: 'added', text: 'Header tracker for the authoritative account-wide daily normal-attack count plus optional user-defined limits on supported attack automations' },
      { kind: 'added', text: 'Settings import and export, browser selection, runtime diagnostics, and version-aware desktop update handling' },
      { kind: 'changed', text: 'Rebuilt CitadelOps around versioned state, configuration, history, telemetry, API, intent, catalog, and Chromium session contracts' },
      { kind: 'changed', text: 'Auto Bird, Auto TCI, recruiting, tools, hospital, Berimond, Rift, and scheduling now use server-authoritative state with legacy settings migration' },
      { kind: 'changed', text: 'Resource logistics now supports capacity-bounded multi-resource sends, full donor availability, market barrows, immediate transport skips, and post-loot redistribution' },
      { kind: 'changed', text: 'Crafting logistics runs independently from crafting queues and can source ready resources from any eligible castle instead of waiting on one configured castle' },
      { kind: 'changed', text: 'Honey demand now uses each Brewery base rate and configured operating percentage instead of scaling from live Mead production' },
      { kind: 'changed', text: 'Attack automation now coordinates live commander availability, exact event presets, movement returns, cooldowns, protection mode, and castle focus through guarded intents' },
      { kind: 'changed', text: 'Battle and spy reports now use canonical local analytics, safe cloud outboxes, PvP eligibility checks, report deduplication, and movement-aware training context' },
      { kind: 'changed', text: 'Equipment effects, caps, compatibility, gems, set bonuses, construction items, buildings, units, and shops now resolve through current official game data' },
      { kind: 'changed', text: 'Alliance Targets keeps inspected rosters separate from the player alliance and derives travel, spy capacity, reports, and attack previews from canonical state' },
      { kind: 'changed', text: 'Connection recovery now validates a fresh login and castle bootstrap before state-dependent automation resumes, including reused Chromium profiles and Auto Restore' },
      { kind: 'fixed', text: 'Bounded stale-plan retries prevent repeated command refresh loops, while concurrent Auto Khan lanes prioritize full-rage taunts without blocking attack chains' },
      { kind: 'fixed', text: 'Corrected Storm fort attack limits, commander release on reported return legs, crafting resource waits, and kingdom shipment settlement' },
      { kind: 'removed', text: 'Removed automatic enforcement of the game 3,500-attack cost threshold; it remains informational unless the user sets an explicit automation limit' },
      { kind: 'removed', text: 'Removed automatic game-panel closing after successful attack dispatch while preserving guarded pre-launch cleanup' },
      { kind: 'removed', text: 'Removed legacy raw WebSocket actions, client-only automation mirrors, UI-shaped compatibility adapters, and stale embedded catalog paths' },
    ],
  },
  {
    version: '1.3.7',
    subtitle: 'Player analytics, alliance intelligence, equipment data, attack planning, and connection recovery',
    date: '2026-07-10',
    items: [
      { kind: 'added', text: 'My Stats analytics with one-minute history for player scores, troops, currencies, and speedups' },
      { kind: 'added', text: 'Interactive trend charts with hover inspection, custom drag-selected time windows, rate-of-change summaries, and 24-hour, 7-day, 30-day, and all-time ranges' },
      { kind: 'added', text: 'Troop analytics with type, role, and food filters plus top-unit trend lines and combat composition summaries' },
      { kind: 'added', text: 'GGE Tracker backfill for supported player-history gaps while locally collected observations remain authoritative' },
      { kind: 'added', text: 'Commander roster and movement status tracking backed by reconciled game movement snapshots' },
      { kind: 'added', text: 'Multi-wave attack planning with inventory-aware troop and tool allocation, reusable presets, and improved Rift launch handling' },
      { kind: 'added', text: 'Commander and castellan base-equipment loadout swapping' },
      { kind: 'added', text: 'Alliance target intelligence with castle selection, travel distance, spy availability, and live espionage launching' },
      { kind: 'added', text: 'Alliance spy-report sharing with six-hour visibility, movement correlation, and battle-training context' },
      { kind: 'changed', text: 'Equipment effects now use the latest official catalogs, localized labels, wearer-aware mappings, PvP and PvE scopes, caps, gems, and set bonuses' },
      { kind: 'changed', text: 'Effective Battle Profile now shows every applicable always-on plus PvP or PvE total instead of an eight-stat preview' },
      { kind: 'changed', text: 'Game connection status and retry handling now provide consistent login, cooldown, reconnecting, and stale-session state across the dashboard' },
      { kind: 'changed', text: 'Automation features coordinate castle focus so manual actions and higher-priority tasks do not compete for the active castle' },
      { kind: 'fixed', text: 'Improved Auto Bird, Auto TCI, recruiting, tool, hospital, movement, and castle-state synchronization during refreshes and reconnects' },
    ],
  },
  {
    version: '1.3.6',
    subtitle: 'Scheduler, recruiting, tools, hospital automation, subscription state, and interface refresh',
    date: '2026-07-02',
    items: [
      { kind: 'added', text: 'Feature scheduling controls for automation windows and weekly run plans' },
      { kind: 'added', text: 'Recruit troops settings backed by production slot and capacity parsing' },
      { kind: 'added', text: 'Auto Tool settings with queueable tool catalog support' },
      { kind: 'added', text: 'Auto Hospital automation with hospital slot detection, subscription stack capacity, scan interval settings, and weekly scheduling' },
      { kind: 'added', text: 'Auto Hospital ruby-heal filtering that discards ruby-cost wounded units before queueing coin-healable units' },
      { kind: 'added', text: 'Subscription and VIP state parsing for game-state snapshots' },
      { kind: 'changed', text: 'Updated dashboard, equipment, TCI, troop picker, logger, and sidebar surfaces for the refreshed UI' },
      { kind: 'changed', text: 'Hospital, recruit, and tool queue parsing now keeps automation state aligned with live focused-castle responses' },
      { kind: 'fixed', text: 'Improved battle report and live equipment stat mapping coverage' },
    ],
  },
  {
    version: '1.3.3',
    subtitle: 'Offline snapshot UI; Auto Bird; decoration preset alerts',
    date: '2026-04-10',
    items: [
      { kind: 'added', text: 'Patch Notes: in-app changelog (System)' },
      { kind: 'added', text: 'Offline dashboard: last local snapshot — castles, resources, focus' },
      { kind: 'added', text: 'Offline castle picker: full list from cached castles' },
      { kind: 'added', text: 'Auto Bird: login gate; refresh if session not valid' },
      { kind: 'added', text: 'Auto Bird: longer post-refresh login wait (~5 min) when game queue is long' },
      { kind: 'fixed', text: 'Offline castle switch: UI matches last snapshot (avoids stale pre-disconnect values)' },
      { kind: 'fixed', text: 'Auto Bird + mid-session disconnect: refresh recovery path instead of idle until next run' },
      {
        kind: 'changed',
        text: 'Decoration preset Apply: SOB/EBU step lines no longer show in the decorations panel — status is only in the global Alerts stack',
      },
      {
        kind: 'added',
        text: 'Decoration preset apply: persistent “apply started” alert with Cancel apply; cleared when the run finishes, fails, is cancelled, or hits a storage shortfall',
      },
      {
        kind: 'added',
        text: 'Preset storage shortfall: missing decorations listed as bullets (e.g. 1x Name) in a persistent red alert until dismissed or apply completes successfully',
      },
      {
        kind: 'changed',
        text: 'Server: decoration apply no longer spams internal sin/storage progress lines to the UI; shortfall uses display names aligned with decoration summaries',
      },
    ],
  },
];

export const APP_VERSION_CURRENT = PATCH_NOTES_RELEASES[0].version;

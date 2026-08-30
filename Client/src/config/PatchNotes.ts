/**
 * In-app changelog. Append a new release at the top when you ship a version.
 */
export const PATCH_NOTE_KIND_ORDER = [
  'added',
  'fixed',
  'security',
  'changed',
  'removed',
  'deprecated',
] as const;

export type PatchNoteKind = (typeof PATCH_NOTE_KIND_ORDER)[number];

export interface PatchNoteItem {
  kind: PatchNoteKind;
  text: string;
}

/** Badge label shown in the UI for each kind */
export const PATCH_NOTE_KIND_LABEL: Record<PatchNoteKind, string> = {
  added: 'Added',
  fixed: 'Fixed',
  security: 'Security',
  changed: 'Changed',
  removed: 'Removed',
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

const PATCH_NOTE_KIND_RANK = PATCH_NOTE_KIND_ORDER.reduce<Record<PatchNoteKind, number>>(
  (rank, kind, index) => {
    rank[kind] = index;
    return rank;
  },
  {} as Record<PatchNoteKind, number>,
);

export function sortPatchNoteItems(items: readonly PatchNoteItem[]): PatchNoteItem[] {
  return items
    .map((item, index) => ({ item, index }))
    .sort((left, right) => (
      PATCH_NOTE_KIND_RANK[left.item.kind] - PATCH_NOTE_KIND_RANK[right.item.kind]
      || left.index - right.index
    ))
    .map(({ item }) => item);
}

const PATCH_NOTES_RELEASES_UNSORTED: PatchNotesRelease[] = [
  {
    version: '2.3.2',
    subtitle: 'Account controls, Khan recovery, and verified releases',
    date: '2026-08-29',
    items: [
      { kind: 'added', text: 'Auto Khan can stop camp attacks after a per-event number of accepted rage retaliations while continuing its full-rage taunt chain, and can optionally require the authoritative timed Rage points booster before another camp attack' },
      { kind: 'added', text: 'My Stats Storage now lets local profiles keep no history, 24 hours, 7 days, 30 days, 90 days, one year, or unlimited history; hosted runtimes expose the same control with their existing 30-day storage cap enforced' },
      { kind: 'fixed', text: 'Auto Khan now scopes its full-rage taunt cursor to the active Nomad occurrence, waits for an accepted movement response before consuming that rage fill, declares defense-tool purchases with the typed castle inventory resource, and keeps attack-only booster and chain limits from blocking cooldown or defense recovery' },
      { kind: 'fixed', text: 'Auto Khan now treats sub-second movement projection jitter as simultaneous at the game wire\'s one-second precision, waits only the attack lane when a newly available faster commander could materially overtake an in-flight hit, and automatically recovers after the relevant arrival' },
      { kind: 'fixed', text: 'Hospital and recruitment alliance-help requests now wait for the current session\'s authoritative own-help list; hospital allows at most one outstanding request while recruitment continues to cover the castle-wide active queue snapshot' },
      { kind: 'fixed', text: 'Attack and defense presets now create and save with section-scoped conflict protection, preserve unrecognized sibling data, and keep troop and tool editors available when unrelated optional catalogs are temporarily unavailable' },
      { kind: 'fixed', text: 'Hosted schedules, automation settings, presets, optimizer priorities, Storm or Berimond blueprints, and saved Rift replay templates now belong to the account control plane and remain editable with no runtime; save dialogs wait for durable acknowledgement and workers apply the exact fenced revision before login or automation can resume' },
      { kind: 'fixed', text: 'Windows component-state persistence no longer attempts an unsupported directory flush that could report Access is denied; state files and recovery directories use write-through Windows moves instead' },
      { kind: 'fixed', text: 'World Intelligence once again leads Storm Islands with its authoritative cargo ranking instead of presenting page-local session ranks as though every player held the same global place' },
      { kind: 'fixed', text: 'Current and recorded event boards now stay under one event choice, with the event-date picker retaining every returned run and dated Storm boards rebuilding inconsistent ranks from their final recorded scores' },
      { kind: 'fixed', text: 'Offense automation daily badges now count confirmed launches from the authoritative server reset boundary instead of a rolling 24-hour window, with the boundary and retained feature telemetry restored after restart' },
      { kind: 'security', text: 'Desktop updates now require an exact SHA-256 match and remain restricted to the official release location; unexpected redirects, paths, filenames, missing checksums, and altered downloads are rejected' },
      { kind: 'security', text: 'Automatic updates now require the version service to return the platform artifact URL and SHA-256 directly; public source no longer embeds the provider storage origin or private release-manifest layout' },
      { kind: 'security', text: 'The public source tree excludes local report data, environment-specific files, provider-specific build configuration, and production rollout controls; release validation consumes an exact reviewed source revision outside this repository' },
      { kind: 'changed', text: 'Release output is now limited to the Windows desktop executable and the hosted tenant image; other desktop platform artifacts are no longer built or published' },
      { kind: 'changed', text: 'Auto Beri Builder configuration now remains in Auto Beri settings while the Automation overview presents its runtime status without a second quick toggle' },
      { kind: 'changed', text: 'The dashboard now uses a kingdom-inspired light and dark color palette without changing its layout or spacing' },
    ],
  },
  {
    version: '2.3.1',
    subtitle: 'Persistent automation and unified release safety',
    date: '2026-08-25',
    items: [
      { kind: 'added', text: 'Auto Beri World now exposes its saved Builder lane toggle beside live lane status, allowing camp construction to stay off while transfers, tools, and tower attacks continue' },
      { kind: 'added', text: 'Auto Beri World now supports a saved daily attack-count limit using the authoritative account-wide server counter, with an immediate launch-time recheck and automatic resumption after the daily reset' },
      { kind: 'added', text: 'Every offense automation now shows its own confirmed rolling 24-hour launch count beside the existing rolling hourly attack badge, with both windows restored from persistent feature telemetry after restart' },
      { kind: 'added', text: 'Attack presets now offer an optional whole-family troop mode that fills each requested allocation from the highest owned official tier downward, retaining partial higher-tier quantities before using lower tiers' },
      { kind: 'fixed', text: 'Automatic equipment cleanup now runs on the backend scheduler with its interval stored in shared configuration, so cleanup remains active when the desktop or hosted dashboard is closed' },
      { kind: 'fixed', text: 'Auto Beri Builder now protects every Stable from its demolition candidate list while continuing to reconcile and remove other eligible extra buildings' },
      { kind: 'changed', text: 'Live Activity is now a combined user-only stream with official names instead of item, castle, commander, and other internal identifiers; raw game frames remain private backend diagnostics rather than user-visible log details' },
      { kind: 'changed', text: 'Desktop binaries and the hosted ARM64 worker image now come from one source revision and pass guarded 60-second candidate and live-runtime stability gates before release pointers are promoted' },
      { kind: 'changed', text: 'Hosted tenant runtimes now keep persisted state, history, catalogs, and configuration available through game disconnects; every table request carries its tenant-shard session, settings save directly to the durable store without a game action, and configuration-only changes publish a fresh dashboard checkpoint' },
    ],
  },
  {
    version: '2.3.0',
    subtitle: 'Dynamic hosted runtimes and sparse revisioned state',
    date: '2026-08-21',
    items: [
      { kind: 'added', text: 'The same binary now keeps the ordinary desktop as its zero-configuration one-account composition while an explicit hosted mode can add or drain bounded Background account runtimes live without restarting or interrupting sibling runtimes' },
      { kind: 'added', text: 'Hosted worker cells expose a private idempotent reconciliation and status stream for desired revisions, placement epochs, credential leases and their active or lapsed state, runtime capacity, and sanitized per-runtime readiness' },
      { kind: 'added', text: 'Hosted runtimes publish dashboard checkpoints — the exact client state document the dashboard renders, the configuration snapshot, the last hundred operation receipts, and the sanitized session situation — every five minutes while the state changes, promptly after a session transition, and once more before the runtime is drained, so the frontend can keep serving the dashboard with stale-but-complete data while no runtime exists; cells can allowlist the frontend\'s browser origins for live shard access with credentialed CORS' },
      { kind: 'added', text: 'A Reconnect control in the Command Center header (session.reconnect intent) forces a fresh game connection while a session is reconnecting, cooling down, suspended, or released, and the header now names suspended and released sessions with their resume time' },
      { kind: 'added', text: 'Hosted assignments carry an on-disconnect policy: release, the hosted default, makes the runtime\'s lifetime follow the game connection, while hold (explicit) keeps the runtime reconnecting on its own like the desktop — after a short immediate retry window it reports the session as released with the earliest retry time (cooldown or suspension end, never sooner than the relog delay) and stops, so the control plane can drain it, give the slot to another account, and create a fresh runtime when the wait elapses or the user presses Reconnect; the control plane can also force a reconnect on a running runtime' },
      { kind: 'added', text: 'Hosted orchestration can install or rotate a runtime\'s game login and server selection through an epoch-fenced no-store control request that is written only to that runtime\'s protected saved-login file and never echoed, logged, or carried in reconcile, status, or event payloads, and can scrub it again before relocation or deletion; cell status now reports the current login obstacle as the raw game login code classified from the official client\'s error constants (20 invalid password, 21/26/368 wrong server, 27 suspended with the remaining suspension time or deactivated, 409/423 invalid login token, 453 cooldown) plus cooldown and retry times, so the control plane can wait out a lockout, hand a suspended or refused account back to the user, or re-place a runtime with the stored credential' },
      { kind: 'added', text: 'Dynamic hosted runtimes can publish one merged private My Stats and Feature Stats sample through an explicitly configured CitadelOpsBackend ingest contract while the ordinary desktop and static canary remain non-publishing by default' },
      { kind: 'added', text: 'A game server directory now maps every official world code to its real connection address and zone — including worlds that share one multi-zone host, such as GB1, INT1, and SK1, which previously could not connect at all — sourced from the game\'s own network config, embedded as a snapshot, refreshable at runtime, and served to login forms at GET /api/v2/session/game-servers; the Settings server field suggests the official worlds by name' },
      { kind: 'added', text: 'Hosted login installs may pin the world\'s resolved connection address and zone from the control plane\'s shared directory, which the runtime validates against the official host shape and prefers over its own catalog, so a world opened after this build ships still connects without a release' },
      { kind: 'added', text: 'Auto Khan now has independent controls for automatic camp attacks and full-rage retaliation, allowing attack-only operation or cooldown, rage, and defense maintenance around a manually driven Khan chain' },
      { kind: 'fixed', text: 'Exact command-response waiters now complete by frame ingress identity even when a successful protocol response has no retained state projection and intentionally creates no revision' },
      { kind: 'fixed', text: 'Dashboard state changes apply sparse component and keyed-entry deltas instead of fetching and replacing the complete account state after every revision' },
      { kind: 'fixed', text: 'Live commander availability now synchronizes from a connection-scoped movement snapshot marker and continues through sparse movement deltas instead of remaining stuck on the retired raw protocol counter' },
      { kind: 'fixed', text: 'Shop purchase counters are cached by castle and kingdom, so Luna, Auto Buyer, and construction-shop responses cannot overwrite one another or trigger a rapid cross-kingdom refresh loop' },
      { kind: 'fixed', text: 'Grouped state persistence now carries every dirty ID and shard key through the complete batch, preventing a later unrelated revision from turning one map or cooldown update into a full partition rewrite' },
      { kind: 'fixed', text: 'Equipment target applicability now follows the official area and fight-scope fields across Effective Battle Report, Reconfigure, and Battle Stats, preventing PvP and castle-lord effects from being counted for Rift Raid area 43' },
      { kind: 'fixed', text: 'World Intelligence subscribers receive every stored row as one complete selected-leaderboard base followed by shared upserts and removals, with compact world revisions and a bounded metadata fallback replacing the former twelve-run response fanout' },
      { kind: 'fixed', text: 'Standalone desktop builds skip absent typed-nil automation policies, preventing live game frames from crashing Amos when hosted shared Storm coordination is unavailable' },
      { kind: 'fixed', text: 'Dashboard commands no longer live or die with the dashboard connection: intents are accepted immediately and executed by the runtime under its own lifetime, completion streams through operation events with a polling fallback, and closing the tab, sleeping, losing the network, or a hosted gateway timeout can no longer cancel a running operation; POST /api/v2/intents/{name} now answers 202 with the accepted receipt unless ?wait=true asks for the final one' },
      { kind: 'fixed', text: 'Private metrics retries now classify the backend answer: transient failures replay the identical sample under the same idempotency key with capped jittered backoff and Retry-After, a durably rejected sample is dropped so the next cadence publishes a fresh one instead of blocking the runtime until the next reconcile, and a refused grant stops being spent until orchestration rotates it' },
      { kind: 'fixed', text: 'Auto Nomad and Samurai now size troop and tool requirements from the camp-capacity-adjusted event preset and keep leveling with every available commander instead of waiting for each earlier camp victory' },
      { kind: 'security', text: 'Orchestrator credentials and short-lived dashboard connection grants are separate, stored only as hashes in process memory, fenced by desired revision and placement epoch, and bound to one exact runtime shard; route mismatches return not found and token rotation invalidates older dashboard sessions' },
      { kind: 'security', text: 'Private metrics require a unique short-lived in-memory grant for one tenant, runtime, cell, placement epoch, and lease; grants never enter payloads or status streams, cannot be shared across runtimes or reused for dashboards, and uploads wait for a matching authoritative game identity and session generation' },
      { kind: 'changed', text: 'Game state keeps the official logical domain model while high-cardinality map, movement, castle, inventory, Storm, tower, report, event-score, and attack-analytics data uses selective immutable copy-on-write partitions shared by the desktop and isolated hosted-runtime compositions' },
      { kind: 'changed', text: 'Reducer ownership and automation wakes are targeted across every state domain: mixed protocol envelopes publish only projections that changed, while disabled or unchanged policies wait for an exact domain, configuration, session, or scheduled deadline instead of being rescanned every coordinator tick' },
      { kind: 'changed', text: 'Luna counter refreshes are limited to a completed purchase response or an expired five-minute castle snapshot, while Auto Buyer evaluates at most every 30 minutes and refreshes its shop counters at most hourly' },
      { kind: 'changed', text: 'Limited-time automation lanes sleep after the authoritative daily event check when their event is unavailable; Auto Buyer skips only the affected event-shop goals while continuing ordinary shops, specialists, and feasts' },
      { kind: 'changed', text: 'State recovery now group-commits only dirty components, IDs, parts, or physical shards on a fixed window, while retaining migration from the legacy complete GameState snapshot' },
      { kind: 'changed', text: 'Unknown map object types and protocol observations are decoded transiently but retained only after an explicit consumer-backed projection is registered, with bounded map retention for supported types' },
      { kind: 'changed', text: 'Raw game telemetry keeps the complete wire stream on disk while using byte-bounded in-memory tails, amortized constant-time retention, and grouped buffered writes to reduce allocation, copying, and filesystem overhead' },
      { kind: 'changed', text: 'Immutable Storm, crafting, resource, currency, building, construction-item, unit, and subscription-effect catalog projections are decoded and indexed once per official data version instead of repeatedly parsing the same JSON during automation evaluation' },
      { kind: 'changed', text: 'The redundant Local desktop control footer was removed from the sidebar' },
      { kind: 'changed', text: 'Dashboard HTTP, event-stream, and WebSocket URLs derive the authenticated runtime shard from the hosted account path, while desktop URLs remain unchanged at the root API path' },
      { kind: 'changed', text: 'Background sessions now stay connected for as long as the account is enabled: every drop reconnects after the user\'s relog delay, a login cooldown waits out the game\'s timer plus that delay, a temporary suspension reports suspended and resumes by itself when it ends, an unknown login code retries with a doubling delay capped at one hour, and only invalid credentials, a wrong server selection, a permanent suspension, or a deactivated account park the session until the saved login changes, at which point Start no longer needs a preceding Stop' },
      { kind: 'changed', text: 'Hosted account runtimes are long-lived: a runtime and its game session persist for as long as the account remains in the desired set, a lapsed placement lease only pauses private metrics publishing and dashboard-grant renewal while being reported as lapsed instead of draining the runtime, and only a reconcile that omits the account (or an explicit session stop) ends it, so a control-plane outage or missed reconcile can no longer log a hosted account out' },
      { kind: 'changed', text: 'Each hosted runtime keeps a compact in-memory projection of its attacker report history keyed to the analytics store write generation, so a steady-state private metrics sample rolls its 24-hour, 7-day, 30-day, and lifetime feature windows in under a millisecond instead of re-reading and decoding up to ten thousand report rows every minute; refreshes use a narrow attacker-only query, one publish attempt is bounded by a timeout, and placement rotation follows the regular cadence instead of bursting an extra upload from every runtime in the cell' },
      { kind: 'removed', text: 'Per-revision deep cloning, persistence cloning, and dashboard retransmission of the complete game state were removed from normal runtime processing' },
    ],
  },
  {
    version: '2.2.1',
    subtitle: 'Title-aware recruiting, independent Background login, and safer automation decisions',
    date: '2026-08-13',
    items: [
      { kind: 'added', text: 'Background mode can now be configured directly with a game username, password, and world code, then connect without first capturing a Full application session; the current official client build and remaining WebSocket handshake values are derived automatically' },
      { kind: 'added', text: 'Auto Recruit now tracks the current Glory and Gallantry titles from live game updates while keeping the two title systems independent; only the active connection\'s observed Glory title can authorize Glory-title recruits' },
      { kind: 'added', text: 'A new off-by-default Recruit level 10 if glory title is lost option can queue the corresponding level 10 unit after title loss; while it is off, affected slots are softly paused and rotating schedules continue with another available slot' },
      { kind: 'fixed', text: 'Auto Bird retains castle focus from its final source refresh through dispatch and respects each castle\'s retry window, preventing focus races, rapid back-and-forth preparation, and one waiting castle from blocking another castle\'s cycle' },
      { kind: 'fixed', text: 'The authoritative daily attack count is refreshed on a bounded interval after the server maximum is reached, so attack automations can resume after the game resets the count without unrelated policy churn' },
      { kind: 'fixed', text: 'Spy reports now preserve official wall, gate, moat, keep, tower, general, and general-ability fields instead of interpreting unrelated packet values as calculated defense percentages' },
      { kind: 'fixed', text: 'Horse travel boosts fail closed when placed building data does not identify one active compatible building set, avoiding a travel selection based on ambiguous stable, faction-stable, or harbor state' },
      { kind: 'security', text: 'Background credentials are kept in a separate protected profile file, excluded from settings exports, API responses, receipts, and logs, and can only be changed through a size-bounded same-origin local request' },
      { kind: 'changed', text: 'Level 11 Protector of the North and Valkyrie Sniper selections are checked against their official Glory-title unlocks whenever Auto Recruit plans a queue and again immediately before each recruit command' },
      { kind: 'changed', text: 'Recruit selections and weekly schedules now follow official unit upgrade families and automatically choose the highest member that the target castle currently exposes, while queue capacity and scheduled selections are revalidated before enqueueing' },
      { kind: 'changed', text: 'Storm Food and Mead kingdom deliveries now use a configurable minimum shipment of at least 10,000 after transport tolls, with 10,000 as the default' },
      { kind: 'changed', text: 'World Intelligence desktop access is now a read-only facade over the shared CitadelOps backend event, ranking, player, and alliance data; obsolete local catalog collection, scheduling, and storage were removed' },
      { kind: 'changed', text: 'Experimental battle prediction now uses the catalog-effect combat v2 model with per-wave and per-lane forces, tools, capped equipment and skill effects, fortification levels, courtyard control, and explicit uncertainty for hidden defender tools' },
    ],
  },
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

export const PATCH_NOTES_RELEASES: PatchNotesRelease[] = PATCH_NOTES_RELEASES_UNSORTED.map((release) => ({
  ...release,
  items: sortPatchNoteItems(release.items),
}));

export const APP_VERSION_CURRENT = PATCH_NOTES_RELEASES[0].version;

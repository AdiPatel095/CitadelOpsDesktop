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

export const APP_VERSION_CURRENT = '1.3.7';

export const PATCH_NOTES_RELEASES: PatchNotesRelease[] = [
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

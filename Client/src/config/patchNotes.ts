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

export const APP_VERSION_CURRENT = '1.3.3';

export const PATCH_NOTES_RELEASES: PatchNotesRelease[] = [
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

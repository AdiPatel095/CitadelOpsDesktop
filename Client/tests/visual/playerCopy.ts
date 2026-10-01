// CIT-68 Revision 1: only complete, exact visible strings are excluded.
export const copyExclusions = [
  { text: 'soft-locked', owner: 'CIT-66 PR-C', removeWhen: 'PR-C merges' },
  { text: 'Foreign Lords or Bloodcrow event is not active in the authoritative event inventory; this lane is softly locked until the next opening or event update', owner: 'CIT-83', removeWhen: 'recorded public/mock-runtime capture is replaced with synthetic data' },
  { text: 'Monitoring canonical movement snapshots for incoming attacks', owner: 'CIT-83', removeWhen: 'recorded public/mock-runtime capture is replaced with synthetic data' },
] as const;

export const internalCopy = /canonical|soft[- ]lock(?:ed)?|authoritative|managed fleet|on cell-|Checkpoint:\s*never|code\s+\d{2,}|tenant|ep-live-[\w-]+|goodgamestudios\.com|collected runs|score rows|leaderboards? cached|Citadel Ops/i;

export function nonExcludedPlayerCopy(text: string): string {
  return text.split(/\r?\n/).filter(line => !copyExclusions.some(entry => entry.text === line.trim())).join('\n');
}

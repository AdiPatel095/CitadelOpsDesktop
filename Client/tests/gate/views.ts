import { cases } from '../visual/cases';

// CIT-93 views.spec.ts keeps this list private. Keep the union of both repos here.
export const views = [
  { label: 'Castle', view: 'castle' },
  { label: 'Automation', view: 'automation' },
  { label: 'Feature Stats', view: 'events' },
  { label: 'Attack Presets', view: 'attack-presets' },
  { label: 'Defense Presets', view: 'defense-presets' },
  { label: 'Equipment', view: 'equipment' },
  { label: 'Commanders', view: 'movement' },
  { label: 'Battle Stats', view: 'battle-stats' },
  { label: 'My Stats', view: 'player-tracker' },
  { label: 'Alliance Targets', view: 'alliance-targets' },
  { label: 'World Intel', view: 'world-intelligence' },
  { label: 'Rift Raid', view: 'rift' },
  { label: 'Settings', view: 'settings' },
  { label: 'Patch Notes', view: 'patch-notes' },
  { label: 'Support', view: 'support' },
] as const;

export type GateCase = {
  name: string;
  variant: 'public' | 'app';
  path: string;
  ready: string;
  label?: string;
  settings?: string;
};

type SourceCase = {
  name: string; variant?: 'public' | 'app'; path?: string; ready?: string;
  label?: string; view?: string; settings?: string | boolean;
};
const existing = (cases as readonly SourceCase[]).map((entry): GateCase => ({
  name: entry.name,
  variant: entry.variant ?? 'app',
  path: entry.path ?? '/dashboard',
  ready: entry.ready ?? (entry.settings ? '[role="dialog"]' : `[data-view="${entry.view}"]`),
  label: entry.label ?? entry.view,
  settings: entry.settings === true ? 'Auto Towers' : entry.settings || undefined,
}));

export const gateCases: readonly GateCase[] = [
  ...existing,
  ...views.filter(entry => !existing.some(testCase => testCase.name === entry.view)).map(entry => ({
    name: entry.view, variant: 'app' as const, path: '/dashboard',
    label: entry.label, ready: `[data-view="${entry.view}"]`,
  })),
];

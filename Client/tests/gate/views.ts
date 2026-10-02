import { cases } from '../visual/cases';
import { AUTOMATION_FEATURE_NAMES, isDesktop } from './adapter';

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
  view?: string;
  states?: Record<string, 'empty' | 'loading' | 'error'>;
  scenario?: string;
  dialog?: 'add' | 'login' | 'access' | 'delete';
  toast?: 'success' | 'failure';
};

type SourceCase = {
  name: string; variant?: 'public' | 'app'; path?: string; ready?: string;
  label?: string; view?: string; settings?: string | boolean; states?: GateCase['states']; scenario?: string;
};
const existing = (cases as readonly SourceCase[]).map((entry): GateCase => ({
  name: entry.name,
  variant: entry.variant ?? 'app',
  path: entry.path ?? '/dashboard',
  ready: entry.ready ?? (entry.settings ? '[role="dialog"]' : `[data-view="${entry.view}"]`),
  label: entry.label ?? entry.view,
  view: views.find(view => view.label === (entry.label ?? entry.view))?.view ?? entry.view ?? views.find(view => view.view === entry.name)?.view,
  states: entry.states, scenario: entry.scenario,
  settings: entry.settings === true ? 'Auto Towers' : entry.settings || undefined,
}));

export const gateCases: readonly GateCase[] = [
  ...existing,
  ...Object.values(AUTOMATION_FEATURE_NAMES).filter(name => name !== 'Auto Towers').map(settings => ({
    name: `settings-${settings.toLowerCase().replace(/[^a-z0-9]+/g, '-')}`, variant: 'app' as const, path: '/dashboard',
    view: 'automation', label: 'Automation', settings, ready: '[role="dialog"]',
  })),
  ...(['success', 'failure'] as const).map(toast => ({
    name: `toast-${toast}`, variant: 'app' as const, path: `/dashboard${toast === 'failure' ? '?mockFail=start' : '?mockToast=success'}`,
    view: 'automation', label: 'Automation', toast, ready: '[role="alert"]',
  })),
  ...(isDesktop ? [] : [
  { name: 'auth', variant: 'public', path: '/login', ready: 'main' },
  { name: 'auth-signup', variant: 'public', path: '/login?mode=signup', ready: 'main' },
  ...(['add', 'login', 'access', 'delete'] as const).map(dialog => ({
    name: `account-dialog-${dialog}`, variant: 'app' as const, path: '/accounts', dialog, ready: '[role="dialog"]',
  })),
  ] as GateCase[]),
  ...views.filter(entry => !existing.some(testCase => testCase.view === entry.view && !testCase.settings)).map(entry => ({
    name: entry.view, variant: 'app' as const, path: '/dashboard',
    label: entry.label, view: entry.view, ready: `[data-view="${entry.view}"]`,
  })),
];

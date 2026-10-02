export type VisualCase = { name: string; label: string; view: string; settings?: boolean; scenario?: string };
export const cases: readonly VisualCase[] = [
  { name: 'castle', label: 'Castle', view: 'castle' },
  { name: 'stale-session', label: 'Castle', view: 'castle', scenario: 'stale-data-disconnected' },
  { name: 'automation', label: 'Automation', view: 'automation' },
  { name: 'equipment', label: 'Equipment', view: 'equipment' },
  { name: 'feature-stats', label: 'Feature Stats', view: 'events' },
  { name: 'settings-auto-towers', label: 'Automation', view: 'automation', settings: true },
];
export const themes = ['dark', 'light'] as const;

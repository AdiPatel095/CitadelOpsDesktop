export type VisualCase = { name: string; label: string; view: string; settings?: boolean; scenario?: string; states?: Partial<Record<'feature-history', 'empty' | 'loading' | 'error'>> };
const baseCases: readonly VisualCase[] = [
  { name: 'header-panel', label: 'Castle', view: 'castle' },
  { name: 'castle', label: 'Castle', view: 'castle' },
  { name: 'stale-session', label: 'Castle', view: 'castle', scenario: 'stale-data-disconnected' },
  { name: 'automation', label: 'Automation', view: 'automation' },
  { name: 'equipment', label: 'Equipment', view: 'equipment' },
  { name: 'feature-stats', label: 'Feature Stats', view: 'events' },
  { name: 'settings-auto-towers', label: 'Automation', view: 'automation', settings: true },
];
export const themes = ['dark', 'light'] as const;

const sources = { 'feature-stats': 'feature-history' } as const;
export const cases: readonly VisualCase[] = baseCases.flatMap(visualCase => {
  const source = sources[visualCase.name as keyof typeof sources];
  if (!source) return [visualCase];
  return [visualCase, ...(['empty', 'loading', 'error'] as const).map(state => ({
    ...visualCase, name: `${visualCase.name}-${state}`, states: { [source]: state },
  }))];
});

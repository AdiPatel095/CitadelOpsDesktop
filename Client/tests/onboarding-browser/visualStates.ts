export const VISUAL_STATES_KEY = 'citadelops.visualStates';
export type VisualState = 'empty' | 'loading' | 'error';
export type VisualStateSource = 'feature-history' | 'world-intel' | 'hosted-accounts';

export function parseVisualStates(raw: string | null | undefined): Partial<Record<VisualStateSource, VisualState>> {
  try {
    if (!raw) return {};
    const parsed: unknown = JSON.parse(raw);
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {};
    const result: Partial<Record<VisualStateSource, VisualState>> = {};
    for (const source of ['feature-history', 'world-intel', 'hosted-accounts'] as const) {
      const state = (parsed as Record<string, unknown>)[source];
      if (state === 'empty' || state === 'loading' || state === 'error') result[source] = state;
    }
    return result;
  } catch {
    return {};
  }
}

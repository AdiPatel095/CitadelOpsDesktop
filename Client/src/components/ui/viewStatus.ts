export type ViewStatus = 'error' | 'loading' | 'empty' | 'content';

export function viewStatus(i: { hasData: boolean; loading: boolean; error: boolean }): ViewStatus {
  if (i.hasData) return 'content';
  if (i.error) return 'error';
  if (i.loading) return 'loading';
  return 'empty';
}

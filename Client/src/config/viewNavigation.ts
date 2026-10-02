import type { ViewId } from './Navigation';

export const OPEN_VIEW_EVENT = 'citadelops:open-view';

export function requestView(view: ViewId): void {
  window.dispatchEvent(new CustomEvent(OPEN_VIEW_EVENT, { detail: view }));
}

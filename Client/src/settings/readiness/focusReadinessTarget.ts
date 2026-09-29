/** Brings a settings control into view and focuses it for an in-context readiness fix. */
export function focusReadinessTarget(elementId: string): void {
  const element = globalThis.document?.getElementById(elementId);
  if (!element) return;
  element.scrollIntoView({ block: 'center', behavior: 'smooth' });
  const focusable = element.matches('button, input, select, textarea, [tabindex]')
    ? element
    : element.querySelector<HTMLElement>('button, input, select, textarea, [tabindex]:not([tabindex="-1"])') ?? element;
  focusable.focus({ preventScroll: true });
}

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

/**
 * Like `focusReadinessTarget`, but waits (up to `frames` animation frames) for the control to exist, for a
 * fix that opens its editor first. Returns a cancel function.
 */
export function focusReadinessTargetWhenReady(elementId: string, frames = 30): () => void {
  let handle = 0;
  let remaining = frames;
  const attempt = () => {
    if (globalThis.document?.getElementById(elementId)) {
      focusReadinessTarget(elementId);
      return;
    }
    if (--remaining > 0) handle = globalThis.requestAnimationFrame(attempt);
  };
  handle = globalThis.requestAnimationFrame(attempt);
  return () => globalThis.cancelAnimationFrame(handle);
}

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

/** The parts of the DOM the focus retry reads, so the retry is testable without a browser. */
export interface FocusableLike {
  isConnected: boolean;
  disabled?: boolean;
  matches(selector: string): boolean;
  closest(selector: string): unknown;
  querySelector(selector: string): FocusableLike | null;
  scrollIntoView(options?: unknown): void;
  focus(options?: unknown): void;
}

export interface FocusDocumentLike {
  getElementById(id: string): FocusableLike | null;
  readonly activeElement: unknown;
}

export interface FocusScheduler {
  request(callback: () => void): number;
  cancel(handle: number): void;
  now(): number;
}

const INNER = 'button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex="-1"])';

/** The element a fix should focus: the control itself or its first focusable child. */
export function focusTargetOf(element: FocusableLike): FocusableLike {
  return element.matches('button, input, select, textarea, [tabindex]') ? element : element.querySelector(INNER) ?? element;
}

/** Connected, enabled and interactive: not inert, not in a disabled fieldset, not aria-disabled. */
export function readyToFocus(element: FocusableLike | null): element is FocusableLike {
  if (!element || !element.isConnected) return false;
  const target = focusTargetOf(element);
  if (target.disabled === true) return false;
  return !target.closest('[inert], fieldset:disabled, [aria-disabled="true"]');
}

export const FOCUS_DEADLINE_MS = 3000;
export const FOCUS_MAX_FRAMES = 240;
/** How many times focus is re-asserted after the first attempt (the modal's own initial focus can run later). */
export const FOCUS_REASSERTS = 2;

const browserScheduler: FocusScheduler = {
  request: (callback) => globalThis.requestAnimationFrame(callback),
  cancel: (handle) => globalThis.cancelAnimationFrame(handle),
  now: () => Date.now(),
};

/**
 * Waits for the control to exist AND be interactive (a settings editor mounts its content after its draft session
 * loads, and keeps it inert until then), focuses it, and re-asserts focus a couple of frames later if something else
 * (the modal's initial focus) took it. Bounded by a frame budget and a 3 s deadline (same shape as `dialogFocus.ts`).
 * Returns a cancel function.
 */
export function focusReadinessTargetWhenReady(
  elementId: string,
  scheduler: FocusScheduler = browserScheduler,
  doc: FocusDocumentLike | undefined = globalThis.document as unknown as FocusDocumentLike | undefined,
): () => void {
  let handle = 0;
  let cancelled = false;
  let frames = 0;
  let reasserts = 0;
  const startedAt = scheduler.now();
  let focused: FocusableLike | null = null;

  // After focusing, look again a few frames later, twice: the modal's own initial focus can run after ours.
  const CHECK_GAPS = [2, 6];
  let checks = 0;
  const afterFocus = (remaining: number) => {
    if (cancelled) return;
    if (remaining > 0) { handle = scheduler.request(() => afterFocus(remaining - 1)); return; }
    if (focused && doc && doc.activeElement !== focused && reasserts < FOCUS_REASSERTS) {
      reasserts += 1;
      focused.focus({ preventScroll: true });
    }
    checks += 1;
    if (checks < CHECK_GAPS.length) handle = scheduler.request(() => afterFocus(CHECK_GAPS[checks] - CHECK_GAPS[checks - 1]));
  };

  const step = () => {
    if (cancelled || !doc) return;
    frames += 1;
    const expired = frames > FOCUS_MAX_FRAMES || scheduler.now() - startedAt > FOCUS_DEADLINE_MS;
    const element = doc.getElementById(elementId);
    if (readyToFocus(element)) {
      element.scrollIntoView({ block: 'center', behavior: 'smooth' });
      focused = focusTargetOf(element);
      focused.focus({ preventScroll: true });
      handle = scheduler.request(() => afterFocus(CHECK_GAPS[0]));
      return;
    }
    if (!expired) handle = scheduler.request(step);
  };
  handle = scheduler.request(step);
  return () => { cancelled = true; scheduler.cancel(handle); };
}

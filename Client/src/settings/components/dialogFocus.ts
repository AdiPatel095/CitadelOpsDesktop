/**
 * Focus handoff for a confirm dialog opened inside another modal (CIT-18 QA).
 * Pure so the order is testable without a DOM: the caller supplies the frame
 * scheduler. Focus moves only to an element that is connected, enabled and not
 * inside an `inert` subtree; while it is not ready (for example the settings
 * content is briefly inert), the move is retried for a few frames.
 */
export interface FocusTarget {
  isConnected: boolean;
  disabled?: boolean;
  closest(selector: string): unknown;
  focus(): void;
}

export interface FrameScheduler {
  request(callback: () => void): number;
  cancel(handle: number): void;
}

export const browserFrames: FrameScheduler = {
  request: (callback) => window.requestAnimationFrame(callback),
  cancel: (handle) => window.cancelAnimationFrame(handle),
};

export function focusable(target: FocusTarget | null | undefined): target is FocusTarget {
  return target != null && target.isConnected && target.disabled !== true && target.closest('[inert]') == null;
}

/**
 * Moves focus to `resolve()` after the dialog's own open or close focus handling
 * (two frames), retrying up to `attempts` frames while the target is not ready.
 * Returns a cancel function.
 */
export function moveFocusAfterDialog(
  resolve: () => FocusTarget | null | undefined,
  frames: FrameScheduler,
  attempts = 10,
): () => void {
  let handle = 0;
  let remaining = attempts;
  const tryFocus = () => {
    const target = resolve();
    if (focusable(target)) {
      target.focus();
      return;
    }
    if (--remaining > 0) handle = frames.request(tryFocus);
  };
  handle = frames.request(() => {
    handle = frames.request(tryFocus);
  });
  return () => frames.cancel(handle);
}

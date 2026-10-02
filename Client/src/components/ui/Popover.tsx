import { useLayoutEffect, useRef, useSyncExternalStore, type ReactNode, type RefObject } from 'react';
import { createPortal } from 'react-dom';
import './popover.css';
const query = '(max-width: 767.98px)';
const subscribe = (notify: () => void) => { const media = window.matchMedia(query); media.addEventListener('change', notify); return () => media.removeEventListener('change', notify); };
export function Popover({ open, onClose, anchorRef, labelledBy, children }: {
  open: boolean; onClose(): void; anchorRef: RefObject<HTMLElement | null>; labelledBy: string; children: ReactNode;
}) {
  const sheet = useSyncExternalStore(subscribe, () => window.matchMedia(query).matches, () => false);
  const panel = useRef<HTMLDivElement>(null);
  const closeRef = useRef(onClose);
  useLayoutEffect(() => { closeRef.current = onClose; });
  useLayoutEffect(() => {
    if (!open) return;
    const node = panel.current!;
    const focusable = () => Array.from(node.querySelectorAll<HTMLElement>('button:not(:disabled), [href], input:not(:disabled), select:not(:disabled), [tabindex="0"]')).filter(element => element.getClientRects().length > 0);
    const reposition = () => {
      const anchor = anchorRef.current;
      if (!anchor) return;
      const rect = anchor.getBoundingClientRect();
      const left = getComputedStyle(anchor).direction === 'rtl' ? rect.left : rect.right - node.offsetWidth;
      node.style.setProperty('--popover-left', `${Math.max(16, Math.min(left, window.innerWidth - node.offsetWidth - 16))}px`);
      node.style.setProperty('--popover-top', `${rect.bottom + 4}px`);
      node.style.setProperty('--popover-max-height', `${Math.max(0, window.innerHeight - rect.bottom - 20)}px`);
    };
    reposition();
    (focusable()[0] ?? node).focus();
    const key = (event: KeyboardEvent) => {
      if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); closeRef.current(); }
      if (sheet && event.key === 'Tab') {
        const targets = focusable(); const first = targets[0] ?? node; const last = targets.at(-1) ?? node;
        if (event.shiftKey && (document.activeElement === first || document.activeElement === node)) { event.preventDefault(); last.focus(); }
        else if (!event.shiftKey && (document.activeElement === last || document.activeElement === node)) { event.preventDefault(); first.focus(); }
      }
    };
    const outside = (event: PointerEvent) => {
      if (!node.contains(event.target as Node) && !anchorRef.current?.contains(event.target as Node)) { event.preventDefault(); closeRef.current(); }
    };
    const contain = (event: FocusEvent) => { if (sheet && !node.contains(event.target as Node) && !anchorRef.current?.contains(event.target as Node)) (focusable()[0] ?? node).focus(); };
    document.addEventListener('keydown', key, true); document.addEventListener('pointerdown', outside); document.addEventListener('focusin', contain);
    window.addEventListener('resize', reposition); window.addEventListener('scroll', reposition, true);
    const resize = new ResizeObserver(reposition); resize.observe(node);
    const previousOverflow = document.body.style.overflow;
    if (sheet) document.body.style.overflow = 'hidden';
    return () => {
      document.removeEventListener('keydown', key, true); document.removeEventListener('pointerdown', outside); document.removeEventListener('focusin', contain);
      window.removeEventListener('resize', reposition); window.removeEventListener('scroll', reposition, true); resize.disconnect();
      if (sheet) document.body.style.overflow = previousOverflow;
    };
  }, [open, sheet, anchorRef]);
  // Keep child-owned dialogs mounted when opening one closes this disclosure.
  return createPortal(<div className="cit-popover-layer" data-popover-mode={sheet ? 'sheet' : 'anchored'} hidden={!open}>
    {sheet && <div className="cit-popover-scrim" aria-hidden="true" />}
    <div ref={panel} role={open ? "dialog" : undefined} tabIndex={-1} aria-modal={open && sheet ? true : undefined} aria-labelledby={labelledBy} className="cit-popover">{children}</div>
  </div>, document.body);
}

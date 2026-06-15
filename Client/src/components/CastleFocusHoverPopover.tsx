import React, { useCallback, useLayoutEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useMetadata } from '../context/MetadataContext';
import {
  castleFocusDecorationTooltipContent,
  castleFocusDecorationsTooltip,
  type CastleFocusState,
} from '../types/CastleFocusState.ts';

type Props = {
  castleFocus: CastleFocusState | null;
  children: React.ReactNode;
  /** Outer wrapper classes (e.g. max-width, flex). */
  className?: string;
  /** Popover horizontal alignment under the trigger (compact mode only). */
  align?: 'center' | 'start';
  /**
   * When true, tooltip is portaled to `document.body` so it is not clipped by parent overflow.
   * Width grows with content (capped to viewport); list height uses remaining viewport.
   */
  expandToViewport?: boolean;
};

const TOOLTIP_Z = 450;
const VIEWPORT_MARGIN = 16;
const HIDE_MS = 220;

/**
 * Visible hover panel — native `title` is often empty/broken in Electron/Chromium desktop shells.
 */
const CastleFocusHoverPopover: React.FC<Props> = ({
  castleFocus,
  children,
  className = '',
  align = 'center',
  expandToViewport = false,
}) => {
  const { getDecoration } = useMetadata();
  const { heading, lines } = castleFocusDecorationTooltipContent(castleFocus, getDecoration);
  const ariaLabel = castleFocusDecorationsTooltip(castleFocus, getDecoration);
  const linesSig = lines.join('\u0001');

  const triggerRef = useRef<HTMLSpanElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const hideTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [open, setOpen] = useState(false);
  const [panelGeom, setPanelGeom] = useState({ top: 0, left: VIEWPORT_MARGIN, maxHeight: 320 });

  const clearHideTimer = useCallback(() => {
    if (hideTimer.current !== null) {
      clearTimeout(hideTimer.current);
      hideTimer.current = null;
    }
  }, []);

  /** Position under trigger; width is content-driven (panel ref), kept on-screen horizontally. */
  const syncPanelPosition = useCallback(() => {
    const trigger = triggerRef.current;
    if (!trigger) return;
    const r = trigger.getBoundingClientRect();
    const top = r.bottom + 6;
    const maxHeight = Math.max(140, window.innerHeight - top - VIEWPORT_MARGIN);
    const margin = VIEWPORT_MARGIN;
    let left = r.left;
    const panel = panelRef.current;
    if (panel) {
      const w = panel.getBoundingClientRect().width;
      if (left + w > window.innerWidth - margin) {
        left = window.innerWidth - margin - w;
      }
    } else {
      left = Math.max(margin, Math.min(left, window.innerWidth - margin - 200));
    }
    if (left < margin) left = margin;
    setPanelGeom({ top, left, maxHeight });
  }, []);

  const scheduleClose = useCallback(() => {
    clearHideTimer();
    hideTimer.current = setTimeout(() => setOpen(false), HIDE_MS);
  }, [clearHideTimer]);

  const handleTriggerEnter = useCallback(() => {
    clearHideTimer();
    syncPanelPosition();
    setOpen(true);
  }, [clearHideTimer, syncPanelPosition]);

  useLayoutEffect(() => {
    if (!expandToViewport || !open) return;
    syncPanelPosition();
  }, [expandToViewport, open, syncPanelPosition, heading, linesSig]);

  useLayoutEffect(() => {
    if (!expandToViewport || !open) return;
    const onMove = () => syncPanelPosition();
    window.addEventListener('resize', onMove);
    window.addEventListener('scroll', onMove, true);
    return () => {
      window.removeEventListener('resize', onMove);
      window.removeEventListener('scroll', onMove, true);
    };
  }, [expandToViewport, open, syncPanelPosition]);

  useLayoutEffect(
    () => () => {
      clearHideTimer();
    },
    [clearHideTimer]
  );

  const alignClass = align === 'center' ? 'left-1/2 -translate-x-1/2' : 'left-0 translate-x-0';

  const tooltipInner = heading ? (
    <>
      <div className="mb-1.5 shrink-0 font-bold uppercase tracking-wide text-text-muted">{heading}</div>
      <ul
        className={`m-0 list-none space-y-1.5 overflow-y-auto p-0 pr-1 marker:hidden [scrollbar-gutter:stable] ${
          expandToViewport ? 'min-h-0 flex-1' : 'max-h-[min(70vh,28rem)]'
        }`}
      >
        {lines.map((line, i) => (
          <li key={i} className="break-words text-text-main [list-style:none]">
            {line}
          </li>
        ))}
      </ul>
    </>
  ) : (
    <p className="m-0 text-text-muted">{lines[0]}</p>
  );

  const portalTooltip =
    expandToViewport &&
    open &&
    typeof document !== 'undefined' &&
    createPortal(
      <div
        ref={panelRef}
        role="tooltip"
        aria-hidden={!open}
        onMouseEnter={clearHideTimer}
        onMouseLeave={scheduleClose}
        style={{
          position: 'fixed',
          top: panelGeom.top,
          left: panelGeom.left,
          maxHeight: panelGeom.maxHeight,
          zIndex: TOOLTIP_Z,
        }}
        className="pointer-events-auto flex w-max max-w-[calc(100vw-2rem)] min-w-[10rem] flex-col rounded-global border border-border-base bg-bg-card px-3 py-2 text-left text-xs text-text-main shadow-xl shadow-black/20"
      >
        {tooltipInner}
      </div>,
      document.body
    );

  if (expandToViewport) {
    return (
      <>
        <span ref={triggerRef} className={`inline-flex max-w-full ${className}`}>
          <span
            className="group inline-flex max-w-full cursor-help"
            aria-label={ariaLabel}
            onMouseEnter={handleTriggerEnter}
            onMouseLeave={scheduleClose}
          >
            {children}
          </span>
        </span>
        {portalTooltip}
      </>
    );
  }

  return (
    <span className={`relative inline-flex max-w-full ${className}`}>
      <span className="group inline-flex max-w-full cursor-help" aria-label={ariaLabel}>
        {children}
        <span
          className={`pointer-events-none absolute top-full z-[300] mt-1.5 ${alignClass} w-max max-w-[min(100vw-2rem,20rem)] rounded-global border border-border-base bg-bg-card px-3 py-2 text-left text-xs text-text-main shadow-xl opacity-0 shadow-black/20 transition-opacity duration-150 group-hover:opacity-100 group-focus-within:opacity-100`}
          role="tooltip"
        >
          {tooltipInner}
        </span>
      </span>
    </span>
  );
};

export default CastleFocusHoverPopover;

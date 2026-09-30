import React, { useCallback, useId, useLayoutEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { LocalizedText } from '../i18n/LocalizedText';
import { useLocalizedMessage } from '../i18n/useLocalizedMessage';
import { describeMessage } from '../i18n/messages';

const PANEL_WIDTH = 320;
const VIEWPORT_MARGIN = 12;
const HIDE_DELAY_MS = 180;

/**
 * Hover/focus panel for the Auto Station header chip (CIT-20). It only hosts the compact automation feedback
 * (phase, next step, a failed Start/Stop, first result) so the header says the same thing as the Automation page
 * row; the chip itself is unchanged and stays the one-click on/off control.
 */
const AutoStationHoverPopover: React.FC<{ feedback: React.ReactNode; children: React.ReactNode }> = ({ feedback, children }) => {
  const tooltipId = useId();
  const triggerRef = useRef<HTMLSpanElement>(null);
  const hideTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [open, setOpen] = useState(false);
  const [position, setPosition] = useState({ top: 0, left: VIEWPORT_MARGIN, maxHeight: 320, width: PANEL_WIDTH });
  const label = useLocalizedMessage(describeMessage('automationPopover.station.label'), 'Auto Station status');

  const clearHideTimer = useCallback(() => {
    if (hideTimer.current !== null) {
      window.clearTimeout(hideTimer.current);
      hideTimer.current = null;
    }
  }, []);

  const syncPosition = useCallback(() => {
    const trigger = triggerRef.current;
    if (!trigger) return;
    const rect = trigger.getBoundingClientRect();
    const width = Math.min(PANEL_WIDTH, window.innerWidth - VIEWPORT_MARGIN * 2);
    const left = Math.max(VIEWPORT_MARGIN, Math.min(rect.left + rect.width / 2 - width / 2, window.innerWidth - VIEWPORT_MARGIN - width));
    const top = rect.bottom + 8;
    setPosition({ top, left, width, maxHeight: Math.max(160, window.innerHeight - top - VIEWPORT_MARGIN) });
  }, []);

  const show = useCallback(() => {
    clearHideTimer();
    syncPosition();
    setOpen(true);
  }, [clearHideTimer, syncPosition]);

  const scheduleHide = useCallback(() => {
    clearHideTimer();
    hideTimer.current = window.setTimeout(() => {
      const focused = document.activeElement;
      if (focused && document.getElementById(tooltipId)?.contains(focused)) return;
      setOpen(false);
    }, HIDE_DELAY_MS);
  }, [clearHideTimer, tooltipId]);

  useLayoutEffect(() => {
    if (!open) return undefined;
    syncPosition();
    window.addEventListener('resize', syncPosition);
    window.addEventListener('scroll', syncPosition, true);
    return () => {
      window.removeEventListener('resize', syncPosition);
      window.removeEventListener('scroll', syncPosition, true);
    };
  }, [open, syncPosition]);

  useLayoutEffect(() => () => clearHideTimer(), [clearHideTimer]);

  const panel = open && typeof document !== 'undefined' && createPortal(
    <div
      id={tooltipId}
      role="dialog"
      aria-label={label.text}
      onFocusCapture={clearHideTimer}
      onBlurCapture={scheduleHide}
      onKeyDown={(event) => { if (event.key === 'Escape') { event.stopPropagation(); triggerRef.current?.querySelector('button')?.focus(); setOpen(false); } }}
      onMouseEnter={clearHideTimer}
      onMouseLeave={scheduleHide}
      style={{ position: 'fixed', top: position.top, left: position.left, width: position.width, maxHeight: position.maxHeight, zIndex: 460 }}
      className="flex flex-col overflow-hidden rounded-global border border-border-base bg-bg-card text-left text-xs text-text-main shadow-2xl shadow-black/30"
    >
      <div className="shrink-0 border-b border-border-base px-3.5 py-3">
        <div className="font-bold text-text-main"><LocalizedText messageKey="automationPopover.station.title" /></div>
        <div className="mt-0.5 text-[11px] text-text-muted"><LocalizedText messageKey="automationPopover.station.detail" /></div>
      </div>
      <div className="custom-scrollbar min-h-0 flex-1 overflow-y-auto px-3.5 py-2" data-popover-feedback="autoStation">{feedback}</div>
    </div>,
    document.body,
  );

  return (
    <>
      <span
        ref={triggerRef}
        className="inline-flex max-w-full"
        onMouseEnter={show}
        onMouseLeave={scheduleHide}
        onFocusCapture={show}
        onBlurCapture={scheduleHide}
        onKeyDown={(event) => { if (event.key === 'Escape') setOpen(false); }}
        aria-controls={open ? tooltipId : undefined}
        aria-haspopup="dialog"
        aria-expanded={open}
      >
        {children}
      </span>
      {panel}
    </>
  );
};

export default AutoStationHoverPopover;

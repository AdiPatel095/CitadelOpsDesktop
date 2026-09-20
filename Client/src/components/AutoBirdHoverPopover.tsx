import React, { useCallback, useId, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { RotateCw, Timer } from 'lucide-react';
import { useCitadelAPI } from '../api/ApiContext';
import { AutomationDurationModal } from '../settings/components/AutomationDurationModal';
import { createPortal } from 'react-dom';
import type { AutoBirdCastleCycle } from '../context/AuthContext';

interface AutoBirdHoverPopoverProps {
	cycles: AutoBirdCastleCycle[];
	enabled: boolean;
 canControl?: boolean;
	now: number;
	hint: string;
	children: React.ReactNode;
}

const PANEL_WIDTH = 352;
const VIEWPORT_MARGIN = 12;
const HIDE_DELAY_MS = 180;

function formatBirdCycle(msLeft: number): string {
	if (msLeft <= 0) return 'Due now';
	const totalMinutes = Math.ceil(msLeft / 60000);
	const days = Math.floor(totalMinutes / 1440);
	const hours = Math.floor((totalMinutes % 1440) / 60);
	const minutes = totalMinutes % 60;
	if (days > 0) return hours > 0 ? `${days}d ${hours}h` : `${days}d`;
	if (hours > 0) return minutes > 0 ? `${hours}h ${minutes}m` : `${hours}h`;
	return `${Math.max(1, minutes)}m`;
}

function kingdomName(kingdomId: number): string {
	switch (kingdomId) {
		case 0: return 'Great Empire';
		case 1: return 'Everwinter Glacier';
		case 2: return 'Burning Sands';
		case 3: return 'Fire Peaks';
		case 4: return 'Storm Islands';
		default: return `Kingdom ${kingdomId}`;
	}
}

function cyclePhaseLabel(cycle: AutoBirdCastleCycle): string {
	switch (cycle.phase) {
		case 'target-ready': return 'Target ready';
		case 'dispatch-ready': return 'Troops ready';
		case 'away': return cycle.nextCycleAtMs > 0 ? 'Returning' : 'Tracking movement';
		case 'waiting': return 'Waiting';
		default: return 'Not started';
	}
}

const AutoBirdHoverPopover: React.FC<AutoBirdHoverPopoverProps> = ({
	cycles,
	enabled,
 canControl = false,
	now,
	hint,
	children,
}) => {
 const { submitIntent } = useCitadelAPI();
 const [pending, setPending] = useState<number | null>(null);
 const [error, setError] = useState('');
 const [durationCastle, setDurationCastle] = useState<AutoBirdCastleCycle | null>(null);
 const controlCastle = async (castleId: number, action: 'pause' | 'resume' | 'resend', durationMinutes = 0) => {
  if (pending !== null) return;
  setPending(castleId);
  setError('');
  try {
   await submitIntent('auto_bird.castle_control', { sourceCastleId: castleId, action, durationMinutes }, { actor: 'ui:auto-bird' });
  } catch (value) {
   setError(value instanceof Error ? value.message : 'Could not update this castle.');
   throw value;
  } finally { setPending(null); }
 };
 const tooltipId = useId();
	const triggerRef = useRef<HTMLSpanElement>(null);
	const hideTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
	const [open, setOpen] = useState(false);
	const [position, setPosition] = useState({ top: 0, left: VIEWPORT_MARGIN, maxHeight: 360, width: PANEL_WIDTH });

	const activeCount = useMemo(
		() => cycles.filter((cycle) => cycle.nextCycleAtMs > 0).length,
		[cycles],
	);

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
		const left = Math.max(
			VIEWPORT_MARGIN,
			Math.min(rect.left + rect.width / 2 - width / 2, window.innerWidth - VIEWPORT_MARGIN - width),
		);
		const top = rect.bottom + 8;
		setPosition({
			top,
			left,
			width,
			maxHeight: Math.max(180, window.innerHeight - top - VIEWPORT_MARGIN),
		});
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
		if (!open) return;
		syncPosition();
		window.addEventListener('resize', syncPosition);
		window.addEventListener('scroll', syncPosition, true);
		return () => {
			window.removeEventListener('resize', syncPosition);
			window.removeEventListener('scroll', syncPosition, true);
		};
	}, [open, syncPosition, cycles.length]);

	useLayoutEffect(
		() => () => clearHideTimer(),
		[clearHideTimer],
	);

	const tooltip = open && typeof document !== 'undefined' && createPortal(
		<div
			id={tooltipId}
			role="dialog"
 aria-label="Auto Bird castle controls"
 onFocusCapture={clearHideTimer}
 onBlurCapture={scheduleHide}
 onKeyDown={(event) => { if (event.key === 'Escape') { event.stopPropagation(); triggerRef.current?.querySelector('button')?.focus(); setOpen(false); } }}
			onMouseEnter={clearHideTimer}
			onMouseLeave={scheduleHide}
			style={{
				position: 'fixed',
				top: position.top,
				left: position.left,
				width: position.width,
				maxHeight: position.maxHeight,
				zIndex: 460,
			}}
			className="flex flex-col overflow-hidden rounded-global border border-border-base bg-bg-card text-left text-xs text-text-main shadow-2xl shadow-black/30"
		>
			<div className="flex shrink-0 items-start justify-between gap-3 border-b border-border-base px-3.5 py-3">
				<div>
					<div className="font-bold text-text-main">Auto Bird cycles</div>
					<div className="mt-0.5 text-[11px] text-text-muted">Every owned castle’s next troop return</div>
				</div>
				<span className={`shrink-0 rounded-full border px-2 py-0.5 text-[10px] font-semibold ${
					enabled
						? 'border-success/35 bg-success/10 text-success'
						: 'border-error/35 bg-error/10 text-error'
				}`}>
					{enabled ? `${activeCount}/${cycles.length} active` : 'Off'}
				</span>
			</div>

			<div className="custom-scrollbar min-h-0 flex-1 overflow-y-auto px-2 py-2">
				{cycles.length === 0 ? (
					<div className="px-2 py-3 text-text-muted">No castles are available in the current game state.</div>
				) : (
					<ul className="m-0 list-none space-y-1 p-0 marker:hidden">
						{cycles.map((cycle) => {
							const active = cycle.nextCycleAtMs > 0;
       const paused = cycle.paused && (!cycle.pausedUntilMs || cycle.pausedUntilMs > now);
							return (
								<li
									key={cycle.castleId}
									className="flex items-center justify-between gap-3 rounded-global border border-transparent px-2 py-2 hover:border-border-base hover:bg-bg-tertiary/60"
								>
									<button type="button"
 disabled={!canControl || pending !== null}
 aria-pressed={!!paused}
 aria-label={`${cycle.castleName}: ${paused ? 'resume' : 'pause'} Auto Bird`}
 title="Click to pause or resume. Right-click for a timed pause."
 onClick={() => { void controlCastle(cycle.castleId, paused ? 'resume' : 'pause').catch(() => {}); }}
 onContextMenu={(event) => { event.preventDefault(); if (canControl && pending === null) setDurationCastle(cycle); }}
 className="flex min-w-0 flex-1 items-center gap-2.5 text-left disabled:opacity-50 focus-visible:outline focus-visible:outline-primary">

										<span className={`h-2 w-2 shrink-0 rounded-full ${paused ? 'bg-warning' : active ? 'bg-success shadow-[0_0_8px_var(--color-success)]' : 'bg-text-muted/35'}`} />
										<div className="min-w-0">
											<div className="truncate font-semibold text-text-main">{cycle.castleName}</div>
											<div className="truncate text-[10px] text-text-muted">{kingdomName(cycle.kingdomId)}</div>
											{cycle.statusDetail && (
												<div className="mt-0.5 max-w-[205px] truncate text-[10px] text-text-muted" title={cycle.statusDetail}>
													{cycle.statusDetail}
												</div>
											)}
										</div>
         </button>
         <div className="shrink-0 text-right">
										<div className={paused ? 'font-semibold text-warning' : active ? 'font-mono font-semibold text-success' : 'text-text-muted'}>
											{paused ? (cycle.pausedUntilMs ? `Paused ${formatBirdCycle(cycle.pausedUntilMs - now)}` : 'Paused') : cycle.rescanRequested ? 'Rescan queued' : active ? formatBirdCycle(cycle.nextCycleAtMs - now) : cyclePhaseLabel(cycle)}
										</div>
										{active && (
											<div className="mt-0.5 text-[10px] text-text-muted">
												Return {new Date(cycle.nextCycleAtMs).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })}
											</div>
										)}
										{cycle.travelSeconds != null && cycle.travelSeconds > 0 && (
											<div className="mt-0.5 text-[10px] text-text-muted">
												{formatBirdCycle(cycle.travelSeconds * 1000)} travel
											</div>
										)}
									</div>
         <div className="flex flex-col gap-1">
          <button type="button" disabled={!canControl || pending !== null} title={`Pause ${cycle.castleName} for a duration`} aria-label={`Timed pause for ${cycle.castleName}`} className="rounded p-1 text-text-muted hover:text-primary disabled:opacity-40" onClick={() => setDurationCastle(cycle)}><Timer size={13} /></button>
          <button type="button" disabled={!canControl || pending !== null || !!paused || !enabled} title={`Resend from ${cycle.castleName}: clear this cycle and scan fresh troops and a target`} aria-label={`Resend bird from ${cycle.castleName}`} className="rounded p-1 text-text-muted hover:text-primary disabled:opacity-40" onClick={() => { void controlCastle(cycle.castleId, 'resend').catch(() => {}); }}><RotateCw size={13} className={pending === cycle.castleId ? 'animate-spin' : ''} /></button>
         </div>
        </li>
							);
						})}
					</ul>
				)}
			</div>

			<div className="shrink-0 border-t border-border-base px-3.5 py-2 text-[10px] text-text-muted">
				Click a castle to pause or resume. Right-click or use the timer for a timed pause. Resend scans fresh troops and a target. Birds already away continue their journey.
    {error && <div role="alert" className="mt-1 text-error">{error}</div>}
    <div className="mt-1">{hint}</div>
			</div>
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
				onKeyDown={(event) => {
					if (event.key === 'Escape') setOpen(false);
     if (event.key === 'ArrowDown') {
      event.preventDefault();
      show();
      window.requestAnimationFrame(() => document.getElementById(tooltipId)?.querySelector('button')?.focus());
     }
				}}
				aria-controls={open ? tooltipId : undefined}
    aria-haspopup="dialog"
    aria-expanded={open}
			>
				{children}
			</span>
			{tooltip}
   {durationCastle && <AutomationDurationModal isOpen featureKey={`auto-bird-castle-${durationCastle.castleId}`} featureLabel={`${durationCastle.castleName} Auto Bird`} pausedUntil={durationCastle.pausedUntilMs} onClose={() => { setDurationCastle(null); triggerRef.current?.querySelector('button')?.focus(); }} onPauseFor={(minutes) => controlCastle(durationCastle.castleId, 'pause', minutes)} />}
		</>
	);
};

export default AutoBirdHoverPopover;

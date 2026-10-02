import { Button } from './ui/Button';
import './AutoBirdCycles.css';
import { useLocale } from "../i18n/LocaleContext";
import { formatDurationEnd } from '../i18n/automationDuration';
import { LocalizedText } from "../i18n/LocalizedText";
import React, { useMemo, useState } from 'react';
import { RotateCw, Timer } from 'lucide-react';
import { useCitadelAPI } from '../api/ApiContext';
import { AutomationDurationModal } from '../settings/components/AutomationDurationModal';
import type { AutoBirdCastleCycle } from '../context/AuthContext';

interface AutoBirdCyclesProps {
	cycles: AutoBirdCastleCycle[];
	enabled: boolean;
 canControl?: boolean;
	now: number;
	onBeforeDialog(): void;
	/** CIT-20 feedback (phase, next step, failed Start/Stop, first result), rendered under the castle list. */
}

type Translate = ReturnType<typeof useLocale>['t'];

function formatBirdCycle(msLeft: number, t: Translate): string {
 if (msLeft <= 0) return t('autoBird.cycles.dueNow');
 const totalMinutes = Math.ceil(msLeft / 60000);
 const days = Math.floor(totalMinutes / 1440);
 const hours = Math.floor((totalMinutes % 1440) / 60);
 const minutes = totalMinutes % 60;
 if (days > 0) return t(hours > 0 ? 'autoBird.cycles.daysHours' : 'autoBird.cycles.days', { days, hours });
 if (hours > 0) return t(minutes > 0 ? 'autoBird.cycles.hoursMinutes' : 'autoBird.cycles.hours', { hours, minutes });
 return t('autoBird.cycles.minutes', { minutes: Math.max(1, minutes) });
}

function kingdomName(kingdomId: number, t: Translate): string {
 const keys = ['autoBird.cycles.empire', 'autoBird.cycles.glacier', 'autoBird.cycles.sands', 'autoBird.cycles.peaks', 'autoBird.cycles.islands'] as const;
 return keys[kingdomId] ? t(keys[kingdomId]) : t('autoBird.cycles.kingdom');
}

function cyclePhaseLabel(cycle: AutoBirdCastleCycle, t: Translate): string {
 switch (cycle.phase) {
  case 'target-ready': return t('autoBird.cycles.targetReady');
  case 'dispatch-ready': return t('autoBird.cycles.troopsReady');
  case 'away': return t(cycle.nextCycleAtMs > 0 ? 'autoBird.cycles.returning' : 'autoBird.cycles.tracking');
  case 'waiting': return t('autoBird.cycles.waiting');
  default: return t('autoBird.cycles.notStarted');
 }
}

const AutoBirdCycles: React.FC<AutoBirdCyclesProps> = ({
	cycles,
	enabled,
 canControl = false,
	now,
	onBeforeDialog,
}) => {
  const { t, locale } = useLocale();
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
   setError(value instanceof Error ? value.message : t('autoBird.cycles.controlError'));
   throw value;
  } finally { setPending(null); }
 };
	const activeCount = useMemo(
		() => cycles.filter((cycle) => cycle.nextCycleAtMs > 0).length,
		[cycles],
	);

	return <>
		<div className="header-bird-cycles">
			<div className="flex shrink-0 items-start justify-between gap-3 border-b border-border-base px-3.5 py-3">
				<div>
					<div className="font-bold text-text-main"><LocalizedText messageKey="ui.components.autoBirdHoverPopover.auto.bird.cycles.6ee33c86" /></div>
					<div className="mt-0.5 text-caption text-text-muted"><LocalizedText messageKey="ui.components.autoBirdHoverPopover.every.owned.castle.s.next.troop.return.10ce7290" /></div>
				</div>
				<span className={`shrink-0 rounded-full border px-2 py-0.5 text-caption font-semibold ${
					enabled
						? 'border-success/35 bg-success/10 text-success'
						: 'border-error/35 bg-error/10 text-error'
				}`}>
					{enabled ? <LocalizedText messageKey="autoBird.cycles.active" params={{ active: activeCount, total: cycles.length }} /> : <LocalizedText messageKey="autoBird.cycles.off" />}
				</span>
			</div>

			<div className="custom-scrollbar min-h-0 flex-1 overflow-y-auto px-2 py-2">
				{cycles.length === 0 ? (
					<div className="px-2 py-3 text-text-muted"><LocalizedText messageKey="ui.components.autoBirdHoverPopover.no.castles.are.available.in.the.current.2c572692" /></div>
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
									<button data-button-pattern="tile" type="button"
 disabled={!canControl || pending !== null}
 aria-pressed={!!paused}
 aria-label={t('autoBird.cycles.toggle', { castle: cycle.castleName, action: paused ? 'resume' : 'pause' })}
 title={t('ui.components.autoBirdHoverPopover.title.click.to.pause.or.resume.right.click.4129d41e')}
 onClick={() => { void controlCastle(cycle.castleId, paused ? 'resume' : 'pause').catch(() => {}); }}
 onContextMenu={(event) => { event.preventDefault(); if (canControl && pending === null) { onBeforeDialog(); setDurationCastle(cycle); } }}
 className="flex min-w-0 flex-1 items-center gap-2.5 text-left disabled:opacity-50 focus-visible:outline focus-visible:outline-primary">

										<span className={`h-2 w-2 shrink-0 rounded-full ${paused ? 'bg-warning' : active ? 'bg-success' : 'bg-text-muted/35'}`} />
										<div className="min-w-0">
											<div className="truncate font-semibold text-text-main"><bdi>{cycle.castleName}</bdi></div>
											<div className="truncate text-caption text-text-muted"><bdi>{kingdomName(cycle.kingdomId, t)}</bdi></div>
										</div>
         </button>
         <div className="shrink-0 text-right">
										<div className={paused ? 'font-semibold text-warning' : active ? 'font-mono font-semibold text-success' : 'text-text-muted'}>
											<bdi>{paused ? (cycle.pausedUntilMs ? t('autoBird.cycles.pausedFor', { duration: formatBirdCycle(cycle.pausedUntilMs - now, t) }) : t('autoBird.cycles.paused')) : cycle.rescanRequested ? t('autoBird.cycles.rescanQueued') : active ? formatBirdCycle(cycle.nextCycleAtMs - now, t) : cyclePhaseLabel(cycle, t)}</bdi>
										</div>
										{active && (
											<div className="mt-0.5 text-caption text-text-muted">
												<LocalizedText messageKey="autoBird.cycles.returnTime" params={{ time: formatDurationEnd(cycle.nextCycleAtMs, locale) }} />
											</div>
										)}
										{cycle.travelSeconds != null && cycle.travelSeconds > 0 && (
											<div className="mt-0.5 text-caption text-text-muted">
												<LocalizedText messageKey="autoBird.cycles.travel" params={{ duration: formatBirdCycle(cycle.travelSeconds * 1000, t) }} />
											</div>
										)}
									</div>
         <div className="flex flex-col gap-1">
          <Button iconOnly variant="ghost" type="button" disabled={!canControl || pending !== null} title={t('autoBird.cycles.pauseTitle', { castle: cycle.castleName })} aria-label={t('autoBird.cycles.pauseLabel', { castle: cycle.castleName })}  onClick={() => { onBeforeDialog(); setDurationCastle(cycle); }}><Timer size={13} /></Button>
          <Button iconOnly variant="ghost" type="button" disabled={!canControl || pending !== null || !!paused || !enabled} title={t('autoBird.cycles.resendTitle', { castle: cycle.castleName })} aria-label={t('autoBird.cycles.resendLabel', { castle: cycle.castleName })}  onClick={() => { void controlCastle(cycle.castleId, 'resend').catch(() => {}); }}><RotateCw size={13} className={pending === cycle.castleId ? 'animate-spin' : ''} /></Button>
         </div>
        </li>
							);
						})}
					</ul>
				)}
			</div>

    {error && <div role="alert">{error}</div>}
  </div>
  {durationCastle && <AutomationDurationModal isOpen featureKey={`auto-bird-castle-${durationCastle.castleId}`} featureLabel={t('autoBird.cycles.featureLabel', { castle: durationCastle.castleName })} pausedUntil={durationCastle.pausedUntilMs} onClose={() => setDurationCastle(null)} onPauseFor={(minutes) => controlCastle(durationCastle.castleId, 'pause', minutes)} />}
  </>;
};
export default AutoBirdCycles;

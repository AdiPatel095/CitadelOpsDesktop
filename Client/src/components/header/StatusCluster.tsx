import { useId, useRef, useState } from 'react';
import { CircleMinus, CirclePause, Clock, ShieldAlert, Swords, TriangleAlert } from 'lucide-react';
import { useLocale } from '../../i18n/LocaleContext';
import { formatMessage } from '../../i18n/formatMessage';
import { automationDuration } from '../../i18n/automationDuration';
import { PLAYER_STATUS_ROLE } from '../../settings/readiness/playerStatus';
import { PLAYER_STATUS_ICON } from '../ui/StatusBadge';
import { Popover } from '../ui/Popover';
import { attacksText, clusterName, compactCount } from './headerStatus';
import { useHeaderStatus } from './useHeaderStatus';
import { StatusPanel, type StatusPanelActions } from './StatusPanel';
import './header-status.css';
export type HeaderStatus = ReturnType<typeof useHeaderStatus>;
export function StatusCluster({ surface, data, ...actions }: StatusPanelActions & { surface: 'desktop' | 'hosted'; data: HeaderStatus }) {
  const locale = useLocale(); const { t, message, catalog } = locale;
  const [open, setOpen] = useState(false); const anchor = useRef<HTMLButtonElement>(null); const titleId = useId();
  const close = () => { setOpen(false); anchor.current?.focus(); };
  const label = message(`playerStatus.${data.connection.status}`);
  const reason = formatMessage(data.connection.reason, locale.locale, catalog);
  const offlineTime = data.presence.checkpointObservedAt ? Date.parse(data.presence.checkpointObservedAt) : NaN;
  const offline = data.presence.mode === 'checkpoint' && Number.isFinite(offlineTime);
  const connection = offline ? t('copy.offlineSaved', { time: new Intl.DateTimeFormat(locale.locale, { dateStyle: 'short', timeStyle: 'short' }).format(offlineTime) }) : `${label.text} · ${reason.text}`;
  const connectionWord = offline ? connection.split(' · ')[0] : label.text;
  const connectionReason = offline ? connection.slice(connection.indexOf(' · ') + 3) : reason.text;
  const signal = data.signal;
  const count = compactCount(signal);
  const CountIcon = count?.glyph === 'shield-alert' ? ShieldAlert : count?.glyph === 'triangle-alert' ? TriangleAlert : count?.glyph === 'circle-minus' ? CircleMinus : CirclePause;
  const ConnectionIcon = PLAYER_STATUS_ICON[data.connection.status];
  const duration = (ms: number) => automationDuration(Math.max(0, Math.ceil(ms / 60_000)), locale.locale);
  const signalText = !signal ? '' : signal.kind === 'incoming' ? t('header.signal.incoming', { count: signal.count, state: signal.firstImpactInMs === null ? 'other' : 'known', duration: signal.firstImpactInMs === null ? '' : duration(signal.firstImpactInMs) })
    : signal.kind === 'attention' ? t('header.signal.attention', { count: signal.count })
    : t('header.signal.nextBird', { castle: signal.castleName, state: signal.dueInMs === 0 ? 'due' : 'other', duration: duration(signal.dueInMs) });
  const attacks = attacksText(data, locale);
  const SignalIcon = signal?.kind === 'nextBird' ? Clock : CountIcon;
  return <>
    <button type="button" ref={anchor} className="header-status-cluster" data-button-pattern="disclosure" aria-haspopup="dialog" aria-expanded={open} aria-controls={titleId + '-panel'}
      aria-label={clusterName([connection, signalText, attacks.text], locale.locale)} onClick={() => open ? close() : setOpen(true)}>
      <span className="header-status-connection" data-status-role={PLAYER_STATUS_ROLE[data.connection.status]}>
        <ConnectionIcon aria-hidden="true" /><span className="header-status-word" lang={label.resolvedLocale}>{connectionWord}</span><span className="header-status-reason" lang={reason.resolvedLocale}> · {connectionReason}</span>
      </span>
      {signal && <span className="header-status-signal" data-status-role={signal.kind === 'nextBird' ? 'neutral' : count?.tone}><SignalIcon aria-hidden="true" /><span>{signalText}</span></span>}
      {count && <span className="header-status-count" data-status-role={count.tone}><CountIcon aria-hidden="true" />{locale.number(count.count)}</span>}
      <span className="header-status-attacks" title={attacks.title}><Swords aria-hidden="true" />{attacks.text}</span>
    </button>
    <span className="sr-only" aria-live="polite">{connection}</span>
    <Popover open={open} onClose={close} anchorRef={anchor} labelledBy={titleId}>
      <StatusPanel {...actions} surface={surface} data={data} open={open} titleId={titleId} onBeforeDialog={close} />
    </Popover>
  </>;
}

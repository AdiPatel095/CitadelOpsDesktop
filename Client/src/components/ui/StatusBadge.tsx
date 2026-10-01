import { useEffect, useId, useRef, useState } from 'react';
import { CirclePlay, Clock, CircleCheck, CirclePause, CircleMinus, TriangleAlert, Power, CircleHelp } from 'lucide-react';
import { useLocale } from '../../i18n/LocaleContext';
import { useLocalizedMessage } from '../../i18n/useLocalizedMessage';
import { messageLanguageAttributes } from '../../i18n/messageLanguage';
import { PLAYER_STATUS_ROLE } from '../../settings/readiness/playerStatus';
import type { PlayerStatusDescription } from '../../settings/readiness/playerStatus';
import './StatusBadge.css';

export const PLAYER_STATUS_ICON = {
  running: CirclePlay, waiting: Clock, done: CircleCheck, paused: CirclePause,
  blocked: CircleMinus, 'needs-attention': TriangleAlert, off: Power, unknown: CircleHelp,
};
export type PlayerStatusMessage = { text: string; translated: boolean; resolvedLocale: string };
export interface StatusBadgeProps extends PlayerStatusDescription {
  inline?: boolean;
  /** A parent header panel or item exposes the full reason on touch. */
  canRevealReason?: boolean;
  indicator?: boolean;
  /** Portal shell uses its own locale provider. */
  labelMessage?: PlayerStatusMessage;
  reasonMessage?: PlayerStatusMessage;
}
export function StatusBadge({ status, reason, inline = false, canRevealReason = false, indicator = false, labelMessage, reasonMessage }: StatusBadgeProps) {
  const { message } = useLocale();
  const label = labelMessage ?? message(`playerStatus.${status}`);
  const localizedDetail = useLocalizedMessage(reason, '');
  const detail = reasonMessage ?? localizedDetail;
  const tooltipId = useId();
  const Icon = PLAYER_STATUS_ICON[status];
  const full = `${label.text} · ${detail.text}`;
  const compact = inline && canRevealReason;
  const rowRef = useRef<HTMLSpanElement>(null);
  const badgeRef = useRef<HTMLSpanElement>(null);
  const reasonRef = useRef<HTMLSpanElement>(null);
  const separatorRef = useRef<HTMLSpanElement>(null);
  const [reasonLayout, setReasonLayout] = useState({ wrapped: false, separatorWidth: 0 });
  useEffect(() => {
    if (compact) return;
    const row = rowRef.current;
    const badge = badgeRef.current;
    const detail = reasonRef.current;
    const separator = separatorRef.current;
    if (!row || !badge || !detail || !separator) return;
    const measure = () => {
      const wrapped = detail.getBoundingClientRect().top >= badge.getBoundingClientRect().bottom;
      const separatorWidth = separator.getBoundingClientRect().width;
      setReasonLayout(previous => previous.wrapped === wrapped && previous.separatorWidth === separatorWidth
        ? previous : { wrapped, separatorWidth });
    };
    measure();
    const observer = new ResizeObserver(measure);
    for (const element of [row, badge, detail, separator]) observer.observe(element);
    return () => observer.disconnect();
  }, [compact, label.text, detail.text]);
  const badge = <span ref={badgeRef} className={`player-status-badge${indicator ? ' player-status-indicator' : ''}`} data-player-status={status} data-status-role={PLAYER_STATUS_ROLE[status]}>
    <Icon aria-hidden="true" className="player-status-glyph" />
    <span className="player-status-word" {...messageLanguageAttributes(label)}>{label.text}</span>
    {compact ? <span className="player-status-inline-reason" {...messageLanguageAttributes(detail)}> · {detail.text}</span> : null}
  </span>;
  return compact ? <span className="player-status-inline" role="status" aria-label={full} aria-describedby={tooltipId} tabIndex={0}>
    {badge}<span className="player-status-tooltip" id={tooltipId} role="tooltip">{full}</span>
  </span> : <span ref={rowRef} className="player-status-row" role="status" aria-label={full}>
    {badge}<span ref={reasonRef} className="player-status-card-reason" data-wrapped={reasonLayout.wrapped}
      style={reasonLayout.wrapped ? { textIndent: -reasonLayout.separatorWidth, paddingInlineEnd: reasonLayout.separatorWidth } : undefined} {...messageLanguageAttributes(detail)}>
      <span ref={separatorRef} className="player-status-reason-separator" aria-hidden={true}>{'· '}</span>{detail.text}
    </span>
  </span>;
}

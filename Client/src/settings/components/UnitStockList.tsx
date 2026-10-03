import React from 'react';
import { AlertTriangle, CheckCircle2, HelpCircle, XCircle, type LucideIcon } from 'lucide-react';
import ToolImage from '../../components/ToolImage';
import UnitImage from '../../components/UnitImage';
import { useMetadata } from '../../context/MetadataContext';
import { useLocale } from '../../i18n/LocaleContext';
import { LocalizedText } from '../../i18n/LocalizedText';
import type { MessageKey } from '../../i18n/messages';
import type { ObservationFreshness } from '../requirements/observationFreshness';
import type { UnitStockLine, UnitStockLineState } from '../requirements/unitRequirements';

const LINE_ICON: Record<UnitStockLineState, { icon: LucideIcon; tone: string; label: MessageKey }> = {
  valid: { icon: CheckCircle2, tone: 'text-success', label: 'unitStock.state.validInCastle' },
  short: { icon: AlertTriangle, tone: 'text-warning', label: 'unitStock.state.short' },
  missing: { icon: XCircle, tone: 'text-error', label: 'unitStock.state.missingInCastle' },
  unknown: { icon: HelpCircle, tone: 'text-error', label: 'unitStock.state.unknown' },
};

export interface UnitStockListProps {
  lines: readonly UnitStockLine[];
  /** `reserve` lines are amounts kept in the castle rather than sent. */
  mode?: 'required' | 'reserve';
  /** Shown under the list, for example when quantities are decided at launch. */
  note?: React.ReactNode;
  /** How current the counts are; a real per-castle time is shown as "As of <time>" (CIT-20). */
  freshness?: ObservationFreshness | null;
}

/** Required (or reserved) troops and tools versus observed stationed stock. */
export const UnitStockList: React.FC<UnitStockListProps> = ({ lines, mode = 'required', note, freshness }) => {
  const { getTroop, getTool } = useMetadata();
  const { t: localizeStatic } = useLocale();
  if (lines.length === 0 && !note) return null;
  return (
    <div className="space-y-1.5">
      {lines.length > 0 ? (
        <ul className="space-y-1">
          {lines.map((line) => {
            const meta = LINE_ICON[line.state];
            const Icon = meta.icon;
            const item = line.kind === 'troop' ? getTroop(line.itemId) : getTool(line.itemId);
            return (
              <li key={`${line.kind}:${line.itemId}`} className="flex items-center gap-2 text-xs text-text-main">
                {line.kind === 'troop'
                  ? <UnitImage unitId={line.itemId} size={24} />
                  : <ToolImage toolId={line.itemId} size={24} showLevel={false} />}
                <span className="min-w-0 flex-1 truncate">{item?.name ?? `#${line.itemId}`}</span>
                <span className="shrink-0 font-mono text-[11px] tabular-nums text-text-muted">
                  <LocalizedText
                    messageKey={mode === 'reserve' ? 'unitStock.reserveInCastle' : 'unitStock.requiredInCastle'}
                    params={{ required: line.required, stationed: line.stationed }}
                  />
                </span>
                <Icon className={`h-3.5 w-3.5 shrink-0 ${meta.tone}`} aria-label={localizeStatic(meta.label)} />
              </li>
            );
          })}
        </ul>
      ) : null}
      {note ? <p className="text-[11px] text-text-muted">{note}</p> : null}
      {freshness?.state === 'observed' && freshness.scope === 'castle' && freshness.observedAt ? (
        <p className="text-[11px] text-text-muted"><LocalizedText messageKey="observedAt.castleUnits" params={{ observedAt: Date.parse(freshness.observedAt) }} /></p>
      ) : null}
    </div>
  );
};

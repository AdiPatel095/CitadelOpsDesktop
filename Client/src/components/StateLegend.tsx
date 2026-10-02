import React from 'react';
import { LocalizedText } from '../i18n/LocalizedText';
import type { StateLegendValues } from '../settings/onboarding/useChecklist';

/**
 * The five things a first setup keeps apart (CIT-19): the draft, what is saved, whether the game is connected, whether
 * the automation is on, and whether it produced a confirmed first result. Each is shown with its own value and none
 * implies another. Only a confirmed first result is green; "on" is the primary color; everything else is neutral or a
 * warning. No sentence here combines them.
 */
export const StateLegend: React.FC<{ values: StateLegendValues; compact?: boolean; className?: string }> = ({ values, compact = false, className = '' }) => {
  const savedAt = values.saved.at ? Date.parse(values.saved.at) : undefined;
  const confirmedAt = values.firstResult.at ? Date.parse(values.firstResult.at) : undefined;
  const items: Array<{ id: string; label: React.ReactNode; value: React.ReactNode; tone: string }> = [
    {
      id: 'draft', label: <LocalizedText messageKey="legend.draft" />, tone: values.draft === 'none' ? 'text-text-muted' : 'text-warning',
      value: <LocalizedText messageKey="legend.draft.value" params={{ state: values.draft }} />,
    },
    {
      id: 'saved', label: <LocalizedText messageKey="legend.saved" />, tone: 'text-text-main',
      value: !values.saved.exists
        ? <LocalizedText messageKey="legend.saved.value" params={{ state: 'never' }} />
        : savedAt !== undefined
          ? <LocalizedText messageKey="legend.saved.value" params={{ state: 'at', time: savedAt }} />
          : <LocalizedText messageKey="legend.saved.value" params={{ state: 'known' }} />,
    },
    {
      id: 'connected', label: <LocalizedText messageKey="legend.connected" />,
      tone: values.connected === 'yes' ? 'text-text-main' : values.connected === 'unknown' ? 'text-text-muted' : 'text-warning',
      value: <LocalizedText messageKey="legend.connected.value" params={{ state: values.connected }} />,
    },
    {
      id: 'on', label: <LocalizedText messageKey="legend.on" />, tone: values.on.on ? 'text-primary' : 'text-text-muted',
      value: <LocalizedText messageKey="legend.on.value" params={values.on.on
        ? values.on.timedUntil ? { state: 'until', time: values.on.timedUntil } : { state: 'on' }
        : { state: 'off' }} />,
    },
    {
      id: 'first-result', label: <LocalizedText messageKey="legend.firstResult" />,
      tone: values.firstResult.state === 'confirmed' ? 'text-success' : values.firstResult.state === 'failed' ? 'text-error' : 'text-text-muted',
      value: <LocalizedText messageKey="legend.firstResult.value" params={values.firstResult.state === 'confirmed' && confirmedAt !== undefined
        ? { state: 'confirmed', time: confirmedAt }
        : { state: values.firstResult.state === 'failed' ? 'failed' : values.firstResult.state === 'confirmed' ? 'confirmedUnknown' : 'none' }} />,
    },
  ];
  return (
    <dl className={`flex flex-wrap gap-x-3 gap-y-1 text-caption ${className}`} data-state-legend data-legend-compact={compact ? 'true' : undefined} aria-label={undefined}>
      {items.map((item) => (
        <div key={item.id} className="flex items-baseline gap-1" data-legend={item.id}>
          <dt className="font-bold text-text-muted">{item.label}:</dt>
          <dd className={`m-0 font-semibold ${item.tone}`}>{item.value}</dd>
        </div>
      ))}
    </dl>
  );
};

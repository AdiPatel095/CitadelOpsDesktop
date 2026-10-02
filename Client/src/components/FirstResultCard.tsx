import React from 'react';
import { CheckCircle2, CircleDashed, Clock3, XCircle } from 'lucide-react';
import { LocalizedText } from '../i18n/LocalizedText';
import { messageLanguageAttributes } from '../i18n/messageLanguage';
import { useLocalizedMessage } from '../i18n/useLocalizedMessage';
import type { FirstResult, FirstResultState } from '../settings/readiness/firstResult';

const TONE: Record<FirstResultState, string> = {
  confirmed: 'text-success',
  'in-progress': 'text-primary',
  failed: 'text-error',
  none: 'text-text-muted',
  unattributable: 'text-text-muted',
};

const Icon: React.FC<{ state: FirstResultState }> = ({ state }) => {
  const className = `mt-0.5 h-3.5 w-3.5 shrink-0 ${TONE[state]}`;
  if (state === 'confirmed') return <CheckCircle2 className={className} aria-hidden="true" />;
  if (state === 'failed') return <XCircle className={className} aria-hidden="true" />;
  if (state === 'in-progress') return <Clock3 className={className} aria-hidden="true" />;
  return <CircleDashed className={className} aria-hidden="true" />;
};

/**
 * First confirmed result (CIT-20). Green only for `confirmed`: a receipt attributed to this automation that the
 * engine finished, after it was turned on for this account. Everything else (nothing yet, in progress, failed,
 * not attributable) is neutral, blue or red, and never reads as success.
 */
export const FirstResultCard: React.FC<{ result: FirstResult; accountLabel?: string; className?: string }> = ({ result, accountLabel, className = '' }) => {
  const reason = useLocalizedMessage(result.failure?.descriptor, result.failure?.text ?? '');
  return (
    <div className={`rounded-lg border border-border-base bg-bg-app/40 px-3 py-2 text-xs leading-relaxed ${className}`} data-first-result={result.state}>
      <div className="flex items-start gap-2">
        <Icon state={result.state} />
        <div className="min-w-0 flex-1">
          <div className="text-[10px] font-black uppercase tracking-wider text-text-muted"><LocalizedText messageKey="firstResult.title" /></div>
          <p className={result.state === 'confirmed' ? 'font-semibold text-text-main' : 'text-text-main'}>
            <LocalizedText messageKey={result.summaryKey} params={result.params} />
          </p>
          {result.state === 'failed' && result.failure ? (
            <p className="text-text-muted" {...messageLanguageAttributes(reason)}>{reason.text}</p>
          ) : null}
          {accountLabel && (result.state === 'confirmed' || result.state === 'in-progress' || result.state === 'failed') ? (
            <p className="text-text-muted"><LocalizedText messageKey="firstResult.account" params={{ account: accountLabel }} /></p>
          ) : null}
          {result.counter !== undefined && result.state !== 'confirmed' ? (
            <p className="text-text-muted"><LocalizedText messageKey="firstResult.counterOnly" params={{ count: result.counter }} /></p>
          ) : null}
          {(result.state === 'confirmed' || result.state === 'in-progress') && !result.sinceKnown ? (
            <p className="text-text-muted"><LocalizedText messageKey="firstResult.sinceUnknown" /></p>
          ) : null}
        </div>
      </div>
    </div>
  );
};

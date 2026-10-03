import type { AttackLaunchRatesV2, AutomationStateV2, IntentReceipt } from '../../api/Contracts';
import type { LocalizedMessage } from '../../i18n/formatMessage';
import type { MessageKey, MessageParameters } from '../../i18n/messages';
import { attributedReceipts, automationActor, isActiveReceipt, isGameAction } from './automationAttribution';
import { observationTimestamp } from '../requirements/observationFreshness';

/**
 * First confirmed action of an automation (CIT-20).
 *
 * Confirmed means a receipt attributed to this feature (actor rule in
 * `automationAttribution.ts`) that the engine finished as `succeeded` with
 * phase `completed`, that is a game action (not a read or a config
 * follow-up), that carries executed-step evidence, and that was submitted
 * after the automation was turned on for this account. Saving, turning on,
 * the game reporting `running` or `ready`, and launch counters can never
 * confirm anything: they are not inputs to the decision (counters are only
 * reported back as a note).
 */
export type FirstResultState = 'none' | 'in-progress' | 'confirmed' | 'failed' | 'unattributable';

export type FirstResultOutcome = 'failed' | 'partial' | 'unconfirmed';

export interface FirstResult {
  state: FirstResultState;
  receiptId?: string;
  /** Machine intent name, kept for context. */
  intent?: string;
  /** The engine's own plan summary, verbatim, when it supplied one. */
  summary?: string;
  completedAt?: string;
  outcome?: FirstResultOutcome;
  failure?: { text: string; descriptor?: LocalizedMessage };
  /** True when the turn-on time of this account is known, so "first" means since turning it on. */
  sinceKnown: boolean;
  /** Launches the counters report for this feature; context only, never a confirmation. */
  counter?: number;
  summaryKey: MessageKey;
  params: MessageParameters;
}

export interface FirstResultInputs {
  operations: Record<string, IntentReceipt>;
  /** Accepted to make explicit that the game's status never decides the result. */
  runtime?: AutomationStateV2;
  launchRates?: Pick<AttackLaunchRatesV2, 'launchesByFeature'> | null;
  /** When this account turned the automation on (ISO); receipts submitted earlier are ignored. */
  enabledSince?: string;
  accountKey: string;
  /** Player-facing account name, shown with the result. */
  accountLabel?: string;
}

const message = (key: MessageKey): MessageKey => key;

function millis(value: string | undefined): number | undefined {
  const usable = observationTimestamp(value);
  return usable ? Date.parse(usable) : undefined;
}

function hasStepEvidence(receipt: IntentReceipt): boolean {
  return (receipt.completedStepIndexes?.length ?? 0) > 0
    || (receipt.evidence?.length ?? 0) > 0
    || (receipt.exchanges ?? []).some((exchange) => exchange.response != null);
}

function failureOf(receipt: IntentReceipt): FirstResult['failure'] {
  const presentation = receipt.failure;
  if (presentation?.message) return { text: presentation.message, descriptor: presentation.messageDescriptor };
  return receipt.error ? { text: receipt.error } : undefined;
}

export function firstConfirmedResult(featureId: string, inputs: FirstResultInputs): FirstResult {
  const account = inputs.accountLabel?.trim() ?? '';
  // `scope` selects "(this account)" in the sentence; the account name itself is shown by the card.
  const context = { scope: account ? 'account' : 'plain' };
  const since = millis(inputs.enabledSince);
  const sinceKnown = since !== undefined;
  const counterValue = inputs.launchRates?.launchesByFeature?.[featureId];
  const counter = typeof counterValue === 'number' && counterValue > 0 ? counterValue : undefined;
  const common = { sinceKnown, ...(counter !== undefined ? { counter } : {}) };

  if (!automationActor(featureId)) {
    return { ...common, state: 'unattributable', summaryKey: message('firstResult.unattributable'), params: context };
  }

  const candidates = attributedReceipts(featureId, inputs.operations)
    .filter(isGameAction)
    .filter((receipt) => {
      if (since === undefined) return true;
      const submitted = millis(receipt.submittedAt);
      return submitted !== undefined && submitted >= since;
    })
    .sort((left, right) => (millis(left.submittedAt) ?? 0) - (millis(right.submittedAt) ?? 0));

  const confirmed = candidates
    .filter((receipt) => receipt.status === 'succeeded' && receipt.phase === 'completed'
      && millis(receipt.completedAt) !== undefined && hasStepEvidence(receipt))
    .sort((left, right) => (millis(left.completedAt) ?? 0) - (millis(right.completedAt) ?? 0))[0];
  if (confirmed) {
    return {
      ...common, state: 'confirmed', receiptId: confirmed.id, intent: confirmed.intent,
      summary: confirmed.plan?.summary?.trim() || undefined, completedAt: confirmed.completedAt,
      summaryKey: message('firstResult.confirmed'),
      params: { ...context, what: confirmed.plan?.summary?.trim() || confirmed.intent, completedAt: Date.parse(confirmed.completedAt!) },
    };
  }

  const running = candidates.find((receipt) => isActiveReceipt(receipt));
  if (running) {
    return {
      ...common, state: 'in-progress', receiptId: running.id, intent: running.intent,
      summary: running.plan?.summary?.trim() || undefined,
      summaryKey: message('firstResult.inProgress'), params: { ...context, what: running.plan?.summary?.trim() || running.intent },
    };
  }

  const failed = candidates.find((receipt) => (
    receipt.status === 'failed' || receipt.status === 'partially_succeeded' || receipt.status === 'indeterminate'
  ));
  if (failed) {
    const outcome: FirstResultOutcome = failed.status === 'failed' ? 'failed'
      : failed.status === 'partially_succeeded' ? 'partial' : 'unconfirmed';
    const failure = failureOf(failed);
    return {
      ...common, state: 'failed', receiptId: failed.id, intent: failed.intent, outcome,
      summary: failed.plan?.summary?.trim() || undefined, completedAt: failed.completedAt,
      ...(failure ? { failure } : {}),
      summaryKey: message(failure ? 'firstResult.failed' : 'firstResult.failedNoReason'),
      params: { ...context, outcome, what: failed.plan?.summary?.trim() || failed.intent },
    };
  }

  return { ...common, state: 'none', summaryKey: message('firstResult.none'), params: context };
}

// ——— Turn-on time, per account ———

const STORAGE_PREFIX = 'citadelops.automation.enabledSince.v1.';

export interface EnabledSinceStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}

function readMap(storage: EnabledSinceStorage | undefined, accountKey: string): Record<string, string> {
  try {
    const raw = storage?.getItem(STORAGE_PREFIX + accountKey);
    const parsed: unknown = raw ? JSON.parse(raw) : {};
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
      ? Object.fromEntries(Object.entries(parsed as Record<string, unknown>).filter((entry): entry is [string, string] => typeof entry[1] === 'string'))
      : {};
  } catch {
    return {};
  }
}

/** Turn-on times of one account (`automation.enabled` key -> ISO time). Scoped by account: another account never reads them. */
export function readEnabledSince(storage: EnabledSinceStorage | undefined, accountKey: string): Record<string, string> {
  return accountKey ? readMap(storage, accountKey) : {};
}

export function recordEnabledSince(storage: EnabledSinceStorage | undefined, accountKey: string, enabledKey: string, at: string): Record<string, string> {
  const next = { ...readMap(storage, accountKey), [enabledKey]: at };
  if (accountKey) {
    try { storage?.setItem(STORAGE_PREFIX + accountKey, JSON.stringify(next)); } catch { /* private mode: the time stays in memory */ }
  }
  return next;
}

export function clearEnabledSince(storage: EnabledSinceStorage | undefined, accountKey: string, enabledKey: string): Record<string, string> {
  const next = { ...readMap(storage, accountKey) };
  delete next[enabledKey];
  if (accountKey) {
    try { storage?.setItem(STORAGE_PREFIX + accountKey, JSON.stringify(next)); } catch { /* private mode */ }
  }
  return next;
}

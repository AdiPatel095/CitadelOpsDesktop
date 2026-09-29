import type { IntentReceipt, IntentStatus } from '../../api/Contracts';
import type { MessageKey, MessageParameters } from '../../i18n/messages';
import type { AutomationEnabledControl } from '../AutomationEnabled';
import { attributedReceipts, isActiveReceipt, isGameAction } from './automationAttribution';

/**
 * What Stop does, stated from the actual code paths (CIT-20). Stop writes
 * `automation.enabled[<key>] = false`. On that configuration change the
 * Automation coordinator makes no new decisions for the feature and cancels
 * the operation it is running (`cancelRunsDisallowedByConfiguration` in
 * `Server/Automation/Coordinator.go`): the engine stops at its next step
 * boundary, and a command that was already sent is marked unconfirmed rather
 * than recalled (`Server/Intent/Engine.go`). Nothing here promises recall,
 * undo or instant cancellation, and no per-operation cancel is offered: Stop
 * already asks the running operation to stop, and receipts do not say
 * whether a single operation can still be cancelled.
 */
export interface InFlightOperation {
  id: string;
  intent: string;
  status: IntentStatus;
  summary?: string;
  startedAt?: string;
}

export interface StopLine {
  key: MessageKey;
  params?: MessageParameters;
}

export interface StopSemantics {
  /** Actions of this feature that are still running or waiting to run. */
  inFlight: InFlightOperation[];
  /** The truthful statements to show beside Stop, in reading order. */
  lines: StopLine[];
  timed: boolean;
}

const line = (key: MessageKey, params?: MessageParameters): StopLine => (params ? { key, params } : { key });

export function describeStopSemantics(
  featureId: string,
  input: {
    enabled: AutomationEnabledControl | undefined;
    operations: Record<string, IntentReceipt>;
    connected: boolean;
    now: number;
  },
): StopSemantics {
  const inFlight = attributedReceipts(featureId, input.operations)
    .filter((receipt) => isActiveReceipt(receipt) && isGameAction(receipt))
    .sort((left, right) => Date.parse(left.submittedAt) - Date.parse(right.submittedAt))
    .map((receipt): InFlightOperation => ({
      id: receipt.id,
      intent: receipt.intent,
      status: receipt.status,
      ...(receipt.plan?.summary?.trim() ? { summary: receipt.plan.summary.trim() } : {}),
      ...(receipt.startedAt ? { startedAt: receipt.startedAt } : {}),
    }));
  const timed = Boolean(input.enabled?.enabled && input.enabled.expiresAtMs && input.enabled.expiresAtMs > input.now);
  const lines: StopLine[] = [
    line('stopSemantics.switch'),
    line('stopSemantics.newDecisions'),
    line('stopSemantics.running'),
    line('stopSemantics.notRecalled'),
  ];
  if (inFlight.length > 0) lines.push(line('stopSemantics.inFlight', { count: inFlight.length }));
  if (timed) lines.push(line('stopSemantics.timed', { until: input.enabled!.expiresAtMs! }));
  if (!input.connected) lines.push(line('stopSemantics.offline'));
  return { inFlight, lines, timed };
}

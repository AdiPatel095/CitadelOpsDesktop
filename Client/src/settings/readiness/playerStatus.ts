import type { AutomationStateV2 } from '../../api/Contracts';
import type { LocalizedMessage } from '../../i18n/formatMessage';
import { formatMessage } from '../../i18n/formatMessage';
import { describeMessage } from '../../i18n/messages';
import type { AutomationEnabledControl } from '../AutomationEnabled';
import type { RepairInput } from '../connection/connectionExplain';
import { describeAutomationState } from './runtimeState';
import type { AutomationStateContext, RuntimeDetail } from './runtimeState';

export type PlayerStatus = 'running' | 'waiting' | 'done' | 'paused' | 'blocked' | 'needs-attention' | 'off' | 'unknown';
export const PLAYER_STATUS_ROLE = {
  running: 'success', waiting: 'info', done: 'success', paused: 'warning',
  blocked: 'warning', 'needs-attention': 'danger', off: 'neutral', unknown: 'neutral',
} as const satisfies Record<PlayerStatus, string>;

const RUNNING = ['running', 'defending', 'discovering', 'evacuating', 'preparing', 'protecting', 'recalling', 'reconciling', 'refreshing', 'replenishing', 'resolving', 'taunting', 'threat'] as const;
const WAITING = ['waiting', 'ready', 'idle', 'armed', 'cooldown', 'scheduled', 'enabled', 'gated', 'yielding', 'pending', 'retrying', 'soft-locked', 'opening-check'] as const;
export const KNOWN_RAW_AUTOMATION_STATUSES: readonly string[] = [
  ...RUNNING, ...WAITING, 'protected', 'blocked', 'warning', 'error', 'failed', 'complete', 'completed', 'success', 'disabled', 'stopped',
];

export interface PlayerStatusDescription {
  status: PlayerStatus;
  reason: LocalizedMessage;
}
export interface AutomationPlayerStatusInput {
  featureId: string;
  runtime?: AutomationStateV2;
  enabled?: AutomationEnabledControl;
  context: AutomationStateContext;
  /** The portal's synthetic missing-decorations lane, not a new wire field. */
  laneId?: string;
  /** Desktop's currently active Lock Bot control. */
  desktopLocked?: boolean;
}

/** Keep the exact game text and its atomically bound descriptor, including future producer values. */
export function playerStatusDetail(detail: RuntimeDetail | undefined, fallback: LocalizedMessage): LocalizedMessage {
  return detail?.text.trim() ? detail.descriptor ?? { key: '', fallback: detail.text, fallbackText: detail.text } : fallback;
}

/** Saved data cannot make a green claim; other badges retain their localized reason. */
function savedDataStatus(description: PlayerStatusDescription, observedAt?: string): PlayerStatusDescription {
  const time = observedAt ? Date.parse(observedAt) : NaN;
  const dated = Number.isFinite(time);
  if (description.status === 'running' || description.status === 'done' || description.status === 'unknown') {
    return {
      status: 'unknown',
      reason: dated ? describeMessage('playerStatus.checkpoint', { time }) : describeMessage('playerStatus.checkpointUndated'),
    };
  }
  const { context, ...reason } = description.reason;
  const suffix = dated ? describeMessage('playerStatus.checkpointSuffix', { time }) : describeMessage('playerStatus.checkpointSuffixUndated');
  const fallback = formatMessage({ ...suffix, params: { ...suffix.params, reason: formatMessage(description.reason, 'en', {}).text } }, 'en', {}).text;
  return {
    status: description.status,
    reason: {
      ...suffix,
      context,
      // A single localized leaf keeps its descriptor; existing context stays before it.
      listParams: { reason: [{ ...reason, key: reason.key || 'playerStatus.rawSavedReason' }] },
      fallbackText: fallback,
    },
  };
}

/** CIT-20 owns freshness, enabled/schedule state and safety-lock classification. */
export function automationPlayerStatus(input: AutomationPlayerStatusInput): PlayerStatusDescription {
  if (input.context.presence?.mode === 'checkpoint') {
    // Classify the saved state through CIT-20, then remove any green claim.
    const saved = automationPlayerStatus({
      ...input,
      context: { ...input.context, presence: undefined, connected: true, connectionSince: undefined },
    });
    return savedDataStatus(saved, input.context.presence.checkpointObservedAt ?? input.runtime?.updatedAt);
  }
  const phase = describeAutomationState(input.featureId, input.runtime, input.enabled, input.context);
  const reason = playerStatusDetail(phase.runtimeDetail, describeMessage(phase.messageKey, phase.params));
  let status: PlayerStatus;
  if (phase.phase === 'unknown') status = 'unknown';
  else if (phase.phase === 'disabled' || phase.phase === 'stopped') status = 'off';
  else if (phase.phase === 'locked') status = phase.reason === 'lock-review' ? 'needs-attention' : 'paused';
  else if (input.desktopLocked) status = 'paused';
  else if (phase.phase === 'error') status = 'needs-attention';
  else if (phase.phase === 'blocked') status = 'blocked';
  else if (phase.reason === 'schedule') status = 'waiting';
  else {
    const raw = input.runtime?.status?.toLowerCase() ?? '';
    if (raw === 'protected') status = input.featureId === 'autoStation' ? 'waiting' : 'paused';
    else if (raw === 'warning' && input.laneId === 'builder-missing-decorations') status = 'blocked';
    else if (raw === 'disabled' || raw === 'stopped') status = 'off';
    else if (phase.phase === 'completed') status = 'done';
    else if ((RUNNING as readonly string[]).includes(raw)) status = 'running';
    else status = 'waiting';
  }
  return {
    status,
    reason: input.desktopLocked && status === 'paused'
      ? playerStatusDetail(phase.runtimeDetail, describeMessage('playerStatus.desktopLocked'))
      : reason,
  };
}

export interface ConnectionPlayerStatusInput extends RepairInput {
  /** Desktop has received Start at least once. */
  started?: boolean;
  detail?: RuntimeDetail;
  /** Portal computes this with accountPlayerStatus; shared code never imports portal services. */
  accountStatus?: PlayerStatusDescription;
  checkpointObservedAt?: string;
}

export function connectionPlayerStatus(input: ConnectionPlayerStatusInput): PlayerStatusDescription {
  if (input.surface === 'hosted' && input.checkpoint) {
    return savedDataStatus(input.accountStatus ?? { status: 'unknown', reason: describeMessage('playerStatus.noConnection') }, input.checkpointObservedAt);
  }
  const raw = input.status.toLowerCase();
  let status: PlayerStatus;
  let reason: LocalizedMessage;
  if (raw === 'suspended' || input.loginFailure?.class === 'suspended') {
    status = 'blocked';
    const at = input.loginFailure?.suspendedUntil ? Date.parse(input.loginFailure.suspendedUntil) : NaN;
    reason = Number.isFinite(at) ? describeMessage('playerStatus.suspended', { time: at }) : describeMessage('playerStatus.suspendedUndated');
  } else if (input.loginFailure?.class === 'cooldown' || raw === 'cooldown') {
    status = 'waiting'; reason = describeMessage('playerStatus.loginCooldown');
  } else if (input.loginFailure?.fatal) {
    status = 'needs-attention'; reason = describeMessage('playerStatus.loginFailed');
  } else if (raw === 'connected' && input.loggedIn) {
    status = 'running'; reason = describeMessage('playerStatus.connected');
  } else if (['connecting', 'starting', 'authenticating', 'reconnecting', 'released'].includes(raw) || input.dashboard === 'Connecting') {
    status = 'waiting'; reason = describeMessage('playerStatus.connecting');
  } else if (input.surface === 'desktop' && input.started === false) {
    status = 'off'; reason = describeMessage('playerStatus.notStarted');
  } else {
    status = 'unknown'; reason = describeMessage('playerStatus.noConnection');
  }
  return { status, reason: playerStatusDetail(input.detail, reason) };
}

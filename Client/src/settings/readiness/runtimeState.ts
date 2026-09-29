import type { AutomationStateV2 } from '../../api/Contracts';
import type { LocalizedMessage } from '../../i18n/formatMessage';
import { automationDetailMessage } from '../../i18n/automationMessages';
import type { MessageKey, MessageParameters } from '../../i18n/messages';
import type { AutomationEnabledControl } from '../AutomationEnabled';
import { observationTimestamp } from '../requirements/observationFreshness';
import type { ReadinessFix } from './Readiness';

/**
 * What an automation is doing, in player language (CIT-20). Pure: it only
 * reads the saved enabled switch and the status the game reports in
 * `state.automations[id]`. It invents no reasons: the game's own detail is
 * carried verbatim (with its localization descriptor) and this module adds
 * the phase, a plain sentence about what that phase means, and a next step.
 * A missing, stale or checkpoint status is `unknown`, never a green claim.
 */
export type AutomationPhase =
  | 'disabled'
  | 'enabled-waiting'
  | 'running'
  | 'blocked'
  | 'error'
  | 'locked'
  | 'completed'
  | 'stopped'
  | 'unknown';

export type AutomationPhaseReason =
  | 'off'
  | 'timed-ended'
  | 'stopped-after-run'
  | 'schedule'
  | 'ready'
  | 'waiting-detail'
  | 'waiting-no-detail'
  | 'running'
  | 'blocked'
  | 'error'
  | 'lock-timed'
  | 'lock-review'
  | 'completed'
  | 'checkpoint'
  | 'disconnected'
  | 'no-status'
  | 'before-connection';

export interface AutomationStateContext {
  /** Game connection usable: Connected && loggedIn && socketReady (AuthContext.gameLoggedIn). */
  connected: boolean;
  /** Hosted runtime presence; desktop passes nothing. */
  presence?: { mode: 'live' | 'checkpoint'; checkpointObservedAt?: string };
  /** The feature's weekly schedule: `allowedNow` is false outside every slot of an enabled schedule. */
  schedule?: { enabled: boolean; allowedNow: boolean };
  /** Start of the current game connection (`session.changedAt`); a status older than this is not current. */
  connectionSince?: string;
  /** Running operations attributed to this feature; reported when it is off. */
  inFlight?: number;
  now: number;
}

/** The game's own words, shown verbatim. */
export interface RuntimeDetail {
  text: string;
  descriptor?: LocalizedMessage;
}

export interface AutomationStateDescription {
  phase: AutomationPhase;
  reason: AutomationPhaseReason;
  messageKey: MessageKey;
  params?: MessageParameters;
  runtimeDetail?: RuntimeDetail;
  nextStep?: ReadinessFix | 'clear-lock' | 'retry-stop';
  /** True only for a live status that belongs to the current connection. */
  evidenceFresh: boolean;
  /** When the game last reported this status. */
  observedAt?: string;
}

const message = (key: MessageKey): MessageKey => key;

function millis(value: string | undefined): number | undefined {
  const usable = observationTimestamp(value);
  return usable ? Date.parse(usable) : undefined;
}

function detailOf(runtime: AutomationStateV2 | undefined): RuntimeDetail | undefined {
  if (!runtime) return undefined;
  const text = typeof runtime.detail === 'string' && runtime.detail.trim() ? runtime.detail : undefined;
  if (text) return { text, descriptor: automationDetailMessage(runtime.detail, runtime.detailDescriptor) };
  const error = typeof runtime.lastError === 'string' && runtime.lastError.trim() ? runtime.lastError : undefined;
  return error ? { text: error } : undefined;
}

/** A lock still holds: it has an operation, was not cleared and has not expired (mirrors AutomationSafetyPanel). */
export function activeSafetyLock(runtime: AutomationStateV2 | undefined, now: number) {
  const lock = runtime?.safetyLock;
  if (!lock?.operationId || millis(lock.clearedAt)) return undefined;
  const until = millis(lock.until);
  return !until || until > now ? lock : undefined;
}

export function describeAutomationState(
  featureId: string,
  runtime: AutomationStateV2 | undefined,
  enabled: AutomationEnabledControl | undefined,
  context: AutomationStateContext,
): AutomationStateDescription {
  void featureId;
  const observedAt = observationTimestamp(runtime?.updatedAt);
  const on = enabled?.enabled === true;

  if (!on) {
    if (enabled?.configured && enabled.expiresAtMs && enabled.expiresAtMs <= context.now) {
      return {
        phase: 'stopped', reason: 'timed-ended', messageKey: message('runtimeState.timedEnded'),
        params: { endedAt: enabled.expiresAtMs }, evidenceFresh: true,
      };
    }
    if ((context.inFlight ?? 0) > 0 || millis(runtime?.lastRunAt)) {
      return {
        phase: 'stopped', reason: 'stopped-after-run', messageKey: message('runtimeState.stopped'),
        params: { inFlight: context.inFlight ?? 0 }, evidenceFresh: true,
      };
    }
    return { phase: 'disabled', reason: 'off', messageKey: message('runtimeState.off'), evidenceFresh: true };
  }

  // Turned on: the game's status decides, and only while it is live and current.
  if (context.presence?.mode === 'checkpoint') {
    const checkpointAt = millis(context.presence.checkpointObservedAt) ?? millis(runtime?.updatedAt);
    return {
      phase: 'unknown', reason: 'checkpoint', evidenceFresh: false, observedAt,
      ...(checkpointAt !== undefined
        ? { messageKey: message('runtimeState.checkpoint'), params: { observedAt: checkpointAt } }
        : { messageKey: message('runtimeState.checkpointUndated') }),
    };
  }
  if (!context.connected) {
    return {
      phase: 'unknown', reason: 'disconnected', evidenceFresh: false, observedAt,
      ...(observedAt
        ? { messageKey: message('runtimeState.disconnected'), params: { observedAt: Date.parse(observedAt) } }
        : { messageKey: message('runtimeState.disconnectedUndated') }),
    };
  }
  if (!runtime) {
    return { phase: 'unknown', reason: 'no-status', messageKey: message('runtimeState.noStatus'), evidenceFresh: false };
  }
  const since = millis(context.connectionSince);
  const reported = millis(runtime.updatedAt);
  if (since !== undefined && (reported === undefined || reported < since)) {
    return {
      phase: 'unknown', reason: 'before-connection', evidenceFresh: false, observedAt,
      ...(reported !== undefined
        ? { messageKey: message('runtimeState.beforeConnection'), params: { observedAt: reported } }
        : { messageKey: message('runtimeState.beforeConnectionUndated') }),
    };
  }

  const detail = detailOf(runtime);
  const status = (runtime.status ?? '').toLowerCase();
  const base = { evidenceFresh: true, observedAt, ...(detail ? { runtimeDetail: detail } : {}) };

  const lock = activeSafetyLock(runtime, context.now);
  if (lock) {
    const until = millis(lock.until);
    return until
      ? { ...base, phase: 'locked', reason: 'lock-timed', messageKey: message('runtimeState.lockTimed'), params: { until } }
      : { ...base, phase: 'locked', reason: 'lock-review', messageKey: message('runtimeState.lockReview'), nextStep: 'clear-lock' };
  }
  if (status === 'error' || status === 'failed') {
    return {
      ...base, phase: 'error', reason: 'error',
      messageKey: message(detail ? 'runtimeState.error' : 'runtimeState.errorNoDetail'),
    };
  }
  if (status === 'blocked') {
    return {
      ...base, phase: 'blocked', reason: 'blocked', nextStep: 'settings',
      messageKey: message(detail ? 'runtimeState.blocked' : 'runtimeState.blockedNoDetail'),
    };
  }
  if (status === 'gated' || (context.schedule?.enabled && !context.schedule.allowedNow)) {
    return { ...base, phase: 'enabled-waiting', reason: 'schedule', messageKey: message('runtimeState.schedule'), nextStep: 'settings' };
  }
  if (status === 'running') {
    return { ...base, phase: 'running', reason: 'running', messageKey: message('runtimeState.running') };
  }
  if (status === 'ready') {
    // "Ready" says the policy is watching for an opportunity; it is not an action and not a result.
    return { ...base, phase: 'enabled-waiting', reason: 'ready', messageKey: message('runtimeState.ready') };
  }
  if (status === 'complete' || status === 'completed' || status === 'success') {
    return { ...base, phase: 'completed', reason: 'completed', messageKey: message('runtimeState.completed') };
  }
  return {
    ...base, phase: 'enabled-waiting',
    reason: detail ? 'waiting-detail' : 'waiting-no-detail',
    messageKey: message(detail ? 'runtimeState.waiting' : 'runtimeState.waitingNoDetail'),
  };
}

const PHASE_PRIORITY: Record<AutomationPhase, number> = {
  locked: 8, error: 7, blocked: 6, unknown: 5, running: 4, 'enabled-waiting': 3, completed: 2, stopped: 1, disabled: 0,
};

export interface FeatureStateLane {
  id: string;
  runtime: AutomationStateV2 | undefined;
  /** False for a lane whose own switch is off (for example a disabled builder). */
  active?: boolean;
}

/**
 * Whole-feature description for features that report several lanes (Khan,
 * Storm, Beri World, Sceat Resources): the most urgent lane speaks for the
 * feature, matching the aggregation the Automation page already uses
 * (locked, error, blocked first).
 */
export function describeFeatureState(
  featureId: string,
  lanes: readonly FeatureStateLane[],
  enabled: AutomationEnabledControl | undefined,
  context: AutomationStateContext,
): { overall: AutomationStateDescription; lanes: Array<{ id: string; description: AutomationStateDescription }> } {
  const described = lanes.map((lane) => ({
    id: lane.id,
    active: lane.active !== false,
    description: describeAutomationState(featureId, lane.runtime, lane.active === false ? { configured: false, enabled: false } : enabled, context),
  }));
  const candidates = described.filter((lane) => lane.active);
  let overall = candidates[0]?.description ?? describeAutomationState(featureId, undefined, enabled, context);
  for (const lane of candidates.slice(1)) {
    if (PHASE_PRIORITY[lane.description.phase] > PHASE_PRIORITY[overall.phase]) overall = lane.description;
  }
  return { overall, lanes: described.map(({ id, description }) => ({ id, description })) };
}

/**
 * `state.automations` ids that report for a feature. Lane policies publish their own status under the ids
 * below (`Server/Automation`: `autoKhan:cooldown`, `autoStormShop`, `autoBeriWorldAttack`, ...); a feature
 * without lanes reports under its own id.
 */
export const FEATURE_STATE_LANES: Readonly<Record<string, readonly string[]>> = {
  autoKhan: ['autoKhan', 'autoKhan:cooldown', 'autoKhan:rage', 'autoKhan:defense'],
  autoStorm: ['autoStorm', 'autoStormShop', 'autoStormBuild'],
  autoBeriWorld: ['autoBeriWorld', 'autoBeriWorldAttack', 'autoBeriWorldTools', 'autoBeriWorldBuild'],
  autoSceatRes: ['autoSceatRes', 'autoSceatResLogistics'],
};

export function featureStateLaneIds(featureId: string): readonly string[] {
  return FEATURE_STATE_LANES[featureId] ?? [featureId];
}

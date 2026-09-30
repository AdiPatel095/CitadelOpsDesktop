import type { MessageKey, MessageParameters } from '../../i18n/messages';
import type { ReadinessCheck, ReadinessReport } from '../readiness/Readiness';
import type { AutomationPhase } from '../readiness/runtimeState';
import type { FirstResult } from '../readiness/firstResult';
import type { AutomationGoal } from './goals';

/**
 * The goal checklist (CIT-19). A read-only lens over state that already exists: it computes each step from the
 * observed connection, the saved configuration, the readiness report of the SAVED settings, the saved `automation.enabled`
 * switch, the reported phase and the first attributed receipt. It never reads a draft, a visit, a click or a preflight
 * as progress, sends nothing and writes nothing. Every step records the `evidence` it read, so tests and QA can assert it.
 */
export type ChecklistStepId = 'connection' | 'choices' | 'readiness' | 'activation' | 'first-result';

export const CHECKLIST_STEP_IDS: readonly ChecklistStepId[] = ['connection', 'choices', 'readiness', 'activation', 'first-result'];

/**
 * `done`: proven by observed state. `current`: the next thing the player can do. `todo`: not reached yet.
 * `blocked`: observed state says it cannot proceed. `waiting`: data or the game has not reported yet.
 * `unknown`: the state cannot be read (a saved checkpoint, or a status that is not current).
 */
export type ChecklistStepState = 'done' | 'current' | 'todo' | 'blocked' | 'waiting' | 'unknown';

export type ChecklistNext =
  | { kind: 'repair-connection' }
  | { kind: 'open-editor'; check?: ReadinessCheck }
  | { kind: 'start' }
  | { kind: 'wait' }
  | { kind: 'account-center' };

export interface ChecklistEvidence {
  /** The observed facts this step was decided from, by name. */
  read: readonly string[];
  /** ISO time the observation was made, when one exists. */
  at?: string;
}

export interface ChecklistStep {
  id: ChecklistStepId;
  state: ChecklistStepState;
  messageKey: MessageKey;
  params?: MessageParameters;
  /** The one action that moves this step forward. Present for every step that is not `done`. */
  nextStep?: ChecklistNext;
  /** A check or first-result reason to show under the sentence, verbatim from its owner. */
  detail?: { messageKey: MessageKey; params?: MessageParameters; text?: string };
  evidence: ChecklistEvidence;
}

/** Session fields the connection step reads (`SessionStateV2`). */
export interface ConnectionSession {
  status?: string;
  loggedIn?: boolean;
  socketReady?: boolean;
  generation?: number;
  baselineGeneration?: number;
  detail?: string;
  retryAt?: string;
  cooldownUntil?: string;
  loginFailure?: { class?: string; fatal?: boolean };
}

/** What the connection step is decided from. The platform adapter builds it; desktop passes no `hosted`. */
export interface ConnectionEvidence {
  /** Dashboard connection to the local or hosted runtime. */
  dashboard: 'Disconnected' | 'Connecting' | 'Connected';
  session: ConnectionSession | null;
  /** `AuthContext.gameLoggedIn`: connected, logged in and the socket ready. */
  gameLoggedIn: boolean;
  hosted?: {
    presence: 'live' | 'checkpoint';
    /** `HostedAccount.status`. */
    accountStatus?: string;
    /** `HostedAccount.observed.loggedIn`. */
    observedLoggedIn?: boolean;
    runtimePresent?: boolean;
    loginFailure?: { class?: string; fatal?: boolean };
  };
  observedAt?: string;
}

export interface ChecklistInputs {
  connection: ConnectionEvidence;
  /** The feature's saved configuration section exists and is not empty. */
  saved: { exists: boolean };
  /** `evaluateFeatureReadiness` over the SAVED settings; null until a configuration snapshot has been read. */
  report: ReadinessReport | null;
  enabled: { on: boolean; timedUntil?: number };
  /** A remembered failed Start (`automationWriteFailures[key]` with `intent: 'start'`). */
  failedStart?: { message?: string } | null;
  phase: { phase: AutomationPhase };
  firstResult: FirstResult;
}

export interface Checklist {
  steps: ChecklistStep[];
  /** The first step that is not `done`. */
  current: ChecklistStepId | null;
  /** True only when all five steps are `done`. */
  complete: boolean;
}

const message = (key: MessageKey): MessageKey => key;

const BLOCKING_DESKTOP_STATUS = new Set(['cooldown', 'suspended', 'stopped', 'unavailable', 'disconnected', 'error']);
const WAITING_DESKTOP_STATUS = new Set(['starting', 'connecting', 'authenticating', 'reconnecting', 'released']);
const BLOCKING_HOSTED_STATUS = new Set(['action_required', 'suspended', 'expired', 'disabled']);

/** Step 1. Done only for a live, logged-in connection whose first sync has finished. */
export function connectionStep(input: ConnectionEvidence): ChecklistStep {
  const { session, hosted } = input;
  const evidence = (read: string[]): ChecklistEvidence => ({ read, ...(input.observedAt ? { at: input.observedAt } : {}) });
  const firstSyncDone = session != null
    && session.baselineGeneration !== undefined
    && session.generation !== undefined
    && session.baselineGeneration === session.generation;

  if (hosted?.presence === 'checkpoint') {
    return {
      id: 'connection', state: 'unknown', messageKey: message('checklist.connection.checkpoint'),
      nextStep: hosted.runtimePresent === false || BLOCKING_HOSTED_STATUS.has(hosted.accountStatus ?? '')
        ? { kind: 'account-center' } : { kind: 'repair-connection' },
      evidence: evidence(['hosted.presence']),
    };
  }
  if (hosted && (hosted.loginFailure?.fatal || BLOCKING_HOSTED_STATUS.has(hosted.accountStatus ?? ''))) {
    return {
      id: 'connection', state: 'blocked', messageKey: message('checklist.connection.blocked'),
      nextStep: { kind: 'repair-connection' }, evidence: evidence(['hosted.accountStatus', 'hosted.loginFailure']),
    };
  }
  if (input.dashboard === 'Connecting') {
    return { id: 'connection', state: 'waiting', messageKey: message('checklist.connection.waiting'), nextStep: { kind: 'wait' }, evidence: evidence(['dashboard']) };
  }
  if (!session || input.dashboard === 'Disconnected') {
    return {
      id: 'connection', state: 'blocked', messageKey: message('checklist.connection.blocked'),
      nextStep: { kind: 'repair-connection' }, evidence: evidence(['dashboard', 'session']),
    };
  }
  if (input.gameLoggedIn && firstSyncDone && (!hosted || hosted.observedLoggedIn !== false)) {
    return { id: 'connection', state: 'done', messageKey: message('checklist.connection.done'), evidence: evidence(['session.status', 'session.baselineGeneration', ...(hosted ? ['hosted.observedLoggedIn'] : [])]) };
  }
  if (input.gameLoggedIn && !firstSyncDone) {
    return { id: 'connection', state: 'waiting', messageKey: message('checklist.connection.syncing'), nextStep: { kind: 'wait' }, evidence: evidence(['session.baselineGeneration']) };
  }
  const status = session.status ?? '';
  if (session.loginFailure?.fatal || BLOCKING_DESKTOP_STATUS.has(status)) {
    return { id: 'connection', state: 'blocked', messageKey: message('checklist.connection.blocked'), nextStep: { kind: 'repair-connection' }, evidence: evidence(['session.status', 'session.loginFailure']) };
  }
  if (WAITING_DESKTOP_STATUS.has(status)) {
    return { id: 'connection', state: 'waiting', messageKey: message('checklist.connection.waiting'), nextStep: { kind: 'repair-connection' }, evidence: evidence(['session.status']) };
  }
  // Connected socket without a confirmed login (or a status this build does not know): the player has to look.
  return { id: 'connection', state: 'blocked', messageKey: message('checklist.connection.notLoggedIn'), nextStep: { kind: 'repair-connection' }, evidence: evidence(['session.status', 'session.loggedIn']) };
}

/** Checks about the game's live state at start (commanders, timing), not about the choices the player saved. */
const READINESS_ONLY_CHECKS = new Set(['commanders', 'commander-assignment', 'schedule', 'daily-limit', 'travel-boost']);

const isChoiceCheck = (check: ReadinessCheck): boolean => check.fix === undefined || check.fix === 'settings' || check.fix === 'presets';

function readinessReadNames(report: ReadinessReport | null): string[] {
  return report ? ['report.overall', ...report.checks.map((check) => `check:${check.id}${check.slot ? `:${check.slot}` : ''}=${check.state}`)] : ['report'];
}

/** Step 2. Done only from a SAVED section whose required choices have no blocked check. */
export function choicesStep(input: ChecklistInputs): ChecklistStep {
  const evidence: ChecklistEvidence = { read: ['saved.exists', ...readinessReadNames(input.report)] };
  if (!input.saved.exists) {
    return { id: 'choices', state: 'current', messageKey: message('checklist.choices.none'), nextStep: { kind: 'open-editor' }, evidence };
  }
  const checks = input.report?.checks ?? [];
  const blocked = checks.find((check) => check.state === 'blocked' && isChoiceCheck(check));
  if (blocked) {
    return {
      id: 'choices', state: 'blocked', messageKey: message('checklist.choices.blocked'),
      nextStep: { kind: 'open-editor', check: blocked }, detail: { messageKey: blocked.messageKey, params: blocked.params }, evidence,
    };
  }
  // Data the choices depend on (castle data, official lists) has not arrived: waiting, never done.
  const unavailable = checks.find((check) => check.state === 'unavailable' && !READINESS_ONLY_CHECKS.has(check.id));
  if (!input.report || unavailable) {
    return {
      id: 'choices', state: 'waiting', messageKey: message('checklist.choices.waiting'),
      nextStep: input.connection.gameLoggedIn ? { kind: 'wait' } : { kind: 'repair-connection' },
      ...(unavailable ? { detail: { messageKey: unavailable.messageKey, params: unavailable.params } } : {}), evidence,
    };
  }
  return { id: 'choices', state: 'done', messageKey: message('checklist.choices.done'), evidence };
}

/** Step 3. Done when the saved settings have no blocked check and nothing waits for data; `pending` checks are the game's call. */
export function readinessStep(input: ChecklistInputs, choices: ChecklistStep): ChecklistStep {
  const evidence: ChecklistEvidence = { read: readinessReadNames(input.report) };
  if (!input.saved.exists) {
    return { id: 'readiness', state: 'todo', messageKey: message('checklist.readiness.todo'), nextStep: { kind: 'open-editor' }, evidence };
  }
  const report = input.report;
  if (!report) {
    return { id: 'readiness', state: 'waiting', messageKey: message('checklist.readiness.waiting'), nextStep: { kind: 'wait' }, evidence };
  }
  const blocked = report.checks.find((check) => check.state === 'blocked');
  if (blocked) {
    return {
      id: 'readiness', state: 'blocked', messageKey: message('checklist.readiness.blocked'),
      nextStep: { kind: 'open-editor', check: blocked }, detail: { messageKey: blocked.messageKey, params: blocked.params }, evidence,
    };
  }
  const unavailable = report.checks.find((check) => check.state === 'unavailable');
  if (unavailable || choices.state === 'waiting') {
    return {
      id: 'readiness', state: 'waiting', messageKey: message('checklist.readiness.waiting'),
      nextStep: input.connection.gameLoggedIn ? { kind: 'wait' } : { kind: 'repair-connection' },
      ...(unavailable ? { detail: { messageKey: unavailable.messageKey, params: unavailable.params } } : {}), evidence,
    };
  }
  return { id: 'readiness', state: 'done', messageKey: message('checklist.readiness.done'), evidence };
}

/** Step 4. Done only from the saved `automation.enabled` switch; a remembered failed Start blocks it. */
export function activationStep(input: ChecklistInputs, before: readonly ChecklistStep[]): ChecklistStep {
  const timed = input.enabled.timedUntil;
  if (input.enabled.on) {
    if (input.phase.phase === 'unknown') {
      return { id: 'activation', state: 'unknown', messageKey: message('checklist.activation.unknown'), nextStep: { kind: 'wait' }, evidence: { read: ['automation.enabled', 'phase'] } };
    }
    return {
      id: 'activation', state: 'done',
      messageKey: message(timed ? 'checklist.activation.doneTimed' : 'checklist.activation.done'),
      ...(timed ? { params: { until: timed } } : {}),
      evidence: { read: ['automation.enabled', 'phase'] },
    };
  }
  if (input.failedStart) {
    return {
      id: 'activation', state: 'blocked', messageKey: message('checklist.activation.failed'), nextStep: { kind: 'start' },
      ...(input.failedStart.message ? { detail: { messageKey: message('checklist.activation.failed'), text: input.failedStart.message } } : {}),
      evidence: { read: ['automationWriteFailures'] },
    };
  }
  const ready = before.every((step) => step.state === 'done');
  return {
    id: 'activation', state: ready ? 'current' : 'todo',
    messageKey: message(ready ? 'checklist.activation.current' : 'checklist.activation.todo'),
    nextStep: { kind: 'start' }, evidence: { read: ['automation.enabled'] },
  };
}

/** Step 5. Done only from an attributed succeeded receipt after this account turned the automation on. */
export function firstResultStep(input: ChecklistInputs): ChecklistStep {
  const result = input.firstResult;
  const evidence: ChecklistEvidence = { read: ['firstResult.state', 'operations'], ...(result.completedAt ? { at: result.completedAt } : {}) };
  if (result.state === 'confirmed') {
    return { id: 'first-result', state: 'done', messageKey: message('checklist.firstResult.done'), evidence };
  }
  if (result.state === 'failed') {
    return {
      id: 'first-result', state: 'blocked', messageKey: message('checklist.firstResult.failed'), nextStep: { kind: 'wait' },
      ...(result.failure ? { detail: { messageKey: message('checklist.firstResult.failed'), text: result.failure.text } } : {}), evidence,
    };
  }
  if (!input.enabled.on) {
    return { id: 'first-result', state: 'todo', messageKey: message('checklist.firstResult.todo'), nextStep: { kind: 'wait' }, evidence };
  }
  // Nothing yet, in progress, or not attributable: the honest state is waiting, with the CIT-20 sentence.
  return { id: 'first-result', state: 'waiting', messageKey: message('checklist.firstResult.waiting'), nextStep: { kind: 'wait' }, evidence };
}

export function evaluateChecklist(goal: AutomationGoal, inputs: ChecklistInputs): Checklist {
  void goal;
  const connection = connectionStep(inputs.connection);
  const choices = choicesStep(inputs);
  const readiness = readinessStep(inputs, choices);
  const activation = activationStep(inputs, [connection, choices, readiness]);
  const firstResult = firstResultStep(inputs);
  const steps = [connection, choices, readiness, activation, firstResult];
  // Only the first step that is not done is "current"; a later step can be done (a custom setup, a dropped connection).
  const currentIndex = steps.findIndex((step) => step.state !== 'done');
  const marked = steps.map((step, index) => (
    index > currentIndex && step.state === 'current' ? { ...step, state: 'todo' as const } : step
  ));
  return { steps: marked, current: currentIndex < 0 ? null : marked[currentIndex].id, complete: currentIndex < 0 };
}

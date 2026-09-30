import type { MessageKey, MessageParameters } from '../../i18n/messages';

/**
 * What to tell the player about a connection that blocks the chosen task (CIT-19). Pure. It reads only what the game
 * or the hosted account reports: the session status, the game's own detail (shown verbatim by the caller) and, when the
 * game classified a failed login, that typed class. It never guesses: an untyped rejection is explained by listing both
 * things to check (the login and the world) and showing the saved world, and it never says "wrong world" or "wrong
 * password" unless the typed class `wrong_server` / `invalid_credentials` says so.
 */
export type RepairSurface = 'desktop' | 'hosted';

export type RepairAction =
  | 'start-bot'
  | 'stop-bot'
  | 'reconnect'
  | 'reenable-saved-login'
  | 'open-settings'
  | 'account-center';

export interface RepairInput {
  surface: RepairSurface;
  /** Desktop connection mode; hosted has no such choice. */
  mode?: 'full' | 'background';
  /** `SessionStateV2.status`, or `disconnected` when the dashboard has no session at all. */
  status: string;
  loggedIn: boolean;
  /** Dashboard connection to the local runtime or hosted gateway. */
  dashboard?: 'Disconnected' | 'Connecting' | 'Connected';
  retryAt?: string;
  cooldownUntil?: string;
  /** The game's typed classification of the last failed login (`session.loginFailure`). */
  loginFailure?: { class?: string; fatal?: boolean; suspendedUntil?: string };
  /** Hosted only: the saved checkpoint is shown instead of a live runtime. */
  checkpoint?: boolean;
  /** Background mode: the saved server resolved against the official directory, as "code · label". */
  savedWorld?: string;
  now: number;
}

export interface RepairExplanation {
  messageKey: MessageKey;
  params?: MessageParameters;
  /** True when the game's own `detail` belongs directly under the sentence (shown verbatim by the caller). */
  showDetail: boolean;
  /** The class the sentence rests on; `undefined` when nothing typed was reported. */
  typedClass?: string;
  actions: RepairAction[];
}

const message = (key: MessageKey): MessageKey => key;

function timeOf(value: string | undefined): number | undefined {
  if (!value) return undefined;
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) && parsed > 0 && new Date(parsed).getUTCFullYear() > 1971 ? parsed : undefined;
}

/** The time of the next login attempt the game reported, only while it is still in the future. */
export function nextAttemptAt(input: Pick<RepairInput, 'retryAt' | 'cooldownUntil' | 'now'>): number | undefined {
  const times = [timeOf(input.cooldownUntil), timeOf(input.retryAt)].filter((value): value is number => value !== undefined && value > input.now);
  return times.length > 0 ? Math.min(...times) : undefined;
}

function classExplanation(input: RepairInput, cls: string): RepairExplanation | null {
  const hosted = input.surface === 'hosted';
  const at = nextAttemptAt(input);
  switch (cls) {
    case 'cooldown':
      return at !== undefined
        ? { messageKey: message('connectionRepair.explain.cooldown'), params: { retryAt: at }, showDetail: true, typedClass: cls, actions: ['open-settings'] }
        : { messageKey: message('connectionRepair.explain.cooldownNoTime'), showDetail: true, typedClass: cls, actions: ['open-settings'] };
    case 'suspended':
      return { messageKey: message('connectionRepair.explain.suspended'), showDetail: true, typedClass: cls, actions: [] };
    case 'account_deleted':
      return { messageKey: message('connectionRepair.explain.accountDeleted'), showDetail: true, typedClass: cls, actions: [] };
    case 'client_version_rejected':
      return { messageKey: message('connectionRepair.explain.clientVersion'), showDetail: true, typedClass: cls, actions: [] };
    case 'invalid_credentials':
      return {
        messageKey: message(hosted ? 'connectionRepair.explain.invalidCredentialsHosted' : 'connectionRepair.explain.invalidCredentials'),
        showDetail: true, typedClass: cls, actions: [hosted ? 'account-center' : 'open-settings'],
      };
    case 'wrong_server':
      return {
        messageKey: message(hosted ? 'connectionRepair.explain.wrongServerHosted' : 'connectionRepair.explain.wrongServer'),
        showDetail: true, typedClass: cls, actions: [hosted ? 'account-center' : 'open-settings'],
      };
    default:
      return null;
  }
}

/** The one sentence for the connection as reported; see the rules in the module comment. */
export function explainConnection(input: RepairInput): RepairExplanation {
  const hosted = input.surface === 'hosted';
  const settingsAction: RepairAction = hosted ? 'account-center' : 'open-settings';
  if (input.checkpoint) {
    return { messageKey: message('connectionRepair.explain.checkpoint'), showDetail: false, actions: ['account-center'] };
  }
  const cls = input.loginFailure?.class;
  if (cls) {
    const typed = classExplanation(input, cls);
    if (typed) return typed;
  }
  if (input.dashboard === 'Connecting') {
    return { messageKey: message('connectionRepair.explain.dashboardConnecting'), showDetail: false, actions: [] };
  }
  if (input.dashboard === 'Disconnected') {
    return { messageKey: message('connectionRepair.explain.dashboardOffline'), showDetail: false, actions: hosted ? ['account-center'] : [] };
  }
  const at = nextAttemptAt(input);
  switch (input.status) {
    case 'cooldown':
      return at !== undefined
        ? { messageKey: message('connectionRepair.explain.cooldown'), params: { retryAt: at }, showDetail: true, actions: [] }
        : { messageKey: message('connectionRepair.explain.cooldownNoTime'), showDetail: true, actions: [] };
    case 'suspended':
      return { messageKey: message('connectionRepair.explain.suspended'), showDetail: true, actions: [] };
    case 'unavailable':
    case 'stopped':
    case 'disconnected':
      return {
        messageKey: message(hosted ? 'connectionRepair.explain.notRunningHosted' : 'connectionRepair.explain.notRunning'),
        showDetail: true, actions: hosted ? ['account-center'] : ['start-bot'],
      };
    case 'starting':
      return { messageKey: message('connectionRepair.explain.starting'), showDetail: true, actions: [] };
    case 'connecting':
    case 'authenticating':
      return { messageKey: message('connectionRepair.explain.opening'), showDetail: true, actions: [] };
    case 'released':
    case 'reconnecting':
      return at !== undefined
        ? { messageKey: message('connectionRepair.explain.waitingRetry'), params: { retryAt: at }, showDetail: true, actions: ['reconnect'] }
        : { messageKey: message('connectionRepair.explain.waiting'), showDetail: true, actions: ['reconnect'] };
    default:
      break;
  }
  if (input.status === 'connected' && !input.loggedIn) {
    if (!hosted && input.mode !== 'background') {
      return { messageKey: message('connectionRepair.explain.fullLoggedOut'), showDetail: true, actions: ['reconnect'] };
    }
    if (!hosted && input.savedWorld) {
      return { messageKey: message('connectionRepair.explain.backgroundRejected'), params: { world: input.savedWorld }, showDetail: true, actions: ['open-settings'] };
    }
    return {
      messageKey: message(hosted ? 'connectionRepair.explain.genericHosted' : 'connectionRepair.explain.backgroundRejectedNoWorld'),
      showDetail: true, actions: [settingsAction],
    };
  }
  return { messageKey: message('connectionRepair.explain.generic'), showDetail: true, actions: [settingsAction] };
}

/** Words that must never appear in a sentence unless the game's typed class named the cause. */
export const CAUSE_CLAIMS = /wrong world|wrong password|wrong server|wrong login/i;

export interface SavedWorldEntry {
  code: string;
  label?: string;
  zone?: string;
}

export interface SavedWorldDescription {
  kind: 'none' | 'listed' | 'not-listed';
  /** "code · label" for the generic rejection sentence; "code · label · zone" in the world line. */
  short?: string;
  long?: string;
  code?: string;
}

/** Resolves the saved background server code against the official world directory. */
export function describeSavedWorld(savedCode: string | undefined, directory: readonly SavedWorldEntry[]): SavedWorldDescription {
  const code = savedCode?.trim();
  if (!code) return { kind: 'none' };
  const entry = directory.find((server) => server.code.toLowerCase() === code.toLowerCase());
  if (!entry) return { kind: 'not-listed', code, short: code, long: code };
  const label = entry.label?.trim();
  const zone = entry.zone?.trim();
  return {
    kind: 'listed', code: entry.code,
    short: label ? `${entry.code} · ${label}` : entry.code,
    long: [entry.code, label, zone].filter(Boolean).join(' · '),
  };
}

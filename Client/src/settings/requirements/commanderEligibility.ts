import type { GameStateV2 } from '../../api/Contracts';
import type { MessageKey, MessageParameters } from '../../i18n/messages';
import { movementSnapshotFresh, statusForRow } from '../../Movement/types/CommanderActivity';
import {
  COMMANDER_FEATURE_IDS,
  commanderIDsAssignedToFeature,
  commanderMeetsFeatureRequirements,
  featureRequirementsSupported,
  isCommanderAssigned,
  type CommanderFeatureConfigurationV2,
  type CommanderFeatureID,
} from '../../Movement/types/CommanderFeatureAssignments';
import type { CommanderActivity, MovementViewModel } from '../../Movement/types/MovementState';
import type { CheckState, ReadinessCheck, ReadinessFix } from '../readiness/Readiness';

/**
 * Client preview of the runtime's commander rules (Server/CommanderFeatures/Policy.go
 * stays authoritative). Pure: reads state, the movement view model and the saved
 * assignment document; never dispatches anything and never chooses a commander.
 */

const message = (key: MessageKey): MessageKey => key;

export const COMMANDER_FEATURE_LABEL_KEYS: Readonly<Record<CommanderFeatureID, MessageKey>> = {
  autoTowers: message('commanderFeatures.autoTowers'),
  autoFortress: message('commanderFeatures.autoFortress'),
  autoInvasion: message('commanderFeatures.autoInvasion'),
  autoNomad: message('commanderFeatures.autoNomad'),
  autoAdvisor: message('commanderFeatures.autoAdvisor'),
  autoKhan: message('commanderFeatures.autoKhan'),
  autoBeriWorld: message('commanderFeatures.autoBeriWorld'),
  autoStorm: message('commanderFeatures.autoStorm'),
  riftMaiden: message('commanderFeatures.riftMaiden'),
  riftReplay: message('commanderFeatures.riftReplay'),
};

export interface CommanderEligibilityRow {
  commanderId: number;
  name: string;
  visiblePosition: number;
  assigned: boolean;
  /** True when the feature has an explicit list (not the "all commanders" default). */
  explicitAssignment: boolean;
  meetsRequirements: boolean;
  activity: CommanderActivity;
  eligibleNow: boolean;
  /** Other features that list this commander explicitly. */
  otherFeatures: CommanderFeatureID[];
}

export type CommanderRowStatus = { kind: 'activity'; activity: CommanderActivity } | { kind: 'off' };
/** CIT-80: activity is shown only for commanders this automation may use. */
export function commanderRowStatus(row: Pick<CommanderEligibilityRow, 'assigned' | 'activity'>): CommanderRowStatus {
  return row.assigned ? { kind: 'activity', activity: row.activity } : { kind: 'off' };
}

export interface CommanderEligibilitySummary {
  observed: boolean;
  assignedCount: number;
  eligibleCount: number;
  freeNowCount: number;
  snapshotFresh: boolean;
  state: CheckState;
  messageKey: MessageKey;
  params?: MessageParameters;
  fix?: ReadinessFix;
}

export interface CommanderEligibilityReport {
  featureId: CommanderFeatureID;
  /** True while the feature has no explicit list, so every commander is allowed. */
  implicitAll: boolean;
  requirementsSupported: boolean;
  rows: CommanderEligibilityRow[];
  /** Assignment part: is at least one commander allowed and qualified? */
  assignment: ReadinessCheck;
  /** Activity part: can one of them leave now? */
  activity: ReadinessCheck;
  summary: CommanderEligibilitySummary;
}

export interface CommanderEligibilityInput {
  featureId: CommanderFeatureID;
  state: GameStateV2 | null;
  assignments: CommanderFeatureConfigurationV2;
  movement: MovementViewModel | null;
  gameLoggedIn: boolean;
  /** Milliseconds since the epoch. */
  now: number;
}

const severity: Record<CheckState, number> = { valid: 0, pending: 1, unavailable: 2, blocked: 3 };

export function evaluateCommanderEligibility(input: CommanderEligibilityInput): CommanderEligibilityReport {
  const { featureId, state, assignments, movement, gameLoggedIn } = input;
  const nowUnix = Math.floor(input.now / 1000);
  const commanders = Object.values(state?.commanders ?? {});
  const observed = state != null && commanders.length > 0;
  const implicitAll = assignments.assignments[featureId] == null;
  const requirementsSupported = featureRequirementsSupported(assignments, featureId);
  const snapshotReady = movement?.snapshotReady === true;
  const snapshotFresh = movementSnapshotFresh(movement, gameLoggedIn, nowUnix);
  const statusRows = new Map((movement?.commanderStatuses ?? []).map((row) => [row.commanderId, row]));

  const rows: CommanderEligibilityRow[] = commanders.map((commander) => {
    const assigned = isCommanderAssigned(assignments, featureId, commander.id);
    const meetsRequirements = commanderMeetsFeatureRequirements(assignments, featureId, commander.id, state);
    const statusRow = statusRows.get(commander.id);
    const activity: CommanderActivity = statusRow
      ? statusForRow(statusRow, gameLoggedIn, snapshotReady, snapshotFresh, nowUnix)
      : !gameLoggedIn ? 'unknown' : !snapshotReady ? 'syncing' : 'unknown';
    return {
      commanderId: commander.id,
      name: commander.name?.trim() ?? '',
      visiblePosition: commander.visiblePosition ?? commander.id,
      assigned,
      explicitAssignment: !implicitAll,
      meetsRequirements,
      activity,
      eligibleNow: assigned && meetsRequirements && activity === 'free',
      otherFeatures: COMMANDER_FEATURE_IDS.filter((other) => other !== featureId
        && (assignments.assignments[other] ?? []).includes(commander.id)),
    };
  }).sort((left, right) => left.visiblePosition - right.visiblePosition || left.commanderId - right.commanderId);

  const assignedCount = rows.filter((row) => row.assigned).length;
  const eligibleCount = rows.filter((row) => row.assigned && row.meetsRequirements).length;
  const freeNowCount = rows.filter((row) => row.eligibleNow).length;

  let assignment: ReadinessCheck;
  if (!observed) {
    assignment = { id: 'commander-assignment', state: 'unavailable', messageKey: message('ui.settings.requirements.commanderEligibility.commander.assignments.are.checked.once.commanders.are.497afa18') };
  } else if (!requirementsSupported) {
    assignment = { id: 'commander-assignment', state: 'blocked', messageKey: message('ui.settings.requirements.commanderEligibility.a.commander.requirement.for.this.automation.is.040482cf'), fix: 'assignment' };
  } else if (!implicitAll && (assignments.assignments[featureId] ?? []).length === 0) {
    assignment = { id: 'commander-assignment', state: 'blocked', messageKey: message('ui.settings.requirements.commanderEligibility.no.commander.is.assigned.to.this.automation.3af941d5'), fix: 'assignment' };
  } else if (assignedCount === 0) {
    assignment = { id: 'commander-assignment', state: 'blocked', messageKey: message('ui.settings.requirements.commanderEligibility.none.of.the.assigned.commanders.is.in.4f6b6ddd'), fix: 'assignment' };
  } else if (eligibleCount === 0) {
    assignment = { id: 'commander-assignment', state: 'blocked', messageKey: message('ui.settings.requirements.commanderEligibility.no.assigned.commander.meets.the.equipment.requirement.7d6a9fed'), fix: 'assignment' };
  } else {
    assignment = {
      id: 'commander-assignment',
      state: 'valid',
      messageKey: message(implicitAll ? 'commanderEligibility.allAllowed' : 'commanderEligibility.assigned'),
      params: { count: eligibleCount },
    };
  }

  let activity: ReadinessCheck;
  if (!observed) {
    activity = { id: 'commanders', state: 'unavailable', messageKey: message('ui.settings.requirements.commanderEligibility.commanders.have.not.been.observed.for.this.ac14289c'), fix: 'connection' };
  } else if (!gameLoggedIn || !snapshotReady) {
    activity = { id: 'commanders', state: 'unavailable', messageKey: message('ui.settings.requirements.commanderEligibility.commander.positions.have.not.been.observed.on.75091ce5'), fix: 'connection' };
  } else if (!snapshotFresh) {
    activity = { id: 'commanders', state: 'unavailable', messageKey: message('ui.settings.requirements.commanderEligibility.commander.positions.are.stale.they.update.when.d4a40014'), fix: 'connection' };
  } else if (eligibleCount > 0 && freeNowCount === 0) {
    activity = { id: 'commanders', state: 'pending', messageKey: message('ui.settings.requirements.commanderEligibility.every.eligible.commander.is.busy.or.traveling.4ab8f1af') };
  } else if (eligibleCount === 0) {
    activity = { id: 'commanders', state: 'pending', messageKey: message('ui.settings.requirements.commanderEligibility.commander.activity.is.checked.once.a.commander.7d8ffe3c') };
  } else {
    activity = { id: 'commanders', state: 'valid', messageKey: message('commanderEligibility.freeNow'), params: { count: freeNowCount } };
  }

  const worst = severity[assignment.state] >= severity[activity.state] ? assignment : activity;
  return {
    featureId,
    implicitAll,
    requirementsSupported,
    rows,
    assignment,
    activity,
    summary: {
      observed,
      assignedCount,
      eligibleCount,
      freeNowCount,
      snapshotFresh,
      state: worst.state,
      messageKey: worst.messageKey,
      params: worst.params,
      fix: worst.fix,
    },
  };
}

export interface AssignmentChange {
  commanderId: number;
  featureId: CommanderFeatureID;
}

export interface AssignmentImpact {
  added: AssignmentChange[];
  removed: AssignmentChange[];
  /** Features that had at least one allowed commander and now have none. */
  featuresLeftEmpty: CommanderFeatureID[];
  /** Newly added commanders that other features also list explicitly. */
  commandersNowShared: Array<{ commanderId: number; features: CommanderFeatureID[] }>;
}

/**
 * What saving `next` over `current` changes, for the confirmation dialog.
 * Assigning here never removes a commander from another feature.
 */
export function assignmentImpact(
  current: CommanderFeatureConfigurationV2,
  next: CommanderFeatureConfigurationV2,
  commanderIds: readonly number[],
): AssignmentImpact {
  const added: AssignmentChange[] = [];
  const removed: AssignmentChange[] = [];
  const featuresLeftEmpty: CommanderFeatureID[] = [];
  for (const featureId of COMMANDER_FEATURE_IDS) {
    const before = new Set(commanderIDsAssignedToFeature(current, featureId, commanderIds));
    const after = new Set(commanderIDsAssignedToFeature(next, featureId, commanderIds));
    for (const commanderId of after) if (!before.has(commanderId)) added.push({ commanderId, featureId });
    for (const commanderId of before) if (!after.has(commanderId)) removed.push({ commanderId, featureId });
    if (before.size > 0 && after.size === 0) featuresLeftEmpty.push(featureId);
  }
  const commandersNowShared: AssignmentImpact['commandersNowShared'] = [];
  for (const change of added) {
    const features = COMMANDER_FEATURE_IDS.filter((featureId) => featureId !== change.featureId
      && (next.assignments[featureId] ?? []).includes(change.commanderId));
    if (features.length > 0 && !commandersNowShared.some((entry) => entry.commanderId === change.commanderId)) {
      commandersNowShared.push({ commanderId: change.commanderId, features });
    }
  }
  return { added, removed, featuresLeftEmpty, commandersNowShared };
}

export function assignmentImpactIsEmpty(impact: AssignmentImpact): boolean {
  return impact.added.length === 0 && impact.removed.length === 0;
}

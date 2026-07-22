export const COMMANDER_FEATURE_SECTION = 'automation.commanderFeatures';

export type CommanderFeatureID =
  | 'autoTowers'
  | 'autoInvasion'
  | 'autoNomad'
  | 'autoAdvisor'
  | 'autoKhan'
  | 'autoStorm'
  | 'riftMaiden'
  | 'riftReplay';

export interface CommanderFeatureAssignmentsV1 {
  version: 1;
  assignments: Record<string, number[]>;
}

export function defaultCommanderFeatureAssignments(): CommanderFeatureAssignmentsV1 {
  return { version: 1, assignments: {} };
}

export function parseCommanderFeatureAssignments(value: unknown): CommanderFeatureAssignmentsV1 {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return defaultCommanderFeatureAssignments();
  }
  const rawAssignments = (value as Record<string, unknown>).assignments;
  if (!rawAssignments || typeof rawAssignments !== 'object' || Array.isArray(rawAssignments)) {
    return defaultCommanderFeatureAssignments();
  }
  const assignments: Record<string, number[]> = {};
  for (const [featureID, candidateIDs] of Object.entries(rawAssignments)) {
    if (!featureID.trim() || !Array.isArray(candidateIDs)) continue;
    assignments[featureID] = [...new Set(candidateIDs.flatMap((candidateID) => {
      const parsed = Number(candidateID);
      return Number.isFinite(parsed) && parsed >= 0 ? [Math.trunc(parsed)] : [];
    }))].sort((left, right) => left - right);
  }
  return { version: 1, assignments };
}

export function isCommanderAssigned(
  document: CommanderFeatureAssignmentsV1,
  featureID: CommanderFeatureID,
  commanderID: number,
): boolean {
  const assignments = document.assignments[featureID];
  return assignments?.includes(commanderID) === true;
}

export function setCommanderAssignment(
  document: CommanderFeatureAssignmentsV1,
  featureID: CommanderFeatureID,
  commanderID: number,
  assigned: boolean,
): CommanderFeatureAssignmentsV1 {
  const current = document.assignments[featureID] ?? [];
  const next = new Set(current);
  if (assigned) {
    next.add(commanderID);
  } else {
    next.delete(commanderID);
  }
  return {
    version: 1,
    assignments: {
      ...document.assignments,
      [featureID]: [...next].filter((id) => id >= 0).sort((left, right) => left - right),
    },
  };
}

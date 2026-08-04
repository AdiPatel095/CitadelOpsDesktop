import type { GameStateV2 } from '../../api/Contracts';

export const COMMANDER_FEATURE_SECTION = 'automation.commanderFeatures';

export type CommanderFeatureID =
  | 'autoTowers'
  | 'autoInvasion'
  | 'autoNomad'
  | 'autoAdvisor'
  | 'autoKhan'
  | 'autoBeriWorld'
  | 'autoStorm'
  | 'riftMaiden'
  | 'riftReplay';

export interface CommanderFeatureRequirement {
  kind: string;
  effectDefinitionId?: number;
  unitId?: number;
  minimumValue?: number;
  maximumValue?: number;
  [key: string]: unknown;
}

export interface CommanderEquipmentEffectRequirement extends CommanderFeatureRequirement {
  kind: 'equipmentEffect';
  effectDefinitionId: number;
}

export interface CommanderFeatureConfigurationV2 {
  version: 2;
  // Missing feature keys allow every current and future commander. An explicit empty array disables the feature for all.
  assignments: Record<string, number[]>;
  // Requirements are ANDed. Unknown future requirement kinds are preserved and fail closed until supported.
  requirements: Record<string, CommanderFeatureRequirement[]>;
}

export function defaultCommanderFeatureAssignments(): CommanderFeatureConfigurationV2 {
  return { version: 2, assignments: {}, requirements: {} };
}

export function parseCommanderFeatureAssignments(value: unknown): CommanderFeatureConfigurationV2 {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return defaultCommanderFeatureAssignments();
  }
  const rawAssignments = (value as Record<string, unknown>).assignments;
  const assignments: Record<string, number[]> = {};
  if (rawAssignments && typeof rawAssignments === 'object' && !Array.isArray(rawAssignments)) {
    for (const [featureID, candidateIDs] of Object.entries(rawAssignments)) {
      if (!featureID.trim() || !Array.isArray(candidateIDs)) continue;
      assignments[featureID] = [...new Set(candidateIDs.flatMap((candidateID) => {
        const parsed = Number(candidateID);
        return Number.isFinite(parsed) && parsed >= 0 ? [Math.trunc(parsed)] : [];
      }))].sort((left, right) => left - right);
    }
  }
  const requirements: Record<string, CommanderFeatureRequirement[]> = {};
  const rawRequirements = (value as Record<string, unknown>).requirements;
  if (rawRequirements && typeof rawRequirements === 'object' && !Array.isArray(rawRequirements)) {
    for (const [featureID, candidates] of Object.entries(rawRequirements)) {
      if (!featureID.trim() || !Array.isArray(candidates)) continue;
      const parsed = candidates.flatMap((candidate) => {
        if (!candidate || typeof candidate !== 'object' || Array.isArray(candidate)) return [];
        const requirement = { ...(candidate as Record<string, unknown>) };
        const kind = typeof requirement.kind === 'string' ? requirement.kind.trim() : '';
        if (!kind) return [];
        if (kind === 'equipmentEffect') {
          const effectDefinitionId = requirement.effectDefinitionId == null
            ? Number.NaN
            : Number(requirement.effectDefinitionId);
          const unitId = requirement.unitId == null ? Number.NaN : Number(requirement.unitId);
          const minimumValue = requirement.minimumValue == null ? Number.NaN : Number(requirement.minimumValue);
          const maximumValue = requirement.maximumValue == null ? Number.NaN : Number(requirement.maximumValue);
          return [{
            ...requirement,
            kind,
            effectDefinitionId: Number.isFinite(effectDefinitionId) ? Math.trunc(effectDefinitionId) : 0,
            ...(Number.isFinite(unitId) && unitId > 0 ? { unitId: Math.trunc(unitId) } : { unitId: undefined }),
            ...(Number.isFinite(minimumValue) ? { minimumValue } : { minimumValue: undefined }),
            ...(Number.isFinite(maximumValue) ? { maximumValue } : { maximumValue: undefined }),
          } satisfies CommanderFeatureRequirement];
        }
        return [{ ...requirement, kind } satisfies CommanderFeatureRequirement];
      });
      if (parsed.length > 0) requirements[featureID] = parsed;
    }
  }
  return { version: 2, assignments, requirements };
}

export function isCommanderAssigned(
  document: CommanderFeatureConfigurationV2,
  featureID: CommanderFeatureID,
  commanderID: number,
): boolean {
  const assignments = document.assignments[featureID];
  return assignments == null || assignments.includes(commanderID);
}

export function commanderIDsAssignedToFeature(
  document: CommanderFeatureConfigurationV2,
  featureID: CommanderFeatureID,
  commanderIDs: readonly number[],
): number[] {
  const candidates = normalizeCommanderIDs(commanderIDs);
  const assignments = document.assignments[featureID];
  if (assignments == null) return candidates;
  const allowed = new Set(assignments);
  return candidates.filter((commanderID) => allowed.has(commanderID));
}

export function commanderIDsEligibleForFeature(
  document: CommanderFeatureConfigurationV2,
  featureID: CommanderFeatureID,
  commanderIDs: readonly number[],
  state: GameStateV2 | null,
): number[] {
  return commanderIDsAssignedToFeature(document, featureID, commanderIDs)
    .filter((commanderID) => commanderMeetsFeatureRequirements(document, featureID, commanderID, state));
}

export function commanderMeetsFeatureRequirements(
  document: CommanderFeatureConfigurationV2,
  featureID: CommanderFeatureID,
  commanderID: number,
  state: GameStateV2 | null,
): boolean {
  const requirements = document.requirements[featureID] ?? [];
  if (requirements.length === 0) return true;
  if (!state) return false;
  return requirements.every((requirement) => (
    isEquipmentEffectRequirement(requirement)
      ? satisfiesRequirement(commanderEquipmentEffectValue(state, commanderID, requirement), requirement)
      : false
  ));
}

export function equipmentRequirementForFeature(
  document: CommanderFeatureConfigurationV2,
  featureID: CommanderFeatureID,
): CommanderEquipmentEffectRequirement | null {
  return (document.requirements[featureID] ?? []).find(isEquipmentEffectRequirement) ?? null;
}

export function setCommanderEquipmentRequirement(
  document: CommanderFeatureConfigurationV2,
  featureID: CommanderFeatureID,
  requirement: CommanderEquipmentEffectRequirement | null,
): CommanderFeatureConfigurationV2 {
  const requirements = { ...document.requirements };
  const preserved = (requirements[featureID] ?? []).filter((candidate) => !isEquipmentEffectRequirement(candidate));
  const next = requirement ? [...preserved, requirement] : preserved;
  if (next.length > 0) {
    requirements[featureID] = next;
  } else {
    delete requirements[featureID];
  }
  return { ...document, version: 2, requirements };
}

export function setCommanderFeatureForAll(
  document: CommanderFeatureConfigurationV2,
  featureID: CommanderFeatureID,
  assigned: boolean,
): CommanderFeatureConfigurationV2 {
  const assignments = { ...document.assignments };
  if (assigned) {
    delete assignments[featureID];
  } else {
    assignments[featureID] = [];
  }
  return { ...document, version: 2, assignments };
}

export function setCommanderAssignment(
  document: CommanderFeatureConfigurationV2,
  featureID: CommanderFeatureID,
  commanderID: number,
  assigned: boolean,
  commanderIDs: readonly number[],
): CommanderFeatureConfigurationV2 {
  const current = document.assignments[featureID] ?? normalizeCommanderIDs(commanderIDs);
  const next = new Set(current);
  if (assigned) {
    next.add(commanderID);
  } else {
    next.delete(commanderID);
  }
  return {
    ...document,
    version: 2,
    assignments: {
      ...document.assignments,
      [featureID]: normalizeCommanderIDs([...next]),
    },
  };
}

function isEquipmentEffectRequirement(
  requirement: CommanderFeatureRequirement,
): requirement is CommanderEquipmentEffectRequirement {
  return requirement.kind === 'equipmentEffect'
    && Number.isFinite(requirement.effectDefinitionId)
    && Number(requirement.effectDefinitionId) > 0;
}

function commanderEquipmentEffectValue(
  state: GameStateV2,
  commanderID: number,
  requirement: CommanderEquipmentEffectRequirement,
): number | null {
  const commander = state.commanders[String(commanderID)];
  if (!commander) return null;
  let total = 0;
  let found = false;
  for (const equipmentID of Object.values(commander.equipment)) {
    const equipment = state.inventory.equipment[String(equipmentID)];
    if (!equipment) continue;
    for (const effect of Array.isArray(equipment.effects) ? equipment.effects : []) {
      if (effect.definitionId !== requirement.effectDefinitionId) continue;
      if ((requirement.unitId ?? 0) > 0) {
        for (let index = 0; index + 1 < effect.values.length; index += 2) {
          if (Math.trunc(effect.values[index]) !== requirement.unitId) continue;
          const value = Number(effect.values[index + 1]);
          if (!Number.isFinite(value)) continue;
          total += value;
          found = true;
        }
      } else {
        const value = Number(effect.values.at(-1));
        if (!Number.isFinite(value)) continue;
        total += value;
        found = true;
      }
    }
  }
  return found ? total : null;
}

function satisfiesRequirement(
  value: number | null,
  requirement: CommanderEquipmentEffectRequirement,
): boolean {
  if (value == null) return false;
  const minimum = Number(requirement.minimumValue);
  const maximum = Number(requirement.maximumValue);
  if (Number.isFinite(minimum) && value < minimum) return false;
  if (Number.isFinite(maximum) && value > maximum) return false;
  return true;
}

function normalizeCommanderIDs(candidateIDs: readonly number[]): number[] {
  return [...new Set(candidateIDs.flatMap((candidateID) => (
    Number.isFinite(candidateID) && candidateID >= 0 ? [Math.trunc(candidateID)] : []
  )))].sort((left, right) => left - right);
}

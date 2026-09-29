import type { ConfigurationSnapshot } from '../../api/Contracts';
import {
  COMMANDER_FEATURE_SECTION,
  parseCommanderFeatureAssignments,
  setCommanderAssignment,
  setCommanderFeatureForAll,
  type CommanderFeatureConfigurationV2,
  type CommanderFeatureID,
} from '../../Movement/types/CommanderFeatureAssignments';

/**
 * Panel-local draft of the shared `automation.commanderFeatures` document.
 * Only the panel's feature key changes; saving is a separate, confirmed write
 * that never touches the module draft and never enables an automation.
 */

export interface CommanderAssignmentSession {
  sections: Record<string, unknown> | undefined;
  saveSection: (section: string, value: unknown) => Promise<ConfigurationSnapshot>;
}

export function savedCommanderAssignments(sections: Record<string, unknown> | undefined): CommanderFeatureConfigurationV2 {
  return parseCommanderFeatureAssignments(sections?.[COMMANDER_FEATURE_SECTION]);
}

/** Toggle one commander for one feature. An implicit "all allowed" list becomes explicit using the observed ids. */
export function toggleFeatureCommander(
  document: CommanderFeatureConfigurationV2,
  featureId: CommanderFeatureID,
  commanderId: number,
  assigned: boolean,
  observedCommanderIds: readonly number[],
): CommanderFeatureConfigurationV2 {
  return setCommanderAssignment(document, featureId, commanderId, assigned, observedCommanderIds);
}

/** "All" restores the default (every current and future commander); "None" is an explicit empty list. */
export function setFeatureCommandersAll(
  document: CommanderFeatureConfigurationV2,
  featureId: CommanderFeatureID,
  allowed: boolean,
): CommanderFeatureConfigurationV2 {
  return setCommanderFeatureForAll(document, featureId, allowed);
}

export type CommanderAssignmentSaveResult =
  | { ok: true; saved: CommanderFeatureConfigurationV2 }
  | { ok: false; error: unknown };

/**
 * Writes the whole document through the draft session (CAS on the modal's
 * whole-configuration baseline). A failure leaves the caller's draft intact.
 */
export async function saveCommanderAssignmentDraft(
  session: CommanderAssignmentSession,
  next: CommanderFeatureConfigurationV2,
): Promise<CommanderAssignmentSaveResult> {
  try {
    const snapshot = await session.saveSection(COMMANDER_FEATURE_SECTION, next);
    return { ok: true, saved: savedCommanderAssignments(snapshot?.sections) };
  } catch (error) {
    return { ok: false, error };
  }
}

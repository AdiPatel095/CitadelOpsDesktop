import type {
  AttackSetupDraft,
  AttackSetupCourtyardSupport,
  AttackSetupLane,
  AttackSetupSlot,
  AttackSetupWave,
} from '../components/AttackSetupModal';

export const ATTACK_PRESETS_SECTION = 'attacks.presets';

export type AttackPresetTargetType = 'pve' | 'pvp';

export interface AttackPresetToolLimits {
  L: number;
  M: number;
  R: number;
}

export interface AppAttackPreset extends AttackSetupDraft {
  id: string;
  targetType: AttackPresetTargetType;
  createdAt: string;
  updatedAt: string;
}

export interface AttackPresetDocument {
  version: 1;
  presets: AppAttackPreset[];
}

export interface AttackPresetSummary {
  waves: number;
  troops: number;
  tools: number;
  courtyardTroops: number;
  courtyardTools: number;
  troopTypes: number[];
  toolTypes: number[];
}

export function parseAttackPresetDocument(value: unknown): AttackPresetDocument {
  if (!isRecord(value) || !Array.isArray(value.presets)) return emptyAttackPresetDocument();
  return {
    version: 1,
    presets: value.presets.map(parseAttackPreset).filter((preset): preset is AppAttackPreset => preset != null),
  };
}

export function emptyAttackPresetDocument(): AttackPresetDocument {
  return { version: 1, presets: [] };
}

export function attackPresetToolLimits(
  targetType: AttackPresetTargetType,
  hallToolBonus: number,
): AttackPresetToolLimits {
  const bonus = targetType === 'pvp'
    ? Math.max(0, Math.min(10, Math.trunc(Number.isFinite(hallToolBonus) ? hallToolBonus : 0)))
    : 0;
  return { L: 30 + bonus, M: 40 + bonus, R: 30 + bonus };
}

export function summarizeAttackPreset(preset: AttackSetupDraft): AttackPresetSummary {
  const troopTypes = new Set<number>();
  const toolTypes = new Set<number>();
  let troops = 0;
  let tools = 0;
  for (const wave of preset.waves) {
    for (const lane of [wave.L, wave.M, wave.R]) {
      for (const slot of lane.troops) {
        if (slot.itemId == null || slot.quantity <= 0) continue;
        troops += slot.quantity;
        troopTypes.add(slot.itemId);
      }
      for (const slot of lane.tools) {
        if (slot.itemId == null || slot.quantity <= 0) continue;
        tools += slot.quantity;
        toolTypes.add(slot.itemId);
      }
    }
  }
  let courtyardTroops = 0;
  let courtyardTools = 0;
  for (const slot of preset.courtyardSupport?.troops ?? []) {
    if (slot.itemId == null || slot.quantity <= 0) continue;
    courtyardTroops += slot.quantity;
    troops += slot.quantity;
    troopTypes.add(slot.itemId);
  }
  for (const slot of preset.courtyardSupport?.tools ?? []) {
    if (slot.itemId == null) continue;
    courtyardTools += 1;
    tools += 1;
    toolTypes.add(slot.itemId);
  }
  return {
    waves: preset.waves.length,
    troops,
    tools,
    courtyardTroops,
    courtyardTools,
    troopTypes: Array.from(troopTypes),
    toolTypes: Array.from(toolTypes),
  };
}

function parseAttackPreset(value: unknown): AppAttackPreset | null {
  if (!isRecord(value) || typeof value.id !== 'string' || typeof value.name !== 'string' || !Array.isArray(value.waves)) {
    return null;
  }
  const waves = value.waves.map(parseWave).filter((wave): wave is AttackSetupWave => wave != null);
  if (waves.length === 0) return null;
  const createdAt = validDate(value.createdAt) ?? new Date(0).toISOString();
  return {
    id: value.id,
    name: value.name,
    targetType: value.targetType === 'pvp' ? 'pvp' : 'pve',
    waves,
    courtyardSupport: parseCourtyardSupport(value.courtyardSupport),
    createdAt,
    updatedAt: validDate(value.updatedAt) ?? createdAt,
  };
}

function parseCourtyardSupport(value: unknown): AttackSetupCourtyardSupport {
  if (!isRecord(value)) return emptyCourtyardSupport();
  return {
    troops: parseFixedSlots(value.troops, 8, false),
    tools: parseFixedSlots(value.tools, 3, true),
  };
}

function parseFixedSlots(value: unknown, count: number, fixedQuantity: boolean): AttackSetupSlot[] {
  const slots = Array.isArray(value)
    ? value.map(parseSlot).filter((slot): slot is AttackSetupSlot => slot != null)
    : [];
  return Array.from({ length: count }, (_, index) => {
    const slot = slots[index];
    if (!slot || slot.itemId == null) return { itemId: null, quantity: 0 };
    return { itemId: slot.itemId, quantity: fixedQuantity ? 1 : slot.quantity };
  });
}

function emptyCourtyardSupport(): AttackSetupCourtyardSupport {
  return {
    troops: Array.from({ length: 8 }, () => ({ itemId: null, quantity: 0 })),
    tools: Array.from({ length: 3 }, () => ({ itemId: null, quantity: 0 })),
  };
}

function parseWave(value: unknown): AttackSetupWave | null {
  if (!isRecord(value)) return null;
  const L = parseLane(value.L);
  const M = parseLane(value.M);
  const R = parseLane(value.R);
  return L && M && R ? { L, M, R } : null;
}

function parseLane(value: unknown): AttackSetupLane | null {
  if (!isRecord(value) || !Array.isArray(value.troops) || !Array.isArray(value.tools)) return null;
  return {
    troops: value.troops.map(parseSlot).filter((slot): slot is AttackSetupSlot => slot != null),
    tools: value.tools.map(parseSlot).filter((slot): slot is AttackSetupSlot => slot != null),
  };
}

function parseSlot(value: unknown): AttackSetupSlot | null {
  if (!isRecord(value)) return null;
  const itemID = value.itemId == null ? null : Number(value.itemId);
  const quantity = Number(value.quantity);
  if (itemID != null && (!Number.isFinite(itemID) || itemID <= 0)) return null;
  if (!Number.isFinite(quantity) || quantity < 0) return null;
  return { itemId: itemID == null ? null : Math.trunc(itemID), quantity: Math.trunc(quantity) };
}

function validDate(value: unknown): string | null {
  if (typeof value !== 'string' || !Number.isFinite(Date.parse(value))) return null;
  return value;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

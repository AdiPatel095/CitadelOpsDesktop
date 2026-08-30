import type {
  CastleStateV2,
  DefenseMoatStateV2,
  DefenseToolSlotV2,
  DefenseWallSectionV2,
} from '../api/Contracts';

export const DEFENSE_PRESETS_SECTION = 'defense.presets';
export const DEFENSE_WALL_FLANK_TOOL_SLOT_COUNT = 4;
export const DEFENSE_WALL_MIDDLE_TOOL_SLOT_COUNT = 6;
export const DEFENSE_MOAT_TOOL_SLOT_COUNT = 1;
export const DEFENSE_KEEP_TOOL_SLOT_COUNT = 3;

// DFW M.S is positional: zero-based indexes 1 and 4 are the two gate slots.
export function isDefenseWallMiddleGateSlot(index: number): boolean {
  return index === 1 || index === 4;
}

export interface DefensePresetKeep {
  mauct: number;
  unitTypePercent: number;
  // Older presets omit both rows and preserve the target castle's current
  // courtyard tools. New and captured presets store both fixed DFK rows.
  primaryToolSlots?: DefenseToolSlotV2[];
  secondaryToolSlots?: DefenseToolSlotV2[];
}

export interface DefensePresetDraft {
  name: string;
  wall: {
    left: DefenseWallSectionV2;
    middle: DefenseWallSectionV2;
    right: DefenseWallSectionV2;
  };
  moat: Pick<DefenseMoatStateV2, 'leftToolSlots' | 'middleToolSlots' | 'rightToolSlots'>;
  keep?: DefensePresetKeep;
  sourceCastleId?: number;
  sourceCastleName?: string;
}

export interface AppDefensePreset extends DefensePresetDraft {
  id: string;
  createdAt: string;
  updatedAt: string;
}

export interface DefensePresetDocument {
  version: 1;
  presets: AppDefensePreset[];
}

export interface DefensePresetSummary {
  toolAmount: number;
  toolTypes: number[];
  wallSlots: number;
  moatSlots: number;
  courtyardSlots: number;
}

export function emptyDefensePresetDocument(): DefensePresetDocument {
  return { version: 1, presets: [] };
}

export function emptyDefensePresetDraft(): DefensePresetDraft {
  return {
    name: '',
    wall: {
      left: emptyWallSection(DEFENSE_WALL_FLANK_TOOL_SLOT_COUNT, 33),
      middle: emptyWallSection(DEFENSE_WALL_MIDDLE_TOOL_SLOT_COUNT, 34),
      right: emptyWallSection(DEFENSE_WALL_FLANK_TOOL_SLOT_COUNT, 33),
    },
    moat: {
      leftToolSlots: emptyToolSlots(DEFENSE_MOAT_TOOL_SLOT_COUNT),
      middleToolSlots: emptyToolSlots(DEFENSE_MOAT_TOOL_SLOT_COUNT),
      rightToolSlots: emptyToolSlots(DEFENSE_MOAT_TOOL_SLOT_COUNT),
    },
  };
}

export function defensePresetDraftFromCastle(castle: CastleStateV2): DefensePresetDraft {
  return {
    name: `${castle.name?.trim() || `Castle ${castle.id}`} defense`,
    wall: {
      left: cloneWallSection(castle.defense.wall.left),
      middle: cloneWallSection(castle.defense.wall.middle),
      right: cloneWallSection(castle.defense.wall.right),
    },
    moat: {
      leftToolSlots: cloneToolSlots(castle.defense.moat.leftToolSlots),
      middleToolSlots: cloneToolSlots(castle.defense.moat.middleToolSlots),
      rightToolSlots: cloneToolSlots(castle.defense.moat.rightToolSlots),
    },
    keep: {
      mauct: Math.max(0, Math.trunc(castle.defense.keep.mauct ?? 0)),
      unitTypePercent: clampPercent(castle.defense.keep.unitTypePercent),
      primaryToolSlots: cloneToolSlots(castle.defense.keep.primaryToolSlots),
      secondaryToolSlots: cloneToolSlots(castle.defense.keep.secondaryToolSlots),
    },
    sourceCastleId: castle.id,
    sourceCastleName: castle.name?.trim() || undefined,
  };
}

export function cloneDefensePresetDraft(preset: DefensePresetDraft): DefensePresetDraft {
  return {
    name: preset.name,
    wall: {
      left: cloneWallSection(preset.wall.left),
      middle: cloneWallSection(preset.wall.middle),
      right: cloneWallSection(preset.wall.right),
    },
    moat: {
      leftToolSlots: cloneToolSlots(preset.moat.leftToolSlots),
      middleToolSlots: cloneToolSlots(preset.moat.middleToolSlots),
      rightToolSlots: cloneToolSlots(preset.moat.rightToolSlots),
    },
    ...(preset.keep ? {
      keep: {
        ...preset.keep,
        ...(preset.keep.primaryToolSlots ? { primaryToolSlots: cloneToolSlots(preset.keep.primaryToolSlots) } : {}),
        ...(preset.keep.secondaryToolSlots ? { secondaryToolSlots: cloneToolSlots(preset.keep.secondaryToolSlots) } : {}),
      },
    } : {}),
    ...(preset.sourceCastleId != null ? { sourceCastleId: preset.sourceCastleId } : {}),
    ...(preset.sourceCastleName ? { sourceCastleName: preset.sourceCastleName } : {}),
  };
}

export function normalizeDefensePresetSlots(preset: DefensePresetDraft): DefensePresetDraft {
  const draft = cloneDefensePresetDraft(preset);
  draft.wall.left.toolSlots = fixedToolSlots(draft.wall.left.toolSlots, DEFENSE_WALL_FLANK_TOOL_SLOT_COUNT);
  draft.wall.middle.toolSlots = fixedToolSlots(draft.wall.middle.toolSlots, DEFENSE_WALL_MIDDLE_TOOL_SLOT_COUNT);
  draft.wall.right.toolSlots = fixedToolSlots(draft.wall.right.toolSlots, DEFENSE_WALL_FLANK_TOOL_SLOT_COUNT);
  draft.moat.leftToolSlots = fixedToolSlots(draft.moat.leftToolSlots, DEFENSE_MOAT_TOOL_SLOT_COUNT);
  draft.moat.middleToolSlots = fixedToolSlots(draft.moat.middleToolSlots, DEFENSE_MOAT_TOOL_SLOT_COUNT);
  draft.moat.rightToolSlots = fixedToolSlots(draft.moat.rightToolSlots, DEFENSE_MOAT_TOOL_SLOT_COUNT);
  if (draft.keep?.primaryToolSlots && draft.keep.secondaryToolSlots) {
    draft.keep.primaryToolSlots = fixedToolSlots(draft.keep.primaryToolSlots, DEFENSE_KEEP_TOOL_SLOT_COUNT);
    draft.keep.secondaryToolSlots = fixedToolSlots(draft.keep.secondaryToolSlots, DEFENSE_KEEP_TOOL_SLOT_COUNT);
  }
  return draft;
}

export function parseDefensePresetDocument(value: unknown): DefensePresetDocument {
  if (!isRecord(value) || (value.version != null && value.version !== 1) || !Array.isArray(value.presets)) {
    return emptyDefensePresetDocument();
  }
  return {
    version: 1,
    presets: value.presets.map(parseDefensePreset).filter((preset): preset is AppDefensePreset => preset != null),
  };
}

export function summarizeDefensePreset(preset: DefensePresetDraft): DefensePresetSummary {
  const slots = [
    ...preset.wall.left.toolSlots,
    ...preset.wall.middle.toolSlots,
    ...preset.wall.right.toolSlots,
    ...preset.moat.leftToolSlots,
    ...preset.moat.middleToolSlots,
    ...preset.moat.rightToolSlots,
    ...(preset.keep?.primaryToolSlots ?? []),
    ...(preset.keep?.secondaryToolSlots ?? []),
  ];
  const toolTypes = new Set<number>();
  let toolAmount = 0;
  for (const slot of slots) {
    if (slot.definitionId <= 0 || slot.amount <= 0) continue;
    toolTypes.add(slot.definitionId);
    toolAmount += slot.amount;
  }
  return {
    toolAmount,
    toolTypes: Array.from(toolTypes),
    wallSlots: preset.wall.left.toolSlots.length + preset.wall.middle.toolSlots.length + preset.wall.right.toolSlots.length,
    moatSlots: preset.moat.leftToolSlots.length + preset.moat.middleToolSlots.length + preset.moat.rightToolSlots.length,
    courtyardSlots: preset.keep?.primaryToolSlots && preset.keep.secondaryToolSlots
      ? preset.keep.primaryToolSlots.length + preset.keep.secondaryToolSlots.length
      : 0,
  };
}

function parseDefensePreset(value: unknown): AppDefensePreset | null {
  if (!isRecord(value) || typeof value.id !== 'string' || typeof value.name !== 'string') return null;
  const draft = parseDefensePresetDraft(value);
  if (!draft) return null;
  const normalized = normalizeDefensePresetSlots(draft);
  const createdAt = validDate(value.createdAt) ?? new Date(0).toISOString();
  return {
    ...normalized,
    id: value.id,
    createdAt,
    updatedAt: validDate(value.updatedAt) ?? createdAt,
  };
}

function parseDefensePresetDraft(value: Record<string, unknown>): DefensePresetDraft | null {
  if (!isRecord(value.wall) || !isRecord(value.moat)) return null;
  const left = parseWallSection(value.wall.left);
  const middle = parseWallSection(value.wall.middle);
  const right = parseWallSection(value.wall.right);
  const leftToolSlots = parseToolSlots(value.moat.leftToolSlots);
  const middleToolSlots = parseToolSlots(value.moat.middleToolSlots);
  const rightToolSlots = parseToolSlots(value.moat.rightToolSlots);
  if (!left || !middle || !right || !leftToolSlots || !middleToolSlots || !rightToolSlots) return null;
  const keep = value.keep == null ? undefined : parseKeep(value.keep);
  if (value.keep != null && !keep) return null;
  const sourceCastleId = positiveInteger(value.sourceCastleId);
  return {
    name: typeof value.name === 'string' ? value.name : '',
    wall: { left, middle, right },
    moat: { leftToolSlots, middleToolSlots, rightToolSlots },
    ...(keep ? { keep } : {}),
    ...(sourceCastleId != null ? { sourceCastleId } : {}),
    ...(typeof value.sourceCastleName === 'string' && value.sourceCastleName.trim()
      ? { sourceCastleName: value.sourceCastleName.trim() }
      : {}),
  };
}

function parseWallSection(value: unknown): DefenseWallSectionV2 | null {
  if (!isRecord(value)) return null;
  const toolSlots = parseToolSlots(value.toolSlots);
  const unitPercent = integerInRange(value.unitPercent, 0, 100);
  const unitTypePercent = integerInRange(value.unitTypePercent, 0, 100);
  return toolSlots && unitPercent != null && unitTypePercent != null
    ? { toolSlots, unitPercent, unitTypePercent }
    : null;
}

function parseToolSlots(value: unknown): DefenseToolSlotV2[] | null {
  if (!Array.isArray(value)) return null;
  const slots: DefenseToolSlotV2[] = [];
  for (const candidate of value) {
    const slot = parseToolSlot(candidate);
    if (!slot) return null;
    slots.push(slot);
  }
  return slots;
}

function parseToolSlot(value: unknown): DefenseToolSlotV2 | null {
  if (!isRecord(value)) return null;
  const definitionId = Number(value.definitionId);
  const amount = Number(value.amount);
  if (!Number.isInteger(definitionId) || !Number.isInteger(amount)) return null;
  if (definitionId === -1 && amount === 0) return { definitionId, amount };
  if (definitionId <= 0 || amount <= 0 || amount > 999) return null;
  return { definitionId, amount };
}

function parseKeep(value: unknown): DefensePresetKeep | null {
  if (!isRecord(value)) return null;
  const mauct = Number(value.mauct);
  const unitTypePercent = integerInRange(value.unitTypePercent, 0, 100);
  if (!Number.isSafeInteger(mauct) || mauct < 0 || unitTypePercent == null) return null;
  const hasPrimaryToolSlots = value.primaryToolSlots != null;
  const hasSecondaryToolSlots = value.secondaryToolSlots != null;
  if (hasPrimaryToolSlots !== hasSecondaryToolSlots) return null;
  if (!hasPrimaryToolSlots) return { mauct, unitTypePercent };
  const primaryToolSlots = parseToolSlots(value.primaryToolSlots);
  const secondaryToolSlots = parseToolSlots(value.secondaryToolSlots);
  if (!primaryToolSlots || !secondaryToolSlots) return null;
  return { mauct, unitTypePercent, primaryToolSlots, secondaryToolSlots };
}

function cloneWallSection(section: DefenseWallSectionV2): DefenseWallSectionV2 {
  return {
    toolSlots: cloneToolSlots(section.toolSlots),
    unitPercent: Math.trunc(section.unitPercent),
    unitTypePercent: Math.trunc(section.unitTypePercent),
  };
}

function cloneToolSlots(slots: DefenseToolSlotV2[]): DefenseToolSlotV2[] {
  return slots.map((slot) => ({ definitionId: Math.trunc(slot.definitionId), amount: Math.trunc(slot.amount) }));
}

function fixedToolSlots(slots: DefenseToolSlotV2[], count: number): DefenseToolSlotV2[] {
  return Array.from({ length: count }, (_, index) => {
    const slot = slots[index];
    return slot
      ? { definitionId: Math.trunc(slot.definitionId), amount: Math.trunc(slot.amount) }
      : { definitionId: -1, amount: 0 };
  });
}

function emptyWallSection(slotCount: number, unitPercent: number): DefenseWallSectionV2 {
  return { toolSlots: emptyToolSlots(slotCount), unitPercent, unitTypePercent: 0 };
}

function emptyToolSlots(count: number): DefenseToolSlotV2[] {
  return Array.from({ length: count }, () => ({ definitionId: -1, amount: 0 }));
}

function clampPercent(value: number): number {
  return Math.max(0, Math.min(100, Math.trunc(value)));
}

function integerInRange(value: unknown, minimum: number, maximum: number): number | null {
  const numeric = Number(value);
  return Number.isInteger(numeric) && numeric >= minimum && numeric <= maximum ? numeric : null;
}

function positiveInteger(value: unknown): number | null {
  const numeric = Number(value);
  return Number.isSafeInteger(numeric) && numeric > 0 ? numeric : null;
}

function validDate(value: unknown): string | null {
  return typeof value === 'string' && Number.isFinite(Date.parse(value)) ? value : null;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

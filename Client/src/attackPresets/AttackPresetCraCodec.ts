import type {
  AttackSetupCourtyardSupport,
  AttackSetupDraft,
  AttackSetupLane,
  AttackSetupSlot,
  AttackSetupWave,
} from '../components/AttackSetupModal';

const MAX_INPUT_LENGTH = 250_000;
const MAX_WAVES = 30;
const COURTYARD_TROOP_SLOTS = 8;
const COURTYARD_TOOL_SLOTS = 3;

const laneCapacities = {
  L: { troops: 2, tools: 2 },
  M: { troops: 6, tools: 3 },
  R: { troops: 2, tools: 2 },
} as const;

type LaneKey = keyof typeof laneCapacities;

export function parseCRAAttackPresetString(value: string): AttackSetupDraft {
  const source = value.trim();
  if (!source) throw new Error('Paste a CRA command or CRA JSON payload.');
  if (source.length > MAX_INPUT_LENGTH) throw new Error('The CRA string is too large to import.');

  const payload = parseCRAPayload(source);
  if (!Array.isArray(payload.A)) throw new Error('The CRA payload does not contain an A formation array.');
  if (payload.A.length < 1 || payload.A.length > MAX_WAVES) {
    throw new Error(`The CRA formation must contain between 1 and ${MAX_WAVES} waves.`);
  }

  const waves = payload.A.map((wave, index) => parseWave(wave, index));
  const troopCount = waves.reduce((total, wave) => (
    total + sumLaneTroops(wave.L) + sumLaneTroops(wave.M) + sumLaneTroops(wave.R)
  ), 0);
  if (troopCount <= 0) throw new Error('The CRA formation does not allocate any troops.');

  return {
    name: 'Imported CRA preset',
    waves,
    courtyardSupport: parseCourtyardSupport(payload),
  };
}

export function formatCRAAttackPresetString(preset: AttackSetupDraft): string {
  if (preset.waves.length < 1 || preset.waves.length > MAX_WAVES) {
    throw new Error(`The attack preset must contain between 1 and ${MAX_WAVES} waves.`);
  }
  const payload = {
    A: preset.waves.map((wave, index) => formatWave(wave, index)),
    AST: formatSupportTools(preset.courtyardSupport?.tools),
    RW: formatSupportTroops(preset.courtyardSupport?.troops),
    ASCT: 0,
  };
  return `%xt%EmpireEx_21%cra%1%${JSON.stringify(payload)}%`;
}

function parseCRAPayload(source: string): Record<string, unknown> {
  if (source.startsWith('"') && source.endsWith('"')) {
    try {
      const decoded = JSON.parse(source);
      if (typeof decoded === 'string') return parseCRAPayload(decoded);
    } catch {
      // Continue so the caller receives the more specific CRA wire or JSON error.
    }
  }
  if (source.includes('%xt%')) return parseCRAWire(source);

  let decoded: unknown;
  try {
    decoded = JSON.parse(source);
  } catch {
    throw new Error('The pasted value is not a valid CRA command or JSON payload.');
  }
  if (typeof decoded === 'string') return parseCRAPayload(decoded);
  if (!isRecord(decoded)) throw new Error('The CRA JSON payload must be an object.');

  const command = isRecord(decoded.command) ? decoded.command : decoded;
  if (typeof command.opcode === 'string' || Object.hasOwn(command, 'payload')) {
    if (typeof command.opcode !== 'string' || command.opcode.trim().toLowerCase() !== 'cra') {
      throw new Error('The command envelope is not a CRA command.');
    }
    return parseEnvelopePayload(command.payload);
  }
  return decoded;
}

function parseCRAWire(source: string): Record<string, unknown> {
  const start = source.indexOf('%xt%');
  let wire = source.slice(start);
  const lineEnd = wire.search(/[\r\n]/);
  if (lineEnd >= 0) wire = wire.slice(0, lineEnd);

  let cursor = 4;
  const readSegment = (label: string): string => {
    const end = wire.indexOf('%', cursor);
    if (end < 0) throw new Error(`The CRA wire string is missing its ${label}.`);
    const segment = wire.slice(cursor, end);
    cursor = end + 1;
    return segment;
  };

  const namespaceOrOpcode = readSegment('opcode');
  const opcode = namespaceOrOpcode.toLowerCase() === 'cra'
    ? namespaceOrOpcode
    : readSegment('opcode');
  if (opcode.trim().toLowerCase() !== 'cra') throw new Error(`Expected a CRA command, received ${opcode || 'an empty opcode'}.`);

  const sequence = readSegment('sequence');
  if (!sequence.trim()) throw new Error('The CRA wire string has an empty sequence.');
  const payloadEnd = wire.lastIndexOf('%');
  if (payloadEnd < cursor) throw new Error('The CRA wire string is missing its JSON payload.');
  return parseEnvelopePayload(wire.slice(cursor, payloadEnd));
}

function parseEnvelopePayload(value: unknown): Record<string, unknown> {
  let decoded = value;
  if (typeof decoded === 'string') {
    try {
      decoded = JSON.parse(decoded);
    } catch {
      throw new Error('The CRA command payload is not valid JSON.');
    }
  }
  if (!isRecord(decoded)) throw new Error('The CRA command payload must be a JSON object.');
  return decoded;
}

function parseWave(value: unknown, waveIndex: number): AttackSetupWave {
  if (!isRecord(value)) throw new Error(`CRA wave ${waveIndex + 1} is not an object.`);
  return {
    L: parseLane(value.L, 'L', waveIndex),
    M: parseLane(value.M, 'M', waveIndex),
    R: parseLane(value.R, 'R', waveIndex),
  };
}

function parseLane(value: unknown, laneKey: LaneKey, waveIndex: number): AttackSetupLane {
  if (value == null) return emptyLane(laneKey);
  if (!isRecord(value)) throw new Error(`CRA wave ${waveIndex + 1} ${laneLabel(laneKey)} lane is not an object.`);
  return {
    troops: parseSlots(value.U, laneCapacities[laneKey].troops, waveIndex, laneKey, 'troop'),
    tools: parseSlots(value.T, laneCapacities[laneKey].tools, waveIndex, laneKey, 'tool'),
  };
}

function parseSlots(
  value: unknown,
  capacity: number,
  waveIndex: number,
  laneKey: LaneKey,
  kind: 'troop' | 'tool',
): AttackSetupSlot[] {
  if (value == null) return emptySlots(capacity);
  if (!Array.isArray(value)) {
    throw new Error(`CRA wave ${waveIndex + 1} ${laneLabel(laneKey)} ${kind} slots are not an array.`);
  }
  if (value.length > capacity) {
    throw new Error(`CRA wave ${waveIndex + 1} ${laneLabel(laneKey)} has more than ${capacity} ${kind} slots.`);
  }

  return Array.from({ length: capacity }, (_, index) => {
    const pair = value[index];
    if (pair == null) return emptySlot();
    if (!Array.isArray(pair) || pair.length !== 2) {
      throw new Error(`CRA wave ${waveIndex + 1} ${laneLabel(laneKey)} ${kind} slot ${index + 1} is invalid.`);
    }
    const [itemID, quantity] = pair;
    if (!Number.isSafeInteger(itemID) || !Number.isSafeInteger(quantity)) {
      throw new Error(`CRA wave ${waveIndex + 1} ${laneLabel(laneKey)} ${kind} slot ${index + 1} must use whole numbers.`);
    }
    if (itemID <= 0) {
      if (quantity === 0) return emptySlot();
      throw new Error(`CRA wave ${waveIndex + 1} ${laneLabel(laneKey)} ${kind} slot ${index + 1} has an invalid item.`);
    }
    if (quantity < 0) {
      throw new Error(`CRA wave ${waveIndex + 1} ${laneLabel(laneKey)} ${kind} slot ${index + 1} has a negative quantity.`);
    }
    return { itemId: itemID, quantity };
  });
}

function parseCourtyardSupport(payload: Record<string, unknown>): AttackSetupCourtyardSupport {
  return {
    troops: parseSupportTroops(payload.RW),
    tools: parseSupportTools(payload.AST),
  };
}

function parseSupportTroops(value: unknown): AttackSetupSlot[] {
  if (value == null) return emptySlots(COURTYARD_TROOP_SLOTS);
  if (!Array.isArray(value)) throw new Error('CRA courtyard support troops (RW) must be an array.');
  if (value.length > COURTYARD_TROOP_SLOTS) {
    throw new Error(`CRA courtyard support has more than ${COURTYARD_TROOP_SLOTS} troop slots.`);
  }
  return Array.from({ length: COURTYARD_TROOP_SLOTS }, (_, index) => {
    const pair = value[index];
    if (pair == null) return emptySlot();
    if (!Array.isArray(pair) || pair.length !== 2) {
      throw new Error(`CRA courtyard troop slot ${index + 1} is invalid.`);
    }
    const [itemID, quantity] = pair;
    if (!Number.isSafeInteger(itemID) || !Number.isSafeInteger(quantity)) {
      throw new Error(`CRA courtyard troop slot ${index + 1} must use whole numbers.`);
    }
    if (itemID <= 0) {
      if (quantity === 0) return emptySlot();
      throw new Error(`CRA courtyard troop slot ${index + 1} has an invalid item.`);
    }
    if (quantity < 0) throw new Error(`CRA courtyard troop slot ${index + 1} has a negative quantity.`);
    return { itemId: itemID, quantity };
  });
}

function parseSupportTools(value: unknown): AttackSetupSlot[] {
  if (value == null) return emptySlots(COURTYARD_TOOL_SLOTS);
  if (!Array.isArray(value)) throw new Error('CRA Sceat support tools (AST) must be an array.');
  if (value.length > COURTYARD_TOOL_SLOTS) {
    throw new Error(`CRA courtyard support has more than ${COURTYARD_TOOL_SLOTS} Sceat tool slots.`);
  }
  return Array.from({ length: COURTYARD_TOOL_SLOTS }, (_, index) => {
    const itemID = value[index];
    if (itemID == null) return emptySlot();
    if (!Number.isSafeInteger(itemID)) throw new Error(`CRA Sceat support tool slot ${index + 1} must use a whole number.`);
    return itemID > 0 ? { itemId: itemID, quantity: 1 } : emptySlot();
  });
}

function formatWave(wave: AttackSetupWave, waveIndex: number): Record<LaneKey, { U: [number, number][]; T: [number, number][] }> {
  return {
    L: formatLane(wave.L, 'L', waveIndex),
    M: formatLane(wave.M, 'M', waveIndex),
    R: formatLane(wave.R, 'R', waveIndex),
  };
}

function formatLane(
  lane: AttackSetupLane,
  laneKey: LaneKey,
  waveIndex: number,
): { U: [number, number][]; T: [number, number][] } {
  return {
    U: formatSlots(lane.troops, laneCapacities[laneKey].troops, waveIndex, laneKey, 'troop'),
    T: formatSlots(lane.tools, laneCapacities[laneKey].tools, waveIndex, laneKey, 'tool'),
  };
}

function formatSlots(
  slots: AttackSetupSlot[],
  capacity: number,
  waveIndex: number,
  laneKey: LaneKey,
  kind: 'troop' | 'tool',
): [number, number][] {
  if (slots.length > capacity) {
    throw new Error(`Wave ${waveIndex + 1} ${laneLabel(laneKey)} has more than ${capacity} ${kind} slots.`);
  }
  return Array.from({ length: capacity }, (_, index) => {
    const slot = slots[index];
    if (!slot || slot.itemId == null) {
      if (slot && slot.quantity > 0) throw new Error(`Wave ${waveIndex + 1} contains a ${kind} quantity without an item.`);
      return [-1, 0];
    }
    if (!Number.isSafeInteger(slot.itemId) || slot.itemId <= 0 ||
        !Number.isSafeInteger(slot.quantity) || slot.quantity < 0) {
      throw new Error(`Wave ${waveIndex + 1} ${laneLabel(laneKey)} ${kind} slot ${index + 1} is invalid.`);
    }
    return [slot.itemId, slot.quantity];
  });
}

function formatSupportTroops(slots: AttackSetupSlot[] | undefined): [number, number][] {
  if ((slots?.length ?? 0) > COURTYARD_TROOP_SLOTS) {
    throw new Error(`Courtyard support has more than ${COURTYARD_TROOP_SLOTS} troop slots.`);
  }
  return Array.from({ length: COURTYARD_TROOP_SLOTS }, (_, index) => {
    const slot = slots?.[index];
    if (!slot || slot.itemId == null) {
      if (slot && slot.quantity > 0) throw new Error(`Courtyard troop slot ${index + 1} has a quantity without an item.`);
      return [-1, 0];
    }
    if (!Number.isSafeInteger(slot.itemId) || slot.itemId <= 0 ||
        !Number.isSafeInteger(slot.quantity) || slot.quantity < 0) {
      throw new Error(`Courtyard troop slot ${index + 1} is invalid.`);
    }
    return [slot.itemId, slot.quantity];
  });
}

function formatSupportTools(slots: AttackSetupSlot[] | undefined): number[] {
  if ((slots?.length ?? 0) > COURTYARD_TOOL_SLOTS) {
    throw new Error(`Courtyard support has more than ${COURTYARD_TOOL_SLOTS} Sceat tool slots.`);
  }
  return Array.from({ length: COURTYARD_TOOL_SLOTS }, (_, index) => {
    const slot = slots?.[index];
    if (!slot || slot.itemId == null) {
      if (slot && slot.quantity > 0) throw new Error(`Sceat support tool slot ${index + 1} has a quantity without an item.`);
      return -1;
    }
    if (!Number.isSafeInteger(slot.itemId) || slot.itemId <= 0 || slot.quantity !== 1) {
      throw new Error(`Sceat support tool slot ${index + 1} must contain exactly one valid item.`);
    }
    return slot.itemId;
  });
}

function emptyLane(laneKey: LaneKey): AttackSetupLane {
  return {
    troops: emptySlots(laneCapacities[laneKey].troops),
    tools: emptySlots(laneCapacities[laneKey].tools),
  };
}

function emptySlots(count: number): AttackSetupSlot[] {
  return Array.from({ length: count }, emptySlot);
}

function emptySlot(): AttackSetupSlot {
  return { itemId: null, quantity: 0 };
}

function sumLaneTroops(lane: AttackSetupLane): number {
  return lane.troops.reduce((total, slot) => total + slot.quantity, 0);
}

function laneLabel(laneKey: LaneKey): string {
  if (laneKey === 'L') return 'left';
  if (laneKey === 'M') return 'middle';
  return 'right';
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

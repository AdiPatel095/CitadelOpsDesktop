/**
 * Synthetic official-data catalogs for the onboarding preview (CIT-22). Rows use the game data's own field names so the
 * production loaders read them unchanged; names, ids and values are invented (a few ids match the fictional realm's
 * castles and units). Scenarios may replace any catalog's rows (`patch.catalogs.<name>`).
 */
type Row = Record<string, unknown>;

const metadata = () => ({
  itemVersion: 'fixture-1',
  sourceUrl: 'fixture://synthetic-catalogs',
  digestSha256: 'fixture-onboarding-preview',
  languageVersion: 'fixture-1',
  fetchedAt: '2026-09-29T00:00:00Z',
  loadedAt: '2026-09-29T00:00:00Z',
});

const catalog = (name: string, items: Row[]) => ({ metadata: metadata(), catalog: { name, kind: 'fixture', count: items.length }, items });

const troop = (wodID: number, name: string, meleeAttack: number, rangeAttack: number, meleeDefence: number, rangeDefence: number, level: number): Row => (
  { wodID, name, type: rangeAttack > meleeAttack ? 'ranged' : 'melee', level, meleeAttack, rangeAttack, meleeDefence, rangeDefence }
);

export const DEFAULT_CATALOG_ROWS: Readonly<Record<string, Row[]>> = {
  units: [
    troop(1, 'unit_spearman', 20, 0, 8, 8, 1),
    troop(2, 'unit_bowman', 0, 25, 5, 5, 1),
    troop(6, 'unit_macebearer', 32, 0, 12, 12, 2),
    troop(7, 'unit_crossbowman', 0, 38, 8, 8, 2),
    troop(39, 'unit_royal_guard', 6, 0, 45, 45, 4),
    troop(40, 'unit_veteran_archer', 0, 55, 10, 10, 4),
    troop(277, 'unit_direwolf', 90, 0, 30, 30, 5),
    { wodID: 101, name: 'tool_ladder', type: 'AttackTool', slotTypes: ['attack'], level: 1 },
    { wodID: 102, name: 'tool_ram', type: 'AttackTool', slotTypes: ['attack'], level: 2 },
    { wodID: 103, name: 'tool_mantlet', type: 'AttackTool', slotTypes: ['attack'], level: 2 },
    { wodID: 104, name: 'tool_siege_tower', type: 'AttackTool', slotTypes: ['attack'], level: 3 },
    { wodID: 201, name: 'tool_wall_shield', typ: 'defence', slotTypes: [1], level: 1 },
    { wodID: 202, name: 'tool_wall_brazier', typ: 'defence', slotTypes: [1], level: 2 },
    { wodID: 203, name: 'tool_gate_brace', typ: 'defence', slotTypes: [2], level: 2 },
    { wodID: 204, name: 'tool_moat_spikes', typ: 'defence', slotTypes: [4], level: 1 },
    { wodID: 205, name: 'tool_courtyard_barricade', typ: 'defence', slotTypes: [5], level: 2 },
    { wodID: 206, name: 'tool_keep_cauldron', typ: 'defence', slotTypes: [5], level: 3 },
    { wodID: 210, name: 'tool_sceat_banner', typ: 'defence', slotTypes: [6], level: 1 },
    { wodID: 211, name: 'tool_sceat_totem', typ: 'defence', slotTypes: [6], level: 2 },
  ],
  equipments: [
    { equipmentID: 7101, name: 'equipment_dread_helm', slot: 1, rarity: 4 },
    { equipmentID: 7102, name: 'equipment_storm_plate', slot: 2, rarity: 3 },
    { equipmentID: 7103, name: 'equipment_ember_blade', slot: 3, rarity: 5 },
    { equipmentID: 7104, name: 'equipment_scout_cowl', slot: 1, rarity: 2 },
    { equipmentID: 7105, name: 'equipment_bastion_shield', slot: 4, rarity: 4 },
    { equipmentID: 7106, name: 'equipment_ranger_mail', slot: 2, rarity: 3 },
  ],
  gems: [
    { gemID: 7201, name: 'gem_ruby_of_wrath' },
    { gemID: 7202, name: 'gem_sapphire_of_guard' },
    { gemID: 7203, name: 'gem_topaz_of_march' },
  ],
  effecttypes: [
    { effectTypeID: 1, name: 'AttackStrength' },
    { effectTypeID: 2, name: 'DefenceStrength' },
    { effectTypeID: 3, name: 'MarchSpeed' },
  ],
  effects: [
    { effectID: 8801, name: 'effect_attack_strength', effectTypeID: 1 },
    { effectID: 8802, name: 'effect_defence_strength', effectTypeID: 2 },
    { effectID: 8810, name: 'effect_march_speed', effectTypeID: 3 },
  ],
  buildings: [
    { wodID: 4, name: 'barrack', type: 'barrack', level: 1 },
    { wodID: 5, name: 'workshop', type: 'workshop', level: 1 },
    { wodID: 6, name: 'refinery', type: 'refinery', level: 1 },
    { wodID: 7, name: 'toolsmith', type: 'toolsmith', level: 1 },
  ],
  resources: [
    { wodID: 1, resourceID: 1, name: 'Wood', assetName: 'Wood', JSONKey: 'W' },
    { wodID: 2, resourceID: 2, name: 'Stone', assetName: 'Stone', JSONKey: 'S' },
    { wodID: 3, resourceID: 3, name: 'Food', assetName: 'Food', JSONKey: 'F' },
    { wodID: 4, resourceID: 4, name: 'Coins', assetName: 'Coins', JSONKey: 'C1' },
  ],
  currencies: [
    { currencyID: 1, Name: 'C1', assetName: 'C1' },
    { currencyID: 2, Name: 'C2', assetName: 'C2' },
  ],
  kingdoms: [
    { kingdomID: 0, kID: 0, kingdomName: 'Great Empire' },
    { kingdomID: 1, kID: 1, kingdomName: 'Everwinter Glacier' },
    { kingdomID: 2, kID: 2, kingdomName: 'Burning Sands' },
    { kingdomID: 3, kID: 3, kingdomName: 'Fire Peaks' },
    { kingdomID: 4, kID: 4, kingdomName: 'Storm Islands' },
  ],
  // Event difficulties: events 72/80 (Nomad, Samurai) and 71/103 (Foreign Lords, Bloodcrows).
  eventAutoScalingDifficultyTypes: [
    { difficultyTypeID: 1, name: 'Easy', sortOrder: 1 },
    { difficultyTypeID: 2, name: 'Normal', sortOrder: 2 },
    { difficultyTypeID: 3, name: 'Hard', sortOrder: 3 },
  ],
  eventAutoScalingDifficulties: [
    ...[71, 72, 80, 103].flatMap((eventID) => [
      { difficultyID: eventID * 10 + 1, eventID, difficultyTypeID: 1, isLocked: 0 },
      { difficultyID: eventID * 10 + 2, eventID, difficultyTypeID: 2, isLocked: 0 },
      { difficultyID: eventID * 10 + 3, eventID, difficultyTypeID: 3, isLocked: 1 },
    ]),
  ],
  achievements: [
    ...[71, 72, 80, 103].map((eventID) => ({ achievementID: 9000 + eventID, unlocksDifficulty: eventID * 10 + 3 })),
  ],
  // Official Storm starter castles: none offered by default; the `storm-offer` scenario supplies one.
  prebuiltcastles: [],
  'currency-icons': [],
};

export function catalogFor(name: string, overrides: Readonly<Record<string, Row[]>> = {}): ReturnType<typeof catalog> {
  return catalog(name, overrides[name] ?? DEFAULT_CATALOG_ROWS[name] ?? []);
}

export const CATALOG_NAMES = Object.keys(DEFAULT_CATALOG_ROWS);

/** The manifest `GET /api/v2/game-data` answers with. */
export function catalogManifest() {
  return { metadata: metadata(), catalogs: CATALOG_NAMES.map((name) => ({ name, kind: 'fixture', count: (DEFAULT_CATALOG_ROWS[name] ?? []).length })) };
}

/** Readable labels for the localization keys the rows above reference (synthetic names). */
export const LOCALIZED: Readonly<Record<string, string>> = Object.fromEntries(
  Object.entries({
    unit_spearman: 'Spearman', unit_bowman: 'Bowman', unit_macebearer: 'Macebearer', unit_crossbowman: 'Crossbowman',
    unit_royal_guard: 'Royal Guard', unit_veteran_archer: 'Veteran Archer', unit_direwolf: 'Direwolf',
    tool_ladder: 'Ladder', tool_ram: 'Battering Ram', tool_mantlet: 'Mantlet', tool_siege_tower: 'Siege Tower',
    tool_wall_shield: 'Wall Shield', tool_wall_brazier: 'Wall Brazier', tool_gate_brace: 'Gate Brace', tool_moat_spikes: 'Moat Spikes',
    tool_courtyard_barricade: 'Courtyard Barricade', tool_keep_cauldron: 'Keep Cauldron', tool_sceat_banner: 'Sceat Banner', tool_sceat_totem: 'Sceat Totem',
    equipment_dread_helm: 'Dread Helm', equipment_storm_plate: 'Storm Plate', equipment_ember_blade: 'Ember Blade', equipment_scout_cowl: 'Scout Cowl',
    equipment_bastion_shield: 'Bastion Shield', equipment_ranger_mail: 'Ranger Mail',
    gem_ruby_of_wrath: 'Ruby of Wrath', gem_sapphire_of_guard: 'Sapphire of Guard', gem_topaz_of_march: 'Topaz of March',
    effect_attack_strength: 'Attack strength', effect_defence_strength: 'Defence strength', effect_march_speed: 'March speed',
    barrack: 'Barracks', workshop: 'Workshop', refinery: 'Refinery', toolsmith: 'Toolsmith',
  }).flatMap(([key, value]) => [[key, value], [`${key}_name`, value]]),
);

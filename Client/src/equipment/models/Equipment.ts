export interface EquipStat {
    id: number;
    percent: number;
    value: number[];
}

export interface EquipmentModel {
    id: number;
    equipSlotNumber: number;
    equipType: number;
    equipRarity: number;
    placeHolder6: number;
    equipStats: EquipStat[];
    templateId: number;
    setId: number;
    placeHolder8: number;
    equipLevel: number;
    placeHolder9: number;
    gemId: number;
    placeHolder11: number;
    gem?: any; // Define GemSlot type if available
}

export interface EquipmentExtraStat {
    key: string;
    effectId: number;
    name: string;
    label: string;
    category: string;
    unit: 'percent' | 'number';
    value: number;
}

export interface ResolvedEquipmentEffect {
    key: string;
    rawEffectId: number;
    effectId: number;
    name: string;
    label: string;
    category: string;
    unit: 'percent' | 'number';
    scope: 'generic' | 'pvp' | 'pve';
    source: 'equipment' | 'relic_equipment' | 'gem' | 'relic_gem' | 'set_bonus';
    value: number;
    argumentId?: number;
    argumentLabel?: string;
    capId?: number;
    maxTotalBonus?: number;
    sortOrder?: string;
}

export interface CommStat {
    id: number;
    name: string;
    equip1: number;
    equip2: number;
    equip3: number;
    equip4: number;
    hero: number;
    gem1: number;
    gem2: number;
    gem3: number;
    gem4: number;
    meleeCbtStr: number;
    rangeCbtStr: number;
    frontCbtStr: number;
    flankCbtStr: number;
    allCbtStr: number;
    cyCbtStr: number;
    wallStr: number;
    gateStr: number;
    moatStr: number;
    flankLimit: number;
    frontLimit: number;
    meadStr: number;
    horrorStr: number;
    eliteStr: number;
    wave: number;
    cooldown: number;
    relicStr: number;
    beserkerStr: number;
    maidenSupp: number;
    attackReinforcement: number;
    travel: number;
    loot: number;
    NPCMelee: number;
    NPCRange: number;
    NPCFront: number;
    NPCFlank: number;
    NPCCy: number;
    NPCWall: number;
    NPCGate: number;
    NPCMoat: number;
    NPCGlory: number;
    CLMelee: number;
    CLRange: number;
    CLFront: number;
    CLFlank: number;
    CLCy: number;
    CLWall: number;
    CLGate: number;
    CLMoat: number;
    CLLater: number;
    CLFire: number;
    CLGlory: number;
    extraStats?: EquipmentExtraStat[];
    effects?: ResolvedEquipmentEffect[];
}

export interface CastStat {
    equip1: number;
    equip2: number;
    equip3: number;
    equip4: number;
    hero: number;
    gem1: number;
    gem2: number;
    gem3: number;
    gem4: number;
    id: number;
    castleID: number;
    name: string;
    castlePosition: number;
    meleeCbtStr: number;
    rangeCbtStr: number;
    opCbtStr: number;
    mainCbtStr: number;
    cyCbtStr: number;
    allCbtStr: number;
    frontCbtStr: number;
    flankCbtStr: number;
    wallStr: number;
    gateStr: number;
    moatStr: number;
    wallLimit: number;
    protectorSupp: number;
    lootStr: number;
    recruit: number;
    meadProd: number;
    research: number;
    hospital: number;
    construction: number;
    baseRes: number;
    kingRes: number;
    po: number;
    resTransport: number;
    honeyProd: number;
    meadStorage: number;
    honeyStorage: number;
    NPCMelee: number;
    NPCRange: number;
    NPCFront: number;
    NPCFlank: number;
    NPCCy: number;
    NPCWall: number;
    NPCGate: number;
    NPCMoat: number;
    NPCWallLimit: number;
    CLMelee: number;
    CLRange: number;
    CLCy: number;
    CLWall: number;
    CLGate: number;
    CLMoat: number;
    CLWallLimit: number;
    CLFire: number;
    CLGlory: number;
    CLEarly: number;
    extraStats?: EquipmentExtraStat[];
    effects?: ResolvedEquipmentEffect[];
}

export interface CastStatObject {
    mainCastleCast: CastStat;
    outpost1Cast: CastStat;
    outpost2Cast: CastStat;
    outpost3Cast: CastStat;
    iceCastleCast: CastStat;
    desertCastleCast: CastStat;
    dungeonCastleCast: CastStat;
    stormCastleCast: CastStat;
    metropolisCastleCast: CastStat;
    capitalCastleCast: CastStat;
    extraCast1: CastStat;
    extraCast2: CastStat;
    extraCast3: CastStat;
    extraCast4: CastStat;
    extraCast5: CastStat;
}

export interface EquipmentData {
    commStats: CommStat[];
    castStats: CastStatObject | null;
}

export type EquipmentMode = 'Commander' | 'Castellan';
export type CombatMode = 'PvP' | 'PvE';

export interface ProcessedEquipmentStatSource {
    key: string;
    label: string;
    value: number;
    cappedValue: number;
    cap?: number;
    capped: boolean;
}

export interface ProcessedEquipmentStat {
    key: string;
    value: number;
    rawValue: number;
    capped: boolean;
    sources: ProcessedEquipmentStatSource[];
}

export type EquipmentEffectScope = 'Always' | 'PvP' | 'PvE';
export type EquipmentEffectRowKind = 'total' | 'parsed' | 'catalog' | 'extra';

export interface EquipmentEffectRow {
    key: string;
    label: string;
    value: number;
    rawValue?: number;
    capped?: boolean;
    cap?: number;
    unit: 'percent' | 'number';
    scope: EquipmentEffectScope;
    kind: EquipmentEffectRowKind;
    sources?: ProcessedEquipmentStatSource[];
    effectId?: number;
    sourceLabel?: string;
    maxTotalBonus?: number;
    sortOrder?: string;
}

export interface EquipmentEffectSection {
    key: string;
    title: string;
    description: string;
    rows: EquipmentEffectRow[];
}

export const statDisplayName: { [key: string]: string } = {
    // CommStat & CastStat
    meleeCbtStr: 'Melee Strength',
    rangeCbtStr: 'Ranged Strength',
    cyCbtStr: 'Courtyard Strength',
    wallStr: 'Wall Reduction',
    gateStr: 'Gate Reduction',
    moatStr: 'Moat Reduction',
    allCbtStr: 'All Combat Strength',
    frontCbtStr: 'Front Combat Strength',
    flankCbtStr: 'Flank Combat Strength',
    loot: 'Resources Plundered When Looting',
    lootStr: 'Resources Lost After Being Looted',



    // CommStat only
    flankLimit: 'Flank Unit Limit',
    frontLimit: 'Front Unit Limit',
    meadStr: 'Mead Unit Attack Strength',
    horrorStr: 'Horror Unit Attack Strength',
    eliteStr: 'Kingsguard/Imperial Unit Attack Strength',
    wave: 'Additional Waves',
    cooldown: 'Cooldown Reduction',
    relicStr: 'Relic Unit Attack Strength',
    beserkerStr: 'Beef/Berserker Unit Attack Strength',
    maidenSupp: 'Maiden Support Bonus',
    attackReinforcement: 'Troop Capacity for Final Assault',
    travel: 'Army Travel Speed',


    // CastStat only
    opCbtStr: 'Outpost Strength Bonus',
    mainCbtStr: 'Main Castle Strength Bonus',
    wallLimit: 'Wall Unit Limit',
    protectorSupp: 'Protector Support Bonus',
    recruit: 'Recruit Speed Boost',
    meadProd: 'Mead Production Bonus',
    research: 'Research Speed Boost',
    hospital: 'Hospital Speed Boost',
    construction: 'Construction Speed Boost',
    baseRes: 'Base Production Bonus',
    kingRes: 'King Production Bonus',
    po: 'Public Order Bonus',
    resTransport: 'Resource Transport Capacity Bonus',
    honeyProd: 'Honey Production Bonus',
    meadStorage: 'Mead Storage Bonus',
    honeyStorage: 'Honey Storage Bonus',


    // Special Keys for combined stats
    glory: 'Glory Bonus',
    later: 'Later Army Detection',
    fire: 'Fire Increase Bonus',
    early: 'Earlier Attack Warning',

    // Castle lord scoped stats
    CLMelee: 'Melee Strength Against Castle Lords',
    CLRange: 'Ranged Strength Against Castle Lords',
    CLFront: 'Front Unit Limit Against Castle Lords',
    CLFlank: 'Flank Unit Limit Against Castle Lords',
    CLCy: 'Courtyard Strength Against Castle Lords',
    CLWall: 'Wall Protection Against Castle Lords',
    CLGate: 'Gate Protection Against Castle Lords',
    CLMoat: 'Moat Protection Against Castle Lords',
    CLWallLimit: 'Wall Unit Limit Against Castle Lords',
    CLLater: 'Later Army Detection Against Castle Lords',
    CLFire: 'Fire Damage Against Castle Lords',
    CLGlory: 'Glory Against Castle Lords',
    CLEarly: 'Earlier Attack Warning Against Castle Lords',

    // NPC scoped stats
    NPCMelee: 'Melee Strength Against NPC Targets',
    NPCRange: 'Ranged Strength Against NPC Targets',
    NPCFront: 'Front Unit Limit Against NPC Targets',
    NPCFlank: 'Flank Unit Limit Against NPC Targets',
    NPCCy: 'Courtyard Strength Against NPC Targets',
    NPCWall: 'Wall Protection Against NPC Targets',
    NPCGate: 'Gate Protection Against NPC Targets',
    NPCMoat: 'Moat Protection Against NPC Targets',
    NPCWallLimit: 'Wall Unit Limit Against NPC Targets',
    NPCGlory: 'Glory Against NPC Targets',
};

export type StatDisplayContext = {
    equipmentMode?: 'Commander' | 'Castellan';
    source?: string;
    combatMode?: CombatMode;
    scope?: EquipmentEffectScope;
};

const commanderPvpStatDisplayName: { [key: string]: string } = {
    frontCbtStr: 'Combat Strength on the Front Against Castle Lords',
    flankCbtStr: 'Combat Strength on the Flanks Against Castle Lords',
    allCbtStr: 'Attack Unit Combat Strength Against Castle Lords',
    meadStr: 'Attack Strength for Mead Units Against Castle Lords',
    horrorStr: 'Attack Strength for Horror Units Against Castle Lords',
    eliteStr: 'Attack Strength for Kingsguard/Imperial Units Against Castle Lords',
    relicStr: 'Attack Strength for Relic Barracks Units Against Castle Lords',
    beserkerStr: 'Attack Strength for Beef/Berserker Units Against Castle Lords',
    wave: 'Additional Waves Against Castle Lords',
    attackReinforcement: 'Troop Capacity for Final Assault Against Castle Lords',
};

const commanderStatDisplayName: { [key: string]: string } = {
    wallStr: 'Wall Reduction',
    gateStr: 'Gate Reduction',
    moatStr: 'Moat Reduction',
    CLWall: 'Wall Reduction Against Castle Lords',
    CLGate: 'Gate Reduction Against Castle Lords',
    CLMoat: 'Moat Reduction Against Castle Lords',
    NPCWall: 'Wall Reduction Against NPC Targets',
    NPCGate: 'Gate Reduction Against NPC Targets',
    NPCMoat: 'Moat Reduction Against NPC Targets',
};

const castellanStatDisplayName: { [key: string]: string } = {
    meleeCbtStr: 'Melee Defense',
    rangeCbtStr: 'Ranged Defense',
    cyCbtStr: 'Courtyard Defense',
    wallStr: 'Wall Protection',
    gateStr: 'Gate Protection',
    moatStr: 'Moat Protection',
    allCbtStr: 'Defense Strength',
    CLMelee: 'Melee Defense Against Castle Lords',
    CLRange: 'Ranged Defense Against Castle Lords',
    CLCy: 'Courtyard Defense Against Castle Lords',
    CLWall: 'Wall Protection Against Castle Lords',
    CLGate: 'Gate Protection Against Castle Lords',
    CLMoat: 'Moat Protection Against Castle Lords',
    NPCMelee: 'Melee Defense Against NPC Targets',
    NPCRange: 'Ranged Defense Against NPC Targets',
    NPCCy: 'Courtyard Defense Against NPC Targets',
    NPCWall: 'Wall Protection Against NPC Targets',
    NPCGate: 'Gate Protection Against NPC Targets',
    NPCMoat: 'Moat Protection Against NPC Targets',
};

export function displayStatName(statKey: string, context: StatDisplayContext = {}): string {
    const mode = context.equipmentMode || displayModeFromSource(context.source);
    const scope = context.scope || context.combatMode;
    if (mode === 'Commander' && scope === 'PvP' && commanderPvpStatDisplayName[statKey]) {
        return commanderPvpStatDisplayName[statKey];
    }
    if (mode === 'Commander' && commanderStatDisplayName[statKey]) {
        return commanderStatDisplayName[statKey];
    }
    if (mode === 'Castellan' && castellanStatDisplayName[statKey]) {
        return castellanStatDisplayName[statKey];
    }
    return statDisplayName[statKey] || humanizeStatKey(statKey);
}

function displayModeFromSource(source?: string): StatDisplayContext['equipmentMode'] {
    const normalized = String(source || '').toLowerCase();
    if (normalized.includes('commander')) {
        return 'Commander';
    }
    if (normalized.includes('castellan')) {
        return 'Castellan';
    }
    return undefined;
}

function humanizeStatKey(statKey: string): string {
    const words = statKey
        .replace(/^CL/, 'Castle Lord ')
        .replace(/^NPC/, 'NPC ')
        .replace(/Cy/g, 'Courtyard')
        .replace(/CbtStr/g, 'Combat Strength')
        .replace(/Str/g, 'Strength')
        .replace(/([a-z])([A-Z])/g, '$1 $2')
        .replace(/\s+/g, ' ')
        .trim();

    return words.charAt(0).toUpperCase() + words.slice(1);
}

export const statGroupDisplayName: { [key: string]: string } = {
    core: 'Core Stats',
    relic2: 'Relic Stats',
    hero: 'Hero Stats',
    defense: 'Defensive Stats',
    specialStats: 'PvP Only Stats',
    miscellaneous: 'Miscellaneous',
    economy: 'Economy Stats',
};

const effectSectionOrder = ['effective', 'combat', 'capacity', 'fortification', 'special', 'mobility', 'economy', 'other'];

const effectSectionMeta: Record<string, { title: string; description: string }> = {
    effective: {
        title: 'Effective Totals',
        description: 'Current mode totals after generic, PvP/PvE, all-unit, and caps are combined.',
    },
    combat: {
        title: 'Combat Effects',
        description: 'Direct strength effects parsed from the selected loadout.',
    },
    capacity: {
        title: 'Capacity Effects',
        description: 'Unit limit, waves, support, and reinforcement effects.',
    },
    fortification: {
        title: 'Wall, Gate, Moat, and Warning Effects',
        description: 'Attack/defense obstacle, detection, warning, and fire effects.',
    },
    special: {
        title: 'Special and Event Effects',
        description: 'Targeted, event, and named unit effects.',
    },
    mobility: {
        title: 'Mobility and Cooldown Effects',
        description: 'Travel speed and cooldown effects.',
    },
    economy: {
        title: 'Economy Effects',
        description: 'Production, storage, loot, construction, research, and recruitment effects.',
    },
    other: {
        title: 'Other Resolved Effects',
        description: 'Known effects that do not fit a standard battle bucket yet.',
    },
};

export const commanderStatGroups = {
    core: ['meleeCbtStr', 'rangeCbtStr', 'cyCbtStr', 'wallStr', 'gateStr', 'moatStr', 'flankLimit', 'frontLimit', 'wave', 'maidenSupp'],
    relic2: ['frontCbtStr', 'flankCbtStr', 'allCbtStr'],
    hero: ['meadStr', 'horrorStr', 'eliteStr', 'relicStr', 'beserkerStr', 'cooldown'],
    specialStats: ['glory', 'later', 'fire', 'attackReinforcement'],
    miscellaneous: ['travel', 'loot'],
};

export const castellanStatGroups = {
    core: ['meleeCbtStr', 'rangeCbtStr', 'cyCbtStr', 'wallStr', 'gateStr', 'moatStr', 'wallLimit'],
    relic2: ['frontCbtStr', 'flankCbtStr', 'allCbtStr'],
    defense: ['protectorSupp', 'opCbtStr', 'mainCbtStr'],
    economy: ['lootStr', 'recruit', 'meadProd', 'research', 'hospital', 'construction', 'baseRes', 'kingRes', 'po', 'resTransport', 'honeyProd', 'meadStorage', 'honeyStorage'],
    specialStats: ['glory', 'fire', 'early'],
};

const identityAndSlotKeys = new Set([
    'id',
    'name',
    'castleID',
    'castlePosition',
    'equip1',
    'equip2',
    'equip3',
    'equip4',
    'hero',
    'gem1',
    'gem2',
    'gem3',
    'gem4',
    'extraStats',
    'effects',
]);

const effectiveTotalKeysByMode: Record<EquipmentMode, string[]> = {
    Commander: [
        'meleeCbtStr',
        'rangeCbtStr',
        'cyCbtStr',
        'wallStr',
        'gateStr',
        'moatStr',
        'frontLimit',
        'flankLimit',
        'frontCbtStr',
        'flankCbtStr',
        'allCbtStr',
        'wave',
        'glory',
        'later',
        'fire',
        'maidenSupp',
        'attackReinforcement',
        'meadStr',
        'horrorStr',
        'eliteStr',
        'relicStr',
        'beserkerStr',
        'cooldown',
        'travel',
        'loot',
    ],
    Castellan: [
        'meleeCbtStr',
        'rangeCbtStr',
        'cyCbtStr',
        'wallStr',
        'gateStr',
        'moatStr',
        'wallLimit',
        'frontCbtStr',
        'flankCbtStr',
        'allCbtStr',
        'glory',
        'fire',
        'early',
        'protectorSupp',
        'opCbtStr',
        'mainCbtStr',
        'lootStr',
        'recruit',
        'meadProd',
        'research',
        'hospital',
        'construction',
        'baseRes',
        'kingRes',
        'po',
        'resTransport',
        'honeyProd',
        'meadStorage',
        'honeyStorage',
    ],
};

const rawStatPriority: Record<string, number> = {
    meleeCbtStr: 10,
    rangeCbtStr: 11,
    cyCbtStr: 12,
    allCbtStr: 13,
    frontCbtStr: 14,
    flankCbtStr: 15,
    CLMelee: 20,
    CLRange: 21,
    CLCy: 22,
    NPCMelee: 30,
    NPCRange: 31,
    NPCCy: 32,
    wallStr: 40,
    gateStr: 41,
    moatStr: 42,
    CLWall: 43,
    CLGate: 44,
    CLMoat: 45,
    NPCWall: 46,
    NPCGate: 47,
    NPCMoat: 48,
    frontLimit: 60,
    flankLimit: 61,
    wallLimit: 62,
    CLFront: 63,
    CLFlank: 64,
    CLWallLimit: 65,
    NPCFront: 66,
    NPCFlank: 67,
    NPCWallLimit: 68,
    wave: 69,
    maidenSupp: 70,
    protectorSupp: 71,
    attackReinforcement: 72,
};

const combatStatMap: Record<string, Partial<Record<CombatMode, string>>> = {
    meleeCbtStr: { PvP: 'CLMelee', PvE: 'NPCMelee' },
    rangeCbtStr: { PvP: 'CLRange', PvE: 'NPCRange' },
    frontLimit: { PvP: 'CLFront', PvE: 'NPCFront' },
    flankLimit: { PvP: 'CLFlank', PvE: 'NPCFlank' },
    cyCbtStr: { PvP: 'CLCy', PvE: 'NPCCy' },
    wallStr: { PvP: 'CLWall', PvE: 'NPCWall' },
    gateStr: { PvP: 'CLGate', PvE: 'NPCGate' },
    moatStr: { PvP: 'CLMoat', PvE: 'NPCMoat' },
    wallLimit: { PvP: 'CLWallLimit', PvE: 'NPCWallLimit' },
};

const specialStatMap: Record<EquipmentMode, Record<string, Partial<Record<CombatMode, string>>>> = {
    Commander: {
        glory: { PvP: 'CLGlory', PvE: 'NPCGlory' },
        later: { PvP: 'CLLater' },
        fire: { PvP: 'CLFire' },
        attackReinforcement: { PvP: 'attackReinforcement' },
    },
    Castellan: {
        glory: { PvP: 'CLGlory' },
        fire: { PvP: 'CLFire' },
        early: { PvP: 'CLEarly' },
    },
};

const commanderCaps: Record<string, number> = {
    meleeCbtStr: 210,
    rangeCbtStr: 210,
    frontCbtStr: 100,
    flankCbtStr: 100,
    allCbtStr: 30,
    cyCbtStr: 180,
    wallStr: 160,
    gateStr: 160,
    moatStr: 120,
    flankLimit: 120,
    frontLimit: 120,
    meadStr: 20,
    horrorStr: 50,
    eliteStr: 50,
    wave: 1,
    cooldown: 50,
    relicStr: 50,
    beserkerStr: 50,
    maidenSupp: 1050,
    attackReinforcement: 1500,
    travel: 180,
    loot: 155,
    NPCMelee: 100,
    NPCRange: 100,
    NPCFront: 80,
    NPCFlank: 80,
    NPCCy: 120,
    NPCWall: 120,
    NPCGate: 120,
    NPCMoat: 60,
    NPCGlory: 150,
    CLMelee: 360,
    CLRange: 360,
    CLFront: 195,
    CLFlank: 195,
    CLCy: 330,
    CLWall: 420,
    CLGate: 420,
    CLMoat: 270,
    CLLater: 180,
    CLFire: 100,
    CLGlory: 230,
};

const castellanCaps: Record<string, number> = {
    meleeCbtStr: 140,
    rangeCbtStr: 140,
    opCbtStr: 50,
    mainCbtStr: 50,
    cyCbtStr: 230,
    allCbtStr: 45,
    frontCbtStr: 20,
    flankCbtStr: 20,
    wallStr: 160,
    gateStr: 160,
    moatStr: 120,
    wallLimit: 210,
    protectorSupp: 1050,
    lootStr: 50,
    recruit: 50,
    meadProd: 50,
    research: 50,
    hospital: 50,
    construction: 50,
    baseRes: 50,
    kingRes: 50,
    po: 50,
    resTransport: 50,
    honeyProd: 50,
    meadStorage: 50,
    honeyStorage: 50,
    NPCMelee: 100,
    NPCRange: 100,
    NPCFront: 50,
    NPCFlank: 50,
    NPCCy: 120,
    NPCWall: 120,
    NPCGate: 120,
    NPCMoat: 60,
    NPCWallLimit: 100,
    CLMelee: 100,
    CLRange: 100,
    CLCy: 120,
    CLWall: 120,
    CLGate: 120,
    CLMoat: 60,
    CLWallLimit: 100,
    CLFire: 100,
    CLGlory: 60,
    CLEarly: 180,
};

const allUnitStrengthTargets = new Set([
    'meleeCbtStr',
    'rangeCbtStr',
    'frontCbtStr',
    'flankCbtStr',
]);

const sourceLabelByKey: Record<string, string> = {
    allCbtStr: 'All unit strength',
    CLMelee: 'Castle Lord melee',
    CLRange: 'Castle Lord ranged',
    CLFront: 'Castle Lord front limit',
    CLFlank: 'Castle Lord flank limit',
    CLCy: 'Castle Lord courtyard',
    CLWall: 'Castle Lord wall',
    CLGate: 'Castle Lord gate',
    CLMoat: 'Castle Lord moat',
    CLWallLimit: 'Castle Lord wall limit',
    CLLater: 'Castle Lord detection',
    CLFire: 'Castle Lord fire',
    CLGlory: 'Castle Lord glory',
    CLEarly: 'Castle Lord warning',
    NPCMelee: 'NPC melee',
    NPCRange: 'NPC ranged',
    NPCFront: 'NPC front limit',
    NPCFlank: 'NPC flank limit',
    NPCCy: 'NPC courtyard',
    NPCWall: 'NPC wall',
    NPCGate: 'NPC gate',
    NPCMoat: 'NPC moat',
    NPCWallLimit: 'NPC wall limit',
    NPCGlory: 'NPC glory',
};

export function processEquipmentStats(
    stats: CommStat | CastStat,
    combatMode: CombatMode,
    equipmentMode: EquipmentMode
): Record<string, ProcessedEquipmentStat> {
    const processed: Record<string, ProcessedEquipmentStat> = {};
    const keys = effectiveTotalKeysByMode[equipmentMode];

    keys.forEach((key) => {
        processed[key] = processEquipmentStat(stats, key, combatMode, equipmentMode);
		});

    return processed;
}

export function buildEquipmentEffectSections(
    stats: CommStat | CastStat,
    processedStats: Record<string, ProcessedEquipmentStat>,
    combatMode: CombatMode,
    equipmentMode: EquipmentMode
): EquipmentEffectSection[] {
    const grouped = new Map<string, EquipmentEffectRow[]>();

    const addRow = (sectionKey: string, row: EquipmentEffectRow) => {
        const rows = grouped.get(sectionKey) ?? [];
        rows.push(row);
        grouped.set(sectionKey, rows);
    };

    for (const key of effectiveTotalKeysByMode[equipmentMode]) {
        const stat = processedStats[key];
        if (!stat || stat.value === 0) continue;
        addRow('effective', {
            key,
            label: displayStatName(key, { equipmentMode, combatMode }),
            value: stat.value,
            rawValue: stat.rawValue,
            capped: stat.capped,
            unit: statUnit(key),
            scope: combatMode,
            kind: 'total',
            sources: stat.sources,
        });
    }

    const catalogEffects = stats.effects ?? [];
    if (catalogEffects.length > 0) {
        for (const effect of catalogEffects) {
            const value = Number(effect.value);
            if (!Number.isFinite(value) || value === 0) continue;
            const scope = scopeForCatalogEffect(effect.scope);
            addRow(sectionForCatalogEffect(effect.category, effect.label), {
                key: `catalog:${effect.key}`,
                label: effect.label || effect.name || `Effect ${effect.effectId}`,
                value: roundStat(value),
                unit: effect.unit === 'number' ? 'number' : 'percent',
                scope,
                kind: 'catalog',
                effectId: effect.effectId,
                sourceLabel: catalogEffectSourceLabel(effect.source, effect.argumentLabel),
                maxTotalBonus: effect.maxTotalBonus,
                sortOrder: effect.sortOrder,
            });
        }
    } else {
        for (const [key, raw] of Object.entries(stats as Record<string, unknown>)) {
            if (!isRawStatKey(key)) continue;
            const value = Number(raw);
            if (!Number.isFinite(value) || value === 0) continue;
            const scope = scopeForRawStatKey(key);
            addRow(sectionForRawStatKey(key), {
                key,
                label: displayStatName(key, { equipmentMode, scope }),
                value: roundStat(value),
                unit: statUnit(key),
                scope,
                kind: 'parsed',
            });
        }

        for (const extra of stats.extraStats ?? []) {
            const value = Number(extra.value);
            if (!Number.isFinite(value) || value === 0) continue;
            addRow(sectionForExtraStat(extra), {
                key: `extra:${extra.key || extra.effectId}`,
                label: extra.label || extra.name || extra.category || 'Unmapped effect',
                value: roundStat(value),
                unit: extra.unit === 'number' ? 'number' : 'percent',
                scope: scopeForExtraStat(extra),
                kind: 'extra',
                effectId: extra.effectId,
                sourceLabel: extra.category,
            });
        }
    }

    return effectSectionOrder
        .filter(sectionKey => grouped.has(sectionKey))
        .map((sectionKey) => {
            const meta = effectSectionMeta[sectionKey] ?? effectSectionMeta.other;
            const rows = grouped.get(sectionKey) ?? [];
            rows.sort(compareEffectRows);
            return {
                key: sectionKey,
                title: meta.title,
                description: meta.description,
                rows,
            };
        });
}

export function getEquipmentShowcaseStats(
    processedStats: Record<string, ProcessedEquipmentStat>,
    equipmentMode: EquipmentMode
): ProcessedEquipmentStat[] {
    const keys = equipmentMode === 'Commander'
        ? ['meleeCbtStr', 'rangeCbtStr', 'cyCbtStr', 'frontLimit', 'flankLimit', 'wallStr', 'gateStr', 'moatStr', 'travel', 'glory']
        : ['meleeCbtStr', 'rangeCbtStr', 'cyCbtStr', 'wallLimit', 'wallStr', 'gateStr', 'moatStr', 'fire', 'early', 'lootStr'];

    return keys
        .map(key => processedStats[key])
        .filter((stat): stat is ProcessedEquipmentStat => !!stat && stat.value !== 0);
}

export function getEquipmentShowcaseRows(sections: EquipmentEffectSection[]): EquipmentEffectRow[] {
    const effective = sections.find(section => section.key === 'effective');
    if (!effective) return [];
    return effective.rows;
}

export function formatEquipmentStatValue(statKey: string, value: number): string {
    const abs = Math.abs(value);
    const precision = Number.isInteger(abs) ? 0 : 1;
    const formatted = abs.toLocaleString(undefined, {
        minimumFractionDigits: precision,
        maximumFractionDigits: precision,
    });
    const sign = value > 0 ? '+' : value < 0 ? '-' : '';
    const unit = numericStatKeys.has(statKey) ? '' : '%';
    return `${sign}${formatted}${unit}`;
}

function processEquipmentStat(
    stats: CommStat | CastStat,
    statKey: string,
    combatMode: CombatMode,
    equipmentMode: EquipmentMode
): ProcessedEquipmentStat {
    const sourceKeys = equipmentStatSourceKeys(statKey, combatMode, equipmentMode);
    const sources = sourceKeys.map((sourceKey) => {
        const value = readStatValue(stats, sourceKey);
        const cap = capForStat(sourceKey, equipmentMode);
        const cappedValue = capValue(value, cap);
        return {
            key: sourceKey,
            label: sourceLabel(sourceKey, equipmentMode, combatMode),
            value,
            cappedValue,
            cap,
            capped: value !== cappedValue,
        };
    }).filter(source => source.value !== 0);

    const rawValue = roundStat(sources.reduce((total, source) => total + source.value, 0));
    const value = roundStat(sources.reduce((total, source) => total + source.cappedValue, 0));

    return {
        key: statKey,
        value,
        rawValue,
        capped: sources.some(source => source.capped),
        sources,
    };
}

function equipmentStatSourceKeys(
    statKey: string,
    combatMode: CombatMode,
    equipmentMode: EquipmentMode
): string[] {
    const specialSource = specialStatMap[equipmentMode][statKey]?.[combatMode];
    if (specialSource) {
        return [specialSource];
    }
    if (statKey in specialStatMap[equipmentMode]) {
        return [];
    }

    const sources = [statKey];
    if (equipmentMode === 'Commander' && allUnitStrengthTargets.has(statKey)) {
        sources.push('allCbtStr');
    }

    const combatSource = combatStatMap[statKey]?.[combatMode];
    if (combatSource) {
        sources.push(combatSource);
    }

    return sources;
}

function isRawStatKey(key: string): boolean {
    if (identityAndSlotKeys.has(key)) {
        return false;
    }
    if (key.startsWith('placeHolder')) {
        return false;
    }
    return true;
}

function scopeForRawStatKey(key: string): EquipmentEffectScope {
    if (key.startsWith('CL')) {
        return 'PvP';
    }
    if (key.startsWith('NPC')) {
        return 'PvE';
    }
    return 'Always';
}

function scopeForExtraStat(stat: EquipmentExtraStat): EquipmentEffectScope {
    const text = `${stat.name || ''} ${stat.label || ''} ${stat.category || ''}`.toLowerCase();
    if (text.includes('pvp') || text.includes('castle lord')) {
        return 'PvP';
    }
    if (text.includes('pve') || text.includes('npc') || text.includes('nomad') || text.includes('samurai') || text.includes('khan') || text.includes('daimyo') || text.includes('berimond') || text.includes('bloodcrow')) {
        return 'PvE';
    }
    return 'Always';
}

function scopeForCatalogEffect(scope: 'generic' | 'pvp' | 'pve'): EquipmentEffectScope {
    if (scope === 'pvp') return 'PvP';
    if (scope === 'pve') return 'PvE';
    return 'Always';
}

function catalogEffectSourceLabel(source: ResolvedEquipmentEffect['source'], argumentLabel?: string): string {
    const sourceLabel: Record<ResolvedEquipmentEffect['source'], string> = {
        equipment: 'Equipment',
        relic_equipment: 'Relic equipment',
        gem: 'Gem',
        relic_gem: 'Relic gem',
        set_bonus: 'Set bonus',
    };
    return argumentLabel ? `${sourceLabel[source]} · ${argumentLabel}` : sourceLabel[source];
}

function sectionForCatalogEffect(category: string, label: string): string {
    const normalized = String(category || '').toLowerCase();
    const text = String(label || '').toLowerCase();
    if (normalized.includes('economy')) return 'economy';
    if (normalized.includes('capacity')) return 'capacity';
    if (normalized.includes('wall') || normalized.includes('gate') || normalized.includes('moat')) return 'fortification';
    if (normalized.includes('unit')) return 'combat';
    if (normalized.includes('event')) return 'special';
    if (isEconomyText(text)) return 'economy';
    if (isCapacityText(text)) return 'capacity';
    if (isFortificationText(text)) return 'fortification';
    if (isMobilityText(text)) return 'mobility';
    if (isSpecialText(text)) return 'special';
    return 'other';
}

function sectionForRawStatKey(key: string): string {
    if (isEconomyStatKey(key)) return 'economy';
    if (isMobilityStatKey(key)) return 'mobility';
    if (isCapacityStatKey(key)) return 'capacity';
    if (isFortificationStatKey(key)) return 'fortification';
    if (isSpecialStatKey(key)) return 'special';
    if (isCombatStatKey(key)) return 'combat';
    return 'other';
}

function sectionForExtraStat(stat: EquipmentExtraStat): string {
    const category = String(stat.category || '').toLowerCase();
    const text = `${stat.name || ''} ${stat.label || ''}`.toLowerCase();
    if (category.includes('economy')) return 'economy';
    if (category.includes('capacity')) return 'capacity';
    if (category.includes('wall') || category.includes('gate') || category.includes('moat')) return 'fortification';
    if (category.includes('unit')) return 'combat';
    if (category.includes('event') || isSpecialText(text)) return 'special';
    if (isEconomyText(text)) return 'economy';
    if (isCapacityText(text)) return 'capacity';
    if (isFortificationText(text)) return 'fortification';
    if (isMobilityText(text)) return 'mobility';
    if (isSpecialText(text)) return 'special';
    return 'other';
}

function isCombatStatKey(key: string): boolean {
    const normalized = key.toLowerCase();
    return /melee|range|cbtstr|cy/.test(normalized) || ['allcbtstr', 'meadstr', 'horrorstr', 'elitestr', 'relicstr', 'beserkerstr'].includes(normalized);
}

function isCapacityStatKey(key: string): boolean {
    const normalized = key.toLowerCase();
    return /limit|supp|reinforcement/.test(normalized) || ['wave', 'protectorsupp', 'maidensupp'].includes(normalized);
}

function isFortificationStatKey(key: string): boolean {
    return /wall|gate|moat|fire|later|early/i.test(key);
}

function isMobilityStatKey(key: string): boolean {
    return ['travel', 'cooldown'].includes(key);
}

function isEconomyStatKey(key: string): boolean {
    return /loot|recruit|prod|research|hospital|construction|res|storage|po/i.test(key);
}

function isSpecialStatKey(key: string): boolean {
    return /glory/i.test(key);
}

function isEconomyText(text: string): boolean {
    return /loot|resource|production|storage|recruit|research|hospital|construction|public order|transport/.test(text);
}

function isCapacityText(text: string): boolean {
    return /limit|capacity|support|wave|reinforcement/.test(text);
}

function isFortificationText(text: string): boolean {
    return /wall|gate|moat|fire|warning|detection/.test(text);
}

function isMobilityText(text: string): boolean {
    return /travel|speed|cooldown/.test(text);
}

function isSpecialText(text: string): boolean {
    return /glory|nomad|samurai|khan|daimyo|berimond|bloodcrow|event/.test(text);
}

function statUnit(key: string): 'percent' | 'number' {
    return numericStatKeys.has(key) ? 'number' : 'percent';
}

function compareEffectRows(a: EquipmentEffectRow, b: EquipmentEffectRow): number {
    const scopeOrder: Record<EquipmentEffectScope, number> = { Always: 0, PvP: 1, PvE: 2 };
    if (a.sortOrder || b.sortOrder) {
        const sortOrder = compareCatalogSortOrder(a.sortOrder, b.sortOrder);
        if (sortOrder !== 0) return sortOrder;
    }
    const priorityA = rawStatPriority[a.key] ?? 1000;
    const priorityB = rawStatPriority[b.key] ?? 1000;
    if (priorityA !== priorityB) return priorityA - priorityB;
    if (scopeOrder[a.scope] !== scopeOrder[b.scope]) return scopeOrder[a.scope] - scopeOrder[b.scope];
    return a.label.localeCompare(b.label);
}

function compareCatalogSortOrder(left = '', right = ''): number {
    const leftParts = left.split('.').map(Number);
    const rightParts = right.split('.').map(Number);
    const length = Math.max(leftParts.length, rightParts.length);
    for (let index = 0; index < length; index += 1) {
        const leftValue = Number.isFinite(leftParts[index]) ? leftParts[index] : 0;
        const rightValue = Number.isFinite(rightParts[index]) ? rightParts[index] : 0;
        if (leftValue !== rightValue) return leftValue - rightValue;
    }
    return 0;
}

function readStatValue(stats: CommStat | CastStat, key: string): number {
    const value = (stats as any)[key];
    return Number.isFinite(Number(value)) ? Number(value) : 0;
}

function capForStat(key: string, equipmentMode: EquipmentMode): number | undefined {
    const caps = equipmentMode === 'Commander' ? commanderCaps : castellanCaps;
    return caps[key] > 0 ? caps[key] : undefined;
}

function capValue(value: number, cap?: number): number {
    if (!cap || cap <= 0) {
        return value;
    }
    return Math.min(Math.abs(value), cap) * Math.sign(value);
}

function sourceLabel(key: string, equipmentMode: EquipmentMode, combatMode?: CombatMode): string {
    return sourceLabelByKey[key] || displayStatName(key, { equipmentMode, combatMode });
}

function roundStat(value: number): number {
    return Math.round(value * 10) / 10;
}

const numericStatKeys = new Set([
    'wave',
    'maidenSupp',
    'protectorSupp',
    'attackReinforcement',
]);

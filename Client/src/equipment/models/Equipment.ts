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
    placeHolder8: number;
    equipLevel: number;
    placeHolder9: number;
    gemId: number;
    placeHolder11: number;
    gem?: any; // Define GemSlot type if available
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
    meadStr: 'Mead Strength Bonus',
    horrorStr: 'Horror Strength Bonus',
    eliteStr: 'Elite Strength Bonus',
    wave: 'Additional Waves',
    cooldown: 'Cooldown Reduction',
    relicStr: 'Relic Strength Bonus',
    beserkerStr: 'Beserker Strength Bonus',
    maidenSupp: 'Maiden Support Bonus',
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

export const commanderStatGroups = {
    core: ['meleeCbtStr', 'rangeCbtStr', 'cyCbtStr', 'wallStr', 'gateStr', 'moatStr', 'flankLimit', 'frontLimit', 'wave', 'maidenSupp'],
    relic2: ['frontCbtStr', 'flankCbtStr', 'allCbtStr'],
    hero: ['meadStr', 'horrorStr', 'eliteStr', 'relicStr', 'beserkerStr', 'cooldown'],
    specialStats: ['glory', 'later', 'fire'],
    miscellaneous: ['travel', 'loot'],
};

export const castellanStatGroups = {
    core: ['meleeCbtStr', 'rangeCbtStr', 'cyCbtStr', 'wallStr', 'gateStr', 'moatStr', 'wallLimit'],
    relic2: ['frontCbtStr', 'flankCbtStr', 'allCbtStr'],
    defense: ['protectorSupp', 'opCbtStr', 'mainCbtStr'],
    economy: ['lootStr', 'recruit', 'meadProd', 'research', 'hospital', 'construction', 'baseRes', 'kingRes', 'po', 'resTransport', 'honeyProd', 'meadStorage', 'honeyStorage'],
    specialStats: ['glory', 'fire', 'early'],
};

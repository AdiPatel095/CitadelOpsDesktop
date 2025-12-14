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
    placeHolder7: number;
    placeHolder8: number;
    equipLevel: number;
    placeHolder9: number;
    placeHolder10: number;
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
    loot: 'Loot Capacity Bonus',
    lootStr: 'Loot Lost Reduction Bonus',



    // CommStat only
    flankLimit: 'Flank Limit Bonus',
    frontLimit: 'Front Limit Bonus',
    meadStr: 'Mead Strength Bonus',
    horrorStr: 'Horror Strength Bonus',
    eliteStr: 'Elite Strength Bonus',
    wave: 'Wave Bonus',
    cooldown: 'Cooldown Reduction',
    relicStr: 'Relic Strength Bonus',
    beserkerStr: 'Beserker Strength Bonus',
    maidenSupp: 'Maiden Support Bonus',
    travel: 'Travel Speed Bonus',


    // CastStat only
    opCbtStr: 'Outpost Strength Bonus',
    mainCbtStr: 'Main Castle Strength Bonus',
    wallLimit: 'Wall Limit Bonus',
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
    later: 'Later Detect Bonus',
    fire: 'Fire Increase Bonus',
    early: 'Early Detect Bonus',
};

export const statGroupDisplayName: { [key: string]: string } = {
    core: 'Core Stats',
    relic2: 'Relic Stats',
    hero: 'Hero Stats',
    specialStats: 'PvP Only Stats',
    miscellaneous: 'Miscellaneous',
    economy: 'Economy Stats',
    special: 'Special Stats',
};

export const commanderStatGroups = {
    core: ['meleeCbtStr', 'rangeCbtStr', 'cyCbtStr', 'wallStr', 'gateStr', 'moatStr', 'flankLimit', 'frontLimit', 'wave', 'maidenSupp'],
    relic2: ['frontCbtStr', 'flankCbtStr', 'allCbtStr'],
    hero: ['meadStr', 'horrorStr', 'eliteStr', 'relicStr', 'beserkerStr', 'cooldown'],
    specialStats: ['glory', 'later', 'fire'],
    miscellaneous: ['travel', 'loot'],
};

export const castellanStatGroups = {
    core: ['meleeCbtStr', 'rangeCbtStr', 'opCbtStr', 'mainCbtStr', 'cyCbtStr', 'allCbtStr', 'frontCbtStr', 'flankCbtStr', 'wallStr', 'gateStr', 'moatStr', 'wallLimit', 'protectorSupp'],
    economy: ['lootStr', 'recruit', 'meadProd', 'research', 'hospital', 'construction', 'baseRes', 'kingRes', 'po', 'resTransport', 'honeyProd', 'meadStorage', 'honeyStorage'],
    special: ['glory', 'fire', 'early'],
};

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
    meleeCbtStr: 'Melee Combat Strength',
    rangeCbtStr: 'Range Combat Strength',
    cyCbtStr: 'Courtyard Combat Strength',
    wallStr: 'Wall Strength',
    gateStr: 'Gate Strength',
    moatStr: 'Moat Strength',
    allCbtStr: 'All Combat Strength',
    frontCbtStr: 'Frontline Combat Strength',
    flankCbtStr: 'Flank Combat Strength',
    lootStr: 'Loot Strength',
    NPCMelee: 'NPC Melee',
    NPCRange: 'NPC Range',
    NPCFront: 'NPC Frontline',
    NPCFlank: 'NPC Flank',
    NPCCy: 'NPC Courtyard',
    NPCWall: 'NPC Wall',
    NPCGate: 'NPC Gate',
    NPCMoat: 'NPC Moat',
    CLMelee: 'CL Melee',
    CLRange: 'CL Range',
    CLCy: 'CL Courtyard',
    CLWall: 'CL Wall',
    CLGate: 'CL Gate',
    CLMoat: 'CL Moat',
    CLFire: 'CL Fire',
    CLGlory: 'CL Glory',

    // CommStat only
    flankLimit: 'Flank Limit',
    frontLimit: 'Frontline Limit',
    meadStr: 'Mead Strength',
    horrorStr: 'Horror Strength',
    eliteStr: 'Elite Strength',
    wave: 'Wave',
    cooldown: 'Cooldown',
    relicStr: 'Relic Strength',
    beserkerStr: 'Beserker Strength',
    maidenSupp: 'Maiden Support',
    travel: 'Travel Speed',
    NPCGlory: 'NPC Glory',
    CLFront: 'CL Frontline',
    CLFlank: 'CL Flank',
    CLLater: 'CL Later',

    // CastStat only
    opCbtStr: 'Outpost Combat Strength',
    mainCbtStr: 'Main Combat Strength',
    wallLimit: 'Wall Limit',
    protectorSupp: 'Protector Support',
    recruit: 'Recruit Speed',
    meadProd: 'Mead Production',
    research: 'Research Speed',
    hospital: 'Hospital Capacity',
    construction: 'Construction Speed',
    baseRes: 'Base Resource Production',
    kingRes: 'King Resource Production',
    po: 'PO',
    resTransport: 'Resource Transport',
    honeyProd: 'Honey Production',
    meadStorage: 'Mead Storage',
    honeyStorage: 'Honey Storage', // Note: lootStr is in both, but has different display names. This might need review.
    NPCWallLimit: 'NPC Wall Limit',
    CLWallLimit: 'CL Wall Limit',
    CLEarly: 'CL Early',

    // Special Keys for combined stats
    glory: 'Glory',
    later: 'Later',
    fire: 'Fire',
    early: 'Early',
};

export const commanderStatGroups = {
  core: ['meleeCbtStr', 'rangeCbtStr', 'cyCbtStr', 'wallStr', 'gateStr', 'moatStr', 'flankLimit', 'frontLimit'],
  relic2: ['frontCbtStr', 'flankCbtStr', 'allCbtStr', 'maidenSupp'],
  hero: ['meadStr', 'horrorStr', 'eliteStr', 'relicStr', 'beserkerStr', 'wave', 'cooldown'],
  specialStats: ['glory', 'later', 'fire'],
  miscellaneous: ['travel', 'loot'],
};

export const castellanStatGroups = {
  core: ['meleeCbtStr', 'rangeCbtStr', 'opCbtStr', 'mainCbtStr', 'cyCbtStr', 'allCbtStr', 'frontCbtStr', 'flankCbtStr', 'wallStr', 'gateStr', 'moatStr', 'wallLimit', 'protectorSupp'],
  economy: ['lootStr', 'recruit', 'meadProd', 'research', 'hospital', 'construction', 'baseRes', 'kingRes', 'po', 'resTransport', 'honeyProd', 'meadStorage', 'honeyStorage'],
  special: ['glory', 'fire', 'early'],
};

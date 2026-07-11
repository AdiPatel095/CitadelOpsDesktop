type MockMessage = {
  type: string;
  payload?: unknown;
  optionalData?: string;
};

type EmitMockMessage = (message: MockMessage) => void;
type JsonRecord = Record<string, unknown>;

type CastleSlotKey =
  | 'mainCastle'
  | 'outpost1'
  | 'outpost2'
  | 'outpost3'
  | 'iceCastle'
  | 'desertCastle'
  | 'dungeonCastle'
  | 'stormCastle'
  | 'metropolis'
  | 'capital';

type PlayerCastleOption = {
  aid: number;
  kingdomID: number;
  name: string;
  mapX: number;
  mapY: number;
};

type MockCastleInfo = {
  castleName: string;
  aid: number;
  mapKingdomID?: number;
  mapX?: number;
  mapY?: number;
  amount: Record<string, number>;
  production: Record<string, number>;
  storage: Record<string, number>;
  troops: {
    kingdomID: number;
    x: number;
    y: number;
    troopsI: Record<string, number>;
    troopsTU: Record<string, number>;
    troopsHI: Record<string, number>;
    troopsSHI: Record<string, number>;
    troopsMixed: Record<string, number>;
  };
  bgRows: unknown[];
  bdRows: unknown[];
};

type MockState = {
  gbd: JsonRecord;
  globalResources: Record<string, number>;
  castleSlots: Record<CastleSlotKey, MockCastleInfo>;
  playerCastles: PlayerCastleOption[];
  castleList: Array<{ id: number; name: string; type: string }>;
  castleByAid: Map<number, { key: CastleSlotKey; castle: MockCastleInfo; option: PlayerCastleOption }>;
  focused: PlayerCastleOption | null;
  alliance: JsonRecord;
  commStats: JsonRecord[];
  castStats: JsonRecord[];
  schedulerSettings: JsonRecord;
};

const castleSlotOrder: CastleSlotKey[] = [
  'mainCastle',
  'outpost1',
  'outpost2',
  'outpost3',
  'iceCastle',
  'desertCastle',
  'dungeonCastle',
  'stormCastle',
  'metropolis',
  'capital',
];

const castleTypeBySlot: Record<CastleSlotKey, string> = {
  mainCastle: 'Main',
  outpost1: 'Outpost',
  outpost2: 'Outpost',
  outpost3: 'Outpost',
  iceCastle: 'Ice',
  desertCastle: 'Desert',
  dungeonCastle: 'Dungeon',
  stormCastle: 'Storm',
  metropolis: 'Metropolis',
  capital: 'Capital',
};

const castleResourceOptionalData: Record<CastleSlotKey, string> = {
  mainCastle: 'mainCastle',
  outpost1: 'outpost1',
  outpost2: 'outpost2',
  outpost3: 'outpost3',
  iceCastle: 'iceCastle',
  desertCastle: 'desertCastle',
  dungeonCastle: 'dungeonCastle',
  stormCastle: 'stormCastle',
  metropolis: 'metropolisCastle',
  capital: 'capitalCastle',
};

const globalSceKeyMap: Record<string, string> = {
  STP: 'sceat',
  IDCT: 'ducat',
  LM: 'const_token',
  LT: 'upgr_token',
  SLWT: 'affl_tix',
  PL: 'plaster',
  DST: 'drg_scale',
  DSS: 'drg_spl',
  RF: 'relic_shard',
  MS1: 'min1',
  MS2: 'min5',
  MS3: 'min10',
  MS4: 'min30',
  MS5: 'hr1',
  MS6: 'hr5',
  MS7: 'hr24',
  PTT: 'ptt',
};

const emptyGlobalResources: Record<string, number> = {
  rubies: 0,
  coins: 0,
  ptt: 0,
  relic_shard: 0,
  sceat: 0,
  ducat: 0,
  const_token: 0,
  upgr_token: 0,
  affl_tix: 0,
  plaster: 0,
  drg_scale: 0,
  drg_spl: 0,
  min1: 0,
  min5: 0,
  min10: 0,
  min30: 0,
  hr1: 0,
  hr5: 0,
  hr24: 0,
  might_pt: 0,
  glory_pt: 0,
  gallan_pt: 0,
};

const amountKeys = {
  W: 'wood_amount',
  S: 'stone_amount',
  F: 'food_amount',
  C: 'coal_amount',
  O: 'oil_amount',
  G: 'glass_amount',
  I: 'iron_amount',
  HONEY: 'honey_amount',
  MEAD: 'mead_amount',
  BEEF: 'beef_amount',
} as const;

const productionKeys = {
  W: 'wood_prod',
  S: 'stone_prod',
  F: 'food_prod',
  C: 'coal_prod',
  O: 'oil_prod',
  G: 'glass_prod',
  I: 'iron_prod',
  HONEY: 'honey_prod',
  MEAD: 'mead_prod',
  BEEF: 'beef_prod',
} as const;

const storageKeys = {
  W: 'wood_max',
  S: 'stone_max',
  F: 'food_max',
  C: 'coal_max',
  O: 'oil_max',
  G: 'glass_max',
  I: 'iron_max',
  HONEY: 'honey_max',
  MEAD: 'mead_max',
  BEEF: 'beef_max',
} as const;

const equipmentSlots: Record<number, string> = {
  1: 'equip1',
  2: 'equip2',
  3: 'equip3',
  4: 'equip4',
  6: 'hero',
};

const commStatKeys = [
  'equip1', 'equip2', 'equip3', 'equip4', 'hero', 'gem1', 'gem2', 'gem3', 'gem4',
  'meleeCbtStr', 'rangeCbtStr', 'frontCbtStr', 'flankCbtStr', 'allCbtStr', 'cyCbtStr',
  'wallStr', 'gateStr', 'moatStr', 'flankLimit', 'frontLimit', 'meadStr', 'horrorStr',
  'eliteStr', 'wave', 'cooldown', 'relicStr', 'beserkerStr', 'maidenSupp',
  'attackReinforcement', 'travel', 'loot', 'NPCMelee', 'NPCRange', 'NPCFront',
  'NPCFlank', 'NPCCy', 'NPCWall', 'NPCGate', 'NPCMoat', 'NPCGlory', 'CLMelee',
  'CLRange', 'CLFront', 'CLFlank', 'CLCy', 'CLWall', 'CLGate', 'CLMoat', 'CLLater',
  'CLFire', 'CLGlory',
];

const castStatKeys = [
  'equip1', 'equip2', 'equip3', 'equip4', 'hero', 'gem1', 'gem2', 'gem3', 'gem4',
  'meleeCbtStr', 'rangeCbtStr', 'opCbtStr', 'mainCbtStr', 'cyCbtStr', 'allCbtStr',
  'frontCbtStr', 'flankCbtStr', 'wallStr', 'gateStr', 'moatStr', 'wallLimit',
  'protectorSupp', 'lootStr', 'recruit', 'meadProd', 'research', 'hospital',
  'construction', 'baseRes', 'kingRes', 'po', 'resTransport', 'honeyProd',
  'meadStorage', 'honeyStorage', 'NPCMelee', 'NPCRange', 'NPCFront', 'NPCFlank',
  'NPCCy', 'NPCWall', 'NPCGate', 'NPCMoat', 'NPCWallLimit', 'CLMelee', 'CLRange',
  'CLCy', 'CLWall', 'CLGate', 'CLMoat', 'CLWallLimit', 'CLFire', 'CLGlory', 'CLEarly',
];

export class CommandJsonMockRuntime {
  private readonly emit: EmitMockMessage;
  private state: MockState | null = null;
  private started = false;

  constructor(emit: EmitMockMessage) {
    this.emit = emit;
  }

  async start() {
    if (this.started) {
      this.emitInitialState();
      return;
    }
    this.started = true;
    const gbd = await this.fetchCommandJson('gbd');
    this.state = buildMockState(gbd);
    this.emitInitialState();
  }

  handleClientMessage(message: object): boolean {
    const msg = recordValue(message);
    const type = stringValue(msg?.type);
    if (!type) {
      return true;
    }

    switch (type) {
      case 'getCastleFocus':
        this.emitCastleFocus();
        break;
      case 'focusPlayerCastle':
        this.focusCastle(recordValue(msg?.payload));
        break;
      case 'getCastleResourceUpdate':
        this.emitCastleByID(numberValue(msg.castleId) || numberValue(recordValue(msg.payload)?.castleId));
        break;
      case 'refreshEquipment':
        this.emitEquipment();
        break;
      case 'getCastleList':
        this.emitIfReady('castleList', this.state?.castleList ?? []);
        break;
      case 'fetchAllianceInfo':
        this.emitIfReady('allianceInfo', this.state?.alliance ?? {});
        break;
      case 'getSchedulerSettings':
        this.emitIfReady('schedulerSettings', this.state?.schedulerSettings ?? defaultSchedulerSettings());
        break;
      case 'saveSchedulerSettings':
        this.saveSchedulerSettings(recordValue(msg.payload));
        break;
      case 'getRecruitTroopsSettings':
        this.emitIfReady('recruitTroopsSettings', { mode: 'perCastle', castles: {} });
        break;
      case 'getAutoToolSettings':
        this.emitIfReady('autoToolSettings', { mode: 'perCastle', castles: {} });
        break;
      case 'getAutoHospitalSettings':
        this.emitIfReady('autoHospitalSettings', { checkIntervalSec: 60 });
        break;
      case 'getAutoBirdClientState':
        this.emitIfReady('autoBirdClientState', { presets: { presets: [] }, settings: {}, minSend: 0, minDelay: 6, maxDelay: 12 });
        break;
      case 'getAutoStationClientState':
        this.emitIfReady('autoStationClientState', { version: 1, leadTimeSec: 60, recallWhenClear: true, minRPTDays: 3, settings: {} });
        break;
      case 'saveAutoStationClientState':
        this.emitIfReady('autoStationClientState', msg.payload ?? {});
        break;
      case 'getAutoTCIClientState':
        this.emitIfReady('autoTCIClientState', { presets: { presets: [] }, settings: {} });
        break;
      case 'getAutoBeriWorldSettings':
        this.emitIfReady('autoBeriWorldSettings', {});
        break;
      case 'getMovement':
        this.emitIfReady('movementUpdate', { activeMovements: [] });
        break;
      case 'getRiftMapCoords':
        this.emitIfReady('riftMapCoords', {});
        break;
      case 'getRiftCRALaunch':
        this.emitIfReady('riftCRALaunch', { launches: [] });
        break;
      case 'getRiftMaidenCommsSettings':
        this.emitIfReady('riftMaidenCommsSettings', {});
        break;
      case 'toggleAutoTCI':
        this.emitIfReady('autoTCIStatus', { enabled: false, nextWakeUp: 0 });
        break;
      case 'toggleAutoBird':
        this.emitIfReady('autoBirdStatus', { enabled: false, nextWakeUp: 0 });
        break;
      case 'toggleAutoStation':
        this.emitIfReady('autoStationStatus', { enabled: false, state: 'off', threatCount: 0, nextImpactUnixMs: 0, detail: '' });
        break;
      case 'toggleAutoBeriWorld':
        this.emitIfReady('autoBeriWorldStatus', { enabled: false, nextWakeUp: 0 });
        break;
      default:
        break;
    }
    return true;
  }

  private async fetchCommandJson(name: string): Promise<JsonRecord> {
    const response = await fetch(`/__citadel_mock/command/${encodeURIComponent(name)}.json`, {
      cache: 'no-store',
    });
    if (!response.ok) {
      throw new Error(`Could not load mock command JSON ${name}: HTTP ${response.status}`);
    }
    const payload = await response.json();
    const record = recordValue(payload);
    if (!record) {
      throw new Error(`Mock command JSON ${name} is not an object`);
    }
    return record;
  }

  private emitInitialState() {
    const state = this.state;
    if (!state) {
      return;
    }
    this.emit({ type: 'gameLoginStatus', payload: { loggedIn: true, cooldown: 0 } });
    this.emit({ type: 'schedulerSettings', payload: state.schedulerSettings });
    this.emit({ type: 'recruitTroopsStatus', payload: { enabled: false } });
    this.emit({ type: 'autoToolStatus', payload: { enabled: false } });
    this.emit({ type: 'autoHospitalStatus', payload: { enabled: false } });
    this.emit({ type: 'autoTCIStatus', payload: { enabled: false, nextWakeUp: 0 } });
    this.emit({ type: 'autoBirdStatus', payload: { enabled: false, nextWakeUp: 0 } });
    this.emit({ type: 'autoStationStatus', payload: { enabled: false, state: 'off', threatCount: 0, nextImpactUnixMs: 0, detail: '' } });
    this.emit({ type: 'autoBeriWorldStatus', payload: { enabled: false, nextWakeUp: 0 } });
    this.emit({ type: 'globalResourceUpdate', payload: state.globalResources });
    this.emit({ type: 'allianceInfo', payload: state.alliance });
    this.emitCastleResources();
    this.emitCastleFocus();
    this.emitEquipment();
    this.emit({ type: 'lastKnownGameStateSnapshot', payload: buildSnapshotPayload(state) });
    this.emit({
      type: 'alert',
      payload: {
        category: 'green',
        message: 'Dev mock loaded from Logs/RecvCommandsJSON/gbd.json.',
      },
    });
  }

  private emitCastleResources() {
    const state = this.state;
    if (!state) {
      return;
    }
    for (const key of castleSlotOrder) {
      const castle = state.castleSlots[key];
      if (castle.aid > 0) {
        this.emit({ type: 'castleResourceUpdate', payload: castle, optionalData: castleResourceOptionalData[key] });
      }
    }
  }

  private emitCastleFocus() {
    const state = this.state;
    const focused = state?.focused;
    if (!state || !focused) {
      return;
    }
    const castle = state.castleByAid.get(focused.aid)?.castle;
    this.emit({
      type: 'castleFocus',
      payload: {
        aid: focused.aid,
        kingdomID: focused.kingdomID,
        mapPX: focused.mapX,
        mapPY: focused.mapY,
        castleName: focused.name,
        decorationSummary: [],
        bgRows: castle?.bgRows ?? [],
        bdRows: castle?.bdRows ?? [],
        slotProductionByLid: undefined,
        craftingQueues: undefined,
        catalogVersion: 'mock',
        playerCastles: state.playerCastles,
      },
    });
  }

  private emitCastleByID(castleID: number) {
    if (!this.state || castleID <= 0) {
      return;
    }
    const match = this.state.castleByAid.get(castleID);
    if (!match) {
      return;
    }
    this.emit({
      type: 'castleResourceUpdate',
      payload: match.castle,
      optionalData: castleResourceOptionalData[match.key],
    });
  }

  private focusCastle(payload: JsonRecord | null) {
    if (!this.state || !payload) {
      return;
    }
    const castleID = numberValue(payload.castleId);
    const match = this.state.castleByAid.get(castleID);
    if (!match) {
      return;
    }
    this.state.focused = {
      aid: castleID,
      kingdomID: numberValue(payload.kingdomId) || match.option.kingdomID,
      name: match.option.name,
      mapX: numberValue(payload.mapX) || match.option.mapX,
      mapY: numberValue(payload.mapY) || match.option.mapY,
    };
    this.emitCastleByID(castleID);
    this.emitCastleFocus();
  }

  private emitEquipment() {
    const state = this.state;
    if (!state) {
      return;
    }
    state.commStats.forEach((comm, index) => {
      this.emit({ type: 'commStatUpdate', payload: comm, optionalData: String(index) });
    });
    state.castStats.forEach((cast, index) => {
      this.emit({ type: 'castStatUpdate', payload: cast, optionalData: String(index) });
    });
  }

  private emitIfReady(type: string, payload: unknown) {
    this.emit({ type, payload });
  }

  private saveSchedulerSettings(payload: JsonRecord | null) {
    if (!this.state || !payload) {
      return;
    }
    this.state.schedulerSettings = { ...this.state.schedulerSettings, ...payload };
    this.emit({ type: 'schedulerSettings', payload: this.state.schedulerSettings });
  }
}

function buildMockState(gbd: JsonRecord): MockState {
  const castleSlots = emptyCastleSlots();
  const playerCastles: PlayerCastleOption[] = [];
  const castleByAid = new Map<number, { key: CastleSlotKey; castle: MockCastleInfo; option: PlayerCastleOption }>();
  applyGCL(recordValue(gbd.gcl), castleSlots, playerCastles);
  applyDCL(recordValue(gbd.dcl), castleSlots);

  for (const option of playerCastles) {
    const match = findCastleSlotByAid(castleSlots, option.aid);
    if (match) {
      castleByAid.set(option.aid, { ...match, option });
    }
  }

  const globalResources = parseGlobalResources(gbd);
  const alliance = {
    aid: numberValue(recordValue(gbd.gal)?.AID),
    playerCastleLocations: playerCastles.map((c) => ({
      kingdomID: c.kingdomID,
      castleID: c.aid,
      x: c.mapX,
      y: c.mapY,
    })),
  };
  const focused = playerCastles.find((c) => c.aid === castleSlots.mainCastle.aid) ?? playerCastles[0] ?? null;

  return {
    gbd,
    globalResources,
    castleSlots,
    playerCastles,
    castleList: buildCastleList(castleSlots),
    castleByAid,
    focused,
    alliance,
    commStats: buildCommStats(recordValue(gbd.gli)),
    castStats: buildCastStats(recordValue(gbd.gli), castleSlots),
    schedulerSettings: defaultSchedulerSettings(),
  };
}

function emptyCastleSlots(): Record<CastleSlotKey, MockCastleInfo> {
  return castleSlotOrder.reduce((slots, key) => {
    slots[key] = emptyCastle();
    return slots;
  }, {} as Record<CastleSlotKey, MockCastleInfo>);
}

function emptyCastle(): MockCastleInfo {
  return {
    castleName: '',
    aid: 0,
    amount: emptyAmount(),
    production: emptyProduction(),
    storage: emptyStorage(),
    troops: {
      kingdomID: 0,
      x: 0,
      y: 0,
      troopsI: {},
      troopsTU: {},
      troopsHI: {},
      troopsSHI: {},
      troopsMixed: {},
    },
    bgRows: [],
    bdRows: [],
  };
}

function applyGCL(gcl: JsonRecord | null, slots: Record<CastleSlotKey, MockCastleInfo>, playerCastles: PlayerCastleOption[]) {
  const kingdoms = arrayValue(gcl?.C);
  let outpostIndex = 0;

  for (const kingdomRaw of kingdoms) {
    const kingdom = recordValue(kingdomRaw);
    const kingdomID = numberValue(kingdom?.KID);
    for (const rowRaw of arrayValue(kingdom?.AI)) {
      const row = arrayValue(recordValue(rowRaw)?.AI);
      const cType = numberValue(row[0]);
      const x = numberValue(row[1]);
      const y = numberValue(row[2]);
      const aid = numberValue(row[3]);
      const name = stringValue(row[10]) || `Castle ${aid}`;
      if (aid <= 0) {
        continue;
      }

      const slotKey = resolveCastleSlotKey(cType, kingdomID, outpostIndex);
      if (slotKey?.startsWith('outpost')) {
        outpostIndex += 1;
      }
      if (slotKey) {
        assignCastleSlot(slots[slotKey], aid, name, kingdomID, x, y);
      }
      upsertPlayerCastle(playerCastles, { aid, kingdomID, name, mapX: x, mapY: y });
    }
  }

  playerCastles.sort((a, b) => castleSlotRank(slots, a.aid) - castleSlotRank(slots, b.aid) || a.kingdomID - b.kingdomID || a.aid - b.aid);
}

function resolveCastleSlotKey(cType: number, kingdomID: number, outpostIndex: number): CastleSlotKey | null {
  if (cType === 1 && kingdomID === 0) {
    return 'mainCastle';
  }
  if (cType === 4 && kingdomID === 0) {
    return outpostIndex === 0 ? 'outpost1' : outpostIndex === 1 ? 'outpost2' : outpostIndex === 2 ? 'outpost3' : null;
  }
  if (cType === 22 || cType === 5) {
    return 'metropolis';
  }
  if (cType === 3 || cType === 6) {
    return 'capital';
  }
  if (cType === 12) {
    if (kingdomID === 1) return 'desertCastle';
    if (kingdomID === 2) return 'iceCastle';
    if (kingdomID === 3) return 'dungeonCastle';
    if (kingdomID === 4) return 'stormCastle';
  }
  return null;
}

function assignCastleSlot(castle: MockCastleInfo, aid: number, name: string, kingdomID: number, x: number, y: number) {
  castle.aid = aid;
  castle.castleName = name;
  castle.mapKingdomID = kingdomID;
  castle.mapX = x;
  castle.mapY = y;
  castle.troops.kingdomID = kingdomID;
  castle.troops.x = x;
  castle.troops.y = y;
}

function upsertPlayerCastle(playerCastles: PlayerCastleOption[], option: PlayerCastleOption) {
  const existing = playerCastles.find((c) => c.aid === option.aid);
  if (existing) {
    Object.assign(existing, option);
    return;
  }
  playerCastles.push(option);
}

function applyDCL(dcl: JsonRecord | null, slots: Record<CastleSlotKey, MockCastleInfo>) {
  for (const kingdomRaw of arrayValue(dcl?.C)) {
    const kingdom = recordValue(kingdomRaw);
    for (const castleRaw of arrayValue(kingdom?.AI)) {
      const castleData = recordValue(castleRaw);
      const aid = numberValue(castleData?.AID);
      const match = findCastleSlotByAid(slots, aid);
      if (!castleData || !match) {
        continue;
      }
      parseCastleResources(match.castle, castleData);
    }
  }
}

function parseCastleResources(castle: MockCastleInfo, castleData: JsonRecord) {
  for (const [wireKey, field] of Object.entries(amountKeys)) {
    castle.amount[field] = numberValue(castleData[wireKey]);
  }
  const gpa = recordValue(castleData.gpa);
  if (!gpa) {
    return;
  }
  for (const [wireKey, field] of Object.entries(productionKeys)) {
    castle.production[field] = numberValue(gpa[`D${wireKey}`]) / 10;
  }
  for (const [wireKey, field] of Object.entries(storageKeys)) {
    castle.storage[field] = numberValue(gpa[`MR${wireKey}`]);
  }
}

function parseGlobalResources(gbd: JsonRecord): Record<string, number> {
  const resources = { ...emptyGlobalResources };
  const gcu = recordValue(gbd.gcu);
  resources.coins = numberValue(gcu?.C1);
  resources.rubies = numberValue(gcu?.C2);
  resources.might_pt = numberValue(recordValue(gbd.gmu)?.MP);
  resources.glory_pt = numberValue(recordValue(gbd.ufa)?.CF);
  resources.gallan_pt = numberValue(recordValue(gbd.ufp)?.CFP);

  for (const item of arrayValue(gbd.sce)) {
    const row = arrayValue(item);
    const key = globalSceKeyMap[stringValue(row[0])];
    if (key) {
      resources[key] = numberValue(row[1]);
    }
  }
  return resources;
}

function buildCastleList(slots: Record<CastleSlotKey, MockCastleInfo>) {
  return castleSlotOrder
    .map((key) => {
      const castle = slots[key];
      return castle.aid > 0
        ? { id: castle.aid, name: castle.castleName || `Castle ${castle.aid}`, type: castleTypeBySlot[key] }
        : null;
    })
    .filter((entry): entry is { id: number; name: string; type: string } => entry != null);
}

function buildSnapshotPayload(state: MockState) {
  return {
    gameState: {
      globalResources: state.globalResources,
      alliance: state.alliance,
      castle: state.castleSlots,
      equipment: {
        commStatArray: state.commStats,
        castStatArray: state.castStats,
      },
      castleFocus: {
        castleAID: state.focused?.aid ?? 0,
        kingdomID: state.focused?.kingdomID ?? 0,
        mapPX: state.focused?.mapX ?? 0,
        mapPY: state.focused?.mapY ?? 0,
      },
    },
  };
}

function buildCommStats(gli: JsonRecord | null): JsonRecord[] {
  return arrayValue(gli?.C).map((raw, index) => {
    const row = recordValue(raw);
    const stat = emptyEquipmentStat(commStatKeys);
    stat.id = numberValue(row?.ID) || index;
    stat.name = stringValue(row?.N) || `Commander ${index + 1}`;
    applyEquipmentIDs(stat, arrayValue(row?.EQ));
    return stat;
  });
}

function buildCastStats(gli: JsonRecord | null, slots: Record<CastleSlotKey, MockCastleInfo>): JsonRecord[] {
  const statByPosition = new Map<number, JsonRecord>();
  for (const raw of arrayValue(gli?.B)) {
    const row = recordValue(raw);
    const castleID = numberValue(row?.LICID);
    const rank = castleSlotRank(slots, castleID);
    if (rank < 0 || rank >= castleSlotOrder.length) {
      continue;
    }
    const key = castleSlotOrder[rank];
    const castle = slots[key];
    const stat = emptyEquipmentStat(castStatKeys);
    stat.id = numberValue(row?.ID);
    stat.castleID = castleID;
    stat.name = castle.castleName || `Castle ${castleID}`;
    stat.castlePosition = rank;
    applyEquipmentIDs(stat, arrayValue(row?.EQ));
    statByPosition.set(rank, stat);
  }

  return castleSlotOrder.map((key, index) => {
    const existing = statByPosition.get(index);
    if (existing) {
      return existing;
    }
    const stat = emptyEquipmentStat(castStatKeys);
    stat.id = 0;
    stat.castleID = slots[key].aid;
    stat.name = slots[key].castleName || castleTypeBySlot[key];
    stat.castlePosition = index;
    return stat;
  });
}

function emptyEquipmentStat(keys: string[]): JsonRecord {
  const stat: JsonRecord = {};
  for (const key of keys) {
    stat[key] = 0;
  }
  stat.extraStats = [];
  return stat;
}

function applyEquipmentIDs(stat: JsonRecord, equipmentRows: unknown[]) {
  for (const equipmentRaw of equipmentRows) {
    const equipment = arrayValue(equipmentRaw);
    const field = equipmentSlots[numberValue(equipment[1])];
    if (field) {
      stat[field] = numberValue(equipment[0]);
    }
  }
}

function findCastleSlotByAid(slots: Record<CastleSlotKey, MockCastleInfo>, aid: number) {
  if (aid <= 0) {
    return null;
  }
  for (const key of castleSlotOrder) {
    const castle = slots[key];
    if (castle.aid === aid) {
      return { key, castle };
    }
  }
  return null;
}

function castleSlotRank(slots: Record<CastleSlotKey, MockCastleInfo>, aid: number): number {
  const idx = castleSlotOrder.findIndex((key) => slots[key].aid === aid);
  return idx >= 0 ? idx : 1000;
}

function emptyAmount(): Record<string, number> {
  return Object.values(amountKeys).reduce((out, key) => {
    out[key] = 0;
    return out;
  }, {} as Record<string, number>);
}

function emptyProduction(): Record<string, number> {
  const out = Object.values(productionKeys).reduce((acc, key) => {
    acc[key] = 0;
    return acc;
  }, {} as Record<string, number>);
  out.food_consumption = 0;
  out.mead_consumption = 0;
  out.beef_consumption = 0;
  return out;
}

function emptyStorage(): Record<string, number> {
  return Object.values(storageKeys).reduce((out, key) => {
    out[key] = 0;
    return out;
  }, {} as Record<string, number>);
}

function defaultSchedulerSettings(): JsonRecord {
  return {
    minAttackDelay: 4,
    maxAttackDelay: 6,
    upgradeEreDelayMs: 1200,
    upgradeCoinThreshold: 0,
    manualFocusIdleSec: 30,
    tabPriorities: {},
    featureSchedules: {},
  };
}

function recordValue(value: unknown): JsonRecord | null {
  return value != null && typeof value === 'object' && !Array.isArray(value) ? value as JsonRecord : null;
}

function arrayValue(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function numberValue(value: unknown): number {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return Math.trunc(value);
  }
  if (typeof value === 'string' && value.trim() !== '') {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? Math.trunc(parsed) : 0;
  }
  return 0;
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

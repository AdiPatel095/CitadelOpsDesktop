/**
 * The fictional realm the onboarding preview harness serves (CIT-22): a "rich" existing account with saved settings.
 * Scenarios start from it (or from its emptied form) and patch it as data (see scenario.ts).
 *
 * Every number and name here is invented. Nothing is captured from a real account, and nothing is read from a game
 * or a local CitadelOps process. Adapted from the hosted preview fixture (`src/commandCenter/mock/fixture.ts`).
 */
import type {
    AutomationStateV2,
    CastleStateV2,
    ConfigurationSnapshot,
    EquipmentInstanceV2,
    GameStateV2,
    GemInstanceV2,
    ProductionQueueV2,
} from '../../src/api/Contracts';

/** Stable timestamps derived from load time, so relative clocks look sensible. */
let clock: () => number = () => Date.now();
/** The realm's relative timestamps follow this clock (the scenario builder sets it, so a build is deterministic). */
export const setRealmClock = (next: () => number): void => { clock = next; };
const now = () => new Date(clock());
const iso = (offsetMs = 0) => new Date(now().getTime() + offsetMs).toISOString();

const minutes = (count: number) => count * 60_000;
const hours = (count: number) => count * 3_600_000;

// ---------------------------------------------------------------------------
// Catalog ids. These match the shape the game data uses (numeric definition
// ids) without claiming to be any particular real unit.
// ---------------------------------------------------------------------------
export const UNIT = { spearman: 1, bowman: 2, macebearer: 6, crossbow: 7, royalGuard: 39, veteran: 40, direwolf: 277 };
const RESOURCE = { wood: 1, stone: 2, food: 3, coins: 4 };

const resource = (amount: number, productionPerHour: number, capacity: number) => ({
    amount,
    productionPerHour,
    capacity,
});

const productionLine = (
    lineId: number,
    active: { collection: string; id: number; amount: number; endsInMs: number } | null,
    queued: Array<{ collection: string; id: number; amount: number }>,
    capacity: number,
): ProductionQueueV2 => ({
    lineId,
    active: active
        ? {
            definition: { collection: active.collection, id: active.id },
            amount: active.amount,
            startedAt: iso(-minutes(6)),
            completesAt: iso(active.endsInMs),
        }
        : undefined,
    queued: queued.map((item) => ({
        definition: { collection: item.collection, id: item.id },
        amount: item.amount,
    })),
    capacity,
    observedAt: iso(),
});

const emptyDefense = (): CastleStateV2['defense'] => {
    const section = () => ({ toolSlots: [], unitPercent: 50, unitTypePercent: 50 });
    return {
        wall: { left: section(), middle: section(), right: section(), observedAt: iso() },
        keep: { primaryToolSlots: [], secondaryToolSlots: [], unitTypePercent: 50, observedAt: iso() },
        moat: { leftToolSlots: [], middleToolSlots: [], rightToolSlots: [], observedAt: iso() },
        castellanId: 601,
        rangedUnitIds: [UNIT.bowman, UNIT.crossbow],
        meleeUnitIds: [UNIT.spearman, UNIT.macebearer],
        inventory: {},
        observedAt: iso(),
    };
};

const castle = (input: {
    id: number;
    name: string;
    kingdomId: number;
    x: number;
    y: number;
    focused: boolean;
    units: Record<string, number>;
    traveling?: Record<string, number>;
    food: number;
    foodPerHour: number;
}): CastleStateV2 => ({
    id: input.id,
    kingdomId: input.kingdomId,
    slotType: input.kingdomId === 0 ? 1 : 12,
    name: input.name,
    x: input.x,
    y: input.y,
    focused: input.focused,
    resources: {
        [RESOURCE.wood]: resource(482_140, 18_400, 900_000),
        [RESOURCE.stone]: resource(391_755, 16_900, 900_000),
        [RESOURCE.food]: resource(input.food, input.foodPerHour, 1_200_000),
        [RESOURCE.coins]: resource(2_884_310, 0, 0),
    },
    units: {
        stationed: input.units,
        traveling: input.traveling ?? {},
        hospital: { [UNIT.spearman]: 120, [UNIT.bowman]: 96 },
        specialHospital: {},
        total: Object.entries(input.units).reduce<Record<string, number>>((totals, [id, count]) => {
            totals[id] = count + (input.traveling?.[id] ?? 0);
            return totals;
        }, {}),
    },
    unitsObservedAt: iso(-minutes(1)),
    defense: emptyDefense(),
    buildings: {},
    layout: { ground: {}, objects: {}, fixed: {}, observedAt: iso() },
    buildingQueue: {
        slotCount: 2,
        slots: [
            { index: 0, wireValue: 1, status: 'occupied', buildingId: 4 },
            { index: 1, wireValue: 0, status: 'available' },
        ],
        observedAt: iso(),
    },
    constructionSlots: {},
    production: {
        // "0" is recruitment, "1" is tool production.
        0: productionLine(
            0,
            { collection: 'units', id: UNIT.royalGuard, amount: 420, endsInMs: minutes(4) },
            [{ collection: 'units', id: UNIT.crossbow, amount: 300 }],
            3,
        ),
        1: productionLine(
            1,
            { collection: 'units', id: UNIT.veteran, amount: 60, endsInMs: minutes(17) },
            [],
            2,
        ),
    },
    queueableProduction: {},
    crafting: {
        buildings: {},
        enabledRecipeIds: [],
        enabledRecipeGroupIds: [],
        outputBoostByQueueType: {},
    },
});

// ---------------------------------------------------------------------------
// Automations. Keys are the exact feature ids the Automation view looks up.
// ---------------------------------------------------------------------------
const automation = (
    id: string,
    status: string,
    detail: string,
    extra: Partial<AutomationStateV2> = {},
): AutomationStateV2 => ({
    id,
    enabled: true,
    status,
    detail,
    updatedAt: iso(-minutes(2)),
    ...extra,
});

const automations: Record<string, AutomationStateV2> = {
    autoRecruit: automation('autoRecruit', 'idle', 'All recruitment queues at target strength'),
    autoTool: automation('autoTool', 'running', 'Producing siege tools in 2 workshops'),
    autoHospital: automation('autoHospital', 'idle', 'No wounded units awaiting review'),
    autoTCI: automation('autoTCI', 'waiting', 'Next collection in 12 minutes', {
        nextCheckAt: iso(minutes(12)),
    }),
    autoFoodBalance: automation('autoFoodBalance', 'idle', 'Every castle above its food reserve'),
    autoBuyer: automation('autoBuyer', 'watching', 'Monitoring the offer catalogue'),
    autoFortress: automation('autoFortress', 'waiting', 'Balancing confirmed Direwolf supply across three kingdoms', {
        metrics: {
            enabledKingdoms: 3,
            availableDonorDirewolves: 600,
            stationedDirewolvesKingdom1: 420,
            inboundDirewolvesKingdom1: 640,
            allocatedDirewolvesKingdom1: 1_260,
            outstandingDirewolvesKingdom1: 200,
            stationedDirewolvesKingdom2: 780,
            inboundDirewolvesKingdom2: 0,
            allocatedDirewolvesKingdom2: 1_180,
            outstandingDirewolvesKingdom2: 400,
            stationedDirewolvesKingdom3: 1_000,
            inboundDirewolvesKingdom3: 100,
            allocatedDirewolvesKingdom3: 1_100,
            outstandingDirewolvesKingdom3: 0,
        },
        details: {
            supplyKingdom1: 'Confirmed Direwolves are inbound',
            supplyKingdom2: 'Destination Direwolf inventory needs a fresh observation',
            supplyKingdom3: 'Waiting for the confirmed inbound Direwolves to arrive',
        },
    }),
    autoTowers: automation('autoTowers', 'running', 'Rotation 3 of 6 · tower level 90'),
    autoInvasion: automation('autoInvasion', 'idle', 'Waiting for the next invasion window'),
    autoNomad: automation('autoNomad', 'idle', 'No nomad camps in range'),
    autoAdvisor: automation('autoAdvisor', 'running', 'Collecting available advice'),
    autoEquipmentCleanup: automation('autoEquipmentCleanup', 'idle', '47 items staged for review'),
    autoSceatRes: automation('autoSceatRes', 'idle', 'Resource routes balanced'),
    autoSceatResLogistics: automation('autoSceatResLogistics', 'idle', 'No transfers pending'),
    autoStorm: automation('autoStorm', 'waiting', 'Storm window opens in 34 minutes'),
    autoStormShop: automation('autoStormShop', 'idle', 'Shop stock unchanged'),
    autoStormBuild: automation('autoStormBuild', 'idle', 'Island buildings at target level'),
    autoBeriWorld: automation('autoBeriWorld', 'idle', 'Event not currently running'),
    autoBeriWorldAttack: automation('autoBeriWorldAttack', 'idle', 'No targets locked'),
    autoBeriWorldTools: automation('autoBeriWorldTools', 'idle', 'Tool reserve satisfied'),
    autoBeriWorldBuild: automation('autoBeriWorldBuild', 'idle', 'Nothing queued'),
    autoKhan: automation('autoKhan', 'waiting', 'Next Khan available in 36 minutes'),
    'autoKhan:cooldown': automation('autoKhan:cooldown', 'idle', 'No cooldowns skipped'),
    'autoKhan:rage': automation('autoKhan:rage', 'idle', 'Rage threshold not met'),
    'autoKhan:defense': automation('autoKhan:defense', 'armed', 'Gate policy armed'),
    autoBird: automation('autoBird', 'armed', 'Watching 3 castles · no threats', {
        metrics: {
            nextBirdUnixMs: now().getTime() + minutes(9),
            'birdReturnUnixMs.4101': now().getTime() + minutes(9),
        },
    }),
    autoStation: automation('autoStation', 'armed', 'Station policy armed on 3 castles', {
        metrics: { threatCount: 0 },
    }),
};

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------
export const buildRealmState = (revision: number): GameStateV2 => ({
    schemaVersion: 2,
    revision,
    updatedAt: iso(),
    session: {
        mode: 'full',
        generation: 4,
        baselineGeneration: 4,
        connectionGeneration: 4,
        status: 'connected',
        loggedIn: true,
        socketReady: true,
        serverUrl: 'fixture://synthetic-world',
        detail: 'Simulated connection',
        changedAt: iso(-hours(6)),
    },
    account: { uid: 4_100_220, worldId: 'demo-world', playerId: 4_100_220, boundAt: iso(-hours(72)) },
    player: {
        id: 4_100_220,
        name: 'Preview Commander',
        allianceId: 9_001,
        level: 70,
        legendLevel: 240,
        might: 8_420_000,
        glory: 128_400,
        gallantry: 41_200,
        resources: { [RESOURCE.wood]: 482_140, [RESOURCE.stone]: 391_755, [RESOURCE.food]: 618_020, [RESOURCE.coins]: 2_884_310 },
        currencies: { 1: 12_480, 2: 3_210 },
        vip: { points: 18_400, level: 12, remainingSec: 486_000 },
        achievements: { points: 12_840, completed: {}, progress: {}, observedAt: iso() },
        legendSkills: {
            activeIds: [],
            skillPoints: 18,
            resetRemainingSec: 0,
            resetCount: 0,
            sceatSkillIds: [],
            sceatActivations: [],
            observedAt: iso(),
        },
    },
    castles: {
        4101: castle({
            id: 4101,
            name: 'Stonehaven',
            kingdomId: 0,
            x: 412,
            y: 388,
            focused: true,
            units: {
                [UNIT.spearman]: 24_800,
                [UNIT.bowman]: 31_400,
                [UNIT.macebearer]: 18_900,
                [UNIT.crossbow]: 22_650,
                [UNIT.royalGuard]: 9_420,
                [UNIT.direwolf]: 600,
            },
            traveling: { [UNIT.bowman]: 4_200, [UNIT.crossbow]: 2_100 },
            food: 618_020,
            foodPerHour: 18_400,
        }),
        4102: castle({
            id: 4102,
            name: 'Frostmere',
            kingdomId: 2,
            x: 508,
            y: 271,
            focused: false,
            units: {
                [UNIT.spearman]: 12_100,
                [UNIT.bowman]: 15_600,
                [UNIT.crossbow]: 9_800,
                [UNIT.direwolf]: 780,
            },
            food: 402_500,
            foodPerHour: 12_050,
        }),
        4103: castle({
            id: 4103,
            name: 'Ashford Keep',
            kingdomId: 0,
            x: 366,
            y: 449,
            focused: false,
            units: {
                [UNIT.spearman]: 8_400,
                [UNIT.macebearer]: 6_900,
                [UNIT.royalGuard]: 3_150,
            },
            food: 289_700,
            foodPerHour: 9_600,
        }),
        4104: castle({
            id: 4104,
            name: 'Glacier Watch',
            kingdomId: 1,
            x: 216,
            y: 334,
            focused: false,
            units: { [UNIT.spearman]: 8_900, [UNIT.bowman]: 11_200, [UNIT.direwolf]: 420 },
            food: 318_400,
            foodPerHour: 10_250,
        }),
        4105: castle({
            id: 4105,
            name: 'Ember Crown',
            kingdomId: 3,
            x: 681,
            y: 744,
            focused: false,
            units: { [UNIT.macebearer]: 14_100, [UNIT.crossbow]: 12_800, [UNIT.direwolf]: 1_000 },
            traveling: { [UNIT.direwolf]: 100 },
            food: 507_300,
            foodPerHour: 14_600,
        }),
        4106: castle({
            id: 4106,
            name: 'Marrow Court',
            kingdomId: 0,
            x: 402,
            y: 431,
            focused: false,
            units: {
                [UNIT.spearman]: 6_200,
                [UNIT.bowman]: 9_100,
                [UNIT.crossbow]: 4_800,
            },
            food: 141_300,
            foodPerHour: 7_400,
        }),
        4107: castle({
            id: 4107,
            name: 'Storm Bastion',
            kingdomId: 4,
            x: 90,
            y: 120,
            focused: false,
            units: { [UNIT.spearman]: 3_000, [UNIT.crossbow]: 2_400 },
            food: 210_000,
            foodPerHour: 6_100,
        }),
        4108: castle({
            id: 4108,
            name: 'Berimond Camp',
            kingdomId: 10,
            x: 60,
            y: 60,
            focused: false,
            units: { [UNIT.spearman]: 4_000, [UNIT.macebearer]: 2_500 },
            food: 90_000,
            foodPerHour: 3_000,
        }),
    },
    commanders: {
        501: {
            id: 501, name: 'Aster', visiblePosition: 1, available: true,
            equipment: { '1': 9001, '2': 9002, '3': 9003 },
            gems: { '1': 8001, '3': 8002 },
        },
        502: {
            id: 502, name: 'Vale', visiblePosition: 2, available: true,
            equipment: { '1': 9007, '2': 9008 },
            gems: { '1': 8004 },
        },
        503: { id: 503, name: 'Maren', visiblePosition: 3, available: false, equipment: {}, gems: {} },
    },
    generals: {},
    castellans: {
        601: {
            id: 601, castleId: 4101, name: 'Rowan',
            equipment: { '1': 9010, '4': 9011 },
            gems: { '4': 8005 },
        },
    },
    movements: {},
    movementSnapshot: { version: 1, connectionGeneration: 4, observedAt: iso() },
    stationing: {},
    scheduled: {},
    rift: { launches: {} },
    inventory: {
        constructionItems: {},
        constructionOffers: {},
        constructionOffersKingdomId: 0,
        equipment: mockEquipment,
        gems: mockGems,
        gemStacks: {},
        items: {},
    },
    subscriptions: {},
    market: { castles: {}, caravanLevelLoaded: true, observedAt: iso() },
    kingdomTransport: { unlocks: {}, pending: [], pendingUnits: [], observedAt: iso() },
    beri: { availableTroops: 0, troopsByUnit: {} },
    alliance: {
        id: 9_001,
        name: 'Ironwatch Pact',
        members: [
            { playerId: 4_100_220, name: 'Preview Commander', rankId: 2, level: 70, might: 8_420_000 },
            { playerId: 4_100_318, name: 'Bramble', rankId: 3, level: 66, might: 6_180_000 },
        ],
        holdings: [],
        observedAt: iso(),
    },
    alliances: {},
    allianceHelpRequests: {
        hospitalProductionIds: [],
        recruitmentCastleIds: [],
        pendingOtherListIds: [],
        observedAt: iso(),
    },
    map: {},
    towerCooldowns: {},
    towerQueue: { entriesByCastle: {}, lastScannedAt: {} },
    invasion: {
        lastScannedAt: {},
        fortifiedTargets: {},
        fortifyCurrencies: [],
        fortifyResourceCount: 0,
        fortifyRubyCount: 0,
    },
    storm: {
        lastScannedAt: {},
        map: {
            coveredBounds: { x1: 0, y1: 0, x2: 0, y2: 0 },
            nextBounds: { x1: 0, y1: 0, x2: 0, y2: 0 },
            targets: {},
        },
        islandReturns: {},
    },
    nomadCamps: { lastScannedAt: {}, cooldowns: {} },
    khan: {
        attacksLaunched: 18,
        victoriesConfirmed: 17,
        cooldownsSkipped: 1,
        launches: [],
        taunts: {},
        tauntsObserved: 0,
        tauntsResolved: 0,
        protection: { active: false },
    },
    dailyAttacks: { count: 2_881, serverThreshold: 3_200, growthRate: 1.4, observedAt: iso() },
    attackDialog: null,
    attackPresets: [],
    eventScores: { byEvent: {}, inventory: { observedAt: iso(), activeByEvent: {} } },
    advisor: { summary: { gains: {}, costs: {} } },
    commandContext: {},
    automations,
    reports: { notices: {}, spyCaptures: {}, battleCaptures: {} },
    observations: {},
});


/**
 * Stored presets, so the preset views show their real layout instead of an
 * empty state. Shapes follow `attackPresets/AttackPresetTypes` and
 * `defensePresets/DefensePresetTypes`; ids are stable so re-renders do not
 * reshuffle the list.
 */
const slot = (itemId: number, quantity: number) => ({ itemId, quantity });
const lane = (troops: [number, number][], tools: [number, number][]) => ({
    troops: troops.map(([id, n]) => slot(id, n)),
    tools: tools.map(([id, n]) => slot(id, n)),
});

const mockAttackPresets = () => ({
    version: 1,
    presets: [
        {
            id: 'preset-tower-rush',
            name: 'Tower rush · two flank',
            targetType: 'pve',
            createdAt: '2026-07-02T09:14:00.000Z',
            updatedAt: '2026-08-11T18:22:00.000Z',
            waves: [
                { L: lane([[2, 120]], [[101, 60]]), M: lane([], []), R: lane([[2, 120]], [[101, 60]]) },
                { L: lane([[7, 80]], [[102, 40]]), M: lane([], []), R: lane([[7, 80]], [[102, 40]]) },
            ],
            courtyardSupport: lane([[39, 40]], [[103, 12]]),
        },
        {
            id: 'preset-invasion-standard',
            name: 'Invasion · standard',
            targetType: 'pvp',
            createdAt: '2026-06-18T11:05:00.000Z',
            updatedAt: '2026-08-09T07:40:00.000Z',
            waves: [
                { L: lane([[1, 150]], [[101, 80]]), M: lane([[6, 60]], [[104, 20]]), R: lane([[1, 150]], [[101, 80]]) },
            ],
            courtyardSupport: lane([[39, 25]], []),
        },
    ],
});

const toolSlots = (entries: [number, number][]) =>
    entries.map(([definitionId, amount]) => ({ definitionId, amount }));

const wallSection = (unitPercent: number, entries: [number, number][]) => ({
    toolSlots: toolSlots(entries),
    unitPercent,
    unitTypePercent: 60,
});

const mockDefensePresets = () => ({
    version: 1,
    presets: [
        {
            id: 'defense-anti-melee',
            name: 'Anti-melee · full wall',
            createdAt: '2026-07-21T13:02:00.000Z',
            updatedAt: '2026-08-12T20:11:00.000Z',
            sourceCastleId: 5001,
            sourceCastleName: 'Stonehaven',
            wall: {
                left: wallSection(30, [[201, 250], [202, 250]]),
                middle: wallSection(40, [[201, 300], [203, 200]]),
                right: wallSection(30, [[201, 250], [202, 250]]),
            },
            moat: {
                leftToolSlots: toolSlots([[204, 120]]),
                middleToolSlots: toolSlots([[204, 160]]),
                rightToolSlots: toolSlots([[204, 120]]),
            },
            keep: { mauct: 2, unitTypePercent: 50 },
        },
    ],
});


/**
 * Worn gear and a small pool of spares, so the Equipment view shows slots,
 * rarities and effects instead of six empty states. Slot numbers follow the
 * official order the view renders in; commanders map slot -> instance id.
 */
const equipmentInstance = (
    id: number,
    slot: number,
    definitionId: number,
    rarityId: number,
    level: number,
    wearerId?: number,
): EquipmentInstanceV2 => ({
    id,
    definitionId,
    slot,
    typeId: definitionId,
    rarityId,
    relic: false,
    relicKnown: true,
    setId: 12,
    level,
    ...(wearerId ? { wearerId, wearerKind: 'commander' } : {}),
    effects: [
        { wireId: id * 10 + 1, definitionId: 8801, rollPercent: 78, values: [12] },
        { wireId: id * 10 + 2, definitionId: 8802, rollPercent: 54, values: [7] },
    ],
});

const gemInstance = (id: number, definitionId: number, level: number, equipmentInstanceId?: number): GemInstanceV2 => ({
    id,
    definitionId,
    typeId: definitionId,
    combatMode: 'any',
    level,
    ...(equipmentInstanceId ? { equipmentInstanceId } : {}),
    effects: [{ wireId: id * 10 + 1, definitionId: 8810, rollPercent: 66, values: [9] }],
});

const mockEquipment: Record<string, EquipmentInstanceV2> = {
    '9001': equipmentInstance(9001, 1, 7101, 4, 14, 501),
    '9002': equipmentInstance(9002, 2, 7102, 3, 10, 501),
    '9003': equipmentInstance(9003, 3, 7103, 5, 42, 501),
    '9004': equipmentInstance(9004, 1, 7104, 2, 6),
    '9005': equipmentInstance(9005, 4, 7105, 4, 12),
    '9006': equipmentInstance(9006, 2, 7106, 3, 8),
    // Vale — a lighter, lower-level loadout to contrast with Aster's.
    '9007': equipmentInstance(9007, 1, 7104, 2, 5, 502),
    '9008': equipmentInstance(9008, 2, 7106, 3, 9, 502),
    // Rowan, the castellan: castle defense rather than march gear.
    '9010': equipmentInstance(9010, 1, 7101, 3, 10, 601),
    '9011': equipmentInstance(9011, 4, 7105, 4, 13, 601),
};

const mockGems: Record<string, GemInstanceV2> = {
    '8001': gemInstance(8001, 7201, 5, 9001),
    '8002': gemInstance(8002, 7202, 4, 9003),
    '8003': gemInstance(8003, 7203, 3),
    '8004': gemInstance(8004, 7203, 2, 9007),
    '8005': gemInstance(8005, 7202, 3, 9011),
};

/**
 * Feature toggles. The Automation view reads enablement from configuration,
 * not from state, keyed by these snake_case ids.
 */
export const realmConfiguration = (revision: number): ConfigurationSnapshot => ({
    schemaVersion: 1,
    revision,
    updatedAt: iso(),
    sections: {
        'automation.enabled': {
            recruit_troops: true,
            auto_tool: true,
            auto_sceat_resources: false,
            auto_food_balance: true,
            auto_hospital: true,
            auto_tci: true,
            auto_towers: true,
            auto_invasion: true,
            auto_nomad: false,
            auto_advisor: true,
            auto_buyer: false,
            auto_fortress: true,
            auto_khan: true,
            auto_beri_world: false,
            auto_storm: true,
            auto_bird: true,
            auto_station: true,
            auto_equipment_cleanup: false,
        },
        scheduler: { botLocked: false, featureSchedules: {} },
        'decorations.presets': {},
        'attacks.presets': mockAttackPresets(),
        'defense.presets': mockDefensePresets(),
        'automation.autoFortress': {
            version: 1,
            checkIntervalSec: 5,
            mapRefreshIntervalSec: 1_800,
            dailyAttackLimit: 0,
            horseTravelBoostId: 1009,
            minimumCommanderSpeedBonus: 100,
            direwolfPurchaseLimit: 10_000,
            minimumTabletReserve: 25_000,
            useTimeSkips: false,
            timeSkipReserve: { MS5: 2 },
            kingdoms: { 1: { enabled: true }, 2: { enabled: true }, 3: { enabled: true } },
        },
    },
});

import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({
  root: clientRoot,
  appType: 'custom',
  logLevel: 'silent',
  server: { middlewareMode: true },
});
const readiness = await vite.ssrLoadModule('/src/settings/readiness/eventAttackReadiness.ts');
const model = await vite.ssrLoadModule('/src/settings/readiness/Readiness.ts');
const types = await vite.ssrLoadModule('/src/attackPresets/AttackPresetTypes.ts');
const invasion = await vite.ssrLoadModule('/src/settings/AutoInvasionClientState.ts');
const { messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');

after(async () => {
  await vite.close();
});

const NOW = Date.parse('2026-09-29T12:00:00Z');
const troops = { 1: { id: 1, name: 'Unit 1' }, 2: { id: 2, name: 'Unit 2' } };
const tools = { 500: { id: 500, name: 'Ladder' } };

function lane(troopSlots, toolSlots = []) {
  return {
    troops: troopSlots.map(([itemId, quantity]) => ({ itemId, quantity })),
    tools: toolSlots.map(([itemId, quantity]) => ({ itemId, quantity })),
  };
}

function inline(troopSlots, toolSlots = []) {
  return {
    source: 'inline',
    presetId: '',
    missing: false,
    setup: {
      targetType: 'pve',
      useTroopFamilies: false,
      waves: [{ L: lane(troopSlots, toolSlots), M: lane([]), R: lane([]) }],
      courtyardSupport: { troops: [], tools: [] },
    },
  };
}

const ZERO_TIME = '0001-01-01T00:00:00Z';
const SESSION = { generation: 25, baselineGeneration: 25, connectionGeneration: 3, status: 'ready', loggedIn: true, socketReady: true, changedAt: '2026-09-29T09:00:00Z' };
const LIVE = { session: SESSION, connected: true };

function gameState(overrides = {}) {
  return {
    session: SESSION,
    castles: {
      // Real projection shape: ClientProjection zeroes unitsObservedAt, serialized as the Go zero time.
      7: { id: 7, kingdomId: 0, x: 0, y: 0, units: { stationed: { 1: 100, 2: 5, 500: 10 }, traveling: {}, hospital: {}, specialHospital: {}, total: {} }, unitsObservedAt: ZERO_TIME },
      8: { id: 8, kingdomId: 10, x: 0, y: 0, units: { stationed: {}, traveling: {}, hospital: {}, specialHospital: {}, total: {} }, unitsObservedAt: ZERO_TIME },
      9: { id: 9, kingdomId: 0, x: 0, y: 0, units: { stationed: {}, traveling: {}, hospital: {}, specialHospital: {}, total: {} }, unitsObservedAt: ZERO_TIME },
    },
    commanders: { 1: { id: 1, available: true, equipment: {}, gems: {} } },
    dailyAttacks: { count: 3, serverThreshold: 100, growthRate: 0, observedAt: '2026-09-29T11:00:00Z' },
    market: { boostersObservedAt: '2026-09-29T11:00:00Z', boosters: { 24: { expiresAt: '2026-09-30T00:00:00Z' } } },
    ...overrides,
  };
}

const emptyDocument = { version: 1, presets: [] };

function evaluate(draft = {}, extra = {}) {
  return readiness.evaluateEventAttackReadiness({
    featureId: 'autoNomad',
    draft: { sourceCastleId: 7, slots: [{ slot: 'nomad', ref: inline([[1, 50]]) }], dailyAttackLimit: 0, ...draft },
    state: gameState(),
    document: emptyDocument,
    troops,
    tools,
    metadataReady: true,
    now: NOW,
    observation: LIVE,
    ...extra,
  });
}

function stateOf(report, id, slot) {
  const matches = report.checks.filter((check) => check.id === id && (slot === undefined || check.slot === slot));
  assert.equal(matches.length, 1, `${id} should appear once`);
  return matches[0].state;
}

test('every check references a catalog message', () => {
  const report = evaluate(
    { scoreTarget: 10, dailyAttackLimit: 5, horseTravelBoostId: 1008, requireActiveGallantryBooster: true, fortifyCurrency: 'GTO' },
    { difficulties: { selections: [{ eventId: 72, available: true }], achievementsObserved: true, loading: false } },
  );
  for (const check of report.checks) assert.ok(messages[check.messageKey], check.id);
});

test('source-castle reaches unavailable, blocked and valid', () => {
  assert.equal(stateOf(evaluate({}, { state: null }), 'source-castle'), 'unavailable');
  assert.equal(stateOf(evaluate({ sourceCastleId: 0 }), 'source-castle'), 'blocked');
  assert.equal(stateOf(evaluate({ sourceCastleId: 404 }), 'source-castle'), 'blocked');
  assert.equal(stateOf(evaluate({ sourceCastleId: 8 }), 'source-castle'), 'blocked');
  assert.equal(stateOf(evaluate(), 'source-castle'), 'valid');
});

test('composition is blocked for no setup, a missing record or zero troops', () => {
  assert.equal(stateOf(evaluate({ slots: [{ slot: 'nomad', ref: { source: 'none' } }] }), 'composition', 'nomad'), 'blocked');
  assert.equal(stateOf(evaluate({ slots: [{ slot: 'nomad', ref: { source: 'preset', presetId: 'gone', missing: true } }] }), 'composition', 'nomad'), 'blocked');
  assert.equal(stateOf(evaluate({ slots: [{ slot: 'nomad', ref: { source: 'preset', presetId: 'gone', missing: false } }] }), 'composition', 'nomad'), 'blocked');
  assert.equal(stateOf(evaluate({ slots: [{ slot: 'nomad', ref: inline([]) }] }), 'composition', 'nomad'), 'blocked');
  const document = types.parseAttackPresetDocument({ version: 1, presets: [{ id: 'p', name: 'P', waves: [{ L: lane([[2, 3]]), M: lane([]), R: lane([]) }] }] });
  const report = evaluate({ slots: [{ slot: 'nomad', ref: { source: 'preset', presetId: 'p', missing: false } }] }, { document });
  assert.equal(stateOf(report, 'composition', 'nomad'), 'valid');
  assert.deepEqual(report.checks.find((check) => check.id === 'composition').params, { waves: 1, troops: 3, tools: 0 });
});

test('inventory reaches unavailable, blocked, pending and valid', () => {
  assert.equal(stateOf(evaluate({}, { observation: { session: SESSION, connected: false } }), 'inventory', 'nomad'), 'unavailable');
  assert.equal(stateOf(evaluate({ sourceCastleId: 9 }), 'inventory', 'nomad'), 'blocked', 'an observed empty castle is missing troops, not unknown');
  assert.equal(stateOf(evaluate({ slots: [{ slot: 'nomad', ref: inline([[1, 5], [3, 1]]) }] }), 'inventory', 'nomad'), 'blocked');
  assert.equal(stateOf(evaluate({ slots: [{ slot: 'nomad', ref: inline([[1, 5]], [[500, 99]]) }] }), 'inventory', 'nomad'), 'pending');
  assert.equal(stateOf(evaluate({ slots: [{ slot: 'nomad', ref: inline([[1, 100], [2, 5]], [[500, 10]]) }] }), 'inventory', 'nomad'), 'valid');
  const family = inline([[1, 5]]);
  family.setup.useTroopFamilies = true;
  assert.equal(stateOf(evaluate({ slots: [{ slot: 'nomad', ref: family }] }, { metadataReady: false }), 'inventory', 'nomad'), 'unavailable');
});

test('difficulty reaches unavailable, blocked, pending and valid', () => {
  const run = (difficulties) => stateOf(evaluate({}, { difficulties }), 'difficulty');
  assert.equal(run({ selections: [{ eventId: 72, available: false }], achievementsObserved: false, loading: true }), 'unavailable');
  assert.equal(run({ selections: [{ eventId: 72, available: true }, { eventId: 80, available: false }], achievementsObserved: true, loading: false }), 'blocked');
  assert.equal(run({ selections: [{ eventId: 72, available: true }], achievementsObserved: false, loading: false }), 'pending');
  assert.equal(run({ selections: [{ eventId: 72, available: true }], achievementsObserved: true, loading: false }), 'valid');
  assert.equal(evaluate().checks.some((check) => check.id === 'difficulty'), false, 'omitted input skips the check');
});

test('score-target, commanders, daily-limit and travel boost reach their states', () => {
  assert.equal(stateOf(evaluate({ scoreTarget: 0 }), 'score-target'), 'blocked');
  assert.equal(stateOf(evaluate({ scoreTarget: 1 }), 'score-target'), 'valid');

  assert.equal(stateOf(evaluate({}, { state: gameState({ commanders: {} }) }), 'commanders'), 'unavailable');
  assert.equal(stateOf(evaluate({}, { state: gameState({ commanders: { 1: { id: 1, available: false, equipment: {}, gems: {} } } }) }), 'commanders'), 'pending');
  assert.equal(stateOf(evaluate(), 'commanders'), 'valid');
  assert.equal(stateOf(evaluate(), 'commander-assignment'), 'pending');
  assert.equal(evaluate().checks.find((check) => check.id === 'commander-assignment').fix, 'assignment');

  assert.equal(stateOf(evaluate({ dailyAttackLimit: 0 }), 'daily-limit'), 'valid');
  assert.equal(stateOf(evaluate({ dailyAttackLimit: 5 }, { state: gameState({ dailyAttacks: { count: 0, serverThreshold: 0, growthRate: 0, observedAt: '0001-01-01T00:00:00Z' } }) }), 'daily-limit'), 'unavailable');
  assert.equal(stateOf(evaluate({ dailyAttackLimit: 3 }), 'daily-limit'), 'blocked');
  assert.equal(stateOf(evaluate({ dailyAttackLimit: 4 }), 'daily-limit'), 'valid');

  assert.deepEqual(evaluate({ horseTravelBoostId: -1 }).checks.find((check) => check.id === 'horse-travel-boost').params, { boost: 'feather' });
  assert.deepEqual(evaluate({ horseTravelBoostId: 1007 }).checks.find((check) => check.id === 'horse-travel-boost').params, { boost: 'coins' });
  assert.deepEqual(evaluate({ horseTravelBoostId: 1009 }).checks.find((check) => check.id === 'horse-travel-boost').params, { boost: 'rubies' });
});

test('tool compatibility is never claimed valid', () => {
  for (const report of [evaluate(), evaluate({}, { state: null }), evaluate({ slots: [] })]) {
    assert.equal(stateOf(report, 'tool-compatibility'), 'pending');
  }
});

test('Berimond Gallantry booster is pending when unobserved, blocked when inactive, valid when active or not required', () => {
  const beri = (draft, state) => readiness.evaluateEventAttackReadiness({
    featureId: 'autoBeriWorld',
    draft: { sourceCastleId: 7, slots: [{ slot: 'attack', ref: inline([[1, 1]]) }], ...draft },
    state: state ?? gameState(),
    document: emptyDocument,
    troops,
    tools,
    metadataReady: true,
    now: NOW,
    observation: LIVE,
  });
  assert.equal(stateOf(beri({ requireActiveGallantryBooster: true }, gameState({ market: {} })), 'gallantry-booster'), 'pending');
  assert.equal(stateOf(beri({ requireActiveGallantryBooster: true }, gameState({ market: { boostersObservedAt: 'x', boosters: {} } })), 'gallantry-booster'), 'blocked');
  assert.equal(stateOf(beri({ requireActiveGallantryBooster: true }, gameState({ market: { boostersObservedAt: 'x', boosters: { 24: { expiresAt: '2026-09-29T11:00:00Z' } } } })), 'gallantry-booster'), 'blocked');
  assert.equal(stateOf(beri({ requireActiveGallantryBooster: true }, gameState({ market: { boostersObservedAt: 'x', boosters: { 24: { permanent: true } } } })), 'gallantry-booster'), 'valid');
  assert.equal(stateOf(beri({ requireActiveGallantryBooster: true }), 'gallantry-booster'), 'valid');
  assert.equal(stateOf(beri({ requireActiveGallantryBooster: false }, gameState({ market: {} })), 'gallantry-booster'), 'valid');
});

test('Berimond inventory is decided at launch because the camp, not the source castle, supplies the attack', () => {
  const beri = (ref) => readiness.evaluateEventAttackReadiness({
    featureId: 'autoBeriWorld',
    draft: { sourceCastleId: 7, slots: [{ slot: 'attack', ref }] },
    state: gameState(),
    document: emptyDocument,
    troops,
    tools,
    metadataReady: true,
    now: NOW,
    observation: LIVE,
  });
  assert.equal(stateOf(beri(inline([[1, 5]], [[614, 20]])), 'inventory', 'attack'), 'pending', 'coin tools are bought in the camp');
  assert.equal(stateOf(beri(inline([[3, 5]])), 'inventory', 'attack'), 'pending');
  assert.equal(stateOf(beri(inline([[1, 5]])), 'inventory', 'attack'), 'valid');
});

test('Invasion fortification is reported only when chosen and Rubies are never defaulted', () => {
  assert.equal(invasion.defaultAutoInvasionClientState().fortifyCurrency, '');
  const draft = { fortifyCurrency: invasion.defaultAutoInvasionClientState().fortifyCurrency };
  const off = readiness.evaluateEventAttackReadiness({
    featureId: 'autoInvasion', draft: { sourceCastleId: 7, slots: [], ...draft }, state: gameState(), document: emptyDocument, troops, tools, metadataReady: true, now: NOW, observation: LIVE,
  });
  assert.equal(off.checks.some((check) => check.id === 'fortify-currency'), false);
  assert.equal(off.checks.some((check) => JSON.stringify(check.params ?? {}).includes('C2')), false);
  for (const currency of ['GTO', 'MEDALS', 'C2']) {
    const report = readiness.evaluateEventAttackReadiness({
      featureId: 'autoInvasion', draft: { sourceCastleId: 7, slots: [], fortifyCurrency: currency }, state: gameState(), document: emptyDocument, troops, tools, metadataReady: true, now: NOW, observation: LIVE,
    });
    const check = report.checks.find((candidate) => candidate.id === 'fortify-currency');
    assert.equal(check.state, 'pending');
    assert.deepEqual(check.params, { currency }, 'the explicit choice is reported, never replaced');
  }
});

test('overall is the worst state: blocked > unavailable > pending > valid', () => {
  const check = (state) => ({ id: state, state, messageKey: 'readiness.state' });
  assert.equal(model.aggregateReadiness([]), 'valid');
  assert.equal(model.aggregateReadiness([check('valid'), check('pending')]), 'pending');
  assert.equal(model.aggregateReadiness([check('pending'), check('unavailable'), check('valid')]), 'unavailable');
  assert.equal(model.aggregateReadiness([check('unavailable'), check('blocked'), check('pending')]), 'blocked');
  assert.equal(evaluate({ sourceCastleId: 0 }).overall, 'blocked');
  assert.equal(evaluate().overall, 'pending', 'runtime-decided checks keep a complete draft at pending, never a ready claim');
});

test('evaluation has no side effects on its inputs', () => {
  const draft = { sourceCastleId: 7, slots: [{ slot: 'nomad', ref: inline([[1, 5]]) }], dailyAttackLimit: 2, fortifyCurrency: 'GTO' };
  const state = gameState();
  const before = JSON.stringify([draft, state]);
  readiness.evaluateEventAttackReadiness({ featureId: 'autoNomad', draft, state, document: emptyDocument, troops, tools, metadataReady: true, now: NOW, observation: LIVE });
  assert.equal(JSON.stringify([draft, state]), before);
});

test('inventory is unavailable, with the reason, whenever unit counts are not current (D1, real projection shape)', () => {
  const cases = [
    [{ session: SESSION, connected: false }, 'ui.components.staleSessionBanner.disconnected.last.known.data.166a8c99'],
    [{ session: { ...SESSION, baselineGeneration: 24 }, connected: true }, null],
    [{ session: { ...SESSION, generation: 0, baselineGeneration: 0 }, connected: true }, null],
    [{ session: SESSION, connected: true, hostedPresence: { mode: 'checkpoint', checkpointObservedAt: '2026-09-29T08:00:00Z' } }, null],
    [{ session: null, connected: true }, null],
  ];
  for (const [observation, key] of cases) {
    const check = evaluate({}, { observation }).checks.find((entry) => entry.id === 'inventory');
    assert.equal(check.state, 'unavailable', JSON.stringify(observation));
    assert.equal(check.fix, 'connection');
    assert.ok(messages[check.messageKey], check.messageKey);
    if (key) assert.equal(check.messageKey, key);
  }
  const live = evaluate().checks.find((entry) => entry.id === 'inventory');
  assert.equal(live.state, 'valid', 'on a live, baselined session the sentinel no longer hides stock; 50 of 100 stationed is enough');
  const valid = evaluate({ slots: [{ slot: 'nomad', ref: inline([[1, 100]]) }] }).checks.find((entry) => entry.id === 'inventory');
  assert.equal(valid.state, 'valid');
  assert.match(messages[valid.messageKey], /the game does not report when each castle was last checked/);
  assert.doesNotMatch(messages[valid.messageKey], /runtime/);
  const timed = evaluate({ slots: [{ slot: 'nomad', ref: inline([[1, 100]]) }] }, {
    state: gameState({ castles: { 7: { ...gameState().castles[7], unitsObservedAt: '2026-09-29T10:00:00Z' } } }),
  }).checks.find((entry) => entry.id === 'inventory');
  assert.equal(timed.messageKey, 'eventAttackReadiness.inventoryObservedAt');
  const stale = evaluate({}, {
    state: gameState({ castles: { 7: { ...gameState().castles[7], unitsObservedAt: '2026-09-29T08:00:00Z' } } }),
  }).checks.find((entry) => entry.id === 'inventory');
  assert.equal(stale.state, 'unavailable', 'a real timestamp before the connection changed is stale');
});

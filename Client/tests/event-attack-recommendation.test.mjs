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
const recommendation = await vite.ssrLoadModule('/src/settings/onboarding/EventAttackRecommendation.ts');
const recipes = await vite.ssrLoadModule('/src/settings/onboarding/StarterRecipes.ts');
const { sourceMessages } = await vite.ssrLoadModule('/src/i18n/sourceMessages.ts');

after(async () => {
  await vite.close();
});

const troops = {
  ...Object.fromEntries([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12].map((id) => [id, { id, name: `Unit ${id}`, meleeAttack: 50, meleeDefence: 20 }])),
  // Shield-maiden-like defender: best defence above best attack.
  20: { id: 20, name: 'Defender', meleeAttack: 10, rangeAttack: 5, meleeDefence: 40, rangeDefence: 45 },
  // Ranged attacker whose melee defence is high but ranged attack is higher.
  21: { id: 21, name: 'Ranged attacker', rangeAttack: 60, meleeDefence: 60 },
};
const tools = { 500: { id: 500, name: 'Ladder' } };

// Real projection shape: every castle carries the Go zero time; freshness comes from the session.
const ZERO_TIME = '0001-01-01T00:00:00Z';
const SESSION = { generation: 25, baselineGeneration: 25, connectionGeneration: 3, status: 'ready', loggedIn: true, socketReady: true, changedAt: '2026-09-29T09:00:00Z' };
const LIVE = { session: SESSION, connected: true };

function castle(stationed) {
  return {
    id: 7,
    kingdomId: 0,
    name: 'Main',
    x: 1,
    y: 2,
    units: { stationed, traveling: {}, hospital: {}, specialHospital: {}, total: {} },
    unitsObservedAt: ZERO_TIME,
  };
}

function recommend(input) {
  return recommendation.recommendEventAttackSetup({ troops, tools, metadataReady: true, eventId: 72, observation: LIVE, ...input });
}

test('no source castle yields a specific requirement and no setup', () => {
  const result = recommend({ sourceCastle: null });
  assert.equal(result.setup, null);
  assert.deepEqual(result.requirements.map((requirement) => requirement.id), ['source-castle']);
  assert.equal(result.requirements[0].fix, 'settings');
  assert.ok(sourceMessages[result.requirements[0].messageKey]);
});

test('counts that are not current yield a reason-specific requirement, never a setup (D1)', () => {
  const cases = [
    [{ session: SESSION, connected: false }, 'ui.components.staleSessionBanner.disconnected.last.known.data.166a8c99'],
    [{ session: { ...SESSION, baselineGeneration: 24 }, connected: true }, null],
    [{ session: SESSION, connected: true, hostedPresence: { mode: 'checkpoint' } }, null],
  ];
  const keys = new Set();
  for (const [observation, key] of cases) {
    const result = recommend({ sourceCastle: castle({ 1: 500 }), observation });
    assert.equal(result.setup, null, JSON.stringify(observation));
    assert.deepEqual(result.requirements.map((requirement) => requirement.id), ['units-not-observed']);
    assert.equal(result.requirements[0].fix, 'connection');
    assert.ok(sourceMessages[result.requirements[0].messageKey], result.requirements[0].messageKey);
    if (key) assert.equal(result.requirements[0].messageKey, key);
    keys.add(result.requirements[0].messageKey);
  }
  assert.equal(keys.size, 3, 'each reason has its own message');
  const stale = recommend({ sourceCastle: { ...castle({ 1: 500 }), unitsObservedAt: '2026-09-29T08:00:00Z' } });
  assert.deepEqual(stale.requirements.map((requirement) => requirement.id), ['units-not-observed']);
  assert.ok(recommend({ sourceCastle: castle({ 1: 500 }) }).setup, 'a live, baselined session resolves a setup despite the sentinel');
});

test('unready metadata yields a requirement', () => {
  const result = recommend({ sourceCastle: castle({ 1: 500 }), metadataReady: false });
  assert.equal(result.setup, null);
  assert.deepEqual(result.requirements.map((requirement) => requirement.id), ['metadata-unavailable']);
});

test('a castle without stationed attack troops yields a requirement', () => {
  const result = recommend({ sourceCastle: castle({ 500: 40, 999: 3, 1: 0 }) });
  assert.equal(result.setup, null);
  assert.deepEqual(result.requirements.map((requirement) => requirement.id), ['no-stationed-troops']);
});

test('stationed troops fill one wave deterministically under the lane slot caps with no tools', () => {
  const stationed = { 1: 100, 2: 900, 3: 900, 4: 50, 5: 10, 6: 400, 7: 30, 8: 20, 9: 60, 10: 70, 11: 80, 12: 5, 500: 99999 };
  const result = recommend({ sourceCastle: castle(stationed) });
  assert.deepEqual(result.requirements, []);
  assert.equal(result.setup.waves.length, 1);
  assert.equal(result.setup.targetType, 'pve');
  assert.equal(result.setup.useTroopFamilies, false);
  const [wave] = result.setup.waves;
  assert.equal(wave.L.troops.length, 2);
  assert.equal(wave.M.troops.length, 6);
  assert.equal(wave.R.troops.length, 2);
  const order = [...wave.L.troops, ...wave.M.troops, ...wave.R.troops].map((slot) => [slot.itemId, slot.quantity]);
  assert.deepEqual(order, [[2, 900], [3, 900], [6, 400], [1, 100], [11, 80], [10, 70], [9, 60], [4, 50], [7, 30], [8, 20]]);
  for (const laneKey of ['L', 'M', 'R']) {
    assert.ok(wave[laneKey].tools.every((slot) => slot.itemId == null && slot.quantity === 0), laneKey);
  }
  assert.equal(wave.L.tools.length + wave.M.tools.length + wave.R.tools.length, 7);
  assert.ok(result.setup.courtyardSupport.troops.every((slot) => slot.itemId == null));
  assert.ok(result.setup.courtyardSupport.tools.every((slot) => slot.itemId == null));
  for (const [itemId, quantity] of order) assert.ok(quantity <= stationed[itemId], 'never above stationed counts');
  assert.deepEqual(recommend({ sourceCastle: castle(stationed) }).setup, result.setup, 'deterministic');
});

test('fewer troop types than slots leave the remaining slots empty', () => {
  const result = recommend({ sourceCastle: castle({ 4: 10 }) });
  const [wave] = result.setup.waves;
  assert.deepEqual(wave.L.troops, [{ itemId: 4, quantity: 10 }, { itemId: null, quantity: 0 }]);
  assert.ok(wave.M.troops.every((slot) => slot.itemId == null));
});

test('defensive units are never proposed, using the troop picker role rule', () => {
  const result = recommend({ sourceCastle: castle({ 20: 99999, 21: 10, 4: 5 }) });
  const [wave] = result.setup.waves;
  const ids = [...wave.L.troops, ...wave.M.troops, ...wave.R.troops].map((slot) => slot.itemId).filter((id) => id != null);
  assert.deepEqual(ids, [21, 4]);
  assert.equal(recommend({ sourceCastle: castle({ 20: 500 }) }).requirements[0].id, 'no-stationed-troops');
});

test('pending starter reviews are reported with the recommendation', () => {
  const result = recommend({ sourceCastle: castle({ 1: 1 }) });
  assert.deepEqual(result.pendingReviews, recipes.pendingStarterReviews());
  assert.ok(result.pendingReviews.length > 0);
  assert.ok(result.notes.length >= 3);
  for (const key of result.notes) assert.ok(sourceMessages[key], key);
  assert.deepEqual(result.resolvedFor, { sourceCastleId: 7, eventId: 72 });
});

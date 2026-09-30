import assert from 'node:assert/strict';
import { after, beforeEach, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const goals = await vite.ssrLoadModule('/src/settings/onboarding/goals.ts');
const store = await vite.ssrLoadModule('/src/settings/onboarding/goalStore.ts');
const { SETTINGS_PLACEMENT, AUTOMATION_ENABLED_KEYS } = await vite.ssrLoadModule('/src/settings/disclosure/placement.ts');
const { messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');

after(async () => {
  await vite.close();
});
beforeEach(() => store.resetGoalMemoryForTests());

test('there is exactly one goal per catalog feature, and every title and outcome exists', () => {
  const features = Object.keys(SETTINGS_PLACEMENT).sort();
  assert.deepEqual(goals.AUTOMATION_GOALS.map((goal) => goal.featureId).sort(), features);
  assert.equal(new Set(goals.AUTOMATION_GOALS.map((goal) => goal.id)).size, goals.AUTOMATION_GOALS.length);
  assert.equal(goals.AUTOMATION_GOALS.length, 19);
  for (const goal of goals.AUTOMATION_GOALS) {
    assert.equal(goal.id, goal.featureId);
    assert.ok(messages[goal.titleKey], goal.titleKey);
    assert.ok(messages[goal.outcomeKey], goal.outcomeKey);
    assert.ok(AUTOMATION_ENABLED_KEYS[goal.featureId], `${goal.featureId} has a switch`);
    assert.ok(goals.GOAL_SAVED_SECTION[goal.featureId], `${goal.featureId} has a saved section`);
    assert.doesNotMatch(`${messages[goal.titleKey]} ${messages[goal.outcomeKey]}`, /runtime/i, goal.id);
  }
});

test('the eight curated goals are in the accepted order with the accepted titles and outcomes', () => {
  const curated = goals.AUTOMATION_GOALS.filter((goal) => goal.curated);
  assert.deepEqual(curated.map((goal) => goal.featureId), ['autoTowers', 'autoNomad', 'autoInvasion', 'autoKhan', 'autoFoodBalance', 'autoStation', 'autoRecruit', 'autoHospital']);
  assert.deepEqual(goals.AUTOMATION_GOALS.slice(0, 8).map((goal) => goal.curated), Array(8).fill(true), 'curated goals are listed first');
  const expected = {
    autoTowers: ['Attack towers near my castles', 'Auto Towers scans around each castle you enable and attacks the nearest eligible towers with the troop you choose.'],
    autoNomad: ['Play the Nomad or Samurai event', 'Auto Nomad / Samurai attacks the event camps from your source castle and chains attacks until your score target.'],
    autoInvasion: ['Play the Invasion event', 'Auto Invasion attacks Foreign Lords or Bloodcrows targets at the difficulties you unlock, and stops at your score.'],
    autoKhan: ['Play the Khan event and defend my main castle', 'Auto Khan chains camp attacks and re-applies your main-castle defense between waves.'],
    autoFoodBalance: ['Keep every castle fed', 'Auto Food Balance moves food from castles above your reserve to castles that need it.'],
    autoStation: ['Keep my troops safe when attacked', 'Auto Station moves troops out before an incoming attack lands and brings them back when it is clear.'],
    autoRecruit: ['Keep recruiting troops', 'Recruit Troops queues the units and schedules you set at each castle.'],
    autoHospital: ['Heal wounded troops automatically', 'Auto Hospital heals wounded troops in the windows you allow.'],
  };
  for (const goal of curated) {
    assert.equal(messages[goal.titleKey], expected[goal.featureId][0], goal.id);
    assert.equal(messages[goal.outcomeKey], expected[goal.featureId][1], goal.id);
  }
  const more = goals.AUTOMATION_GOALS.filter((goal) => !goal.curated);
  assert.equal(more.length, 11);
  for (const goal of more) assert.match(messages[goal.titleKey], /^Auto /, `${goal.id}: the feature name is the title`);
});

test('a saved section counts only when it holds a value', () => {
  assert.equal(goals.isSavedSection(undefined), false);
  assert.equal(goals.isSavedSection({}), false);
  assert.equal(goals.isSavedSection([]), false);
  assert.equal(goals.isSavedSection({ version: 1 }), true);
});

const memoryStorage = () => {
  const map = new Map();
  return { map, getItem: (key) => map.get(key) ?? null, setItem: (key, value) => { map.set(key, value); }, removeItem: (key) => { map.delete(key); } };
};

test('the chosen goal is per account and world, holds only a pointer, and survives storage failures', () => {
  const storage = memoryStorage();
  assert.equal(store.readGoal('77:EmpireEx_2', storage), null);
  store.writeGoal('77:EmpireEx_2', { version: 1, goalId: 'autoTowers', startedAt: '2026-09-29T10:00:00Z' }, storage);
  assert.equal(store.readGoal('77:EmpireEx_2', storage).goalId, 'autoTowers');
  assert.equal(store.readGoal('78:EmpireEx_2', storage), null, 'an account switch shows no goal');
  assert.equal(store.readGoal('77:EmpireEx_3', storage), null, 'another world too');
  assert.deepEqual(Object.keys(JSON.parse(storage.map.get('citadelops.goal.v1.77:EmpireEx_2'))).sort(), ['goalId', 'startedAt', 'version'], 'no configuration, credentials or progress');
  storage.setItem('citadelops.goal.v1.77:EmpireEx_2', JSON.stringify({ version: 1, goalId: 'notAGoal' }));
  assert.equal(store.readGoal('77:EmpireEx_2', storage), null, 'an unknown goal id is ignored');
  store.clearGoal('77:EmpireEx_2', storage);
  const broken = { getItem() { throw new Error('blocked'); }, setItem() { throw new Error('quota'); }, removeItem() { throw new Error('blocked'); } };
  assert.equal(store.writeGoal('77:EmpireEx_2', { version: 1, goalId: 'autoFoodBalance', startedAt: '' }, broken), false);
  assert.equal(store.readGoal('77:EmpireEx_2', broken).goalId, 'autoFoodBalance', 'held in memory for this visit');
  assert.doesNotThrow(() => store.clearGoal('77:EmpireEx_2', broken));
  assert.equal(store.readGoal('77:EmpireEx_2', broken), null);
});

test('a goal chosen before any account is known is adopted by the first account, never moved between known accounts', () => {
  const storage = memoryStorage();
  store.writeGoal('', { version: 1, goalId: 'autoStation', startedAt: '' }, storage);
  assert.equal(store.readGoal('', storage).goalId, 'autoStation');
  store.adoptPendingGoal('77:EmpireEx_2', storage);
  assert.equal(store.readGoal('77:EmpireEx_2', storage).goalId, 'autoStation');
  assert.equal(store.readGoal('', storage), null, 'the pending record is consumed');
  store.writeGoal('', { version: 1, goalId: 'autoHospital', startedAt: '' }, storage);
  store.adoptPendingGoal('78:EmpireEx_2', storage);
  assert.equal(store.readGoal('78:EmpireEx_2', storage).goalId, 'autoHospital');
  store.writeGoal('', { version: 1, goalId: 'autoTowers', startedAt: '' }, storage);
  store.adoptPendingGoal('77:EmpireEx_2', storage);
  assert.equal(store.readGoal('77:EmpireEx_2', storage).goalId, 'autoStation', 'an account that already chose a goal keeps it');
});

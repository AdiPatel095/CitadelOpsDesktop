import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const root = fileURLToPath(new URL('..', import.meta.url));
const source = existsSync(`${root}/src/commandCenter`) ? '/src/commandCenter' : '/src';
const vite = await createServer({ root, configFile: false, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true, hmr: false } });
const { automationsActingOnCastle } = await vite.ssrLoadModule(`${source}/settings/castleAutomations.ts`);
const { GOAL_SAVED_SECTION } = await vite.ssrLoadModule(`${source}/settings/onboarding/goals.ts`);
after(() => vite.close());

const cases = [
  ['autoTowers', { castles: { 42: { enabled: true } } }],
  ['autoRecruit', { castles: { 42: { enabled: false, items: [{ id: 1, amount: 5 }] } } }],
  ['autoTool', { castles: { 42: { items: [{ id: 500, amount: 5 }] } } }],
  ['autoSceatRes', { castles: { 42: { buildings: { 1: { enabled: false, steps: [{ recipeID: 5, repeat: 1 }] } } } } }],
  ['autoStation', { settings: { 42: [{ id: 1, amount: 5 }] } }],
  ['autoBird', { ignoreSettings: { settings: { 42: [{ id: 1, amount: 5 }] } } }],
  ...['autoKhan', 'autoNomad', 'autoInvasion', 'autoAdvisor', 'autoBeriWorld', 'autoBuyer'].map((feature) => [feature, { sourceCastleId: 42 }]),
  ['autoStorm', { unlock: { enabled: true, prebuiltCastleId: 42 } }],
];

for (const [feature, saved] of cases) {
  test(`${feature} lists only the castle named by its saved settings`, () => {
    const sections = { [GOAL_SAVED_SECTION[feature]]: saved };
    const before = structuredClone(sections);
    assert.deepEqual(automationsActingOnCastle(sections, '42'), [feature]);
    assert.deepEqual(automationsActingOnCastle(sections, 43), []);
    assert.deepEqual(sections, before);
  });
}

test('disabled Towers and empty or invalid planned items do not count', () => {
  const sections = {
    'automation.autoTowers': { castles: { 42: { enabled: false } } },
    'automation.recruitTroops': { castles: { 42: { enabled: true, items: [] } } },
    'automation.autoTool': { castles: { 42: { items: [{ id: 0, amount: 5 }] } } },
    'automation.autoSceatResources': { castles: { 42: { buildings: { 1: { steps: [{ recipeID: 0 }] } } } } },
    'automation.autoStation': { settings: { 42: [] } },
    'automation.autoBird': { ignoreSettings: { settings: { 42: [] } } },
  };
  assert.deepEqual(automationsActingOnCastle(sections, 42), []);
});

test('Bird uses active preset rows, falling back to ignore settings when the preset is missing', () => {
  const saved = { activePresetId: 'active', ignoreSettings: { settings: { 43: [{ id: 1, amount: 5 }] } },
    presets: { version: 1, presets: [{ id: 'active', name: 'Saved', settings: { 42: [{ id: 1, amount: 5 }] } }, { id: 'unused', name: 'Other', settings: { 44: [{ id: 1, amount: 5 }] } }] } };
  const sections = { 'automation.autoBird': saved };
  assert.deepEqual(automationsActingOnCastle(sections, 42), ['autoBird']);
  assert.deepEqual(automationsActingOnCastle(sections, 43), []);
  assert.deepEqual(automationsActingOnCastle(sections, 44), []);
  saved.activePresetId = 'missing';
  assert.deepEqual(automationsActingOnCastle(sections, 43), ['autoBird']);
});

test('Buyer includes its feast source and lists a feature only once', () => {
  assert.deepEqual(automationsActingOnCastle({ 'automation.autoBuyer': { sourceCastleId: 43, feast: { sourceCastleId: 42 } } }, 42), ['autoBuyer']);
  assert.deepEqual(automationsActingOnCastle({ 'automation.autoBuyer': { sourceCastleId: 42, feast: { sourceCastleId: 42 } } }, 42), ['autoBuyer']);
});

test('Storm requires enabled unlock or a named decoration preset, ignoring unrelated castle fields', () => {
  const check = (saved) => automationsActingOnCastle({ 'automation.autoStorm': saved }, 42);
  assert.deepEqual(check({ unlock: { enabled: false, prebuiltCastleId: 42 } }), []);
  assert.deepEqual(check({ decorationPresetCastleId: 42, decorationPresetId: '' }), []);
  assert.deepEqual(check({ decorationPresetCastleId: 42, decorationPresetId: 'saved-preset' }), ['autoStorm']);
  assert.deepEqual(check({ sourceCastleId: 42 }), []);
});

test('account-wide Food Balance, Hospital, Fortress and unknown features never appear', () => {
  const namesCastle = { sourceCastleId: 42, castles: { 42: { enabled: true } }, settings: { 42: [{ id: 1, amount: 5 }] } };
  assert.deepEqual(automationsActingOnCastle(Object.fromEntries(['autoFoodBalance', 'autoHospital', 'autoFortress', 'unknown'].map((feature) => [`automation.${feature}`, namesCastle])), 42), []);
});

test('unknown or malformed sections and castle ids are ignored', () => {
  for (const value of [undefined, null, [], 'invalid', 42, {}]) {
    const sections = Object.fromEntries(Object.values(GOAL_SAVED_SECTION).map((key) => [key, value]));
    assert.deepEqual(automationsActingOnCastle(sections, 42), []);
  }
  assert.deepEqual(automationsActingOnCastle({ 'automation.autoBird': { ignoreSettings: { settings: { 42: 'invalid' } } } }, 42), []);
  const sections = Object.fromEntries(cases.map(([feature, saved]) => [GOAL_SAVED_SECTION[feature], saved]));
  for (const id of [0, -1, '', 'invalid', 42.5, Number.POSITIVE_INFINITY]) assert.deepEqual(automationsActingOnCastle(sections, id), []);
});

test('feature order is fixed independently of saved-section insertion order', () => {
  const sections = Object.fromEntries([...cases].reverse().map(([feature, saved]) => [GOAL_SAVED_SECTION[feature], saved]));
  assert.deepEqual(automationsActingOnCastle(sections, 42), cases.map(([feature]) => feature));
});

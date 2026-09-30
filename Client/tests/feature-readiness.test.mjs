import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const readiness = await vite.ssrLoadModule('/src/settings/readiness/featureReadiness.ts');
const { SETTINGS_PLACEMENT } = await vite.ssrLoadModule('/src/settings/disclosure/placement.ts');
const { messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');

after(async () => {
  await vite.close();
});

const FEATURES = Object.keys(SETTINGS_PLACEMENT);
const SESSION = { generation: 25, baselineGeneration: 25, changedAt: '2026-09-29T09:00:00Z' };
const LIVE = { session: SESSION, connected: true };
const ZERO = '0001-01-01T00:00:00Z';
const troops = { 1: { id: 1, name: 'Attacker', meleeAttack: 50, meleeDefence: 10 }, 277: { id: 277, name: 'Direwolf' } };
const castle = (id, kingdomId, slotType, stationed = {}) => ({ id, kingdomId, slotType, name: `C${id}`, units: { stationed }, unitsObservedAt: ZERO, resources: {}, buildings: {} });
const inputs = (extra = {}) => ({
  sections: undefined, state: null, observation: LIVE, troops, tools: {}, metadataReady: true,
  movement: null, gameLoggedIn: true, now: Date.parse('2026-09-29T12:00:00Z'), ...extra,
});
const ids = (report) => report.checks.map((check) => check.id);
const state = (castles) => ({ castles, commanders: {}, market: {}, kingdomTransport: { unlocks: {} }, player: { achievements: {} }, session: SESSION });

test('every automation gets a report, and no report reads ready from absent data', () => {
  for (const featureId of FEATURES) {
    const report = readiness.evaluateFeatureReadiness(featureId, inputs());
    assert.equal(report.featureId, featureId);
    assert.ok(report.checks.length > 0, featureId);
    assert.notEqual(report.overall, 'valid', `${featureId}: nothing observed and nothing saved is never ready`);
    for (const check of report.checks) assert.ok(messages[check.messageKey], `${featureId}/${check.id}: ${check.messageKey}`);
    for (const line of report.plan ?? []) assert.ok(messages[line.messageKey], `${featureId}/${line.id}`);
  }
});

test('modules with an evaluator read the SAVED section, not a draft', () => {
  const sections = { 'automation.autoTowers': { version: 4, castles: { 1: { enabled: true, unitId: 1, radius: 10, maidenOnly: false } } } };
  const ready = readiness.evaluateFeatureReadiness('autoTowers', inputs({ sections, state: state({ 1: castle(1, 0, 1, { 1: 50 }) }) }));
  assert.equal(ready.checks.find((check) => check.id === 'enabled-castles').state, 'valid');
  assert.equal(ready.checks.find((check) => check.id === 'inventory').state, 'valid');
  const unsaved = readiness.evaluateFeatureReadiness('autoTowers', inputs({ sections: {}, state: state({ 1: castle(1, 0, 1, { 1: 50 }) }) }));
  assert.equal(unsaved.checks.find((check) => check.id === 'enabled-castles').state, 'blocked');
  assert.equal(unsaved.overall, 'blocked');
});

test('event attacks: a missing preset blocks; the difficulty is decided at launch until the catalogs load', () => {
  const sections = {
    'automation.autoInvasion': { version: 1, sourceCastleId: 5, presetId: 'gone', foreignLordsDifficultyId: 3, bloodcrowDifficultyId: 4, scoreTarget: 100 },
    'attacks.presets': { version: 1, presets: [] },
  };
  const world = state({ 5: castle(5, 0, 1, { 1: 100 }) });
  const withoutCatalog = readiness.evaluateFeatureReadiness('autoInvasion', inputs({ sections, state: world }));
  assert.equal(withoutCatalog.overall, 'blocked');
  assert.equal(withoutCatalog.checks.find((check) => check.id === 'difficulty').state, 'pending');
  assert.equal(withoutCatalog.checks.find((check) => check.id === 'difficulty').messageKey, 'featureReadiness.difficultyAtLaunch');
  const catalogs = { optionsByEvent: { 71: [{ value: '3' }], 103: [] }, achievementsObserved: true, loading: false };
  const withCatalog = readiness.evaluateFeatureReadiness('autoInvasion', inputs({ sections, state: world, difficulties: catalogs }));
  assert.equal(withCatalog.checks.find((check) => check.id === 'difficulty').state, 'blocked', 'an unlocked difficulty is required for every event');
});

test('Recruit and Tool: a saved production plan is required; nothing is valid from absence', () => {
  const none = readiness.evaluateFeatureReadiness('autoRecruit', inputs({ sections: {} }));
  assert.equal(none.checks.find((check) => check.id === 'saved-settings').state, 'pending');
  const empty = readiness.evaluateFeatureReadiness('autoRecruit', inputs({ sections: { 'automation.recruitTroops': { version: 1, mode: 'global', globalItems: [], castles: {} } } }));
  assert.equal(empty.checks.find((check) => check.id === 'plan').state, 'blocked');
  const planned = readiness.evaluateFeatureReadiness('autoRecruit', inputs({ sections: { 'automation.recruitTroops': { version: 1, mode: 'global', globalItems: [{ id: 1, amount: 5 }], castles: {} } } }));
  assert.equal(planned.checks.find((check) => check.id === 'plan').state, 'valid');
  assert.equal(planned.checks.find((check) => check.id === 'saved-settings').state, 'valid');
  const tool = readiness.evaluateFeatureReadiness('autoTool', inputs({ sections: { 'automation.autoTool': { version: 1, mode: 'global', globalItems: [], castles: {} } } }));
  assert.equal(tool.checks.find((check) => check.id === 'plan').state, 'blocked');
});

test('minimal readiness: a closed weekly schedule is pending, spending permission is stated, castle data is awaited', () => {
  const weekly = { enabled: true, timeZone: 'UTC', slotOptionsEnabled: false, slots: [{ day: 1, startMinute: 0, endMinute: 60 }] };
  const now = Date.parse('2026-09-29T12:00:00Z'); // a Tuesday
  const closed = readiness.evaluateFeatureReadiness('autoBooster', inputs({ sections: { 'automation.autoBooster': { version: 1, minimumRubyReserve: 500 } }, schedule: weekly, now }));
  assert.equal(closed.checks.find((check) => check.id === 'schedule').state, 'pending');
  assert.equal(closed.plan[0].messageKey, 'featureReadiness.spendingBooster');
  assert.equal(closed.plan[0].params.reserve, 500);
  assert.equal(closed.checks.find((check) => check.id === 'castles').state, 'unavailable');
  const buyer = readiness.evaluateFeatureReadiness('autoBuyer', inputs({ sections: { 'automation.autoBuyer': { version: 1, allowRubyPackages: true, minimumRubyReserve: 10 } } }));
  assert.equal(buyer.plan[0].params.rubies, 'allowed');
  const noRuby = readiness.evaluateFeatureReadiness('autoSceatRes', inputs({ sections: { 'automation.autoSceatResources': { version: 1 } } }));
  assert.equal(noRuby.plan[0].params.rubies, 'never');
  const equipment = readiness.evaluateFeatureReadiness('autoEquipmentCleanup', inputs({ sections: { 'automation.autoEquipmentCleanup': { version: 1, checkIntervalSec: 60 } } }));
  assert.equal(equipment.checks.some((check) => check.id === 'castles'), false, 'cleanup does not wait for castle data');
});

test('Advisor needs a Great Empire source castle and a preset', () => {
  const report = readiness.evaluateFeatureReadiness('autoAdvisor', inputs({ sections: { 'automation.autoAdvisor': { version: 1, sourceCastleId: 0, presetId: '' } }, state: state({ 1: castle(1, 0, 1) }) }));
  assert.equal(report.checks.find((check) => check.id === 'source-castle').state, 'blocked');
  assert.ok(report.checks.some((check) => check.id === 'composition'));
  assert.equal(report.overall, 'blocked');
});

test('the Start confirmation lists blocked checks and separates what the game decides', () => {
  const report = readiness.evaluateFeatureReadiness('autoTowers', inputs({ sections: { 'automation.autoTowers': { version: 4, castles: { 1: { enabled: true, unitId: 1 } } } }, state: state({ 1: castle(1, 0, 1, {}) }) }));
  assert.ok(readiness.blockedChecks(report).every((check) => check.state === 'blocked'));
  assert.ok(readiness.undecidedChecks(report).every((check) => check.state === 'pending' || check.state === 'unavailable'));
  assert.equal(readiness.blockedChecks(report).length + readiness.undecidedChecks(report).length + report.checks.filter((check) => check.state === 'valid').length, report.checks.length);
});

test('every check id the minimal readiness emits has a fix-target entry (null when there is no control)', async () => {
  const { SETTINGS_FIX_TARGETS } = await vite.ssrLoadModule('/src/settings/disclosure/fixTargets.ts');
  const source = await readFile(new URL('../src/settings/readiness/featureReadiness.ts', import.meta.url), 'utf8');
  const minimal = source.slice(source.indexOf('function productionPlanCheck'), source.indexOf('export function evaluateFeatureReadiness'));
  const emitted = new Set([...minimal.matchAll(/\bid: '([a-z0-9-]+)'/g)].map((match) => match[1]));
  emitted.delete('spending');
  for (const featureId of ['autoRecruit', 'autoTool', 'autoHospital', 'autoTCI', 'autoSceatRes', 'autoBooster', 'autoBuyer', 'autoAdvisor', 'autoEquipmentCleanup']) {
    for (const id of emitted) {
      if (['source-castle', 'composition', 'inventory', 'commander-assignment'].includes(id) && featureId !== 'autoAdvisor') continue;
      assert.ok(Object.hasOwn(SETTINGS_FIX_TARGETS[featureId], id), `${featureId}: check "${id}" has no fix target`);
    }
  }
});

test('the readiness modules are pure: no intents, no configuration writes, no game actions', async () => {
  for (const file of ['featureReadiness.ts', 'runtimeState.ts', 'firstResult.ts', 'stopSemantics.ts', 'automationAttribution.ts']) {
    const source = await readFile(new URL(`../src/settings/readiness/${file}`, import.meta.url), 'utf8');
    assert.doesNotMatch(source, /submitIntent|updateConfiguration|cancelOperation\(|queueConfigurationUpdate|fetch\(|CitadelAPI\./, file);
  }
});

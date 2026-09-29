import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const khan = await vite.ssrLoadModule('/src/settings/readiness/khanReadiness.ts');
const khanState = await vite.ssrLoadModule('/src/settings/AutoKhanClientState.ts');
const defenseTypes = await vite.ssrLoadModule('/src/defensePresets/DefensePresetTypes.ts');
const defenseApp = await vite.ssrLoadModule('/src/defensePresets/AppCreatedDefensePresets.ts');
const starter = await vite.ssrLoadModule('/src/settings/onboarding/KhanDefenseStarter.ts');
const { messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');

after(async () => {
  await vite.close();
});

const ZERO = '0001-01-01T00:00:00Z';
const SESSION = { generation: 5, baselineGeneration: 5, changedAt: '2026-09-29T09:00:00Z' };
const LIVE = { session: SESSION, connected: true };
const troops = { 1: { id: 1, name: 'Attacker', meleeAttack: 50, meleeDefence: 10 } };

function castle(id, slotType, extra = {}) {
  return {
    id, name: `C${id}`, kingdomId: 0, slotType, units: { stationed: { 1: 500 } }, unitsObservedAt: ZERO,
    defense: {
      wall: { left: { toolSlots: [], unitPercent: 33, unitTypePercent: 0 }, middle: { toolSlots: [], unitPercent: 34, unitTypePercent: 0 }, right: { toolSlots: [], unitPercent: 33, unitTypePercent: 0 } },
      moat: { leftToolSlots: [], middleToolSlots: [], rightToolSlots: [] },
      keep: { mauct: 0, unitTypePercent: 50, primaryToolSlots: [], secondaryToolSlots: [] },
      inventory: {}, rangedUnitIds: [], meleeUnitIds: [],
      observedAt: '2026-09-29T09:30:00Z', inventoryObservedAt: '2026-09-29T09:30:00Z',
    },
    ...extra,
  };
}

const lane = (items) => ({ troops: items.map(([itemId, quantity]) => ({ itemId, quantity })), tools: [] });
const attack = { source: 'inline', presetId: '', missing: false, setup: { targetType: 'pve', useTroopFamilies: false, waves: [{ L: lane([]), M: lane([[1, 100]]), R: lane([]) }], courtyardSupport: { troops: [], tools: [] } } };
function defense(amount = 5) {
  const draft = defenseTypes.emptyDefensePresetDraft();
  draft.wall.left.toolSlots[0] = { definitionId: 610, amount };
  return { source: 'inline', presetId: '', missing: false, setup: defenseApp.inlineDefenseFromPreset(draft) };
}

function evaluate(overrides = {}) {
  const draft = { ...khanState.defaultAutoKhanClientState(), sourceCastleId: 1, ...overrides.draft };
  return khan.evaluateKhanReadiness({
    draft,
    attack: overrides.attack ?? attack,
    defense: overrides.defense ?? defense(),
    state: 'state' in overrides ? overrides.state : { castles: { 1: castle(1, 1), 2: castle(2, 4) }, commanders: {}, market: {}, khan: {} },
    attackDocument: { version: 1, presets: [] },
    defenseDocument: { version: 1, presets: [] },
    troops, tools: {}, metadataReady: true, observation: LIVE, now: Date.parse('2026-09-29T12:00:00Z'),
  });
}
const byId = (report, id) => report.checks.find((check) => check.id === id);

test('a complete Khan setup reads the main castle, both compositions, skips and the plan', () => {
  const report = evaluate();
  assert.equal(byId(report, 'source-castle').state, 'valid');
  assert.deepEqual(byId(report, 'main-castle').params, { castle: 'C1' });
  assert.equal(byId(report, 'composition').state, 'valid');
  assert.equal(byId(report, 'inventory').state, 'valid');
  assert.equal(byId(report, 'defense-composition').state, 'valid');
  assert.equal(byId(report, 'defense-tool-stock').state, 'pending');
  assert.equal(byId(report, 'skip-cooldowns').state, 'valid');
  assert.ok(report.plan.some((line) => line.id === 'defense-reapply'));
  assert.ok(report.plan.some((line) => line.id === 'purchase-policy'));
  assert.equal(report.overall, 'pending', 'runtime-decided lines keep it pending, never a ready claim');
  for (const check of report.checks) assert.ok(messages[check.messageKey], check.messageKey);
  for (const line of report.plan) assert.ok(messages[line.messageKey], line.messageKey);
});

test('missing defense, missing main castle and a skip dependency block', () => {
  assert.equal(byId(evaluate({ defense: { source: 'none' } }), 'defense-composition').state, 'blocked');
  assert.equal(byId(evaluate({ defense: { source: 'preset', presetId: 'gone', missing: true } }), 'defense-composition').state, 'blocked');
  const noMain = evaluate({ state: { castles: { 2: castle(2, 4) }, commanders: {}, market: {} }, draft: { sourceCastleId: 2 } });
  assert.equal(byId(noMain, 'main-castle').state, 'blocked');
  const noSkips = evaluate({ draft: { skipCooldowns: false } });
  assert.deepEqual([byId(noSkips, 'skip-cooldowns').state, byId(noSkips, 'skip-cooldowns').fix], ['blocked', 'settings']);
  assert.equal(noSkips.overall, 'blocked');
  assert.equal(byId(evaluate({ state: null }), 'main-castle').state, 'unavailable');
});

test('a shared custom defense preset is accepted as a saved preset', () => {
  const draft = defenseTypes.emptyDefensePresetDraft();
  const report = khan.evaluateKhanReadiness({
    draft: { ...khanState.defaultAutoKhanClientState(), sourceCastleId: 1 },
    attack, defense: { source: 'preset', presetId: 'shared', missing: false },
    state: { castles: { 1: castle(1, 1) }, commanders: {}, market: {} },
    attackDocument: { version: 1, presets: [] },
    defenseDocument: { version: 1, presets: [{ ...draft, name: 'Shared', id: 'shared', createdAt: '', updatedAt: '' }] },
    troops, tools: {}, metadataReady: true, observation: LIVE,
  });
  assert.equal(byId(report, 'defense-composition').state, 'valid');
  assert.equal(byId(report, 'defense-tool-stock'), undefined, 'no tools, no stock line');
});

test('protection lock, rage booster gate and the ruby-costing limits are disclosed', () => {
  const locked = evaluate({ state: { castles: { 1: castle(1, 1) }, commanders: {}, market: {}, khan: { protection: { active: true } } } });
  assert.equal(byId(locked, 'protection').state, 'blocked');
  const waiting = evaluate({ draft: { requireActiveRageBooster: true } });
  assert.equal(byId(waiting, 'rage-booster').state, 'pending');
  const inactive = evaluate({ draft: { requireActiveRageBooster: true }, state: { castles: { 1: castle(1, 1) }, commanders: {}, market: { boostersObservedAt: '2026-09-29T10:00:00Z', boosters: {} } } });
  assert.equal(byId(inactive, 'rage-booster').state, 'blocked');
  const active = evaluate({ draft: { requireActiveRageBooster: true }, state: { castles: { 1: castle(1, 1) }, commanders: {}, market: { boostersObservedAt: '2026-09-29T10:00:00Z', boosters: { 27: { permanent: true } } } } });
  assert.equal(byId(active, 'rage-booster').state, 'valid');
  const limits = evaluate({ draft: { nomadPointThreshold: 5000, maxRageChain: 3, openGateProtection: true, offensiveUnitThreshold: 900, replenishDefenseTools: true, attackLaunchesEnabled: false } });
  const plan = Object.fromEntries(limits.plan.map((line) => [line.id, line]));
  assert.deepEqual(plan['nomad-point-limit'].params, { points: 5000 });
  assert.match(messages[plan['nomad-point-limit'].messageKey], /ruby/);
  assert.deepEqual(plan['rage-chain-limit'].params, { count: 3 });
  assert.deepEqual(plan['open-gate'].params, { threshold: 900 });
  assert.ok(plan['attacks-locked']);
  assert.match(messages[plan['purchase-policy'].messageKey], /Ruby-priced packages are rejected/);
  const outpost = evaluate({ draft: { sourceCastleId: 2, openGateProtection: true } });
  assert.equal(outpost.plan.some((line) => line.id === 'open-gate'), false, 'the safeguard applies only when attacking from the main castle');
});

test('player-facing Khan review text keeps "runtime" out', () => {
  const report = evaluate({ draft: { nomadPointThreshold: 1, maxRageChain: 1, requireActiveRageBooster: true, replenishDefenseTools: true } });
  for (const key of [...report.checks.map((check) => check.messageKey), ...report.plan.map((line) => line.messageKey)]) {
    assert.doesNotMatch(messages[key], /runtime/i, key);
  }
});

test('the main-castle defense starter is account data, offered only while current', () => {
  const main = castle(1, 1);
  const offered = starter.khanDefenseStarter({ mainCastle: main, observation: LIVE });
  assert.ok(offered.setup);
  assert.equal(offered.setup.wall.middle.unitPercent, 34, 'captured from the castle, not invented');
  assert.deepEqual(offered.pendingReviews, ['source']);
  assert.equal(starter.khanDefenseStarter({ mainCastle: null, observation: LIVE }).setup, null);
  assert.equal(starter.khanDefenseStarter({ mainCastle: main, observation: { ...LIVE, connected: false } }).reason, 'ui.components.staleSessionBanner.disconnected.last.known.data.166a8c99');
  const unread = starter.khanDefenseStarter({ mainCastle: castle(1, 1, { defense: { ...main.defense, observedAt: ZERO } }), observation: LIVE });
  assert.equal(unread.setup, null);
  const old = starter.khanDefenseStarter({ mainCastle: castle(1, 1, { defense: { ...main.defense, observedAt: '2026-09-28T09:00:00Z' } }), observation: LIVE });
  assert.equal(old.setup, null, 'a defense read before this connection is not offered');
  for (const result of [unread, old]) assert.ok(messages[result.reason], result.reason);
});

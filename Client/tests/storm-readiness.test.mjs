import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const storm = await vite.ssrLoadModule('/src/settings/readiness/stormReadiness.ts');
const stormState = await vite.ssrLoadModule('/src/settings/AutoStormClientState.ts');
const { messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');

after(async () => {
  await vite.close();
});

const ZERO = '0001-01-01T00:00:00Z';
const LIVE = { session: { generation: 5, baselineGeneration: 5, changedAt: '2026-09-29T09:00:00Z' }, connected: true };
const troops = { 1: { id: 1, name: 'Attacker', meleeAttack: 50, meleeDefence: 10 }, 2: { id: 2, name: 'Guard', meleeAttack: 5, meleeDefence: 40 } };
const tools = { 500: { id: 500, name: 'Ladder' } };
const castle = (id, kingdomId, stationed) => ({ id, name: `C${id}`, kingdomId, slotType: 1, units: { stationed }, unitsObservedAt: ZERO });
const lane = (items) => ({ troops: items.map(([itemId, quantity]) => ({ itemId, quantity })), tools: [] });
const inline = (quantity) => ({ source: 'inline', presetId: '', missing: false, setup: { targetType: 'pve', useTroopFamilies: false, waves: [{ L: lane([]), M: lane([[1, quantity]]), R: lane([]) }], courtyardSupport: { troops: [], tools: [] } } });

function evaluate({ draft = {}, forts = inline(10), islands = inline(10), castles, decorationLabel, buildActive = false, unlocks = { 4: { kingdomId: 4, unlocked: true, created: true } } } = {}) {
  const base = stormState.defaultAutoStormClientState();
  const all = castles ?? { 40: castle(40, 4, { 1: 100 }), 1: castle(1, 0, { 1: 5000 }) };
  const state = { castles: all, kingdomTransport: { unlocks }, commanders: {}, market: {} };
  return storm.evaluateStormReadiness({
    draft: { ...base, ...draft, forts: { ...base.forts, ...draft.forts }, islands: { ...base.islands, ...draft.islands }, troopImport: { ...base.troopImport, ...draft.troopImport }, aquamarine: { ...base.aquamarine, ...draft.aquamarine } },
    forts, islands, state, stormCastle: Object.values(all).find((entry) => entry.kingdomId === 4) ?? null,
    document: { version: 1, presets: [] }, troops, tools, metadataReady: true, observation: LIVE, buildActive, decorationLabel,
  });
}
const ids = (report) => report.checks.map((check) => `${check.id}${check.slot ? `:${check.slot}` : ''}`);

test('forts only, islands only and both: disabled branches impose nothing', () => {
  const fortsOnly = evaluate({ draft: { forts: { enabled: true } } });
  assert.ok(ids(fortsOnly).includes('composition:forts'));
  assert.ok(!ids(fortsOnly).some((id) => id.endsWith(':islands')));
  const islandsOnly = evaluate({ draft: { islands: { enabled: true } }, forts: { source: 'none' } });
  assert.ok(ids(islandsOnly).includes('composition:islands'));
  assert.ok(!ids(islandsOnly).some((id) => id.endsWith(':forts')), 'an unset fort preset is not a prerequisite while forts are off');
  const both = evaluate({ draft: { forts: { enabled: true }, islands: { enabled: true } } });
  assert.ok(ids(both).includes('composition:forts') && ids(both).includes('composition:islands'));
  const neither = evaluate();
  assert.equal(neither.checks.some((check) => check.id === 'branches'), false, 'no attack branch is a plan line, never a check');
  assert.ok(neither.plan.some((line) => line.id === 'branches'));
  for (const report of [fortsOnly, islandsOnly, both, neither]) {
    for (const check of report.checks) assert.ok(messages[check.messageKey], check.messageKey);
    for (const line of report.plan) assert.ok(messages[line.messageKey], line.messageKey);
  }
});

test('donor troops count toward stock only while import is on; missing donors block', () => {
  const short = evaluate({ draft: { forts: { enabled: true } }, forts: inline(1000) });
  assert.equal(short.checks.find((check) => check.id === 'inventory').state, 'pending', 'above Storm stock is decided at launch');
  const imported = evaluate({ draft: { forts: { enabled: true }, troopImport: { enabled: true, donorCastleIds: [1] } }, forts: inline(1000) });
  assert.equal(imported.checks.find((check) => check.id === 'inventory').state, 'valid');
  assert.equal(imported.checks.find((check) => check.id === 'donors').state, 'valid');
  const gone = evaluate({ draft: { forts: { enabled: true }, troopImport: { enabled: true, donorCastleIds: [1, 999] } } });
  assert.deepEqual(gone.checks.find((check) => check.id === 'donors').params, { count: 1 });
  assert.equal(evaluate({ draft: { forts: { enabled: true }, troopImport: { enabled: true, donorCastleIds: [] } } }).checks.find((check) => check.id === 'donors').state, 'blocked');
  const merged = storm.stormStockCastle(castle(40, 4, { 1: 100, 500: 2 }), [castle(1, 0, { 1: 5, 500: 9 })], true, tools);
  assert.deepEqual(merged.units.stationed, { 1: 105, 500: 2 }, 'donors import troops only, never tools');
});

test('the decoration preset is optional and informational; purchases are disclosed', () => {
  const none = evaluate({ draft: { forts: { enabled: true } } });
  assert.equal(none.checks.some((check) => check.id === 'decoration'), false);
  assert.equal(none.plan.some((line) => line.id === 'decoration'), false, 'absent decoration imposes nothing');
  const decorated = evaluate({ draft: { forts: { enabled: true }, decorationPresetId: 'd1' }, decorationLabel: 'Castle A · Garden' });
  assert.deepEqual(decorated.plan.find((line) => line.id === 'decoration').params, { preset: 'Castle A · Garden' });
  const shop = evaluate({ draft: { forts: { enabled: true }, aquamarine: { reserve: 500, purchases: [{ packageId: 3116, targetPurchases: 0, unlimited: true, priority: 1 }] } } });
  assert.deepEqual(shop.plan.find((line) => line.id === 'shop').params, { count: 1, reserve: 500, unlimited: 1 });
  const invalid = evaluate({ draft: { forts: { enabled: true }, aquamarine: { purchases: [{ packageId: 3116, targetPurchases: 0, unlimited: false, priority: 1 }] } } });
  assert.equal(invalid.checks.find((check) => check.id === 'shop').state, 'blocked');
  const build = evaluate({ draft: { forts: { enabled: true }, build: { ...stormState.defaultAutoStormClientState().build, allowPremium: true, allowDemolition: true } }, buildActive: true });
  assert.ok(build.plan.some((line) => line.id === 'build-premium') && build.plan.some((line) => line.id === 'build-demolition'));
});

test('unlock states and island defense units', () => {
  const lockedUnlocks = { 4: { kingdomId: 4, unlocked: false, created: false } };
  const locked = evaluate({ draft: { forts: { enabled: true } }, unlocks: lockedUnlocks, castles: { 1: castle(1, 0, {}) } });
  assert.equal(locked.checks[0].state, 'blocked');
  const planned = evaluate({ draft: { forts: { enabled: true }, unlock: { enabled: true, prebuiltCastleId: 7 } }, unlocks: lockedUnlocks, castles: { 1: castle(1, 0, {}) } });
  assert.equal(planned.checks[0].state, 'pending');
  const guards = evaluate({ draft: { islands: { enabled: true, defenseUnits: [{ unitId: 2, amount: 50 }] } } });
  assert.equal(guards.checks.find((check) => check.id === 'islands-defense-units').state, 'blocked', 'none stationed');
  const bad = evaluate({ draft: { islands: { enabled: true, defenseUnits: [{ unitId: 0, amount: 5 }] } } });
  assert.equal(bad.checks.find((check) => check.id === 'islands-defense-units').fix, 'settings');
});

test('a build, unlock or shop-only setup is not blocked by the absence of attack branches', () => {
  const buildOnly = evaluate({
    draft: { build: { ...stormState.defaultAutoStormClientState().build, allowPremium: true } },
    buildActive: true,
  });
  assert.notEqual(buildOnly.overall, 'blocked');
  assert.equal(buildOnly.checks.some((check) => check.state === 'blocked'), false);
  assert.ok(buildOnly.plan.some((line) => line.id === 'branches'));
  assert.ok(buildOnly.plan.some((line) => line.id === 'build-premium'));
});

test('unobserved castles or unlocks read as waiting for data, not as a configuration error', () => {
  const noCastles = evaluate({ draft: { forts: { enabled: true } }, castles: {}, unlocks: {} });
  assert.deepEqual([noCastles.checks[0].id, noCastles.checks[0].state, noCastles.checks[0].fix], ['unlock', 'unavailable', 'connection']);
  const noUnlocks = evaluate({ draft: { forts: { enabled: true } }, unlocks: {}, castles: { 1: castle(1, 0, {}) } });
  assert.deepEqual([noUnlocks.checks[0].id, noUnlocks.checks[0].state, noUnlocks.checks[0].fix], ['unlock', 'unavailable', 'connection']);
  assert.equal(noUnlocks.checks.some((check) => check.state === 'blocked'), false);
  const observedCastle = evaluate({ draft: { forts: { enabled: true } }, unlocks: {} });
  assert.deepEqual([observedCastle.checks[0].id, observedCastle.checks[0].state], ['storm-castle', 'valid'], 'an observed Storm castle proves the unlock');
  for (const report of [noCastles, noUnlocks]) {
    for (const check of report.checks) assert.ok(messages[check.messageKey], check.messageKey);
  }
});

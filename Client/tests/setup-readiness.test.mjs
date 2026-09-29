import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({
  root: clientRoot,
  appType: 'custom',
  logLevel: 'silent',
  server: { middlewareMode: true },
});
const setup = await vite.ssrLoadModule('/src/settings/requirements/setupReadiness.ts');
const { messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');

after(async () => {
  await vite.close();
});

const troops = { 1: { id: 1, name: 'A' }, 277: { id: 277, name: 'Direwolf' } };
// Zero-time unitsObservedAt as the projection serves it; freshness comes from the session (CIT-15 D1).
const SESSION = { generation: 25, baselineGeneration: 25, changedAt: '2026-09-29T09:00:00Z' };
const metadata = { troops, tools: {}, metadataReady: true, observation: { session: SESSION, connected: true } };
const castle = (id, kingdomId, slotType, stationed = {}, extra = {}) => ({ id, kingdomId, slotType, name: `C${id}`, units: { stationed }, unitsObservedAt: '0001-01-01T00:00:00Z', resources: {}, ...extra });

const allMessagesExist = (report) => report.checks.forEach((check) => assert.ok(messages[check.messageKey], `${check.id}: ${check.messageKey}`));

test('Towers: none enabled blocks; enabled without troop blocks; stationed troop is valid; maiden is pending', () => {
  const state = { castles: { 1: castle(1, 0, 1, { 1: 50 }), 2: castle(2, 0, 4, {}) } };
  const none = setup.evaluateTowerReadiness({ state, castles: {}, ...metadata });
  assert.equal(none.report.overall, 'blocked');
  const noTroop = setup.evaluateTowerReadiness({ state, castles: { 1: { enabled: true, unitId: 0, maidenOnly: false } }, ...metadata });
  assert.equal(noTroop.report.checks[0].state, 'blocked');
  const ready = setup.evaluateTowerReadiness({ state, castles: { 1: { enabled: true, unitId: 1, maidenOnly: true } }, ...metadata });
  assert.equal(ready.report.checks.find((check) => check.id === 'enabled-castles').state, 'valid');
  assert.equal(ready.stockByCastle['1'].check.state, 'valid');
  assert.equal(ready.report.checks.find((check) => check.id === 'maiden-relic').state, 'pending');
  const empty = setup.evaluateTowerReadiness({ state, castles: { 2: { enabled: true, unitId: 1, maidenOnly: false } }, ...metadata });
  assert.equal(empty.stockByCastle['2'].check.state, 'blocked', 'no stationed troop cannot be sized at launch');
  [none, noTroop, ready, empty].forEach((result) => allMessagesExist(result.report));
});

test('Fortress: kingdom main castles, Direwolf stock and the fastest-commander rule', () => {
  const state = { castles: { 11: castle(11, 1, 12, { 277: 400 }), 22: castle(22, 2, 12, {}), 33: castle(33, 3, 3, {}) } };
  const commanders = {
    activity: { id: 'commanders', state: 'valid', messageKey: 'commanderEligibility.freeNow', params: { count: 1 } },
    assignment: { id: 'commander-assignment', state: 'valid', messageKey: 'commanderEligibility.allAllowed', params: { count: 1 } },
  };
  const result = setup.evaluateFortressReadiness({
    state, kingdoms: { 1: { enabled: true }, 2: { enabled: true }, 3: { enabled: false } }, direwolfId: 277, direwolfPurchaseLimit: 0, commanders, ...metadata,
  });
  const byKingdom = Object.fromEntries(result.kingdoms.map((kingdom) => [kingdom.kingdomId, kingdom]));
  assert.equal(byKingdom[1].stock.check.state, 'valid');
  assert.equal(byKingdom[2].stock.check.state, 'blocked', 'no Direwolves and no purchases');
  assert.equal(byKingdom[3].castleCheck.state, 'blocked', 'slot 12 main castle required');
  assert.equal(result.report.checks.find((check) => check.id === 'commander-speed').state, 'pending');
  const staged = setup.evaluateFortressReadiness({ state, kingdoms: { 2: { enabled: true } }, direwolfId: 277, direwolfPurchaseLimit: 1000, ...metadata });
  assert.equal(staged.kingdoms.find((kingdom) => kingdom.kingdomId === 2).stock.check.state, 'pending', 'the supply lane stages Direwolves');
  const locked = setup.evaluateFortressReadiness({ state, kingdoms: { 3: { enabled: true } }, direwolfId: 277, direwolfPurchaseLimit: 0, ...metadata });
  assert.equal(locked.report.checks[0].state, 'blocked');
  const none = setup.evaluateFortressReadiness({ state, kingdoms: {}, direwolfId: 277, direwolfPurchaseLimit: 0, ...metadata });
  assert.equal(none.report.checks[0].state, 'blocked');
  [result, staged, locked, none].forEach((entry) => allMessagesExist(entry.report));
});

test('Fortress: owned Great Empire Direwolves supply a kingdom even without purchases (AutoFortressPolicy)', () => {
  const kingdoms = { 2: { enabled: true } };
  const stockFor = (castles, direwolfPurchaseLimit) => setup.evaluateFortressReadiness({ state: { castles }, kingdoms, direwolfId: 277, direwolfPurchaseLimit, ...metadata })
    .kingdoms.find((kingdom) => kingdom.kingdomId === 2).stock.check;
  const empty = castle(22, 2, 12, {});
  // Neither purchases nor Great Empire stock: blocked.
  assert.equal(stockFor({ 22: empty, 1: castle(1, 0, 1, {}) }, 0).state, 'blocked');
  // Great Empire main castle holds Direwolves: staged at launch.
  const fromGreatEmpire = stockFor({ 22: empty, 1: castle(1, 0, 1, { 277: 50 }) }, 0);
  assert.equal(fromGreatEmpire.state, 'pending');
  assert.equal(fromGreatEmpire.messageKey, 'ui.settings.requirements.setupReadiness.direwolves.are.staged.by.the.supply.lane.448002c8');
  // Direwolves in a non-main Great Empire castle do not count.
  assert.equal(stockFor({ 22: empty, 5: castle(5, 0, 4, { 277: 50 }) }, 0).state, 'blocked');
  // Purchases allowed: staged at launch.
  assert.equal(stockFor({ 22: empty }, 1000).state, 'pending');
  assert.equal(setup.greatEmpireMainCastle({ castles: { 5: castle(5, 0, 4), 1: castle(1, 0, 1) } }).id, 1);
  assert.equal(setup.greatEmpireMainCastle(null), null);
});

test('Station/Bird reserves: covered, above stock and unknown units', () => {
  const state = { castles: { 1: castle(1, 0, 1, { 1: 20 }) } };
  const result = setup.evaluateReserveReadiness({ featureId: 'autoBird', state, reserves: { 1: [{ id: 1, amount: 10 }] }, ...metadata });
  assert.equal(result.stockByCastle['1'].check.state, 'valid');
  const above = setup.evaluateReserveReadiness({ featureId: 'autoBird', state, reserves: { 1: [{ id: 1, amount: 30 }] }, ...metadata });
  assert.equal(above.report.overall, 'pending');
  const unknown = setup.evaluateReserveReadiness({ featureId: 'autoStation', state, reserves: { 1: [{ id: 999, amount: 1 }] }, ...metadata });
  assert.equal(unknown.report.overall, 'blocked');
  assert.equal(setup.evaluateReserveReadiness({ featureId: 'autoStation', state: null, reserves: {}, ...metadata }).report.overall, 'unavailable');
  const noCastles = setup.evaluateReserveReadiness({ featureId: 'autoStation', state: { castles: {} }, reserves: {}, ...metadata });
  const castlesCheck = noCastles.report.checks.find((check) => check.id === 'castles');
  assert.equal(castlesCheck.state, 'unavailable', 'zero observed castles is not a valid observation');
  assert.equal(castlesCheck.fix, 'connection');
  [result, above, unknown].forEach((entry) => allMessagesExist(entry.report));
});

test('Food Balance: observed food, donors by the saved reserve, coins and transport', () => {
  const resources = { 1: { id: 1, name: 'currency1', JSONKey: 'C1' }, 5: { id: 5, name: 'food', JSONKey: 'F' } };
  const state = {
    player: { resources: { 1: 500 } },
    kingdomTransport: { unlocks: { 1: { kingdomId: 1, unlocked: true, created: true } } },
    castles: {
      1: castle(1, 0, 1, {}, { resources: { 5: { amount: 5000 } } }),
      2: castle(2, 1, 12, {}, { resources: { 5: { amount: 200 } } }),
    },
  };
  const input = { state, resources, metadataReady: true, minimumSourceReserve: 1000, minimumCoinReserve: 100, autoKingdomTransport: true };
  const result = setup.evaluateFoodBalanceReadiness(input);
  assert.deepEqual(result.rows.map((row) => [row.castleId, row.role]), [[1, 'donor'], [2, 'recipient']]);
  assert.equal(result.report.overall, 'valid');
  assert.equal(result.coins, 500);
  const unobserved = setup.evaluateFoodBalanceReadiness({ ...input, state: { ...state, castles: { ...state.castles, 3: castle(3, 0, 4) } } });
  assert.equal(unobserved.report.checks.find((check) => check.id === 'food-observations').state, 'unavailable');
  const poor = setup.evaluateFoodBalanceReadiness({ ...input, minimumSourceReserve: 10_000, minimumCoinReserve: 1_000 });
  assert.equal(poor.report.checks.find((check) => check.id === 'donors').state, 'pending');
  assert.equal(poor.report.checks.find((check) => check.id === 'coin-reserve').state, 'pending');
  const noTransport = setup.evaluateFoodBalanceReadiness({ ...input, state: { ...state, kingdomTransport: { unlocks: {} } } });
  assert.equal(noTransport.report.checks.find((check) => check.id === 'kingdom-transport').state, 'pending');
  assert.equal(setup.evaluateFoodBalanceReadiness({ ...input, metadataReady: false }).report.checks[0].state, 'unavailable');
  [result, unobserved, poor, noTransport].forEach((entry) => allMessagesExist(entry.report));
});

test('every CIT-18 module renders the readiness panel; commander modules render the assignment panel', async () => {
  const withCommanders = ['AutoNomadSettingsModal', 'AutoInvasionSettingsModal', 'AutoBeriWorldSettingsModal', 'AutoTowerSettingsModal', 'AutoFortressSettingsModal'];
  const withoutCommanders = ['AutoStationSettingsModal', 'AutoBirdSettingsModal', 'AutoFoodBalanceSettingsModal'];
  for (const file of [...withCommanders, ...withoutCommanders]) {
    const source = await readFile(new URL(`../src/settings/components/${file}.tsx`, import.meta.url), 'utf8');
    assert.match(source, /<ReadinessPanel/, file);
    assert.match(source, /useConfigurationDraftSession/, file);
    assert.doesNotMatch(source, /configuration\??\.sections|persistAuto\w+ClientState|queueConfigurationUpdate/, file);
    if (withCommanders.includes(file)) {
      assert.match(source, /<CommanderAssignmentPanel/, file);
      assert.match(source, /COMMANDER_FEATURE_SECTION/, file);
    } else {
      assert.doesNotMatch(source, /<CommanderAssignmentPanel/, `${file} must not impose commander choices`);
    }
  }
});

test('D1: stock checks are unavailable while disconnected or awaiting the baseline', () => {
  const state = { castles: { 1: castle(1, 0, 1, { 1: 50 }) } };
  for (const observation of [{ session: SESSION, connected: false }, { session: { ...SESSION, baselineGeneration: 24 }, connected: true }]) {
    const towers = setup.evaluateTowerReadiness({ state, castles: { 1: { enabled: true, unitId: 1, maidenOnly: false } }, ...metadata, observation });
    assert.equal(towers.stockByCastle[1].check.state, 'unavailable');
    assert.equal(towers.stockByCastle[1].check.fix, 'connection');
    assert.deepEqual(towers.stockByCastle[1].lines, []);
    const reserves = setup.evaluateReserveReadiness({ featureId: 'autoStation', state, reserves: { 1: [{ id: 1, amount: 5 }] }, ...metadata, observation });
    assert.equal(reserves.stockByCastle[1].check.state, 'unavailable');
    allMessagesExist(towers.report);
    allMessagesExist(reserves.report);
  }
});

test('player-facing requirement text keeps "runtime" out (Daniel, CIT-15 recipe review)', () => {
  const keys = Object.keys(messages).filter((key) => /^(ui\.settings\.requirements\.|setupReadiness\.|unitStock\.|commanderEligibility\.|commanderAssignment\.|castleRequirement\.)/.test(key));
  assert.ok(keys.length > 20);
  for (const key of keys) assert.doesNotMatch(messages[key], /runtime/i, key);
});

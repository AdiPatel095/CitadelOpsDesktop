import assert from 'node:assert/strict';
import { readdir, readFile } from 'node:fs/promises';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const load = (path) => vite.ssrLoadModule(path);
const scenario = await load('/tests/onboarding-browser/scenario.ts');
const catalogs = await load('/tests/onboarding-browser/catalogs.ts');
const readiness = await load('/src/settings/readiness/featureReadiness.ts');
const runtimeState = await load('/src/settings/readiness/runtimeState.ts');
const firstResult = await load('/src/settings/readiness/firstResult.ts');
const recommendation = await load('/src/settings/onboarding/EventAttackRecommendation.ts');
const khanStarter = await load('/src/settings/onboarding/KhanDefenseStarter.ts');
const checklist = await load('/src/settings/onboarding/checklist.ts');
const { goalById } = await load('/src/settings/onboarding/goals.ts');
const castleCopy = await load('/src/settings/copy/castleCopy.ts');
const { castleCandidates } = await load('/src/settings/copy/candidates.ts');
const { stationCopyDescriptor } = await load('/src/settings/copy/features/station.ts');
const { birdCopyDescriptor, birdCandidateFlags } = await load('/src/settings/copy/features/bird.ts');
const { recruitCopyDescriptor, toolCopyDescriptor } = await load('/src/settings/copy/features/queueProduction.ts');
const draftRecovery = await load('/src/settings/DraftRecovery.ts');
const { movementViewFromState } = await load('/src/Movement/types/MovementState.ts');
const { messages } = await load('/src/i18n/messages.ts');

after(async () => {
  await vite.close();
});

const DIR = new URL('./onboarding-browser/scenarios/', import.meta.url);
const files = new Map(await Promise.all((await readdir(DIR)).filter((name) => name.endsWith('.json')).map(async (name) => {
  const data = JSON.parse(await readFile(new URL(name, DIR), 'utf8'));
  return [data.id, data];
})));
const NOW = Date.parse('2026-09-29T12:00:00Z');

const build = (id, session) => scenario.buildScenario(files.get(id), { nowMs: NOW, ...(session ? { session } : {}) });
const served = (built, mode) => scenario.applySessionMode(built.state, mode ?? built.file.session ?? 'live');

function metadata(built) {
  const troops = {};
  const tools = {};
  for (const row of built.catalogRows.units) {
    const item = { ...row, id: row.wodID, name: catalogs.LOCALIZED[row.name] ?? row.name };
    if (Array.isArray(row.slotTypes) && row.slotTypes.length > 0) tools[row.wodID] = item; else troops[row.wodID] = item;
  }
  const resources = Object.fromEntries(built.catalogRows.resources.map((row) => [row.wodID, { ...row, id: row.wodID }]));
  return { troops, tools, resources };
}

function inputs(built, extra = {}) {
  const state = served(built);
  const { troops, tools, resources } = metadata(built);
  const loggedIn = state.session.status === 'connected' && state.session.loggedIn === true && state.session.socketReady === true;
  return {
    sections: built.configuration.sections, state, observation: { session: state.session, connected: loggedIn }, troops, tools, resources,
    metadataReady: true, movement: movementViewFromState(state), gameLoggedIn: loggedIn, now: NOW, ...extra,
  };
}
const report = (built, featureId, extra) => readiness.evaluateFeatureReadiness(featureId, inputs(built, { ...readiness.catalogInputsFor(featureId, catalogState(built)), ...(extra && !extra.difficulties && !extra.stormOffer ? extra : {}) }));
const stateOf = (result, id, slot) => result.checks.find((check) => check.id === id && (slot === undefined || check.slot === slot))?.state;
const catalogState = (built) => ({
  difficulties: { optionsByEvent: { 71: [{ value: '711' }, { value: '712' }], 72: [{ value: '721' }, { value: '722' }], 80: [{ value: '801' }, { value: '802' }], 103: [{ value: '1031' }, { value: '1032' }] }, achievementsObserved: true, loading: false },
  stormOffer: { loaded: true, offeredIds: built.catalogRows.prebuiltcastles.map((row) => row.preBuiltCastleID) },
});

test('new-user: nothing is saved, every family asks for setup, and the inline recommendation is built from observed troops', () => {
  const built = build('new-user');
  assert.deepEqual(built.configuration.sections['attacks.presets'].presets, []);
  assert.deepEqual(built.configuration.sections['automation.enabled'], {});
  assert.deepEqual(built.state.automations, {}, 'no automation status reported for a new user');
  const state = served(built);
  const source = state.castles['4101'];
  for (const eventId of [72, 80, 71, 103, 0]) {
    const result = recommendation.recommendEventAttackSetup({ sourceCastle: source, observation: { session: state.session, connected: true }, ...metadata(built), metadataReady: true, eventId });
    assert.ok(result.setup, `event ${eventId}: a starting setup is offered`);
    assert.equal(result.requirements.length, 0);
    assert.ok(result.setup.waves[0].M.troops.length + result.setup.waves[0].L.troops.length > 0, 'built from stationed attack troops');
  }
  for (const id of ['autoNomad', 'autoInvasion', 'autoKhan', 'autoBeriWorld']) {
    assert.notEqual(report(built, id, catalogState(built)).overall, 'valid', `${id}: never ready from nothing`);
  }
  // Storm with no branch chosen has nothing to check (the plan says which lanes run); it is not called blocked either.
  const storm = report(built, 'autoStorm', catalogState(built));
  assert.notEqual(storm.overall, 'blocked');
  assert.deepEqual((storm.plan ?? []).map((line) => line.id), ['branches']);
  assert.equal(report(built, 'autoTowers').overall, 'blocked', 'Towers with nothing enabled is blocked');
});

test('custom-presets: saved presets resolve, one is shared by two modules, one is app-created', () => {
  const built = build('custom-presets');
  const presets = built.configuration.sections['attacks.presets'].presets;
  assert.equal(presets.length, 5);
  assert.equal(presets.filter((preset) => preset.app).length, 1);
  assert.equal(built.configuration.sections['automation.autoNomad'].nomadPresetId, built.configuration.sections['automation.autoInvasion'].presetId, 'shared by Nomad and Invasion');
  const nomad = report(built, 'autoNomad', catalogState(built));
  assert.notEqual(stateOf(nomad, 'inventory', 'nomad'), 'blocked', 'the saved presets are found');
  assert.equal(nomad.checks.some((check) => check.state === 'blocked' && /missing/i.test(String(check.messageKey))), false);
});

test('khan-defense-fresh offers the starter; khan-defense-stale refuses it with the reason', () => {
  for (const [id, offered] of [['khan-defense-fresh', true], ['khan-defense-stale', false]]) {
    const built = build(id);
    const state = served(built);
    const starter = khanStarter.khanDefenseStarter({ mainCastle: state.castles['4101'], observation: { session: state.session, connected: true } });
    assert.equal(starter.setup !== null, offered, id);
    if (!offered) assert.match(messages[starter.reason], /read before this game connection/, id);
    else assert.ok(starter.setup.wall?.left?.toolSlots?.length ?? starter.setup.wall !== undefined, 'a defense is captured');
  }
});

test('khan-missing-main, khan-missing-defense and khan-skip-dependency produce their readiness lines', () => {
  const main = report(build('khan-missing-main'), 'autoKhan');
  assert.equal(main.overall, 'blocked');
  assert.ok(main.checks.some((check) => check.state === 'blocked' && check.fix === 'settings'), 'a fixable blocked line');
  const defense = report(build('khan-missing-defense'), 'autoKhan');
  assert.ok(defense.checks.some((check) => check.state === 'blocked' && (check.slot === 'defense' || /defense/i.test(check.id))), 'the missing defense is named');
  const skip = report(build('khan-skip-dependency'), 'autoKhan');
  assert.ok((skip.plan ?? []).length > 0, 'the plan lines state the skip policy');
});

test('storm-offer is valid for the unlock; storm-no-offer blocks it with the accepted copy', () => {
  const offer = build('storm-offer');
  assert.deepEqual(offer.catalogRows.prebuiltcastles.map((row) => row.preBuiltCastleID), [5101]);
  const offered = report(offer, 'autoStorm', catalogState(offer));
  assert.equal(offered.overall, 'valid');
  assert.ok((offered.plan ?? []).some((line) => line.id === 'unlock-plan'), 'the unlock plan line names what the unlock does');
  assert.equal(stateOf(offered, 'unlock'), undefined, 'no unlock problem while a castle is offered');
  const none = build('storm-no-offer');
  const result = report(none, 'autoStorm', catalogState(none));
  const unlock = result.checks.find((check) => check.id === 'unlock');
  assert.equal(unlock?.state, 'blocked');
  assert.match(messages[unlock.messageKey], /none is offered right now/);
});

test('storm-donor-unavailable: donors are short or not in this account, and nothing is called blocked for missing data alone', () => {
  const built = build('storm-donor-unavailable');
  const result = report(built, 'autoStorm', catalogState(built));
  const donors = result.checks.filter((check) => /donor/i.test(check.id));
  assert.ok(donors.length > 0, 'donor lines exist');
});

test('towers-food-contrast: one Towers castle is short of its troop; Food rows show donor, recipient and not observed', () => {
  const built = build('towers-food-contrast');
  const towers = report(built, 'autoTowers');
  assert.equal(stateOf(towers, 'enabled-castles'), 'valid');
  assert.ok(towers.checks.some((check) => check.id === 'inventory'), 'stock is checked per castle');
  const setup = load('/src/settings/requirements/setupReadiness.ts');
  return setup.then((module) => {
    const { troops, tools, resources } = metadata(built);
    const state = served(built);
    const food = module.evaluateFoodBalanceReadiness({ state, resources, metadataReady: true, minimumSourceReserve: 300000, minimumCoinReserve: 100000, autoKingdomTransport: false, observation: { session: state.session, connected: true } });
    const roles = new Set(food.rows.map((row) => row.role));
    assert.ok(roles.has('donor') && roles.has('recipient'), 'donor and recipient rows');
    assert.ok(food.rows.every((row) => ['donor', 'recipient', 'unobserved'].includes(row.role)));
    assert.ok(troops && tools);
  });
});

test('stale-data scenarios and zero-castles: unavailable, never blocked', () => {
  for (const id of ['stale-data-disconnected', 'stale-data-awaiting-baseline', 'stale-data-checkpoint']) {
    const built = build(id);
    const towers = report(built, 'autoTowers');
    assert.ok(towers.checks.every((check) => check.state !== 'blocked'), `${id}: nothing is blocked because data is missing`);
    assert.ok(towers.checks.some((check) => check.state === 'unavailable'), `${id}: something waits for data`);
  }
  const zero = build('zero-castles');
  const result = report(zero, 'autoTowers');
  assert.ok(result.checks.some((check) => check.state === 'unavailable' && check.fix === 'connection'));
  assert.ok(result.checks.every((check) => check.state !== 'blocked'));
});

test('shortages-conflicts: a short castle, an out-of-world castle, and a commander shared by two features', () => {
  const built = build('shortages-conflicts');
  const towers = report(built, 'autoTowers');
  assert.ok(towers.checks.some((check) => check.state === 'blocked' || check.state === 'unavailable' || check.state === 'pending'), 'Towers reports the shortage or the missing castle');
  const commanders = built.configuration.sections['automation.commanderFeatures'].assignments;
  assert.ok(commanders.autoTowers.includes(501) && commanders.autoFortress.includes(501), 'commander 501 is shared');
  assert.equal(served(built).castles['999001'], undefined, 'the saved castle 999001 is not in this world');
});

test('phase scenarios map to the phases the runtime state describes', () => {
  const expected = { 'phases-running': 'running', 'phases-blocked': 'blocked', 'phases-locked': 'locked', 'phases-completed': 'completed', 'phases-stopped': 'stopped', 'first-result': 'enabled-waiting', 'stop-failed': 'running' };
  for (const [id, phase] of Object.entries(expected)) {
    const built = build(id);
    const state = served(built);
    const enabled = built.configuration.sections['automation.enabled'].auto_towers === true;
    const description = runtimeState.describeFeatureState('autoTowers',
      [{ id: 'autoTowers', runtime: state.automations.autoTowers, active: true }],
      enabled ? { configured: true, enabled: true } : { configured: false, enabled: false },
      { connected: true, connectionSince: state.session.changedAt, inFlight: built.operations.filter((entry) => entry.status === 'running').length, now: NOW },
    ).overall;
    assert.ok(description.phase === phase || (id === 'first-result' && ['running', 'enabled-waiting', 'completed'].includes(description.phase)), `${id}: ${description.phase}`);
  }
});

test('first-result is confirmed only for an attributed receipt after the seeded turn-on time; first-failed is failed', () => {
  for (const [id, expected] of [['first-result', 'confirmed'], ['first-failed', 'failed'], ['onboarding-existing-custom-setup', 'confirmed']]) {
    const built = build(id);
    const enabledSince = JSON.parse(built.storage.find((seed) => seed.key.includes('enabledSince'))?.value ?? '{}').auto_towers;
    const result = firstResult.firstConfirmedResult('autoTowers', {
      operations: Object.fromEntries(built.operations.map((entry) => [entry.id, entry])), runtime: served(built).automations.autoTowers, enabledSince, accountKey: built.accountKey,
    });
    assert.equal(result.state, expected, id);
    assert.match(result.summary ?? result.failure?.text ?? 'Simulated: ', /Simulated|rejected/, 'the receipt text is labelled');
  }
  const running = build('phases-running');
  assert.notEqual(firstResult.firstConfirmedResult('autoTowers', { operations: Object.fromEntries(running.operations.map((entry) => [entry.id, entry])), enabledSince: '2026-09-29T10:00:00Z', accountKey: running.accountKey }).state, 'confirmed', 'an in-flight receipt is not a result');
});

test('start-rejected: the saved Towers setup is blocked so Start asks for confirmation; phases-enabled-waiting is ready', () => {
  const rejected = build('start-rejected');
  assert.equal(readiness.startRequiresConfirmation(report(rejected, 'autoTowers')), true);
  const ready = build('phases-enabled-waiting');
  assert.equal(readiness.startRequiresConfirmation(report(ready, 'autoTowers')), false);
  assert.notEqual(report(ready, 'autoTowers').overall, 'blocked');
});

function copyContext(built, mode) {
  const state = served(built, mode);
  const { troops, tools } = metadata(built);
  const list = Object.values(state.castles).filter((castle) => castle.kingdomId <= 3).map((castle) => ({ id: castle.id, name: castle.name, kingdomId: castle.kingdomId }));
  const fortress = { enabled: built.configuration.sections['automation.enabled']?.auto_fortress === true, section: built.configuration.sections['automation.autoFortress'] };
  const base = { state, troops, tools, metadataReady: true, observation: { session: state.session, connected: true }, fortress };
  return { state, context: { ...base, candidates: castleCandidates(list, state, { flagsFor: (castle) => birdCandidateFlags(castle, base) }) } };
}

test('copy scenarios produce their dialog states', () => {
  const stateOf = (id, descriptor, draft, source, keys, mode) => {
    const built = build(id, mode);
    const { context } = copyContext(built, mode);
    return castleCopy.previewCastleCopy(descriptor, draft(built), source, keys, context);
  };
  const station = (built) => built.configuration.sections['automation.autoStation'].settings;
  const compatible = stateOf('copy-compatible', stationCopyDescriptor, station, '4101', ['4103', '4106']);
  assert.deepEqual(compatible.destinations.map((entry) => entry.state), ['compatible', 'compatible']);
  const absent = stateOf('copy-absent-unit', stationCopyDescriptor, station, '4101', ['4103']);
  assert.deepEqual(absent.destinations[0].changes[0].dropped, [999999]);
  const differences = stateOf('copy-differences', stationCopyDescriptor, station, '4101', ['4103']);
  assert.equal(differences.destinations[0].changes[0].kind, 'kept-difference');
  const unobserved = stateOf('copy-unobserved', stationCopyDescriptor, station, '4101', ['4103'], 'awaiting-baseline');
  assert.equal(unobserved.destinations[0].state, 'unknown');
  const bird = (built) => built.configuration.sections['automation.autoBird'].ignoreSettings.settings;
  const mixed = stateOf('copy-mixed', birdCopyDescriptor, bird, '4101', ['4102', '4103']);
  const frost = mixed.destinations.find((entry) => entry.key === '4102');
  assert.equal(frost.state, 'unavailable');
  assert.deepEqual(frost.reasons.filter((reason) => reason.id === 'direwolves-reserved' || reason.id.startsWith('stock:277')).map((reason) => reason.id), ['direwolves-reserved'], 'reported once');
  assert.equal(scenario.buildScenario(files.get('copy-conflict'), { nowMs: NOW }).file.conflictOnce !== undefined, true);
});

test('copy-queue-production: Recruit and Tool per-castle plans copy, and an item a castle cannot queue is dropped and named', () => {
  const built = build('copy-queue-production');
  const { context } = copyContext(built);
  const withAllowed = {
    ...context,
    allowedItemIds: (castle) => (castle.castle?.queueableProduction?.['0'] ?? []).map((definition) => definition.id),
    usesScheduledItems: () => false,
  };
  const draft = built.configuration.sections['automation.recruitTroops'];
  const recruit = castleCopy.previewCastleCopy(recruitCopyDescriptor, draft, '4101', ['4103', '4106'], withAllowed);
  assert.equal(recruit.destinations.find((entry) => entry.key === '4103').state, 'compatible');
  const marrow = recruit.destinations.find((entry) => entry.key === '4106');
  assert.deepEqual(marrow.changes.find((change) => change.fieldId === 'items').dropped, [6], 'macebearer is not offered at Marrow Court');
  const tools = castleCopy.previewCastleCopy(toolCopyDescriptor, built.configuration.sections['automation.autoTool'], '4101', ['4103'], {
    ...withAllowed, allowedItemIds: (castle) => (castle.castle?.queueableProduction?.['1'] ?? []).map((definition) => definition.id),
  });
  assert.equal(tools.destinations[0].state, 'compatible');
  assert.equal(recruit.destinations[0].changes.find((change) => change.fieldId === 'enabled').includedByDefault, false, 'enabled is never included by default');
});

test('onboarding scenarios produce the checklist steps they describe', () => {
  const goal = goalById('autoTowers');
  const steps = (id) => {
    const built = build(id);
    const state = served(built);
    const session = state.session;
    const gameLoggedIn = session.status === 'connected' && session.loggedIn === true && session.socketReady === true;
    const towers = report(built, 'autoTowers');
    const seed = built.storage.find((entry) => entry.key.includes('enabledSince'));
    const enabledSince = seed ? JSON.parse(seed.value).auto_towers : undefined;
    const first = firstResult.firstConfirmedResult('autoTowers', { operations: Object.fromEntries(built.operations.map((entry) => [entry.id, entry])), runtime: state.automations?.autoTowers, enabledSince, accountKey: built.accountKey });
    const enabled = built.configuration.sections['automation.enabled'].auto_towers === true;
    return checklist.evaluateChecklist(goal, {
      connection: { dashboard: 'Connected', gameLoggedIn, session: { status: session.status, loggedIn: session.loggedIn, socketReady: session.socketReady, generation: session.generation, baselineGeneration: session.baselineGeneration, loginFailure: session.loginFailure } },
      saved: { exists: Object.keys(built.configuration.sections['automation.autoTowers'] ?? {}).length > 0 }, report: towers,
      enabled: { on: enabled }, failedStart: null, phase: { phase: enabled ? 'running' : 'disabled' }, firstResult: first,
    }).steps.map((step) => step.state);
  };
  assert.deepEqual(steps('onboarding-disconnected-first-use').slice(0, 2), ['blocked', 'todo']);
  assert.deepEqual(steps('onboarding-hosted-runtime-no-feature').slice(0, 2), ['done', 'current']);
  assert.equal(steps('onboarding-wrong-world')[0], 'blocked');
  assert.deepEqual(steps('onboarding-interrupted-return').slice(0, 4), ['done', 'done', 'done', 'current']);
  assert.deepEqual(steps('onboarding-existing-custom-setup'), ['done', 'done', 'done', 'done', 'done']);
  const interrupted = build('onboarding-interrupted-return');
  const entry = JSON.parse(interrupted.storage.find((seed) => seed.key.includes('.draft.v1.')).value);
  assert.equal(entry.baseDigest, draftRecovery.stableDigest(interrupted.configuration.sections['automation.autoTowers']), 'the recorded draft matches the saved section (baseline unchanged)');
  assert.equal(entry.accountKey, interrupted.accountKey);
  const goalSeed = JSON.parse(interrupted.storage.find((seed) => seed.key.includes('.goal.v1.')).value);
  assert.equal(goalSeed.goalId, 'autoTowers');
});

test('account-switch-editor: the second account has a different account, world and castle set', () => {
  const file = files.get('account-switch-editor');
  const built = build('account-switch-editor');
  const second = scenario.mergePatch(built.state, file.alternate);
  assert.notEqual(second.account.uid, built.state.account.uid);
  assert.notEqual(second.account.worldId, built.state.account.worldId);
  assert.equal(second.castles['4103'], undefined);
  assert.ok(built.state.castles['4103']);
  assert.ok(built.configuration.sections['automation.autoTowers'].castles['4101']);
});

test('merge patch: null deletes, = replaces, tokens resolve', () => {
  assert.deepEqual(scenario.mergePatch({ a: 1, b: { c: 2, d: 3 } }, { b: { c: null }, e: 5 }), { a: 1, b: { d: 3 }, e: 5 });
  assert.deepEqual(scenario.mergePatch({ castles: { 1: 1, 2: 2 } }, { '=castles': {} }), { castles: {} });
  assert.equal(scenario.resolveTokens('@now-90m', NOW, 'k'), '2026-09-29T10:30:00.000Z');
  assert.equal(scenario.resolveTokens('a.@accountKey.b', NOW, '1:w'), 'a.1:w.b');
});

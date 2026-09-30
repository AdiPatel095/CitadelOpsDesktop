import assert from 'node:assert/strict';
import { readdir, readFile } from 'node:fs/promises';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const scenario = await vite.ssrLoadModule('/tests/onboarding-browser/scenario.ts');
const { FixtureServer } = await vite.ssrLoadModule('/tests/onboarding-browser/fixtureServer.ts');
const catalogs = await vite.ssrLoadModule('/tests/onboarding-browser/catalogs.ts');

after(async () => {
  await vite.close();
});

const DIR = new URL('./onboarding-browser/scenarios/', import.meta.url);
const files = (await readdir(DIR)).filter((name) => name.endsWith('.json')).sort();
const scenarios = await Promise.all(files.map(async (name) => ({ name, data: JSON.parse(await readFile(new URL(name, DIR), 'utf8')) })));
const NOW = Date.parse('2026-09-29T12:00:00Z');
const BANNER = 'banner: Simulated preview: sample data only. Nothing here reaches the game or an account.';

test('every scenario file is valid, its id matches its file name, and ids are unique', () => {
  assert.ok(scenarios.length >= 40, `${scenarios.length} scenarios`);
  const ids = new Set();
  for (const { name, data } of scenarios) {
    assert.equal(`${data.id}.json`, name);
    assert.deepEqual(scenario.validateScenarioFile(data), [], name);
    assert.ok(!ids.has(data.id), `duplicate ${data.id}`);
    ids.add(data.id);
    assert.ok(data.expect.every((line) => typeof line === 'string' && line.trim()), `${data.id}: expectations are sentences`);
  }
});

test('every scenario names the banner and the candidate label among its Simulated labels; receipts and first results are labelled', () => {
  for (const { data } of scenarios) {
    assert.ok(data.simulated.includes(BANNER), `${data.id}: banner`);
    assert.ok(data.simulated.includes('dock: Candidate <short SHA>'), `${data.id}: candidate`);
    const receipts = [...(data.patch?.operations ?? []), ...(data.runtime ?? []).flatMap((step) => step.operations ?? [])];
    for (const receipt of receipts) {
      assert.match(receipt.plan?.summary ?? '', /^Simulated: /, `${data.id}: ${receipt.id} carries the Simulated label`);
      assert.match(receipt.actor, /^automation:[A-Za-z]+$/, `${data.id}: scenario receipts are declared data, attributed to the automation they simulate`);
    }
    if (receipts.length > 0) assert.ok(data.simulated.some((label) => label.startsWith('receipt summaries')), `${data.id}: names the receipt label`);
  }
});

test('every scenario builds against the typed realm, and the served state, configuration and catalogs are well formed', () => {
  for (const { data } of scenarios) {
    const built = scenario.buildScenario(data, { nowMs: NOW });
    assert.equal(typeof built.state.schemaVersion, 'number', data.id);
    assert.ok(built.state.session && typeof built.state.session.generation === 'number', `${data.id}: session`);
    assert.equal(typeof built.configuration.sections, 'object');
    assert.ok(built.configuration.sections['automation.enabled'] !== undefined, `${data.id}: enabled switches exist`);
    for (const [name, rows] of Object.entries(built.catalogRows)) assert.ok(Array.isArray(rows), `${data.id}: catalog ${name}`);
    for (const seed of built.storage) {
      assert.match(seed.key, /^citadelops\./, `${data.id}: storage seeds stay in the citadelops namespace`);
      assert.doesNotThrow(() => JSON.parse(seed.value));
      assert.doesNotMatch(seed.value, /@(now|accountKey|savedDigest)/, `${data.id}: no unresolved token`);
    }
    assert.doesNotMatch(JSON.stringify(built.configuration), /@(now|accountKey|savedDigest)/, `${data.id}: no unresolved token in configuration`);
    assert.doesNotMatch(JSON.stringify(built.operations), /@(now|accountKey)/, `${data.id}: no unresolved token in receipts`);
  }
});

test('no credential, token, real account id, real player name or real host appears in any fixture file', async () => {
  const forbidden = [
    /password/i, /passwd/i, /secret/i, /\bbearer\s/i, /authorization/i, /api[-_]?key/i, /access[-_]?token/i, /refresh[-_]?token/i, /cookie/i, /sessionid/i,
    /goodgamestudios/i, /amos_?burton/i, /@[a-z0-9-]+\.[a-z]{2,}/i, /https?:\/\/(?!fixture)/i, /\b(?:wss?):\/\/(?!fixture)/i,
  ];
  const targets = [
    ...scenarios.map(({ name, data }) => [`scenarios/${name}`, JSON.stringify(data)]),
    ...await Promise.all(['realm.ts', 'catalogs.ts', 'scenario.ts', 'fixtureServer.ts', 'install.ts', 'fixtureSocket.ts', 'networkGuard.ts', 'product.ts', 'dock.tsx', 'main.tsx', 'vite.config.ts'].map(async (file) => [file, await readFile(new URL(`./onboarding-browser/${file}`, import.meta.url), 'utf8')])),
  ];
  for (const [file, text] of targets) {
    const stripped = text.replace(/\/\*[\s\S]*?\*\//g, '').replace(/^\s*\/\/.*$/gm, '').replace(/@(?:accountKey|now(?:[+-]\d+[smhd])?|savedDigest:[\w.]+)/g, '');
    for (const pattern of forbidden) {
      // Header words such as "authorization" in the transport's guard comments are stripped above; only code and data are checked.
      assert.doesNotMatch(stripped, pattern, `${file}: ${pattern}`);
    }
  }
  // The synthetic account and world are obviously fictional.
  const built = scenario.buildScenario(scenarios.find(({ data }) => data.id === 'new-user').data, { nowMs: NOW });
  assert.equal(built.state.account.worldId, 'demo-world');
  assert.equal(built.state.player.name, 'Preview Commander');
});

test('the fixture server never issues an automation-attributed receipt, records every write and refuses unknown writes', async () => {
  const server = new FixtureServer({ file: scenarios.find(({ data }) => data.id === 'new-user').data, nowMs: () => NOW });
  const receipt = await (await server.handle('/api/v2/intents/attack.launch', 'POST', { id: 'op-1', actor: 'automation:autoNomad', arguments: { x: 1 } })).json();
  assert.equal(receipt.actor, 'ui', 'an automation actor is never echoed');
  assert.equal(receipt.id, 'op-1');
  assert.match(receipt.plan.summary, /^Simulated: attack\.launch$/);
  const own = await (await server.handle('/api/v2/intents/auto_bird.castle_control', 'POST', { actor: 'ui:auto-bird', arguments: {} })).json();
  assert.equal(own.actor, 'ui:auto-bird');
  assert.equal(server.log.filter((entry) => entry.kind === 'intent').length, 2);
  const unknown = await server.handle('/api/v2/danger/launch-missiles', 'POST', {});
  assert.equal(unknown.status, 501);
  assert.ok(server.log.some((entry) => entry.kind === 'blocked'), 'an unknown write is recorded as blocked');
  assert.equal((await server.handle('/api/v2/nothing-here', 'GET')).status, 404);
});

test('configuration writes follow the real compare-and-set rules; ?fail rejects one enabled write per load; a conflict can be scripted once', async () => {
  const file = scenarios.find(({ data }) => data.id === 'stop-failed').data;
  const server = new FixtureServer({ file, nowMs: () => NOW });
  const before = await (await server.handle('/api/v2/config', 'GET')).json();
  assert.equal(before.sections['automation.enabled'].auto_towers, true);
  const stale = await server.handle('/api/v2/config/automation.autoTowers', 'PUT', { value: {}, expectedRevision: before.revision + 5 });
  assert.equal(stale.status, 409);
  assert.equal((await stale.json()).error.code, 'configuration_conflict');
  const ok = await server.handle('/api/v2/config/automation.autoTowers', 'PUT', { value: { version: 4, castles: {} }, expectedValue: before.sections['automation.autoTowers'] });
  assert.equal(ok.status, 200);
  const saved = await ok.json();
  assert.equal(saved.revision, before.revision + 1);
  const wrongValue = await server.handle('/api/v2/config/automation.autoTowers', 'PUT', { value: {}, expectedValue: { changed: true } });
  assert.equal(wrongValue.status, 409, 'a stale expected value conflicts');
  const off = { ...saved.sections['automation.enabled'], auto_towers: false };
  const rejected = await server.handle('/api/v2/config/automation.enabled', 'PUT', { value: off, expectedRevision: saved.revision });
  assert.equal(rejected.status, 500, 'the first Stop is rejected on purpose');
  const retry = await server.handle('/api/v2/config/automation.enabled', 'PUT', { value: off, expectedRevision: saved.revision });
  assert.equal(retry.status, 200, 'the retry succeeds');
  assert.equal((await retry.json()).sections['automation.enabled'].auto_towers, false);

  const conflict = new FixtureServer({ file: scenarios.find(({ data }) => data.id === 'copy-conflict-changed').data, nowMs: () => NOW });
  const opened = await (await conflict.handle('/api/v2/config', 'GET')).json();
  const first = await conflict.handle('/api/v2/config/automation.autoStation', 'PUT', { value: opened.sections['automation.autoStation'], expectedRevision: opened.revision });
  assert.equal(first.status, 409);
  const latest = await (await conflict.handle('/api/v2/config', 'GET')).json();
  assert.ok(latest.revision > opened.revision, 'the other window saved first');
  assert.deepEqual(latest.sections['automation.autoStation'].settings['4103'], [{ id: 39, amount: 100 }], 'and changed a destination');
  const second = await conflict.handle('/api/v2/config/automation.autoStation', 'PUT', { value: latest.sections['automation.autoStation'], expectedValue: latest.sections['automation.autoStation'] });
  assert.equal(second.status, 200, 'the second Save against the latest section succeeds');
});

test('runtime steps advance a scenario and Start Bot restores a disconnected first use', async () => {
  const server = new FixtureServer({ file: scenarios.find(({ data }) => data.id === 'phases-enabled-waiting').data, nowMs: () => NOW });
  assert.equal(server.nextRuntimeLabel(), 'The game reports it waiting');
  assert.equal(server.state().automations.autoTowers, undefined);
  assert.ok(server.advance());
  assert.equal(server.state().automations.autoTowers.status, 'waiting');
  assert.ok(server.advance());
  assert.equal(server.state().automations.autoTowers.status, 'running');
  assert.ok(server.operations().some((entry) => entry.id === 'op-sim-running'));
  assert.ok(server.advance());
  assert.ok(server.operations().some((entry) => entry.id === 'op-sim-first' && /^Simulated: /.test(entry.plan.summary)));
  assert.equal(server.advance(), false);

  const first = new FixtureServer({ file: scenarios.find(({ data }) => data.id === 'onboarding-disconnected-first-use').data, session: 'disconnected', nowMs: () => NOW });
  assert.equal(first.state().session.loggedIn, false);
  assert.deepEqual(first.state().account, {});
  await first.handle('/api/v2/intents/session.start', 'POST', {});
  assert.equal(first.state().session.loggedIn, true, 'Start Bot connects the fixture');
  assert.equal(first.state().account.worldId, 'demo-world');
  assert.ok(Object.keys(first.state().castles).length > 0);
});

test('the served catalogs carry what the production loaders read', () => {
  const units = catalogs.catalogFor('units').items;
  assert.ok(units.some((row) => row.wodID === 2 && row.rangeAttack > 0), 'attack troops have attack values');
  assert.ok(units.some((row) => row.wodID === 39 && row.meleeDefence > row.meleeAttack), 'defensive units are told apart');
  assert.ok(units.some((row) => Array.isArray(row.slotTypes) && row.slotTypes.length > 0), 'tools are marked by slotTypes');
  assert.deepEqual(catalogs.catalogFor('resources').items.filter((row) => row.JSONKey === 'F' || row.JSONKey === 'C1').map((row) => row.wodID).sort(), [3, 4]);
  assert.equal(catalogs.catalogFor('prebuiltcastles').items.length, 0);
  assert.equal(catalogs.catalogFor('prebuiltcastles', { prebuiltcastles: [{ preBuiltCastleID: 1 }] }).items.length, 1);
});

test('hosted account blocks are well formed and agree with the session a scenario serves', () => {
  let blocks = 0;
  for (const { data } of scenarios) {
    if (data.hostedAccount !== undefined) blocks += 1;
    assert.deepEqual(scenario.validateScenarioFile(data), [], data.id);
    const session = data.session ?? 'live';
    if (data.platforms.includes('hosted') && (session === 'disconnected' || session === 'checkpoint')) {
      assert.ok(data.hostedAccount, `${data.id}: serves a ${session} session, so it must say what the hosted account is`);
      assert.equal(data.hostedAccount.runtimePresent, false, `${data.id}: no hosted runtime is present while the session is ${session}`);
      assert.equal(data.hostedAccount.loggedIn, false, `${data.id}: and the game login is not held`);
    }
    if (data.platforms.length === 1 && data.platforms[0] === 'hosted') assert.ok(data.hostedAccount, `${data.id}: a hosted-only scenario says what its hosted account is`);
    if (data.hostedAccount?.loginFailure) assert.equal(data.hostedAccount.loggedIn, false, `${data.id}: a login failure means not logged in`);
    if (data.hostedAccount) assert.doesNotMatch(JSON.stringify(data.hostedAccount), /token|password|secret/i);
  }
  assert.ok(blocks >= 8, `${blocks} scenarios describe their hosted account`);
});

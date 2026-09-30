import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const explain = await vite.ssrLoadModule('/src/settings/connection/connectionExplain.ts');
const controls = await vite.ssrLoadModule('/src/settings/connection/connectionControls.ts');
const { messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');

after(async () => {
  await vite.close();
});

const NOW = Date.parse('2026-09-29T12:00:00Z');
const base = (extra = {}) => ({ surface: 'desktop', mode: 'background', status: 'connected', loggedIn: false, dashboard: 'Connected', now: NOW, ...extra });
const text = (explanation) => messages[explanation.messageKey];
const CLAIM = explain.CAUSE_CLAIMS;

test('accepted wording for the untyped rejection: background lists both checks and the saved world; full asks to log in in the game window', () => {
  const background = explain.explainConnection(base({ savedWorld: 'EmpireEx_2 · Empire' }));
  assert.equal(background.messageKey, 'connectionRepair.explain.backgroundRejected');
  assert.equal(messages[background.messageKey], 'The game did not accept the saved login. Check both the login and the world in Settings. Saved world: {world}.');
  assert.deepEqual(background.params, { world: 'EmpireEx_2 · Empire' });
  assert.equal(background.typedClass, undefined);
  const full = explain.explainConnection(base({ mode: 'full' }));
  assert.equal(text(full), 'Logged out in the game. Log in in the game window, then come back; your changes here are kept.');
});

test('an untyped rejection never says wrong world, wrong password, wrong server or wrong login', () => {
  const inputs = [
    base(), base({ savedWorld: 'EmpireEx_2 · Empire' }), base({ mode: 'full' }), base({ surface: 'hosted' }),
    base({ status: 'unavailable' }), base({ status: 'stopped' }), base({ status: 'suspended' }), base({ status: 'cooldown' }),
    base({ status: 'reconnecting' }), base({ status: 'released' }), base({ status: 'error' }), base({ status: 'weird-new-status' }),
    base({ dashboard: 'Disconnected' }), base({ dashboard: 'Connecting' }), base({ loginFailure: { class: 'unknown', fatal: false } }),
    base({ loginFailure: { class: 'never-heard-of-it' } }), base({ checkpoint: true, surface: 'hosted' }),
  ];
  for (const input of inputs) {
    const explanation = explain.explainConnection(input);
    assert.ok(messages[explanation.messageKey], explanation.messageKey);
    assert.doesNotMatch(text(explanation), CLAIM, JSON.stringify(input));
    assert.equal(explanation.typedClass, undefined, JSON.stringify(input));
  }
});

test('only a typed failure class names the cause, on desktop and hosted', () => {
  const desktopWrongServer = explain.explainConnection(base({ loginFailure: { class: 'wrong_server', fatal: true } }));
  assert.equal(desktopWrongServer.typedClass, 'wrong_server');
  assert.equal(text(desktopWrongServer), 'The game says this account does not play on the saved world. Change the world in Settings.');
  assert.deepEqual(desktopWrongServer.actions, ['open-settings']);
  const hostedWrongServer = explain.explainConnection(base({ surface: 'hosted', mode: undefined, loginFailure: { class: 'wrong_server', fatal: true } }));
  assert.equal(text(hostedWrongServer), 'The saved server does not match this login. Change the server in the Account Center.');
  assert.deepEqual(hostedWrongServer.actions, ['account-center']);
  const hostedCredentials = explain.explainConnection(base({ surface: 'hosted', mode: undefined, loginFailure: { class: 'invalid_credentials', fatal: true } }));
  assert.equal(text(hostedCredentials), 'The game rejected the saved login. Update it in the Account Center.');
  assert.match(text(explain.explainConnection(base({ loginFailure: { class: 'invalid_credentials', fatal: true } }))), /Update the login in Settings/);
  const suspended = explain.explainConnection(base({ loginFailure: { class: 'suspended', fatal: true } }));
  assert.equal(suspended.showDetail, true, 'the game\'s own detail is shown verbatim');
  assert.match(text(suspended), /Check the account in the game/);
  const accountDeleted = explain.explainConnection(base({ surface: 'hosted', loginFailure: { class: 'account_deleted', fatal: true } }));
  assert.equal(accountDeleted.typedClass, 'account_deleted');
  assert.match(text(explain.explainConnection(base({ loginFailure: { class: 'client_version_rejected' } }))), /waiting for an update/);
  const cooldown = explain.explainConnection(base({ loginFailure: { class: 'cooldown' }, retryAt: '2026-09-29T12:05:00Z' }));
  assert.equal(cooldown.messageKey, 'connectionRepair.explain.cooldown');
  assert.equal(cooldown.params.retryAt, Date.parse('2026-09-29T12:05:00Z'));
  // The words appear only in sentences that come from a typed class.
  for (const [key, value] of Object.entries(messages)) {
    if (!key.startsWith('connectionRepair.explain.') || !CLAIM.test(value)) continue;
    assert.match(key, /wrongServer|invalidCredentials/, `${key} claims a cause without a typed class`);
  }
});

test('status rules: cooldown and retry times, not running, waiting, and the zero time is never shown', () => {
  const cooldown = explain.explainConnection(base({ status: 'cooldown', cooldownUntil: '2026-09-29T12:00:55Z' }));
  assert.equal(text(cooldown), 'The game asked to wait. The next attempt is at {retryAt, time, short}.');
  assert.equal(cooldown.params.retryAt, Date.parse('2026-09-29T12:00:55Z'));
  const zero = explain.explainConnection(base({ status: 'cooldown', cooldownUntil: '0001-01-01T00:00:00Z', retryAt: '2026-09-29T11:00:00Z' }));
  assert.equal(zero.messageKey, 'connectionRepair.explain.cooldownNoTime', 'the Go zero time and a past time are not a next attempt');
  const suspended = explain.explainConnection(base({ status: 'suspended' }));
  assert.match(text(suspended), /Check the account in the game/);
  assert.equal(suspended.showDetail, true);
  for (const status of ['unavailable', 'stopped']) {
    const notRunning = explain.explainConnection(base({ status }));
    assert.match(text(notRunning), /not running/);
    assert.deepEqual(notRunning.actions, ['start-bot']);
  }
  const hostedNotRunning = explain.explainConnection(base({ status: 'stopped', surface: 'hosted' }));
  assert.deepEqual(hostedNotRunning.actions, ['account-center']);
  for (const status of ['released', 'reconnecting']) {
    const waiting = explain.explainConnection(base({ status, retryAt: '2026-09-29T12:10:00Z' }));
    assert.equal(waiting.messageKey, 'connectionRepair.explain.waitingRetry', status);
    assert.ok(waiting.actions.includes('reconnect'));
  }
  assert.match(text(explain.explainConnection(base({ dashboard: 'Disconnected' }))), /dashboard is not connected/);
  assert.match(text(explain.explainConnection(base({ checkpoint: true, surface: 'hosted' }))), /saved checkpoint/);
});

test('the saved world is resolved against the official directory and a missing code is said plainly', () => {
  const directory = [{ code: 'EmpireEx_2', label: 'Empire', zone: 'International' }, { code: 'EmpireEx_3', label: 'Second', zone: 'International' }];
  const listed = explain.describeSavedWorld('empireex_2', directory);
  assert.deepEqual([listed.kind, listed.short, listed.long], ['listed', 'EmpireEx_2 · Empire', 'EmpireEx_2 · Empire · International']);
  const missing = explain.describeSavedWorld('Nowhere_9', directory);
  assert.deepEqual([missing.kind, missing.code], ['not-listed', 'Nowhere_9']);
  assert.equal(messages['connectionRepair.world.notListed'], '{code} is not in the official server list. Open connection settings to choose a listed world; the saved login has to be entered again there.');
  assert.equal(explain.describeSavedWorld('', directory).kind, 'none');
  assert.equal(explain.describeSavedWorld(undefined, []).kind, 'none');
});

test('re-enable is offered only for the existing saved-login marker, with the same two intents as Settings', async () => {
  const marker = 'Saved login that has been disabled; re-enable it in Settings';
  assert.equal(controls.backgroundLoginNeedsReauthorization('background', { mode: 'background', loggedIn: false, detail: marker }), true);
  assert.equal(controls.backgroundLoginNeedsReauthorization('background', { mode: 'background', loggedIn: true, detail: marker }), false);
  assert.equal(controls.backgroundLoginNeedsReauthorization('background', { mode: 'background', loggedIn: false, detail: 'Game login handshake timed out' }), false);
  assert.equal(controls.backgroundLoginNeedsReauthorization('full', { mode: 'full', loggedIn: false, detail: marker }), false);
  const calls = [];
  await controls.reauthorizeSavedLogin(async (name) => { calls.push(name); });
  assert.deepEqual(calls, ['session.background.prepare', 'session.start']);
  const settings = await readFile(new URL('../src/views/SettingsView.tsx', import.meta.url), 'utf8');
  assert.match(settings, /reauthorizeSavedLogin\(submitIntent\)/, 'Settings uses the same helper');
  const availability = controls.connectionControlAvailability;
  assert.deepEqual(availability('stopped', true), { canStart: true, canReconnect: false });
  assert.deepEqual(availability('cooldown', true), { canStart: false, canReconnect: true });
  assert.deepEqual(availability('connected', true), { canStart: false, canReconnect: false });
  assert.deepEqual(availability('stopped', false), { canStart: true, canReconnect: false });
});

test('the dialog never collects a password and changes nothing itself', async () => {
  const dialog = await readFile(new URL('../src/components/ConnectionRepairDialog.tsx', import.meta.url), 'utf8');
  const code = dialog.replace(/\/\*[\s\S]*?\*\//g, '').replace(/^\s*\/\/.*$/gm, '');
  assert.doesNotMatch(code, /type="password"|<Input|password|updateConfiguration|putBackgroundLogin|saveBackgroundLogin|session\.background\.save/i);
  assert.match(code, /getBackgroundLoginStatus/, 'reads the saved server (never the password)');
  assert.match(code, /getGameServers/);
  assert.match(code, /onOpenSettings/, 'Settings is where the login is changed');
  const host = await readFile(new URL('../src/components/ConnectionRepairHost.tsx', import.meta.url), 'utf8');
  assert.match(host, /closeConnectionRepair/);
});

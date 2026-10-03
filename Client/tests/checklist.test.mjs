import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const checklist = await vite.ssrLoadModule('/src/settings/onboarding/checklist.ts');
const { goalById } = await vite.ssrLoadModule('/src/settings/onboarding/goals.ts');
const { messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');

after(async () => {
  await vite.close();
});

const goal = goalById('autoTowers');
const LIVE = { dashboard: 'Connected', gameLoggedIn: true, session: { status: 'connected', loggedIn: true, socketReady: true, generation: 4, baselineGeneration: 4 } };
const check = (id, state, extra = {}) => ({ id, state, messageKey: 'featureReadiness.savedSettings', ...extra });
const report = (checks) => {
  const order = { valid: 0, pending: 1, unavailable: 2, blocked: 3 };
  return { featureId: 'autoTowers', checks, overall: checks.reduce((worst, item) => (order[item.state] > order[worst] ? item.state : worst), 'valid') };
};
const READY = report([check('enabled-castles', 'valid', { fix: 'settings' }), check('maiden-relic', 'pending')]);
const NO_RESULT = { state: 'none', sinceKnown: true, summaryKey: 'firstResult.none', params: {} };
const CONFIRMED = { state: 'confirmed', completedAt: '2026-09-29T12:00:00Z', sinceKnown: true, summaryKey: 'firstResult.confirmed', params: {} };
const inputs = (extra = {}) => ({
  connection: LIVE, saved: { exists: true }, report: READY, enabled: { on: false }, failedStart: null, phase: { phase: 'disabled' }, firstResult: NO_RESULT, ...extra,
});
const evaluate = (extra) => checklist.evaluateChecklist(goal, inputs(extra));
const stateOf = (result, id) => result.steps.find((step) => step.id === id).state;

test('the five steps are the ones named in the story and every message exists', () => {
  const result = evaluate();
  assert.deepEqual(result.steps.map((step) => step.id), ['connection', 'choices', 'readiness', 'activation', 'first-result']);
  for (const step of result.steps) {
    assert.ok(messages[step.messageKey], step.messageKey);
    assert.ok(Array.isArray(step.evidence.read) && step.evidence.read.length > 0, `${step.id} records what it read`);
  }
});

test('connection: done only for a live logged-in session that finished its first sync', () => {
  assert.equal(stateOf(evaluate(), 'connection'), 'done');
  const notSynced = evaluate({ connection: { ...LIVE, session: { ...LIVE.session, baselineGeneration: 3 } } });
  assert.equal(stateOf(notSynced, 'connection'), 'waiting', 'logged in but not synced is waiting, never done');
  assert.equal(stateOf(evaluate({ connection: { ...LIVE, gameLoggedIn: false, session: { status: 'cooldown', loggedIn: false, socketReady: false, generation: 4, baselineGeneration: 4, detail: 'Login cooldown: 55s' } } }), 'connection'), 'blocked');
  for (const status of ['suspended', 'stopped', 'unavailable']) {
    const blocked = evaluate({ connection: { ...LIVE, gameLoggedIn: false, session: { status, loggedIn: false, socketReady: false } } });
    assert.equal(stateOf(blocked, 'connection'), 'blocked', status);
    assert.deepEqual(blocked.steps[0].nextStep, { kind: 'repair-connection' });
  }
  for (const status of ['starting', 'connecting', 'authenticating', 'reconnecting', 'released']) {
    assert.equal(stateOf(evaluate({ connection: { ...LIVE, gameLoggedIn: false, session: { status, loggedIn: false, socketReady: false } } }), 'connection'), 'waiting', status);
  }
  assert.equal(stateOf(evaluate({ connection: { ...LIVE, gameLoggedIn: false, session: { status: 'connected', loggedIn: false, socketReady: true } } }), 'connection'), 'blocked', 'a connected socket without a login needs the player');
  assert.equal(stateOf(evaluate({ connection: { dashboard: 'Disconnected', gameLoggedIn: false, session: null } }), 'connection'), 'blocked', 'no session at all (disconnected first use)');
  assert.equal(stateOf(evaluate({ connection: { dashboard: 'Connecting', gameLoggedIn: false, session: null } }), 'connection'), 'waiting');
});

test('connection (hosted): a checkpoint is unknown, a fatal login failure or a blocking account status is blocked', () => {
  const hosted = (extra) => ({ ...LIVE, hosted: { presence: 'live', accountStatus: 'ready', observedLoggedIn: true, runtimePresent: true, ...extra } });
  assert.equal(stateOf(evaluate({ connection: hosted({}) }), 'connection'), 'done');
  assert.equal(stateOf(evaluate({ connection: hosted({ observedLoggedIn: false }) }), 'connection'), 'blocked', 'the account record must agree that it is logged in');
  assert.equal(stateOf(evaluate({ connection: hosted({ presence: 'checkpoint' }) }), 'connection'), 'unknown');
  assert.equal(stateOf(evaluate({ connection: hosted({ loginFailure: { class: 'wrong_server', fatal: true } }) }), 'connection'), 'blocked');
  for (const accountStatus of ['action_required', 'suspended', 'expired', 'disabled']) {
    assert.equal(stateOf(evaluate({ connection: hosted({ accountStatus }) }), 'connection'), 'blocked', accountStatus);
  }
  const noRuntime = evaluate({ connection: hosted({ presence: 'checkpoint', runtimePresent: false }) });
  assert.deepEqual(noRuntime.steps[0].nextStep, { kind: 'account-center' }, 'a checkpoint without a hosted runtime points at the Account Center');
});

test('choices: done only from a SAVED section without a blocked required check', () => {
  const nothing = evaluate({ saved: { exists: false }, report: report([check('enabled-castles', 'blocked', { fix: 'settings' })]) });
  assert.equal(stateOf(nothing, 'choices'), 'current');
  assert.deepEqual(nothing.steps[1].nextStep, { kind: 'open-editor' });
  const blockedCheck = check('inventory', 'blocked', { fix: 'settings' });
  const blocked = evaluate({ report: report([blockedCheck]) });
  assert.equal(stateOf(blocked, 'choices'), 'blocked');
  assert.deepEqual(blocked.steps[1].nextStep, { kind: 'open-editor', check: blockedCheck });
  const waiting = evaluate({ report: report([check('castles', 'unavailable', { fix: 'connection' })]) });
  assert.equal(stateOf(waiting, 'choices'), 'waiting', 'unavailable data is waiting, never done');
  assert.ok(waiting.steps[1].nextStep, 'waiting always carries a next step');
  assert.equal(stateOf(evaluate({ report: null }), 'choices'), 'waiting', 'no configuration snapshot yet');
  assert.equal(stateOf(evaluate({ report: report([check('maiden-relic', 'pending')]) }), 'choices'), 'done', 'pending checks are the game\'s call at start');
});

test('choices: unavailable commander or timing data is a readiness matter, not a missing choice', () => {
  const commanders = evaluate({ report: report([check('enabled-castles', 'valid', { fix: 'settings' }), check('commanders', 'unavailable', { fix: 'connection' })]) });
  assert.equal(stateOf(commanders, 'choices'), 'done');
  assert.equal(stateOf(commanders, 'readiness'), 'waiting', 'readiness still waits for the commander data');
});

test('readiness: blocked or unavailable checks are never done; pending-only is done', () => {
  assert.equal(stateOf(evaluate(), 'readiness'), 'done');
  const assignment = check('commanders', 'blocked', { fix: 'assignment' });
  const blocked = evaluate({ report: report([check('enabled-castles', 'valid', { fix: 'settings' }), assignment]) });
  assert.equal(stateOf(blocked, 'choices'), 'done', 'a commander assignment is a readiness matter, not a required choice');
  assert.equal(stateOf(blocked, 'readiness'), 'blocked');
  assert.deepEqual(blocked.steps[2].nextStep, { kind: 'open-editor', check: assignment });
  const unavailable = evaluate({ report: report([check('inventory', 'unavailable', { fix: 'connection' })]) });
  assert.equal(stateOf(unavailable, 'readiness'), 'waiting');
  assert.deepEqual(unavailable.steps[2].nextStep, { kind: 'wait' });
  const offline = evaluate({ connection: { ...LIVE, gameLoggedIn: false, session: { status: 'stopped', loggedIn: false, socketReady: false } }, report: report([check('inventory', 'unavailable', { fix: 'connection' })]) });
  assert.deepEqual(offline.steps[2].nextStep, { kind: 'repair-connection' }, 'waiting on data while disconnected points at the connection');
  assert.equal(stateOf(evaluate({ saved: { exists: false } }), 'readiness'), 'todo', 'nothing saved: nothing to be ready');
});

test('activation: only the saved switch turns it done; a failed Start blocks; an unreported phase is unknown', () => {
  assert.equal(stateOf(evaluate(), 'activation'), 'current');
  assert.equal(stateOf(evaluate({ enabled: { on: true }, phase: { phase: 'enabled-waiting' } }), 'activation'), 'done');
  const timed = evaluate({ enabled: { on: true, timedUntil: 1_800_000_000_000 }, phase: { phase: 'enabled-waiting' } });
  assert.equal(timed.steps[3].messageKey, 'checklist.activation.doneTimed');
  assert.equal(timed.steps[3].params.until, 1_800_000_000_000);
  const failed = evaluate({ failedStart: { message: 'The server said no' } });
  assert.equal(stateOf(failed, 'activation'), 'blocked');
  assert.deepEqual(failed.steps[3].nextStep, { kind: 'start' });
  assert.equal(failed.steps[3].detail.text, 'The server said no');
  const unknown = evaluate({ enabled: { on: true }, phase: { phase: 'unknown' } });
  assert.equal(stateOf(unknown, 'activation'), 'unknown');
  assert.equal(messages['checklist.activation.unknown'], 'Turned on. The game has not reported on it yet.');
  assert.equal(stateOf(evaluate({ saved: { exists: false } }), 'activation'), 'todo', 'not offered before the steps above');
});

test('first result: only a confirmed attributed result is done; failure blocks; everything else waits with the CIT-20 sentence', () => {
  const on = { enabled: { on: true }, phase: { phase: 'running' } };
  assert.equal(stateOf(evaluate({ ...on, firstResult: CONFIRMED }), 'first-result'), 'done');
  assert.equal(stateOf(evaluate({ ...on, firstResult: { ...NO_RESULT, state: 'failed', failure: { text: 'Not enough troops' } } }), 'first-result'), 'blocked');
  for (const state of ['none', 'in-progress', 'unattributable']) {
    const waiting = evaluate({ ...on, firstResult: { ...NO_RESULT, state, counter: 9 } });
    assert.equal(stateOf(waiting, 'first-result'), 'waiting', state);
    assert.equal(messages[waiting.steps[4].messageKey], 'Waiting for the first action the game confirms.');
  }
  assert.equal(stateOf(evaluate({ firstResult: NO_RESULT }), 'first-result'), 'todo', 'off: not started');
});

test('a step is never done from a visit, a click, a draft, a counter or a running status', () => {
  // A saved draft that would pass is not a saved section; nothing saved stays "current" whatever the readiness says.
  const draftOnly = evaluate({ saved: { exists: false }, report: READY });
  assert.notEqual(stateOf(draftOnly, 'choices'), 'done');
  assert.notEqual(stateOf(draftOnly, 'readiness'), 'done');
  // Turning it on and a status of running/ready do not confirm a first result.
  const running = evaluate({ enabled: { on: true }, phase: { phase: 'running' }, firstResult: { ...NO_RESULT, counter: 12 } });
  assert.notEqual(stateOf(running, 'first-result'), 'done');
  // The click is not an input: the enable write in flight is not a saved switch.
  assert.equal(stateOf(evaluate({ enabled: { on: false } }), 'activation'), 'current');
  assert.equal(new Set(Object.keys(inputs())).has('visited'), false);
});

test('current is the first step that is not done, and complete needs all five', () => {
  const all = evaluate({ enabled: { on: true }, phase: { phase: 'running' }, firstResult: CONFIRMED });
  assert.equal(all.complete, true);
  assert.equal(all.current, null);
  assert.ok(all.steps.every((step) => step.state === 'done'));
  const partial = evaluate({ enabled: { on: true }, phase: { phase: 'running' }, firstResult: NO_RESULT });
  assert.equal(partial.complete, false);
  assert.equal(partial.current, 'first-result');
  // Existing custom setup, then the connection drops: later steps stay done, the first gap is current.
  const dropped = evaluate({
    connection: { ...LIVE, gameLoggedIn: false, session: { status: 'reconnecting', loggedIn: false, socketReady: false } },
    enabled: { on: true }, phase: { phase: 'running' }, firstResult: CONFIRMED,
  });
  assert.equal(dropped.current, 'connection');
  assert.equal(dropped.complete, false);
  assert.equal(stateOf(dropped, 'first-result'), 'done', 'the checklist does not reset what is still true');
  // Only the first non-done step can be "current"; later not-started steps read "todo".
  const fresh = evaluate({ saved: { exists: false }, report: null });
  assert.equal(fresh.steps.filter((step) => step.state === 'current').length, 1);
  for (const step of fresh.steps) if (step.state !== 'done') assert.ok(step.nextStep, `${step.id} carries a next step`);
});

test('every step that is not done carries a next step, in every scenario', () => {
  const scenarios = [
    evaluate(), evaluate({ saved: { exists: false } }), evaluate({ report: null }),
    evaluate({ connection: { dashboard: 'Disconnected', gameLoggedIn: false, session: null } }),
    evaluate({ report: report([check('x', 'blocked', { fix: 'settings' })]) }),
    evaluate({ failedStart: { message: '' } }),
    evaluate({ enabled: { on: true }, phase: { phase: 'unknown' } }),
  ];
  for (const result of scenarios) for (const step of result.steps) if (step.state !== 'done') assert.ok(step.nextStep, `${step.id}:${step.state}`);
});

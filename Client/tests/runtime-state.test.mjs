import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const state = await vite.ssrLoadModule('/src/settings/readiness/runtimeState.ts');
const { messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');

after(async () => {
  await vite.close();
});

const NOW = Date.parse('2026-09-29T12:00:00Z');
const SINCE = '2026-09-29T09:00:00Z';
const ON = { configured: true, enabled: true };
const LIVE = { connected: true, connectionSince: SINCE, now: NOW };
const runtime = (extra = {}) => ({ id: 'autoTowers', enabled: true, status: 'waiting', updatedAt: '2026-09-29T11:59:00Z', ...extra });
const describe = (rt, control = ON, context = LIVE) => state.describeAutomationState('autoTowers', rt, control, context);

test('off, never started: disabled; timed run ended or an earlier run: stopped', () => {
  const off = (rt, context = LIVE) => state.describeAutomationState('autoTowers', rt, undefined, context);
  assert.equal(off(undefined).phase, 'disabled');
  assert.equal(describe(runtime(), { configured: false, enabled: false }).phase, 'disabled');
  const ended = describe(runtime(), { configured: true, enabled: false, expiresAtMs: NOW - 1000 });
  assert.equal(ended.phase, 'stopped');
  assert.equal(ended.reason, 'timed-ended');
  assert.equal(describe(runtime({ lastRunAt: '2026-09-29T10:00:00Z' }), { configured: false, enabled: false }).phase, 'stopped');
  const inFlight = off(undefined, { ...LIVE, inFlight: 2 });
  assert.equal(inFlight.phase, 'stopped');
  assert.equal(inFlight.params.inFlight, 2);
  assert.match(messages['runtimeState.stopped'], /not recalled/, 'a stopped automation with running actions says they are not recalled');
});

test('on and reporting: waiting, running, ready, completed carry the game status without inventing reasons', () => {
  const waiting = describe(runtime({ status: 'waiting', detail: 'Waiting for an active Nomad event' }));
  assert.equal(waiting.phase, 'enabled-waiting');
  assert.equal(waiting.reason, 'waiting-detail');
  assert.equal(waiting.runtimeDetail.text, 'Waiting for an active Nomad event', 'the game detail is verbatim');
  const silent = describe(runtime({ status: 'waiting' }));
  assert.equal(silent.reason, 'waiting-no-detail');
  assert.equal(silent.runtimeDetail, undefined, 'no reason is made up when the game gives none');
  assert.equal(describe(runtime({ status: 'running' })).phase, 'running');
  const ready = describe(runtime({ status: 'ready' }));
  assert.equal(ready.phase, 'enabled-waiting', 'ready is not an action and not a result');
  assert.equal(ready.reason, 'ready');
  assert.equal(describe(runtime({ status: 'complete' })).phase, 'completed');
  assert.equal(describe(runtime({ status: 'completed' })).phase, 'completed');
  for (const description of [waiting, silent, ready]) assert.equal(description.evidenceFresh, true);
});

test('blocked and error keep the game detail; a missing reason says so and points at settings only for blocked', () => {
  const blocked = describe(runtime({ status: 'blocked', detail: 'No castle enabled' }));
  assert.equal(blocked.phase, 'blocked');
  assert.equal(blocked.runtimeDetail.text, 'No castle enabled');
  assert.equal(blocked.nextStep, 'settings');
  assert.equal(describe(runtime({ status: 'blocked' })).messageKey, 'runtimeState.blockedNoDetail');
  const failed = describe(runtime({ status: 'failed', lastError: 'The game rejected the attack' }));
  assert.equal(failed.phase, 'error');
  assert.equal(failed.runtimeDetail.text, 'The game rejected the attack', 'lastError is used when there is no detail');
  assert.equal(failed.nextStep, undefined);
  assert.equal(describe(runtime({ status: 'error' })).messageKey, 'runtimeState.errorNoDetail');
});

test('a localized runtime descriptor is kept only when it matches the adjacent detail', () => {
  const descriptor = { key: 'automation.nextCheck', fallback: 'Waiting for the next scheduled check', params: { state: 'waiting' } };
  const matching = describe(runtime({ detail: 'Waiting for the next scheduled check', detailDescriptor: { ...descriptor, fallbackText: 'Waiting for the next scheduled check' } }));
  assert.ok(matching.runtimeDetail.descriptor);
  const mismatched = describe(runtime({ detail: 'Something else', detailDescriptor: { ...descriptor, fallbackText: 'Waiting for the next scheduled check' } }));
  assert.equal(mismatched.runtimeDetail.descriptor, undefined);
});

test('schedule gates: a closed weekly window, or status gated, waits for the window', () => {
  const closed = describe(runtime({ status: 'waiting' }), ON, { ...LIVE, schedule: { enabled: true, allowedNow: false } });
  assert.equal(closed.phase, 'enabled-waiting');
  assert.equal(closed.reason, 'schedule');
  assert.equal(describe(runtime({ status: 'gated', detail: 'Outside the weekly window' })).reason, 'schedule');
  assert.equal(describe(runtime({ status: 'waiting' }), ON, { ...LIVE, schedule: { enabled: true, allowedNow: true } }).reason, 'waiting-no-detail');
  assert.equal(describe(runtime({ status: 'waiting' }), ON, { ...LIVE, schedule: { enabled: false, allowedNow: false } }).reason, 'waiting-no-detail', 'a disabled schedule is not a gate');
});

test('safety locks: timed locks cannot be cleared early; legacy locks need a review; cleared or expired locks are ignored', () => {
  const lock = { lane: 'autoTowers', opcode: 'msd', code: 1, operationId: 'op-1', intent: 'attack.launch', observedAt: '2026-09-29T11:30:00Z', reason: 'rejected' };
  const timed = describe(runtime({ status: 'running', safetyLock: { ...lock, until: '2026-09-29T12:30:00Z' } }));
  assert.equal(timed.phase, 'locked', 'a lock outranks a running status');
  assert.equal(timed.reason, 'lock-timed');
  assert.equal(timed.nextStep, undefined, 'no early manual clear during the 30-minute lock');
  const legacy = describe(runtime({ safetyLock: lock }));
  assert.equal(legacy.reason, 'lock-review');
  assert.equal(legacy.nextStep, 'clear-lock');
  assert.equal(describe(runtime({ status: 'waiting', safetyLock: { ...lock, until: '2026-09-29T11:45:00Z' } })).phase, 'enabled-waiting', 'an expired lock no longer holds');
  assert.equal(describe(runtime({ status: 'waiting', safetyLock: { ...lock, clearedAt: '2026-09-29T11:50:00Z' } })).phase, 'enabled-waiting');
  assert.equal(describe(runtime({ status: 'waiting', safetyLock: { ...lock, until: '0001-01-01T00:00:00Z' } })).reason, 'lock-review', 'the zero time is not an expiry');
});

test('precedence: lock > error > blocked > schedule gate > running > ready > complete > waiting', () => {
  const lock = { lane: 'x', opcode: 'msd', code: 1, operationId: 'op', intent: 'i', observedAt: '2026-09-29T11:30:00Z', reason: 'r', until: '2026-09-29T12:30:00Z' };
  const schedule = { schedule: { enabled: true, allowedNow: false } };
  assert.equal(describe(runtime({ status: 'error', safetyLock: lock })).phase, 'locked');
  assert.equal(describe(runtime({ status: 'error' }), ON, { ...LIVE, ...schedule }).phase, 'error');
  assert.equal(describe(runtime({ status: 'blocked' }), ON, { ...LIVE, ...schedule }).phase, 'blocked');
  assert.equal(describe(runtime({ status: 'running' }), ON, { ...LIVE, ...schedule }).reason, 'schedule');
  assert.equal(describe(runtime({ status: 'running' })).phase, 'running');
});

test('missing, disconnected, checkpoint and pre-connection statuses are unknown, never a green claim', () => {
  const none = describe(undefined);
  assert.equal(none.phase, 'unknown');
  assert.equal(none.reason, 'no-status');
  assert.equal(none.evidenceFresh, false);
  const disconnected = describe(runtime({ status: 'running' }), ON, { ...LIVE, connected: false });
  assert.equal(disconnected.phase, 'unknown');
  assert.equal(disconnected.reason, 'disconnected');
  assert.equal(disconnected.params.observedAt, Date.parse('2026-09-29T11:59:00Z'), 'the last known time is reported');
  assert.equal(describe(undefined, ON, { ...LIVE, connected: false }).messageKey, 'runtimeState.disconnectedUndated');
  const checkpoint = describe(runtime({ status: 'running' }), ON, { ...LIVE, presence: { mode: 'checkpoint', checkpointObservedAt: '2026-09-29T08:00:00Z' } });
  assert.equal(checkpoint.phase, 'unknown');
  assert.equal(checkpoint.reason, 'checkpoint');
  assert.equal(checkpoint.params.observedAt, Date.parse('2026-09-29T08:00:00Z'));
  const stale = describe(runtime({ status: 'running', updatedAt: '2026-09-29T08:00:00Z' }));
  assert.equal(stale.phase, 'unknown');
  assert.equal(stale.reason, 'before-connection', 'a status from before this connection is not current');
  assert.equal(describe(runtime({ status: 'running', updatedAt: '0001-01-01T00:00:00Z' })).reason, 'before-connection', 'a zero update time proves nothing');
  const noSince = describe(runtime({ status: 'running', updatedAt: '2026-09-29T08:00:00Z' }), ON, { connected: true, now: NOW });
  assert.equal(noSince.phase, 'running', 'without a connection start the status is taken as reported');
});

test('features with lanes: the most urgent lane speaks for the feature and inactive lanes are ignored', () => {
  const lanes = [
    { id: 'autoKhan', runtime: runtime({ id: 'autoKhan', status: 'running' }) },
    { id: 'autoKhan:cooldown', runtime: runtime({ id: 'autoKhan:cooldown', status: 'blocked', detail: 'No skips' }) },
    { id: 'autoKhan:defense', runtime: runtime({ id: 'autoKhan:defense', status: 'waiting' }) },
  ];
  const result = state.describeFeatureState('autoKhan', lanes, ON, LIVE);
  assert.equal(result.overall.phase, 'blocked');
  assert.equal(result.lanes.length, 3);
  const inactive = state.describeFeatureState('autoBeriWorld', [
    { id: 'autoBeriWorld', runtime: runtime({ status: 'waiting' }) },
    { id: 'autoBeriWorldBuild', runtime: runtime({ status: 'error' }), active: false },
  ], ON, LIVE);
  assert.equal(inactive.overall.phase, 'enabled-waiting', 'a disabled builder lane does not raise an error');
  assert.deepEqual(state.featureStateLaneIds('autoStorm'), ['autoStorm', 'autoStormShop', 'autoStormBuild']);
  assert.deepEqual(state.featureStateLaneIds('autoTowers'), ['autoTowers']);
});

test('every phase message exists and player-facing copy never says "runtime"', async () => {
  const keys = Object.keys(messages).filter((key) => /^(runtimeState|firstResult|stopSemantics|featureReadiness|startConfirm|observedAt)\./.test(key) || key === 'stormReadiness.unlockNotOffered' || key.startsWith('settingsRun.'));
  assert.ok(keys.length > 60);
  for (const key of keys) assert.doesNotMatch(messages[key], /runtime/i, key);
  for (const phase of ['disabled', 'enabled_waiting', 'running', 'blocked', 'error', 'locked', 'completed', 'stopped']) {
    assert.match(messages['runtimeState.phase'], new RegExp(`${phase} \\{`), phase);
  }
  for (const file of ['runtimeState.ts', 'firstResult.ts', 'stopSemantics.ts', 'featureReadiness.ts']) {
    const source = await readFile(new URL(`../src/settings/readiness/${file}`, import.meta.url), 'utf8');
    const literals = [...source.matchAll(/'([^'\n]{12,})'/g)].map((match) => match[1]).filter((text) => /\s/.test(text) && !text.startsWith('../') && !text.includes('/'));
    for (const text of literals) assert.doesNotMatch(text, /\bruntime\b/i, `${file}: ${text}`);
  }
  // Every message key the module can return exists.
  const source = await readFile(new URL('../src/settings/readiness/runtimeState.ts', import.meta.url), 'utf8');
  for (const [, key] of source.matchAll(/message\('([\w.]+)'\)/g)) assert.ok(messages[key], key);
});

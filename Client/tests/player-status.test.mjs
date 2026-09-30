import assert from 'node:assert/strict';
import { test, after } from 'node:test';
import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const root = fileURLToPath(new URL('..', import.meta.url));
const portal = existsSync(`${root}/src/commandCenter`);
const src = portal ? '/src/commandCenter' : '/src';
const vite = await createServer({ root, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const model = await vite.ssrLoadModule(`${src}/settings/readiness/playerStatus.ts`);
const phases = await vite.ssrLoadModule(`${src}/settings/readiness/runtimeState.ts`);
const { messages, describeMessage } = await vite.ssrLoadModule(`${src}/i18n/messages.ts`);
const { formatMessage } = await vite.ssrLoadModule(`${src}/i18n/formatMessage.ts`);
const accountPlayerStatus = portal ? (await vite.ssrLoadModule('/src/services/accountPlayerStatus.ts')).accountPlayerStatus : undefined;
after(() => vite.close());

const NOW = Date.parse('2026-09-30T12:00:00Z');
const ON = { configured: true, enabled: true };
const LIVE = { connected: true, now: NOW, connectionSince: '2026-09-30T11:00:00Z' };
const rt = (extra = {}) => ({ id: 'autoTowers', enabled: true, status: 'waiting', updatedAt: '2026-09-30T11:59:00Z', ...extra });
const input = (runtime = rt(), extra = {}) => ({ featureId: 'autoTowers', runtime, enabled: ON, context: LIVE, ...extra });
const text = (result) => formatMessage(result.reason, 'en', {}).text;
const table = {
  running: ['running', 'defending', 'discovering', 'evacuating', 'preparing', 'protecting', 'recalling', 'reconciling', 'refreshing', 'replenishing', 'resolving', 'taunting', 'threat'],
  waiting: ['waiting', 'ready', 'idle', 'armed', 'cooldown', 'scheduled', 'enabled', 'gated', 'yielding', 'pending', 'retrying', 'soft-locked', 'opening-check'],
  paused: ['protected'], blocked: ['blocked', 'warning'], 'needs-attention': ['error', 'failed'], done: ['complete', 'completed', 'success'], off: ['disabled', 'stopped'],
};
for (const [expected, values] of Object.entries(table)) for (const raw of values) {
  test(`automation ${raw} → ${expected}`, () => {
    const result = model.automationPlayerStatus(input(rt({ status: raw, detail: 'Game detail' }), { laneId: 'builder-missing-decorations' }));
    assert.equal(result.status, expected);
    assert.equal(text(result), 'Game detail');
  });
}
test('inventory, eight labels and roles cover the complete planned vocabulary', () => {
  assert.deepEqual([...model.KNOWN_RAW_AUTOMATION_STATUSES].sort(), Object.values(table).flat().sort());
  assert.deepEqual(model.PLAYER_STATUS_ROLE, { running: 'success', waiting: 'info', done: 'success', paused: 'warning', blocked: 'warning', 'needs-attention': 'danger', off: 'neutral', unknown: 'neutral' });
  for (const status of Object.keys(model.PLAYER_STATUS_ROLE)) assert.ok(messages[`playerStatus.${status}`]);
});
const phaseCases = [
  ['disabled', 'off', input(undefined, { runtime: undefined, enabled: { configured: false, enabled: false } })],
  ['stopped', 'off', input(undefined, { runtime: undefined, enabled: { configured: false, enabled: false }, context: { ...LIVE, inFlight: 1 } })],
  ['enabled-waiting', 'waiting', input(rt())], ['running', 'running', input(rt({ status: 'running' }))],
  ['blocked', 'blocked', input(rt({ status: 'blocked' }))], ['error', 'needs-attention', input(rt({ status: 'failed' }))],
  ['locked', 'paused', input(rt({ safetyLock: { operationId: 'sent-action', until: '2026-09-30T13:00:00Z' } }))],
  ['completed', 'done', input(rt({ status: 'complete' }))], ['unknown', 'unknown', input(undefined, { runtime: undefined })],
];
for (const [phase, expected, data] of phaseCases) test(`CIT-20 ${phase} → ${expected}`, () => {
  const old = phases.describeAutomationState(data.featureId, data.runtime, data.enabled, data.context);
  assert.equal(old.phase, phase);
  const result = model.automationPlayerStatus(data);
  assert.equal(result.status, expected);
  assert.equal(result.reason.key, old.messageKey);
  assert.deepEqual(result.reason.params, old.params);
});
test('protected distinguishes Station; warning distinguishes the synthetic missing-decoration lane', () => {
  assert.equal(model.automationPlayerStatus(input(rt({ status: 'protected' }), { featureId: 'autoStation' })).status, 'waiting');
  assert.equal(model.automationPlayerStatus(input(rt({ status: 'protected' }), { featureId: 'autoNomad' })).status, 'paused');
  assert.equal(model.automationPlayerStatus(input(rt({ status: 'warning' }))).status, 'waiting');
});
test('timed/review safety locks, expired/cleared locks and desktop Lock compose the phase model', () => {
  const lock = { operationId: 'sent-action' };
  assert.equal(model.automationPlayerStatus(input(rt({ safetyLock: lock }))).status, 'needs-attention');
  for (const safetyLock of [{ ...lock, until: '2026-09-30T11:00:00Z' }, { ...lock, clearedAt: '2026-09-30T11:58:00Z' }]) {
    assert.equal(model.automationPlayerStatus(input(rt({ status: 'running', safetyLock }))).status, 'running');
  }
  const paused = model.automationPlayerStatus(input(rt(), { desktopLocked: true }));
  assert.equal(paused.status, 'paused'); assert.equal(paused.reason.key, 'playerStatus.desktopLocked');
});
test('CIT-20 schedule, stopped actions and timed expiry retain their sentences and parameters', () => {
  const scheduled = model.automationPlayerStatus(input(rt({ status: 'running' }), { context: { ...LIVE, schedule: { enabled: true, allowedNow: false } } }));
  assert.equal(scheduled.status, 'waiting'); assert.equal(scheduled.reason.key, 'runtimeState.schedule');
  const stopped = model.automationPlayerStatus(input(rt(), { enabled: { configured: false, enabled: false }, context: { ...LIVE, inFlight: 1 } }));
  assert.equal(stopped.status, 'off'); assert.equal(stopped.reason.params.inFlight, 1);
  const ended = model.automationPlayerStatus(input(rt(), { enabled: { configured: true, enabled: false, expiresAtMs: NOW - 1000 } }));
  assert.equal(ended.status, 'off'); assert.equal(ended.reason.key, 'runtimeState.timedEnded'); assert.equal(ended.reason.params.endedAt, NOW - 1000);
});
test('checkpoint, disconnected, missing and pre-connection evidence never uses stale game detail', () => {
  for (const data of [input(rt({ detail: 'Old action' }), { context: { ...LIVE, presence: { mode: 'checkpoint' } } }), input(rt({ detail: 'Old action' }), { context: { ...LIVE, connected: false } }), input(undefined, { runtime: undefined }), input(rt({ status: 'running', detail: 'Old action', updatedAt: '2026-09-30T10:00:00Z' }))]) {
    const result = model.automationPlayerStatus(data);
    assert.equal(result.status, 'unknown'); assert.notEqual(text(result), 'Old action');
  }
});
test('game descriptors win, mismatched descriptors are rejected, and future values keep their detail', () => {
  const descriptor = { key: 'game.action', fallback: 'Game detail', fallbackText: 'Game detail' };
  assert.deepEqual(model.automationPlayerStatus(input(rt({ detail: 'Game detail', detailDescriptor: descriptor }))).reason, descriptor);
  const future = model.automationPlayerStatus(input(rt({ status: 'future-value', detail: 'Future {raw} detail', detailDescriptor: descriptor })));
  assert.equal(future.status, 'waiting'); assert.equal(text(future), 'Future {raw} detail'); assert.equal(future.reason.key, '');
});

const connection = (extra = {}) => ({ surface: 'desktop', status: 'Connected', loggedIn: true, started: true, now: NOW, ...extra });
for (const status of ['connecting', 'starting', 'authenticating', 'reconnecting', 'released', 'cooldown']) test(`connection ${status} waits`, () => {
  assert.equal(model.connectionPlayerStatus(connection({ status, loggedIn: false })).status, 'waiting');
});
test('connection connected, cooldown, suspended, fatal failure, before Start and unknown', () => {
  assert.equal(model.connectionPlayerStatus(connection({ status: 'disconnected', loggedIn: false, dashboard: 'Connecting' })).status, 'waiting');
  const connected = model.connectionPlayerStatus(connection()); assert.equal(connected.status, 'running'); assert.equal(text(connected), 'Connected');
  assert.equal(model.connectionPlayerStatus(connection({ loginFailure: { class: 'cooldown', fatal: true } })).status, 'waiting');
  const suspended = model.connectionPlayerStatus(connection({ loginFailure: { class: 'suspended', fatal: true, suspendedUntil: '2026-10-01T12:00:00Z' } }));
  assert.equal(suspended.status, 'blocked'); assert.equal(suspended.reason.params.time, NOW + 86400000);
  assert.equal(model.connectionPlayerStatus(connection({ status: 'suspended', loggedIn: false })).status, 'blocked');
  const fatal = model.connectionPlayerStatus(connection({ loginFailure: { fatal: true }, detail: { text: 'Check your saved login' } }));
  assert.equal(fatal.status, 'needs-attention'); assert.equal(text(fatal), 'Check your saved login');
  const off = model.connectionPlayerStatus(connection({ status: 'Disconnected', loggedIn: false, started: false }));
  assert.equal(off.status, 'off'); assert.equal(text(off), 'Game not started');
  assert.equal(model.connectionPlayerStatus(connection({ status: 'Connected', loggedIn: false })).status, 'unknown');
  assert.equal(model.connectionPlayerStatus(connection({ status: 'future-value', loggedIn: false })).status, 'unknown');
});
test('hosted checkpoints retain every account status and use the saved-data reason', () => {
  for (const status of Object.keys(model.PLAYER_STATUS_ROLE)) {
    const result = model.connectionPlayerStatus(connection({ surface: 'hosted', checkpoint: true, checkpointObservedAt: '2026-09-30T10:00:00Z', accountStatus: { status, reason: describeMessage('playerStatus.connected') } }));
    assert.equal(result.status, status); assert.equal(result.reason.key, 'playerStatus.checkpoint');
    assert.equal(result.reason.params.time, Date.parse('2026-09-30T10:00:00Z'));
  }
  assert.equal(model.connectionPlayerStatus(connection({ surface: 'hosted', checkpoint: true })).status, 'unknown');
});

if (portal) {
  const account = (extra = {}) => ({ enabled: true, status: 'ready', observed: {}, ...extra });
  for (const [raw, expected, key] of [['ready', 'running', 'gameConnected'], ['provisioning', 'waiting', 'starting'], ['connecting', 'waiting', 'connecting'], ['released', 'waiting', 'accountWait'], ['suspended', 'blocked', 'suspendedUndated'], ['action_required', 'needs-attention', 'loginFailed'], ['disabled', 'off', null], ['future-value', 'unknown', 'noConnection']]) test(`hosted ${raw} → ${expected}`, () => {
    const result = accountPlayerStatus(account({ status: raw }), NOW);
    assert.equal(result.status, expected); if (key) assert.equal(result.reason.key, `playerStatus.${key}`);
  });
  test('hosted draining and expired take both enabled branches', () => {
    for (const [raw, enabled, expected, key] of [['draining', true, 'waiting', 'playerStatus.restarting'], ['draining', false, 'off', 'playerStatus.stopping'], ['expired', true, 'needs-attention', 'playerStatus.licenseExpired'], ['expired', false, 'off', 'runtimeState.off']]) {
      const result = accountPlayerStatus(account({ status: raw, enabled }), NOW); assert.equal(result.status, expected); assert.equal(result.reason.key, key);
    }
  });
  for (const waitReason of ['cooldown', 'relog', 'reconnect', 'unknown']) test(`hosted wait ${waitReason} preserves text and countdown`, () => {
    const result = accountPlayerStatus(account({ status: 'released', waitReason, waitUntil: '2026-09-30T12:02:00Z' }), NOW);
    assert.equal(result.status, 'waiting'); assert.equal(result.reason.context.at(-1).params.reason, waitReason); assert.match(text(result), /2m 00s/);
  });
  test('hosted action detail, released detail descriptor and suspension time stay source-bound', () => {
    const descriptor = { key: 'game.action', fallback: 'Check your saved login', fallbackText: 'Check your saved login' };
    assert.deepEqual(accountPlayerStatus(account({ status: 'action_required', statusDetail: 'Check your saved login', statusDetailDescriptor: descriptor }), NOW).reason, descriptor);
    const waiting = accountPlayerStatus(account({ status: 'released', statusDetail: 'Check your saved login', statusDetailDescriptor: descriptor, waitUntil: '2026-09-30T12:02:00Z' }), NOW);
    assert.equal(waiting.reason.context.at(-1).key, descriptor.key); assert.match(text(waiting), /Check your saved login/); assert.match(text(waiting), /2m 00s/);
    const suspended = accountPlayerStatus(account({ status: 'suspended', observed: { loginFailure: { suspendedUntil: '2026-10-01T12:00:00Z' } } }), NOW);
    assert.equal(suspended.reason.params.time, NOW + 86400000);
  });
}

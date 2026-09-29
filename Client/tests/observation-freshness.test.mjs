import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({
  root: clientRoot,
  appType: 'custom',
  logLevel: 'silent',
  server: { middlewareMode: true },
});
const freshness = await vite.ssrLoadModule('/src/settings/requirements/observationFreshness.ts');
const { messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');

after(async () => {
  await vite.close();
});

const ZERO_TIME = '0001-01-01T00:00:00Z';
const SESSION = { generation: 25, baselineGeneration: 25, connectionGeneration: 3, status: 'ready', loggedIn: true, socketReady: true, changedAt: '2026-09-29T09:00:00Z' };
const castle = (unitsObservedAt) => ({ unitsObservedAt });
const evaluate = (overrides = {}) => freshness.unitObservationFreshness({ castle: castle(ZERO_TIME), session: SESSION, connected: true, ...overrides });

test('observationTimestamp rejects missing, unparsable and zero-time sentinels', () => {
  for (const value of [undefined, null, '', '   ', 'garbage', ZERO_TIME, '0001-01-01T00:00:00.000Z', '0000-12-31T23:59:59Z']) {
    assert.equal(freshness.observationTimestamp(value), undefined, String(value));
  }
  assert.equal(freshness.observationTimestamp('2026-09-29T10:00:00Z'), '2026-09-29T10:00:00Z');
});

test('rule order: checkpoint, disconnected, awaiting baseline, castle time, session baseline', () => {
  assert.deepEqual(evaluate({ connected: false, hostedPresence: { mode: 'checkpoint', checkpointObservedAt: '2026-09-29T08:00:00Z' } }),
    { state: 'unavailable', reason: 'checkpoint', observedAt: '2026-09-29T08:00:00Z' });
  assert.deepEqual(evaluate({ hostedPresence: { mode: 'checkpoint', checkpointObservedAt: ZERO_TIME } }), { state: 'unavailable', reason: 'checkpoint', observedAt: undefined });
  assert.deepEqual(evaluate({ connected: false }), { state: 'unavailable', reason: 'disconnected' });
  assert.deepEqual(evaluate({ hostedPresence: { mode: 'live' }, connected: false }), { state: 'unavailable', reason: 'disconnected' });
  assert.deepEqual(evaluate({ session: { ...SESSION, generation: 0, baselineGeneration: 0 } }), { state: 'unavailable', reason: 'awaiting-baseline' });
  assert.deepEqual(evaluate({ session: { ...SESSION, baselineGeneration: 24 } }), { state: 'unavailable', reason: 'awaiting-baseline' });
  assert.deepEqual(evaluate({ session: null }), { state: 'unavailable', reason: 'awaiting-baseline' });
  assert.deepEqual(evaluate({ castle: castle('2026-09-29T08:59:59Z') }),
    { state: 'unavailable', reason: 'stale-before-connection', observedAt: '2026-09-29T08:59:59Z', since: '2026-09-29T09:00:00Z' });
  assert.deepEqual(evaluate({ castle: castle('2026-09-29T09:30:00Z') }), { state: 'observed', scope: 'castle', observedAt: '2026-09-29T09:30:00Z' });
  assert.deepEqual(evaluate(), { state: 'observed', scope: 'session', since: '2026-09-29T09:00:00Z' });
  assert.deepEqual(evaluate({ castle: null }), { state: 'observed', scope: 'session', since: '2026-09-29T09:00:00Z' });
  assert.deepEqual(evaluate({ session: { ...SESSION, changedAt: ZERO_TIME } }), { state: 'observed', scope: 'session' });
});

test('every unavailable reason has a catalog message; disconnected reuses the banner text', () => {
  for (const reason of ['disconnected', 'checkpoint', 'awaiting-baseline', 'stale-before-connection']) {
    assert.ok(messages[freshness.observationUnavailableMessage(reason)], reason);
  }
  assert.equal(freshness.observationUnavailableMessage('disconnected'), 'ui.components.staleSessionBanner.disconnected.last.known.data.166a8c99');
});

test('player-facing freshness text keeps "runtime" out (Maya, CIT-15 product acceptance)', () => {
  for (const reason of ['disconnected', 'checkpoint', 'awaiting-baseline', 'stale-before-connection']) {
    assert.doesNotMatch(messages[freshness.observationUnavailableMessage(reason)], /runtime/i, reason);
  }
  assert.match(messages[freshness.observationUnavailableMessage('checkpoint')], /when the game connection is live again/);
});

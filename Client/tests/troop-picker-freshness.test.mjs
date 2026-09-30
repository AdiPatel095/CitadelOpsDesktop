import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const freshness = await vite.ssrLoadModule('/src/settings/requirements/observationFreshness.ts');
const { messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');

after(async () => {
  await vite.close();
});

const SESSION = { generation: 5, baselineGeneration: 5, changedAt: '2026-09-29T09:00:00Z' };
const note = (castle, observation) => freshness.stockObservationNote(freshness.unitObservationFreshness({ castle, ...observation }));

test('counts that are not current are captioned last known with the reason', () => {
  const disconnected = note({ unitsObservedAt: '0001-01-01T00:00:00Z' }, { session: SESSION, connected: false });
  assert.equal(disconnected.messageKey, 'observedAt.lastKnown');
  assert.equal(disconnected.reasonKey, freshness.observationUnavailableMessage('disconnected'));
  const checkpoint = note(null, { session: SESSION, connected: true, hostedPresence: { mode: 'checkpoint', checkpointObservedAt: '2026-09-29T08:00:00Z' } });
  assert.equal(checkpoint.reasonKey, freshness.observationUnavailableMessage('checkpoint'));
  const awaiting = note(null, { session: { ...SESSION, baselineGeneration: 4 }, connected: true });
  assert.equal(awaiting.reasonKey, freshness.observationUnavailableMessage('awaiting-baseline'));
  const before = note({ unitsObservedAt: '2026-09-29T08:00:00Z' }, { session: SESSION, connected: true });
  assert.equal(before.reasonKey, freshness.observationUnavailableMessage('stale-before-connection'));
});

test('a real per-castle time shows "as of"; the old zero time and a session-only baseline add no caption', () => {
  const real = note({ unitsObservedAt: '2026-09-29T10:30:00Z' }, { session: SESSION, connected: true });
  assert.equal(real.messageKey, 'observedAt.castleUnits');
  assert.equal(real.params.observedAt, Date.parse('2026-09-29T10:30:00Z'));
  assert.equal(note({ unitsObservedAt: '0001-01-01T00:00:00Z' }, { session: SESSION, connected: true }), null, 'an older runtime keeps sending the zero time: session rule, no caption');
  assert.equal(note(null, { session: SESSION, connected: true }), null);
});

test('the caption keys exist and read as player language', () => {
  assert.match(messages['observedAt.lastKnown'], /Last known/);
  assert.match(messages['observedAt.castleUnits'], /as of/i);
  for (const key of Object.keys(messages).filter((name) => name.startsWith('observedAt.'))) assert.doesNotMatch(messages[key], /runtime/i, key);
});

test('the picker shows the caption without changing selection, and Towers and Storm pass the observation', async () => {
  const picker = await readFile(new URL('../src/components/TroopPickerModal.tsx', import.meta.url), 'utf8');
  assert.match(picker, /stockObservation\?: \{ castle: /);
  assert.match(picker, /data-stock-observation/);
  assert.match(picker, /stockObservationNote\(unitObservationFreshness/);
  const towers = await readFile(new URL('../src/settings/components/AutoTowerSettingsModal.tsx', import.meta.url), 'utf8');
  assert.match(towers, /stockObservation: \{ castle: castle \?\? null, observation: setup\.observation \}/);
  const storm = await readFile(new URL('../src/settings/components/AutoStormSettingsModal.tsx', import.meta.url), 'utf8');
  assert.match(storm, /stockObservation: \{ castle: stormCastle \?\? null, observation \}/);
  const selection = picker.slice(picker.indexOf('const handleConfirm'), picker.indexOf('const handleCancel'));
  assert.doesNotMatch(selection, /stockNote|stockObservation/, 'selection logic is untouched');
});

test('Food Balance donor rows carry the castle food time, and a time before this connection is last known', async () => {
  const setup = await vite.ssrLoadModule('/src/settings/requirements/setupReadiness.ts');
  const castle = (id, foodStateObservedAt) => ({ id, kingdomId: 0, name: `C${id}`, resources: { 1: { amount: 5000 } }, foodStateObservedAt, units: { stationed: {} } });
  const result = setup.evaluateFoodBalanceReadiness({
    state: { castles: { 1: castle(1, '2026-09-29T10:30:00Z'), 2: castle(2, '2026-09-29T08:00:00Z'), 3: castle(3, '0001-01-01T00:00:00Z') }, player: { resources: {} }, kingdomTransport: { unlocks: {} } },
    observation: { session: SESSION, connected: true }, resources: { 1: { id: 1, jsonKey: 'F' } }, metadataReady: true,
    minimumSourceReserve: 100, minimumCoinReserve: 0, autoKingdomTransport: false,
  });
  const byId = Object.fromEntries(result.rows.map((row) => [row.castleId, row]));
  assert.equal(byId[1].observedAt, '2026-09-29T10:30:00Z');
  assert.equal(byId[1].current, true);
  assert.equal(byId[2].current, false, 'a castle time before this connection is last known');
  assert.equal(byId[3].observedAt, undefined, 'the zero time is not a time');
  assert.equal(byId[3].current, true, 'the session rule still applies to older runtimes');
});

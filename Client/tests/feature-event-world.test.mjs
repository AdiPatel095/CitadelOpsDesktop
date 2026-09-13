import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
const vite = await createServer({ root: fileURLToPath(new URL('..', import.meta.url)),
  appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
after(() => vite.close());
const { canonicalEventWorldID, featureHistoryMatchesScope } = await vite.ssrLoadModule('/src/events/components/FeatureEventWorld.ts');
const { featureEventFinals } = await vite.ssrLoadModule('/src/events/components/FeatureEventScores.ts');

test('websocket and canonical world aliases retain the same history', () => {
  for (const value of ['world.example', ' WORLD.EXAMPLE ', 'wss://world.example:443', 'https://world.example:443/path', 'ws://world.example:80', 'world.example:80']) {
    assert.equal(canonicalEventWorldID(value), 'world.example');
    assert.equal(featureHistoryMatchesScope({ worldId: 'world.example', playerId: 42, history: [] }, value, 42), true);
  }
});
test('world normalization preserves nonstandard ports and rejects malformed identities', () => {
  for (const value of ['wss://world.example:80', 'https://world.example:8443', 'world.example:8443']) {
    assert.notEqual(canonicalEventWorldID(value), 'world.example');
    assert.equal(featureHistoryMatchesScope({ worldId: 'world.example', playerId: 42, history: [] }, value, 42), false);
  }
  for (const value of ['', 'bad world', 'file:///world.example', 'https://user@world.example']) assert.equal(canonicalEventWorldID(value), '');
  for (const body of [null, {}, { worldId: 'other.example', playerId: 42, history: [] }, { worldId: 'world.example', playerId: 43, history: [] }, { worldId: 'world.example', playerId: 42, history: null }]) {
    assert.equal(featureHistoryMatchesScope(body, 'wss://world.example:443', 42), false);
  }
});
test('canonical response rows survive websocket-scoped final filtering', () => {
  const row = { worldId: 'world.example', playerId: 42, occurrenceId: 'completed', eventId: 72,
    eventKey: 'nomad', scoreKnown: true, score: 123, rank: 1, listType: 1, leagueId: 1,
    eventEndsAt: '2026-09-12T12:00:00Z', observedAt: '2026-09-12T12:00:00Z' };
  const finals = featureEventFinals([row, { ...row, occurrenceId: 'other-player', playerId: 43 },
    { ...row, occurrenceId: 'other-port', worldId: 'world.example:8443' }], 'wss://world.example:443', 42, Date.parse('2026-09-13T00:00:00Z'));
  assert.deepEqual(finals.map(entry => entry.occurrenceId), ['completed']);
});

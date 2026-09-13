import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

const vite = await createServer({
  root: fileURLToPath(new URL('..', import.meta.url)), appType: 'custom',
  logLevel: 'silent', server: { middlewareMode: true },
  esbuild: { tsconfigRaw: { compilerOptions: { verbatimModuleSyntax: false } } },
});
after(() => vite.close());
const { featureEventFinals, featureEventIds, isFeatureEventRunning } = await vite.ssrLoadModule('/src/events/components/FeatureEventScores.ts');
const { FeatureEventHistory } = await vite.ssrLoadModule('/src/events/components/FeatureEventHistory.tsx');
const now = Date.parse('2026-09-13T13:00:00Z');
const score = (overrides = {}) => ({
  eventId: 72, playerScore: 100, allianceScore: 200, localizationKey: 'nomad',
  observedAt: new Date(now - 60_000).toISOString(), remainingSec: 120, ...overrides,
});
const row = (overrides = {}) => ({
  worldId: 'world.example', playerId: 42, occurrenceId: 'old-nomads', eventId: 72,
  eventName: 'Nomads', eventKey: 'nomad-invasion', score: 123456, scoreKnown: true,
  rank: 10, listType: 1, leagueId: 1, scoreUnit: 'points',
  eventEndsAt: '2026-09-12T12:00:00Z', observedAt: '2026-09-12T11:59:00Z',
  ...overrides,
});
test('live event cards disappear at expiry, including a cached active-event id', () => {
  assert.equal(isFeatureEventRunning(score(), undefined, now), true);
  for (const event of [undefined, score({ remainingSec: 60 }), score({ remainingSec: 0 }), score({ remainingSec: undefined }), score({ observedAt: 'bad' }), score({ observedAt: new Date(now + 1).toISOString() })]) {
    assert.equal(isFeatureEventRunning(event, undefined, now), false);
  }
  assert.equal(isFeatureEventRunning(score(), undefined, now, new Date(now).toISOString()), false);
});
test('a newer event inventory can revoke availability but an older inventory cannot hide a newly observed run', () => {
  const inventory = { observedAt: new Date(now).toISOString(), activeByEvent: {} };
  assert.equal(isFeatureEventRunning(score(), inventory, now), false);
  inventory.activeByEvent[72] = { eventId: 72, endsAt: new Date(now + 30_000).toISOString() };
  assert.equal(isFeatureEventRunning(score(), inventory, now), true);
  inventory.activeByEvent[72].endsAt = new Date(now).toISOString();
  assert.equal(isFeatureEventRunning(score(), inventory, now), false);
  inventory.observedAt = new Date(now - 120_000).toISOString();
  assert.equal(isFeatureEventRunning(score(), inventory, now), true);
});
test('history isolates account and world and excludes active, unknown and invalid scores', () => {
  const finals = featureEventFinals([
    row(), row({ score: 222222, observedAt: '2026-09-12T12:00:00Z' }),
    row({ occurrenceId: 'older', eventEndsAt: '2026-09-11T12:00:00Z', score: 0 }),
    row({ occurrenceId: 'other-player', playerId: 43 }),
    row({ occurrenceId: 'other-world', worldId: 'other.example' }),
    row({ occurrenceId: 'active', eventEndsAt: new Date(now + 1).toISOString() }),
    row({ occurrenceId: 'unknown', scoreKnown: false }),
    row({ occurrenceId: 'invalid', score: -1 }),
    row({ occurrenceId: 'future-observation', observedAt: new Date(now + 1).toISOString() }),
  ], 'world.example', 42, now);
  assert.deepEqual(finals.map((entry) => [entry.occurrenceId, entry.score]), [['old-nomads', 222222], ['older', 0]]);
  assert.deepEqual(featureEventFinals([row()], '', 42, now), []);
  assert.deepEqual(featureEventFinals([row()], 'world.example', 0, now), []);
});
test('feature mappings never mix Invasion, Nomad, Storm and Berimond scores', () => {
  assert.deepEqual(featureEventIds.autoInvasion, [71, 103]);
  assert.deepEqual(featureEventIds.autoNomad, [72, 80]);
  assert.deepEqual(featureEventIds.autoStorm, [102]);
  assert.deepEqual(featureEventIds.autoBeriWorld, [3]);
  assert.equal(featureEventIds.autoTowers, undefined);
});
test('history renders dated, paginated finals and no live or unrelated event rows', () => {
  const html = renderToStaticMarkup(React.createElement(FeatureEventHistory, {
    entries: [...Array.from({ length: 12 }, (_, i) => row({ occurrenceId: 'run-' + i })),
      row({ occurrenceId: 'storm', eventId: 102, eventName: 'Unrelated Storm' }),
      row({ occurrenceId: 'active', eventName: 'Running Nomads', eventEndsAt: new Date(now + 1).toISOString() })],
    worldId: 'world.example', playerId: 42, now, loading: false, error: '', eventIds: [72, 80],
  }));
  assert.match(html, /12 completed runs/);
  assert.match(html, /Page 1 of 2/);
  assert.match(html, /123,456/);
  assert.match(html, /Final known score/);
  assert.doesNotMatch(html, /Unrelated Storm|Running Nomads/);
  assert.equal((html.match(/<tr/g) ?? []).length, 11);
});
test('empty, loading and unavailable history are explicit and never invent scores', () => {
  for (const [props, text] of [
    [{ loading: true, error: '' }, /Loading previous scores/],
    [{ loading: false, error: '' }, /No previous scores recorded/],
    [{ loading: false, error: 'Unavailable' }, /Unavailable/],
  ]) {
    const html = renderToStaticMarkup(React.createElement(FeatureEventHistory, {
      entries: [], worldId: 'world.example', playerId: 42, now, ...props,
    }));
    assert.match(html, text);
    assert.doesNotMatch(html, /<table/);
  }
});

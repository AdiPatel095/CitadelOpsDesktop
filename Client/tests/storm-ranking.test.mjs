import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

const sourceUrl = new URL('../src/worldIntelligence/components/StormRanking.ts', import.meta.url);
const source = await readFile(sourceUrl, 'utf8');
const compiled = ts.transpileModule(source, {
  compilerOptions: {
    module: ts.ModuleKind.ES2022,
    target: ts.ScriptTarget.ES2022,
  },
  fileName: sourceUrl.pathname,
});
const compiledUrl = `data:text/javascript;base64,${Buffer.from(compiled.outputText).toString('base64')}`;
const { normalizeStormSessionRanking } = await import(compiledUrl);

const row = (overrides) => ({
  worldId: 'test-world', occurrenceId: 'run-1', eventId: 102, eventKey: 'storm-islands',
  eventName: 'Storm Islands', listType: 13, leagueId: 1, playerId: 1,
  playerName: 'Player', rank: 1, scoreKnown: true, scoreUnit: 'points',
  runStartedOn: '2026-08-01', eventEndsAt: '2026-08-31T00:00:00Z',
  source: 'gge-highscore', observedAt: '2026-08-26T00:00:00Z', ...overrides,
});

test('rebuilds malformed Storm ranks from final scores', () => {
  const result = normalizeStormSessionRanking(102, [
    row({ playerId: 1, playerName: 'Low', rank: 1, score: 10 }),
    row({ playerId: 2, playerName: 'Top', rank: 1, score: 100 }),
    row({ playerId: 3, playerName: 'Zero', rank: 1 }),
  ]);
  assert.deepEqual(
    [...result].sort((left, right) => left.rank - right.rank).map(({ playerName, rank }) => [playerName, rank]),
    [['Top', 1], ['Low', 2], ['Zero', 3]],
  );
});

test('preserves a score-consistent Storm ranking', () => {
  const input = [
    row({ playerId: 1, playerName: 'Top', rank: 1, score: 100 }),
    row({ playerId: 2, playerName: 'Next', rank: 2, score: 50 }),
  ];
  assert.deepEqual(normalizeStormSessionRanking(102, input), input);
});

test('does not rewrite other event rankings', () => {
  const input = [
    row({ eventId: 89, eventKey: 'wheel-of-affluence', rank: 1, score: 10 }),
    row({ eventId: 89, eventKey: 'wheel-of-affluence', playerId: 2, rank: 1, score: 100 }),
  ];
  assert.deepEqual(normalizeStormSessionRanking(89, input), input);
});

test('does not invent ranks for an incomplete Storm board', () => {
  const input = [
    row({ playerId: 1, playerName: 'Low', rank: 1, score: 10 }),
    row({ playerId: 2, playerName: 'Top', rank: 1, score: 100 }),
    row({ playerId: 3, playerName: 'Unknown', rank: 1, scoreKnown: false }),
  ];
  assert.deepEqual(normalizeStormSessionRanking(102, input), input);
});

test('normalizes malformed boards without changing valid sibling leagues', () => {
  const result = normalizeStormSessionRanking(102, [
    row({ leagueId: 1, playerId: 5, playerName: 'Low', rank: 1, score: 10 }),
    row({ leagueId: 1, playerId: 4, playerName: 'Top', rank: 1, score: 100 }),
    row({ leagueId: 2, playerId: 7, playerName: 'Other Top', rank: 1, score: 80 }),
    row({ leagueId: 2, playerId: 8, playerName: 'Other Next', rank: 2, score: 40 }),
  ]);
  assert.deepEqual(result.map(({ leagueId, playerId, rank }) => [leagueId, playerId, rank]), [
    [1, 5, 2],
    [1, 4, 1],
    [2, 7, 1],
    [2, 8, 2],
  ]);
});

test('breaks equal-score ties by stable player identity', () => {
  const result = normalizeStormSessionRanking(102, [
    row({ playerId: 20, playerName: 'Alpha', rank: 1, score: 50 }),
    row({ playerId: 10, playerName: 'Zulu', rank: 1, score: 50 }),
    row({ playerId: 30, playerName: 'Low', rank: 1, score: 10 }),
  ]);
  assert.deepEqual(
    [...result].sort((left, right) => left.rank - right.rank).map(({ playerId, rank }) => [playerId, rank]),
    [[10, 1], [20, 2], [30, 3]],
  );
});

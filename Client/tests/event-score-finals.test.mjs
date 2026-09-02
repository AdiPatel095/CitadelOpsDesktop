import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const originalTimezone = process.env.TZ;
process.env.TZ = 'America/New_York';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({
  root: clientRoot,
  appType: 'custom',
  logLevel: 'silent',
  server: { middlewareMode: true },
});
const {
  completedEventScoreFinals,
  formatEventEndLocal,
  nextEventScoreEnd,
} = await vite.ssrLoadModule('/src/worldIntelligence/components/WorldEventFinals.ts');

after(async () => {
  await vite.close();
  if (originalTimezone == null) delete process.env.TZ;
  else process.env.TZ = originalTimezone;
});

const row = (overrides = {}) => ({
  worldId: 'world.example',
  occurrenceId: 'run-default',
  eventId: 72,
  eventKey: 'khan',
  eventName: 'Khan',
  listType: 1,
  leagueId: 1,
  playerId: 42,
  playerName: 'Player',
  rank: 1,
  score: 100,
  scoreKnown: true,
  scoreUnit: 'points',
  runStartedOn: '2026-08-30',
  eventEndsAt: '2026-09-01T10:00:00Z',
  source: 'gge-highscore',
  observedAt: '2026-09-01T09:55:00Z',
  ...overrides,
});

test('keeps one latest known final per completed event run', () => {
  const now = Date.parse('2026-09-01T12:00:00Z');
  const finals = completedEventScoreFinals([
    row({ occurrenceId: 'glory-run', eventId: 71, eventKey: 'glory', eventName: 'Glory', score: 900, eventEndsAt: '2026-09-01T08:00:00Z', observedAt: '2026-09-01T07:40:00Z' }),
    row({ occurrenceId: 'glory-run', eventId: 71, eventKey: 'glory', eventName: 'Glory', listType: 2, score: 850, eventEndsAt: '2026-09-01T08:00:00Z', observedAt: '2026-09-01T07:58:00Z' }),
    row({ occurrenceId: 'khan-run', score: 1_500 }),
    row({ occurrenceId: 'storm-active', eventId: 102, eventKey: 'storm-islands', eventName: 'Storm Islands', score: 12_000, eventEndsAt: '2026-09-30T00:00:00Z' }),
    row({ occurrenceId: 'beri-rank-only', eventId: 3, eventKey: 'berimond', eventName: 'Berimond', score: undefined, scoreKnown: false, eventEndsAt: '2026-08-31T00:00:00Z' }),
  ], now);

  assert.deepEqual(finals.map(({ occurrenceId, score }) => [occurrenceId, score]), [
    ['khan-run', 1_500],
    ['glory-run', 850],
  ]);
});

test('reveals the earliest upcoming known-score boundary', () => {
  const now = Date.parse('2026-09-01T12:00:00Z');
  assert.equal(nextEventScoreEnd([
    row({ occurrenceId: 'storm', eventEndsAt: '2026-09-30T00:00:00Z' }),
    row({ occurrenceId: 'beri', eventEndsAt: '2026-09-02T00:00:00Z' }),
    row({ occurrenceId: 'rank-only', score: undefined, scoreKnown: false, eventEndsAt: '2026-09-01T13:00:00Z' }),
  ], now), Date.parse('2026-09-02T00:00:00Z'));
});

test('formats the event end instant in the viewer local timezone', () => {
  const formatted = formatEventEndLocal('2026-09-01T12:00:00Z', 'en-US');
  assert.match(formatted, /8:00 AM/);
  assert.doesNotMatch(formatted, /12:00 PM/);
  assert.equal(formatEventEndLocal('not-a-date', 'en-US'), 'Unknown');
});

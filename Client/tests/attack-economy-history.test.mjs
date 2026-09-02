import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({
  root: clientRoot,
  appType: 'custom',
  logLevel: 'silent',
  server: { middlewareMode: true },
});
const history = await vite.ssrLoadModule('/src/attackAnalytics/components/AttackEconomyHistory.ts');

after(async () => {
  await vite.close();
});

const aggregate = (bucketStart, resources) => ({
  viewKey: 'tower',
  bucketStart,
  bucketSeconds: 60,
  reportCount: 1,
  victories: 1,
  defeats: 0,
  troopsSent: 100,
  troopLosses: 1,
  toolsUsed: 2,
  gallantryPoints: 0,
  lootTotal: Object.values(resources).reduce((total, amount) => total + amount, 0),
  resources,
  revision: 1,
});

test('feature resource history filters by view and follows bounded minute pages', async () => {
  const calls = [];
  const fetcher = async (path) => {
    const requestURL = new URL(path, 'https://runtime.example');
    calls.push(requestURL);
    if (calls.length === 1) {
      return Response.json({
        aggregates: [aggregate('2026-09-01T12:01:00Z', { W: 3 })],
        sourceBucketSeconds: 60,
        nextBefore: '2026-09-01T12:01:00Z',
      });
    }
    return Response.json({
      aggregates: [aggregate('2026-09-01T12:00:00Z', { W: 2 })],
      sourceBucketSeconds: 60,
    });
  };

  const aggregates = await history.fetchAttackEconomyAggregates(fetcher, {
    view: 'tower',
    since: '2026-08-25T12:00:00.000Z',
  });

  assert.deepEqual(aggregates.map((row) => row.resources.W), [2, 3]);
  assert.equal(calls.length, 2);
  assert.equal(calls[0].pathname, '/api/v2/analytics/resource-aggregates');
  assert.equal(calls[0].searchParams.get('view'), 'tower');
  assert.equal(calls[0].searchParams.get('since'), '2026-08-25T12:00:00.000Z');
  assert.equal(calls[0].searchParams.get('limit'), '5000');
  assert.equal(calls[1].searchParams.get('before'), '2026-09-01T12:01:00Z');
});

test('feature resource history rejects a repeated pagination cursor', async () => {
  const fetcher = async () => Response.json({
    aggregates: [aggregate('2026-09-01T12:00:00Z', { W: 2 })],
    nextBefore: '2026-09-01T12:00:00Z',
  });
  await assert.rejects(
    history.fetchAttackEconomyAggregates(fetcher, { view: 'tower' }),
    /repeated cursor/,
  );
});

test('range cutoffs and aggregate timestamps stay UTC based', () => {
  assert.equal(
    history.attackEconomyRangeSince(60, Date.parse('2026-09-01T10:01:00Z')),
    '2026-09-01T10:00:00.000Z',
  );
  assert.equal(
    history.aggregateTimestamp(aggregate('2026-09-01T10:00:00Z', { W: 1 })),
    Date.parse('2026-09-01T10:00:00Z') / 1000,
  );
  assert.equal(
    history.aggregateEndTimestamp({ ...aggregate('2026-09-01T10:00:00Z', { W: 1 }), bucketSeconds: 3_600 }),
    Date.parse('2026-09-01T11:00:00Z') / 1000,
  );
});

test('feature summaries keep every positive resource across rendered views', () => {
  const rows = [
    { ...aggregate('2026-09-01T10:00:00Z', { W: 3, HONEY: 5 }), viewKey: 'tower', gallantryPoints: 1 },
    { ...aggregate('2026-09-01T10:01:00Z', { C2: 2, EVENT_REWARD: 7 }), viewKey: 'storm', gallantryPoints: 2 },
    { ...aggregate('2026-09-01T10:02:00Z', { IAP: 11, ZERO_ONLY: 0 }), viewKey: 'invasion', gallantryPoints: 3 },
    { ...aggregate('2026-09-01T10:03:00Z', { MEAD: 13 }), viewKey: 'berimond', gallantryPoints: 4 },
  ];

  const summary = history.summarizeAttackEconomyResources(rows);
  assert.equal(summary.gallantryPoints, 10);
  assert.deepEqual(summary.loot, {
    W: 3,
    HONEY: 5,
    C2: 2,
    EVENT_REWARD: 7,
    IAP: 11,
    MEAD: 13,
  });
  assert.equal(history.attackEconomyResourceFallbackLabel('EVENT_REWARD'), 'Event Reward');
  assert.equal(history.attackEconomyResourceFallbackLabel('IAP'), 'IAP');
});

test('desktop stat views open retained history locally and expose every stored report category', () => {
  const economyView = readFileSync(new URL('../src/attackAnalytics/components/AttackEconomyView.tsx', import.meta.url), 'utf8');
  const eventsView = readFileSync(new URL('../src/views/EventsView.tsx', import.meta.url), 'utf8');
  const playerTracker = readFileSync(new URL('../src/playerTracker/components/PlayerTrackerView.tsx', import.meta.url), 'utf8');
  assert.match(economyView, /useState<RangeKey>\('30d'\)/);
  assert.match(economyView, /autoNomad: 'nomad'/);
  assert.match(economyView, /autoAdvisor: 'advisor'/);
  assert.match(economyView, /autoKhan: 'khan'/);
  assert.match(economyView, /riftMaiden: 'rift'/);
  assert.match(economyView, /riftReplay: 'rift-replay'/);
  assert.match(economyView, /fetchAttackEconomyAggregates\(runtimeFetch,/);
  assert.match(eventsView, /attackEconomyFeatureDefinitions\.map/);
  assert.equal((playerTracker.match(/useState<RangeKey>\('30d'\)/g) ?? []).length, 2);
  assert.match(playerTracker, /runtimeFetch\(`\/api\/v2\/history\/player-tracker/);
});

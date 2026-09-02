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
const productionState = await vite.ssrLoadModule('/src/settings/QueueProductionClientState.ts');
const recruitState = await vite.ssrLoadModule('/src/settings/RecruitTroopsClientState.ts');

after(async () => {
  await vite.close();
});

test('Auto Recruit normalization preserves the glory fallback and stable castle kingdom', () => {
  const normalized = recruitState.normalizeRecruitTroopsSettings({
    mode: 'perCastle',
    checkIntervalSec: 300,
    recruitLevel10OnTitleLoss: true,
    globalItems: [],
    castles: {
      77: { enabled: true, kingdomId: 4, items: [{ id: 238, minId: 238, maxId: 493 }], cursor: 0 },
    },
  });

  assert.equal(normalized.recruitLevel10OnTitleLoss, true);
  assert.equal(normalized.castles['77'].kingdomId, 4);
  assert.deepEqual(normalized.castles['77'].items, [{ id: 238, minId: 238, maxId: 493, amount: 0 }]);

  const legacy = recruitState.normalizeRecruitTroopsSettings({
    mode: 'perCastle',
    castles: { 78: { enabled: true, items: [{ id: 238 }] } },
  });
  assert.equal(legacy.castles['78'].kingdomId, undefined);
});

test('saved Storm identity keeps an old schedule key bound to the current seasonal castle', () => {
  const settings = recruitState.normalizeRecruitTroopsSettings({
    mode: 'perCastle',
    castles: {
      77: { enabled: true, kingdomId: 4, items: [{ id: 489 }] },
    },
  });
  const liveCastles = [
    { id: 1, kingdomId: 0 },
    { id: 88, kingdomId: 4 },
  ];

  assert.equal(
    productionState.queueProductionCastleConfigurationKey(settings, liveCastles[1], liveCastles),
    '77',
  );
  assert.equal(
    productionState.queueProductionLiveCastleForKey(settings, '77', liveCastles)?.id,
    88,
  );
});

test('legacy Storm history restores only a proven seasonal castle id', () => {
  const settings = recruitState.normalizeRecruitTroopsSettings({
    mode: 'perCastle',
    castles: {
      77: { enabled: true, items: [{ id: 489 }] },
    },
  });
  const liveCastles = [{ id: 88, kingdomId: 4 }];
  const knownStormIDs = productionState.queueProductionKnownStormCastleIDs(
    { 77: '2026-08-01T00:00:00Z' },
    undefined,
    undefined,
  );

  assert.deepEqual(knownStormIDs, [77]);
  assert.equal(
    productionState.queueProductionCastleConfigurationKey(settings, liveCastles[0], liveCastles, knownStormIDs),
    '77',
  );
  assert.equal(
    productionState.queueProductionCastleConfigurationKey(settings, liveCastles[0], liveCastles, []),
    '88',
    'an unrelated missing castle must not be guessed to be Storm',
  );

  const enriched = productionState.applyQueueProductionCastleIdentityMetadata(
    settings,
    liveCastles,
    knownStormIDs,
  );
  assert.equal(enriched.castles['77'].kingdomId, 4);
  assert.equal(enriched.castles['88'], undefined);
});

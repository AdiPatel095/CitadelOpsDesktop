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
const { AUTO_STORM_BASELINE_TROOPS, presentAutoStormTroopCap } = await vite.ssrLoadModule(
  '/src/settings/AutoStormTroopCapPresentation.ts',
);

after(async () => {
  await vite.close();
});

test('normalizes reset-session demand without changing the server-selected cap', () => {
  const presented = presentAutoStormTroopCap({
    available: true,
    maximumTroops: 8_640,
    troopsPerAttack: 1_000,
    minimumTroops: 500,
    baselineTroops: 5_000,
    enabledPresetCount: 2,
    averagePresetTroops: 720,
    resetSessionAvailable: true,
    resetSessionStartedAt: '2026-09-08T00:00:00Z',
    attacksSinceReset: 288,
    averageAttacksPerHour: 12,
    rateBasedTroops: 8_640,
    capBasis: 'reset_rate',
  });

  assert.equal(presented.available, true);
  assert.equal(presented.maximumTroops, 8_640);
  assert.equal(presented.averageAttacksPerHour, 12);
  assert.equal(presented.averagePresetTroops, 720);
  assert.equal(presented.capBasis, 'reset_rate');
});

test('keeps older preview responses safe while backend and client are updated independently', () => {
  const presented = presentAutoStormTroopCap({
    available: true,
    maximumTroops: 1_250,
    troopsPerAttack: 600,
    minimumTroops: 0,
    historyHours: 24,
    attacksInHistory: 4,
    measuredAttacksInHistory: 4,
    troopsSentInHistory: 2_400,
    averageTroopsPerHour: 100,
    bufferedTroops: 200,
  });

  assert.equal(presented.available, true);
  assert.equal(presented.maximumTroops, 1_250);
  assert.equal(presented.baselineTroops, AUTO_STORM_BASELINE_TROOPS);
  assert.equal(presented.resetSessionAvailable, false);
  assert.equal(presented.averageAttacksPerHour, null);
  assert.equal(presented.averagePresetTroops, null);
  assert.equal(presented.capBasis, null);
});

test('does not present a zero preset average when no valid preset contributed', () => {
  const presented = presentAutoStormTroopCap({
    available: false,
    maximumTroops: 5_000,
    troopsPerAttack: 0,
    minimumTroops: 0,
    baselineTroops: 5_000,
    enabledPresetCount: 0,
    averagePresetTroops: 0,
    resetSessionAvailable: false,
    attacksSinceReset: 0,
    averageAttacksPerHour: 0,
    rateBasedTroops: 0,
    capBasis: 'baseline',
  });

  assert.equal(presented.enabledPresetCount, 0);
  assert.equal(presented.averagePresetTroops, null);
});

test('rejects invalid preview numbers instead of formatting NaN or infinity', () => {
  const presented = presentAutoStormTroopCap({
    available: true,
    maximumTroops: Number.NaN,
    troopsPerAttack: Number.POSITIVE_INFINITY,
    minimumTroops: 0,
    baselineTroops: -1,
    enabledPresetCount: Number.NaN,
    averagePresetTroops: Number.POSITIVE_INFINITY,
    resetSessionAvailable: true,
    resetSessionStartedAt: 'not-a-date',
    attacksSinceReset: -2,
    averageAttacksPerHour: Number.NaN,
    rateBasedTroops: Number.POSITIVE_INFINITY,
    capBasis: 'baseline',
  });

  assert.equal(presented.available, false);
  assert.equal(presented.maximumTroops, null);
  assert.equal(presented.baselineTroops, AUTO_STORM_BASELINE_TROOPS);
  assert.equal(presented.resetSessionStartedAt, undefined);
  assert.equal(presented.attacksSinceReset, null);
  assert.equal(presented.rateBasedTroops, null);
});

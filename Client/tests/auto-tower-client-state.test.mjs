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
const towerState = await vite.ssrLoadModule('/src/settings/AutoTowerClientState.ts');

after(async () => {
  await vite.close();
});

test('Auto Towers defaults to regular mode without token consumption or a Time Skip budget', () => {
  const settings = towerState.defaultAutoTowerClientState();

  assert.equal(settings.version, 4);
  assert.equal(settings.useAdvisor, false);
  assert.equal(settings.autoActivateAdvisor, false);
  assert.equal(settings.maximumDailyTimeSkips, 0);
  assert.equal(Object.hasOwn(settings, 'chainSameTower'), false);
});

test('Auto Towers preserves explicit Advisor controls and clamps the daily Time Skip budget', () => {
  const advisor = towerState.parseAutoTowerClientState({
    version: 4,
    useAdvisor: true,
    autoActivateAdvisor: true,
    maximumDailyTimeSkips: 37,
    castles: { 1: { enabled: true, radius: 7, unitId: 77, maidenOnly: false } },
  });
  assert.equal(advisor.version, 4);
  assert.equal(advisor.useAdvisor, true);
  assert.equal(advisor.autoActivateAdvisor, true);
  assert.equal(advisor.maximumDailyTimeSkips, 37);

  const clamped = towerState.parseAutoTowerClientState({ maximumDailyTimeSkips: 100_000 });
  assert.equal(clamped.maximumDailyTimeSkips, 9998);
});

test('Auto Towers retires the legacy chain toggle without inventing a Time Skip budget', () => {
  const legacy = towerState.parseAutoTowerClientState({
    version: 3,
    useAdvisor: true,
    autoActivateAdvisor: true,
    chainSameTower: true,
    castles: {},
  });

  assert.equal(legacy.version, 4);
  assert.equal(legacy.useAdvisor, true);
  assert.equal(legacy.autoActivateAdvisor, true);
  assert.equal(legacy.maximumDailyTimeSkips, 0);
  assert.equal(Object.hasOwn(legacy, 'chainSameTower'), false);
});

test('Auto Towers safely upgrades settings that predate Advisor mode', () => {
  const legacy = towerState.parseAutoTowerClientState({ version: 2, castles: {} });
  assert.equal(legacy.useAdvisor, false);
  assert.equal(legacy.autoActivateAdvisor, false);
  assert.equal(legacy.maximumDailyTimeSkips, 0);
});

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
const fortressState = await vite.ssrLoadModule('/src/settings/AutoFortressClientState.ts');

after(async () => {
  await vite.close();
});

test('Auto Fortress defaults preserve the speed-first attack guards without owning premium spend', () => {
  const settings = fortressState.defaultAutoFortressClientState();

  assert.equal(settings.checkIntervalSec, 5);
  assert.equal('radius' in settings, false);
  assert.equal(settings.horseTravelBoostId, 1009);
  assert.equal(settings.minimumCommanderSpeedBonus, 100);
  assert.equal('dailySpeedBooster' in settings, false);
  assert.deepEqual(settings.kingdoms, {
    1: { enabled: false },
    2: { enabled: false },
    3: { enabled: false },
  });
});

test('Auto Fortress normalization ignores legacy radius and keeps exact 100-unit limits', () => {
  const settings = fortressState.parseAutoFortressClientState({
    checkIntervalSec: 0,
    radius: 999,
    minimumCommanderSpeedBonus: 4,
    direwolfPurchaseLimit: 5_551,
    dailySpeedBooster: { enabled: false, rubyCostCeiling: 1, minimumRubyReserve: 400 },
    kingdoms: {
      1: { enabled: true },
      2: { enabled: false },
      3: { enabled: true },
      99: { enabled: true },
    },
  });

  assert.equal(settings.checkIntervalSec, 1);
  assert.equal('radius' in settings, false);
  assert.equal(settings.minimumCommanderSpeedBonus, 100);
  assert.equal(settings.direwolfPurchaseLimit, 5_600);
  assert.equal('dailySpeedBooster' in settings, false);
  assert.deepEqual(Object.keys(settings.kingdoms), ['1', '2', '3']);
  assert.equal(settings.kingdoms['1'].enabled, true);
  assert.equal(settings.kingdoms['3'].enabled, true);
});

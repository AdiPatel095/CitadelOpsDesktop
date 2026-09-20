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
const boosterState = await vite.ssrLoadModule('/src/settings/AutoBoosterClientState.ts');

after(async () => {
  await vite.close();
});

test('Auto Booster keeps the exact premium ceiling and a conservative daily cadence', () => {
  assert.deepEqual(boosterState.defaultAutoBoosterClientState(), {
    version: 1,
    checkIntervalSec: 60,
    rubyCostCeiling: 2500,
    minimumRubyReserve: 0,
  });
});

test('Auto Booster ignores an altered saved ceiling and normalizes the reserve', () => {
  const settings = boosterState.parseAutoBoosterClientState({
    version: 99,
    checkIntervalSec: 1,
    rubyCostCeiling: 9999,
    minimumRubyReserve: 1234.9,
  });

  assert.equal(settings.version, 1);
  assert.equal(settings.checkIntervalSec, 30);
  assert.equal(settings.rubyCostCeiling, 2500);
  assert.equal(settings.minimumRubyReserve, 1234);
});

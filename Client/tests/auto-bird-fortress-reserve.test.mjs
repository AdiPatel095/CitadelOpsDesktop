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
const reserve = await vite.ssrLoadModule('/src/settings/AutoBirdFortressReserve.ts');
const birdState = await vite.ssrLoadModule('/src/settings/AutoBirdClientState.ts');
const presets = await vite.ssrLoadModule('/src/settings/AutoBirdPresets.ts');

after(async () => {
  await vite.close();
});

const fortressConfig = {
  kingdoms: {
    1: { enabled: true },
    2: { enabled: false },
    3: { enabled: true },
  },
};

test('Auto Fortress Direwolf reserve is limited to enabled outer main castles', () => {
  assert.equal(reserve.autoFortressReservesDirewolves(true, { kingdomId: 1, slotType: 12 }, fortressConfig), true);
  assert.equal(reserve.autoFortressReservesDirewolves(true, { kingdomId: 3, slotType: 12 }, fortressConfig), true);
  assert.equal(reserve.autoFortressReservesDirewolves(false, { kingdomId: 1, slotType: 12 }, fortressConfig), false);
  assert.equal(reserve.autoFortressReservesDirewolves(true, { kingdomId: 2, slotType: 12 }, fortressConfig), false);
  assert.equal(reserve.autoFortressReservesDirewolves(true, { kingdomId: 1, slotType: 1 }, fortressConfig), false);
  assert.equal(reserve.autoFortressReservesDirewolves(true, { kingdomId: 0, slotType: 12 }, fortressConfig), false);
});

test('locked picker edits preserve the hidden manual Direwolf reserve', () => {
  const manual = [{ id: 277, amount: 75 }, { id: 489, amount: 10 }];
  assert.deepEqual(reserve.visibleAutoBirdReserveItems(manual, true), [{ id: 489, amount: 10 }]);
  assert.deepEqual(
    reserve.mergeAutoBirdPickerItems(manual, [{ id: 215, amount: 20 }, { id: 277, amount: 1 }], true),
    [{ id: 215, amount: 20 }, { id: 277, amount: 75 }],
  );
  assert.deepEqual(reserve.visibleAutoBirdReserveItems(manual, false), manual);
});

test('automatic all-Direwolf reserve is never serialized into Auto Bird settings or presets', () => {
  const settings = birdState.defaultAutoBirdSettings();
  const document = birdState.buildAutoBirdClientState(settings, presets.emptyPresetsFile());

  assert.deepEqual(document.ignoreSettings.settings, {});
  assert.deepEqual(document.presets.presets, []);
  assert.equal(reserve.autoFortressReservesDirewolves(true, { kingdomId: 1, slotType: 12 }, fortressConfig), true);
});

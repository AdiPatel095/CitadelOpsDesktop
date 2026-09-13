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
const birdState = await vite.ssrLoadModule('/src/settings/AutoBirdClientState.ts');

after(async () => {
  await vite.close();
});

test('Auto Bird upgrades legacy ignored-troop settings to a runtime-selectable document', () => {
  const state = birdState.parseAutoBirdClientState({
    version: 1,
    ignoreSettings: {
      settings: { 10: [{ id: 489, amount: 25 }] },
      minDelay: 6,
      maxDelay: 12,
      minSend: 50,
      minRPTDays: 3,
    },
    presets: { version: 1, lastSelectedPresetId: null, presets: [] },
  });

  assert.equal(state.version, 2);
  assert.equal(state.activePresetId, null);
  assert.deepEqual(state.ignoreSettings.settings['10'], [{ id: 489, amount: 25 }]);
});

test('another feature can switch Auto Bird by stable preset ID without copying reserves', () => {
  const raw = {
    version: 2,
    activePresetId: 'day',
    ignoreSettings: birdState.defaultAutoBirdSettings(),
    presets: {
      version: 1,
      lastSelectedPresetId: 'day',
      presets: [
        { id: 'day', name: 'Day', settings: {}, minDelay: 6, maxDelay: 12, minSend: 0, minRPTDays: 3 },
        { id: 'night', name: 'Night', settings: {}, minDelay: 4, maxDelay: 8, minSend: 50, minRPTDays: 5 },
      ],
    },
  };

  const selected = birdState.activateAutoBirdPreset(raw, ' night ');
  assert.equal(selected.activePresetId, 'night');
  assert.deepEqual(selected.presets, raw.presets);
  assert.deepEqual(selected.ignoreSettings, raw.ignoreSettings);

  assert.equal(birdState.activateAutoBirdPreset(selected, null).activePresetId, null);
  assert.throws(
    () => birdState.activateAutoBirdPreset(raw, 'deleted'),
    /does not exist/,
  );
});

test('Auto Bird preserves a stale active ID so the UI and runtime can fail closed', () => {
  const state = birdState.parseAutoBirdClientState({
    version: 2,
    activePresetId: 'deleted',
    presets: { version: 1, lastSelectedPresetId: null, presets: [] },
  });

  assert.equal(state.activePresetId, 'deleted');
});

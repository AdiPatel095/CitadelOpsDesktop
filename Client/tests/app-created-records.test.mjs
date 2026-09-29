import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const core = await vite.ssrLoadModule('/src/presets/AppCreatedRecords.ts');
const attackTypes = await vite.ssrLoadModule('/src/attackPresets/AttackPresetTypes.ts');

after(async () => {
  await vite.close();
});

const S = 'automation.autoKhan';
const record = (id, name, extra = {}) => ({ id, name, value: 1, ...extra });
const ops = (value) => ({
  unchanged: (existing) => existing.value === value,
  update: (existing) => ({ ...existing, value }),
  create: (id) => ({ id, name: 'Created', value, app: { section: S, slot: 'defense' } }),
});

test('markers: malformed markers are dropped; attack types re-export the same parser', () => {
  assert.deepEqual(core.parseAppCreatedPresetMarker({ section: ' a ', slot: 'b' }), { section: 'a', slot: 'b' });
  for (const bad of [null, 'x', [], { section: '', slot: 'b' }, { section: 'a' }]) assert.equal(core.parseAppCreatedPresetMarker(bad), undefined);
  assert.equal(attackTypes.parseAppCreatedPresetMarker, core.parseAppCreatedPresetMarker);
  assert.match(core.newAppCreatedRecordId(S, 'defense'), /^app:automation\.autoKhan:defense:[0-9a-f]{8}$/);
  assert.equal(core.uniqueRecordName([{ name: 'A' }, { name: 'a 2' }], 'A'), 'A 3');
});

test('upsert: creates, updates only an owned record with the same id, and skips unchanged', () => {
  const owned = record('own', 'Own', { app: { section: S, slot: 'defense' } });
  const raw = { version: 1, presets: [owned, { id: 'broken' }], extra: true };
  const current = [owned];
  const unchanged = core.upsertOwnedRecord(raw, current, S, 'defense', 'own', ops(1));
  assert.equal(unchanged.changed, false);
  assert.equal(unchanged.presetId, 'own');
  const updated = core.upsertOwnedRecord(raw, current, S, 'defense', 'own', ops(2));
  assert.equal(updated.presetId, 'own');
  assert.deepEqual(updated.document.presets, [{ ...owned, value: 2 }, { id: 'broken' }], 'unparsed siblings are kept');
  assert.equal(updated.document.extra, true);
  const notOwned = core.upsertOwnedRecord(raw, current, S, 'attack', 'own', ops(2));
  assert.notEqual(notOwned.presetId, 'own', 'another slot never edits the record');
  assert.equal(notOwned.document.presets.length, 3);
});

test('promote and cleanup keep raw fields and never touch other sections or user records', () => {
  const mine = record('mine', 'Mine', { app: { section: S, slot: 'defense' }, 'x-extra': 'kept' });
  const shared = record('shared', 'Shared', { app: { section: S, slot: 'defense' } });
  const foreign = record('foreign', 'Foreign', { app: { section: 'other', slot: 'x' } });
  const user = record('user', 'User');
  const raw = { version: 1, presets: [mine, shared, foreign, user] };
  const promoted = core.promoteRecords(raw, [mine, shared, foreign, user], ['mine', 'user']);
  assert.deepEqual(promoted.promoted, ['mine']);
  assert.deepEqual(promoted.document.presets[0], { id: 'mine', name: 'Mine', value: 1, 'x-extra': 'kept' });
  const cleanup = core.removeUnreferencedRecords(raw, [mine, shared, foreign, user], S, [
    { section: 'automation.autoStorm', slot: 'forts', presetId: 'shared' },
  ]);
  assert.deepEqual(cleanup.removed, ['mine']);
  assert.deepEqual(cleanup.promoted, ['shared']);
  assert.deepEqual(cleanup.document.presets.map((entry) => [entry.id, entry.app ?? null]), [['shared', null], ['foreign', { section: 'other', slot: 'x' }], ['user', null]]);
});

import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const types = await vite.ssrLoadModule('/src/defensePresets/DefensePresetTypes.ts');
const app = await vite.ssrLoadModule('/src/defensePresets/AppCreatedDefensePresets.ts');

after(async () => {
  await vite.close();
});

const KHAN = 'automation.autoKhan';

function draft(amount) {
  const value = types.emptyDefensePresetDraft();
  value.wall.middle.toolSlots[1] = { definitionId: 620, amount };
  return value;
}

function raw(id, name, extra = {}) {
  return { id, name, ...draft(4), createdAt: '2026-09-01T00:00:00.000Z', updatedAt: '2026-09-01T00:00:00.000Z', ...extra };
}

test('the app marker round-trips on defense records; malformed markers make a user preset', () => {
  const document = types.parseDefensePresetDocument({ version: 1, presets: [raw('a', 'A', { app: { section: KHAN, slot: 'defense' } }), raw('b', 'B', { app: { section: 7 } })] });
  assert.deepEqual(document.presets[0].app, { section: KHAN, slot: 'defense' });
  assert.equal('app' in document.presets[1], false);
  assert.deepEqual(types.parseDefensePreset(raw('c', 'C')).app, undefined);
});

test('refs: owned records are inline, other app-created records carry their owner, missing ids stay visible', () => {
  const document = types.parseDefensePresetDocument({ version: 1, presets: [
    raw('own', 'Own', { app: { section: KHAN, slot: 'defense' } }),
    raw('other', 'Other', { app: { section: 'automation.other', slot: 'defense' } }),
    raw('user', 'User'),
  ] });
  const own = app.defenseSetupRef('own', document, KHAN, 'defense');
  assert.equal(own.source, 'inline');
  assert.equal(own.setup.wall.middle.toolSlots[1].amount, 4);
  assert.equal('name' in own.setup, false);
  assert.deepEqual(app.defenseSetupRef('other', document, KHAN, 'defense').appCreatedBy, { section: 'automation.other', slot: 'defense' });
  assert.deepEqual(app.defenseSetupRef('gone', document, KHAN, 'defense'), { source: 'preset', presetId: 'gone', missing: true });
  assert.deepEqual(app.defenseSetupRef(' ', document, KHAN, 'defense'), { source: 'none' });
  assert.equal(app.defenseSetupRefUsable(own, document), true);
  assert.equal(app.defenseSetupRefUsable({ source: 'preset', presetId: 'gone', missing: true }, document), false);
  assert.equal(app.summarizeDefenseSetupRef(own, document).badge, 'app');
  assert.equal(app.summarizeDefenseSetupRef(app.defenseSetupRef('user', document, KHAN, 'defense'), document).summary.toolAmount, 4);
});

test('upsert creates a marked record and skips an unchanged composition, even with unnormalized slots', () => {
  const setup = app.inlineDefenseFromPreset(draft(9));
  const created = app.upsertOwnedAppCreatedDefensePreset({ version: 1, presets: [] }, [], KHAN, 'defense', '', setup, 'Auto Khan – Main castle defense (auto)', '2026-09-29T00:00:00.000Z');
  assert.equal(created.changed, true);
  const [record] = created.document.presets;
  assert.deepEqual(record.app, { section: KHAN, slot: 'defense' });
  const current = types.parseDefensePresetDocument(created.document).presets;
  const trimmed = app.cloneInlineDefense(setup);
  trimmed.wall.left.toolSlots = trimmed.wall.left.toolSlots.slice(0, 1);
  const again = app.upsertOwnedAppCreatedDefensePreset(created.document, current, KHAN, 'defense', record.id, trimmed, 'x');
  assert.equal(again.changed, false, 'missing trailing empty slots compare equal after normalization');
  const changed = app.upsertOwnedAppCreatedDefensePreset(created.document, current, KHAN, 'defense', record.id, app.inlineDefenseFromPreset(draft(3)), 'x', '2026-09-30T00:00:00.000Z');
  assert.equal(changed.presetId, record.id);
  assert.equal(changed.document.presets[0].wall.middle.toolSlots[1].amount, 3);
  assert.equal(changed.document.presets[0].name, 'Auto Khan – Main castle defense (auto)', 'the generated name is never regenerated');
});

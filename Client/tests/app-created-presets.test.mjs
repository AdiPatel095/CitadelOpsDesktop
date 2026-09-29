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
const types = await vite.ssrLoadModule('/src/attackPresets/AttackPresetTypes.ts');
const app = await vite.ssrLoadModule('/src/attackPresets/AppCreatedPresets.ts');

after(async () => {
  await vite.close();
});

const NOMAD = 'automation.autoNomad';

function lane(troops, tools) {
  return { troops: troops.map(([itemId, quantity]) => ({ itemId, quantity })), tools: tools.map(([itemId, quantity]) => ({ itemId, quantity })) };
}

function rawPreset(id, name, extra = {}) {
  return {
    id,
    name,
    targetType: 'pve',
    useTroopFamilies: false,
    waves: [{ L: lane([[10, 5]], []), M: lane([[11, 7]], []), R: lane([], []) }],
    courtyardSupport: null,
    createdAt: '2026-09-01T00:00:00.000Z',
    updatedAt: '2026-09-01T00:00:00.000Z',
    ...extra,
  };
}

function setup(quantity = 9) {
  return {
    targetType: 'pve',
    useTroopFamilies: false,
    waves: [{ L: lane([[10, quantity], [null, 0]], [[null, 0], [null, 0]]), M: lane([], []), R: lane([], []) }],
    courtyardSupport: { troops: [], tools: [] },
  };
}

function reference(section, slot, presetId) {
  return { section, slot, presetId, moduleLabelKey: 'attackPresets.module.autoNomad', slotLabelKey: 'attackPresets.slot.nomad' };
}

test('parseAttackPreset round-trips the app marker and drops malformed markers', () => {
  const parsed = types.parseAttackPreset(rawPreset('a', 'A', { app: { section: NOMAD, slot: 'nomad' } }));
  assert.deepEqual(parsed.app, { section: NOMAD, slot: 'nomad' });
  const reparsed = types.parseAttackPreset(JSON.parse(JSON.stringify(parsed)));
  assert.deepEqual(reparsed.app, { section: NOMAD, slot: 'nomad' });

  for (const malformed of [null, 'x', [], { section: NOMAD }, { section: '', slot: 'nomad' }, { section: NOMAD, slot: '   ' }, { section: 1, slot: 'nomad' }]) {
    const preset = types.parseAttackPreset(rawPreset('b', 'B', { app: malformed }));
    assert.ok(preset, 'a malformed marker must not drop the preset');
    assert.equal(Object.hasOwn(preset, 'app'), false, JSON.stringify(malformed));
  }
  assert.equal(Object.hasOwn(types.parseAttackPreset(rawPreset('c', 'C')), 'app'), false);
});

test('attackSetupRef distinguishes user, own app-created, other slot app-created, missing and empty ids', () => {
  const document = types.parseAttackPresetDocument({
    version: 1,
    presets: [
      rawPreset('user', 'User preset'),
      rawPreset('own', 'Own', { app: { section: NOMAD, slot: 'nomad' } }),
      rawPreset('other', 'Other', { app: { section: 'automation.autoKhan', slot: 'attack' } }),
      rawPreset('sibling', 'Sibling', { app: { section: NOMAD, slot: 'samurai' } }),
    ],
  });
  assert.deepEqual(app.attackSetupRef('user', document, NOMAD, 'nomad'), { source: 'preset', presetId: 'user', missing: false });
  const own = app.attackSetupRef('own', document, NOMAD, 'nomad');
  assert.equal(own.source, 'inline');
  assert.equal(own.presetId, 'own');
  assert.equal(own.missing, false);
  assert.equal(own.setup.waves[0].L.troops[0].quantity, 5);
  assert.deepEqual(app.attackSetupRef('other', document, NOMAD, 'nomad'), {
    source: 'preset', presetId: 'other', missing: false, appCreatedBy: { section: 'automation.autoKhan', slot: 'attack' },
  });
  assert.deepEqual(app.attackSetupRef('sibling', document, NOMAD, 'nomad').appCreatedBy, { section: NOMAD, slot: 'samurai' });
  assert.deepEqual(app.attackSetupRef('gone', document, NOMAD, 'nomad'), { source: 'preset', presetId: 'gone', missing: true });
  assert.deepEqual(app.attackSetupRef('', document, NOMAD, 'nomad'), { source: 'none' });
  assert.deepEqual(app.attackSetupRef('  ', document, NOMAD, 'nomad'), { source: 'none' });
});

test('appCreatedPresetName suffixes collisions case-insensitively', () => {
  const format = (module, slot) => `${module} – ${slot} (auto)`;
  const existing = [
    { name: 'Auto Nomad – Nomad (auto)' },
    { name: 'auto nomad – nomad (auto) 2' },
  ];
  assert.equal(app.appCreatedPresetName([], 'Auto Nomad', 'Nomad', format), 'Auto Nomad – Nomad (auto)');
  assert.equal(app.appCreatedPresetName(existing, 'Auto Nomad', 'Nomad', format), 'Auto Nomad – Nomad (auto) 3');
  assert.equal(app.appCreatedPresetName([], 'Auto Invasion', 'Attack'), 'Auto Invasion – Attack (auto)');
});

test('newAppCreatedPresetId is scoped and not derivable', () => {
  const first = app.newAppCreatedPresetId(NOMAD, 'nomad');
  const second = app.newAppCreatedPresetId(NOMAD, 'nomad');
  assert.match(first, /^app:automation\.autoNomad:nomad:[0-9a-f]{8}$/);
  assert.notEqual(first, second);
});

test('upsertOwnedAppCreatedPreset reuses the id only on a marker match and otherwise creates a new record', () => {
  const raw = {
    version: 1,
    presets: [
      rawPreset('own', 'Own', { app: { section: NOMAD, slot: 'nomad' } }),
      rawPreset('promoted', 'Promoted'),
      rawPreset('sibling', 'Sibling', { app: { section: NOMAD, slot: 'samurai' } }),
    ],
  };
  const current = types.parseAttackPresetDocument(raw).presets;
  const now = '2026-09-29T12:00:00.000Z';

  const updated = app.upsertOwnedAppCreatedPreset(raw, current, NOMAD, 'nomad', 'own', setup(12), 'ignored', now);
  assert.equal(updated.presetId, 'own');
  assert.equal(updated.changed, true);
  const updatedRecord = updated.document.presets.find((preset) => preset.id === 'own');
  assert.equal(updatedRecord.name, 'Own', 'the stored title is never regenerated');
  assert.equal(updatedRecord.waves[0].L.troops[0].quantity, 12);
  assert.equal(updatedRecord.updatedAt, now);
  assert.equal(updatedRecord.createdAt, '2026-09-01T00:00:00.000Z');
  assert.deepEqual(updatedRecord.app, { section: NOMAD, slot: 'nomad' });
  assert.equal(updated.document.presets.length, 3);

  for (const currentId of ['promoted', 'sibling', 'missing', '']) {
    const created = app.upsertOwnedAppCreatedPreset(raw, current, NOMAD, 'nomad', currentId, setup(), 'Auto Nomad – Nomad (auto)', now);
    assert.notEqual(created.presetId, currentId, currentId);
    assert.match(created.presetId, /^app:automation\.autoNomad:nomad:/);
    assert.equal(created.document.presets.length, 4);
    const record = created.document.presets.at(-1);
    assert.deepEqual(record.app, { section: NOMAD, slot: 'nomad' });
    assert.equal(record.name, 'Auto Nomad – Nomad (auto)');
    assert.deepEqual(created.document.presets.slice(0, 3), raw.presets, 'existing raw records stay verbatim');
  }

  const same = app.upsertOwnedAppCreatedPreset(raw, current, NOMAD, 'nomad', 'own', app.inlineSetupFromPreset(current[0]), 'x', now);
  assert.equal(same.changed, false);
  assert.equal(same.presetId, 'own');
  assert.deepEqual(same.document.presets, raw.presets);
});

test('promoteAppCreatedPresets clears only the marker', () => {
  const raw = { version: 1, presets: [rawPreset('own', 'Own', { app: { section: NOMAD, slot: 'nomad' } }), rawPreset('user', 'User')] };
  const current = types.parseAttackPresetDocument(raw).presets;
  const result = app.promoteAppCreatedPresets(raw, current, ['own', 'user', 'missing']);
  assert.deepEqual(result.promoted, ['own']);
  const promoted = result.document.presets[0];
  const { app: _marker, ...expected } = current[0];
  assert.deepEqual(promoted, expected);
  assert.equal(result.document.presets[1], raw.presets[1], 'user presets remain the exact raw record');
});

test('removeUnreferencedAppCreated deletes only this section\'s unreferenced records and never user presets', () => {
  const raw = {
    version: 1,
    presets: [
      rawPreset('orphan', 'Orphan', { app: { section: NOMAD, slot: 'nomad' } }),
      rawPreset('kept', 'Kept', { app: { section: NOMAD, slot: 'samurai' } }),
      rawPreset('shared', 'Shared', { app: { section: NOMAD, slot: 'nomad' } }),
      rawPreset('khan-orphan', 'Khan orphan', { app: { section: 'automation.autoKhan', slot: 'attack' } }),
      rawPreset('user', 'User'),
    ],
  };
  const current = types.parseAttackPresetDocument(raw).presets;
  const references = [
    reference(NOMAD, 'samurai', 'kept'),
    reference('automation.autoInvasion', 'attack', 'shared'),
  ];
  const result = app.removeUnreferencedAppCreated(raw, current, NOMAD, references);
  assert.deepEqual(result.removed, ['orphan']);
  assert.deepEqual(result.promoted, ['shared']);
  const ids = result.document.presets.map((preset) => preset.id);
  assert.deepEqual(ids, ['kept', 'shared', 'khan-orphan', 'user']);
  assert.equal(result.document.presets[0], raw.presets[1]);
  assert.equal(Object.hasOwn(result.document.presets[1], 'app'), false);
  assert.equal(result.document.presets[2], raw.presets[3], 'another section\'s orphan is not this cleanup\'s business');
  assert.equal(result.document.presets[3], raw.presets[4]);
});

test('raw unparsed siblings and unknown document fields are preserved by every helper', () => {
  const unparsed = { id: 'future', name: 'Future', waves: 'v2-format' };
  const raw = { version: 1, extra: { keep: true }, presets: [unparsed, rawPreset('own', 'Own', { app: { section: NOMAD, slot: 'nomad' } })] };
  const current = types.parseAttackPresetDocument(raw).presets;
  const results = [
    app.upsertOwnedAppCreatedPreset(raw, current, NOMAD, 'nomad', 'own', setup(3), 'x').document,
    app.promoteAppCreatedPresets(raw, current, ['own']).document,
    app.removeUnreferencedAppCreated(raw, current, NOMAD, []).document,
  ];
  for (const document of results) {
    assert.equal(document.presets[0], unparsed);
    assert.deepEqual(document.extra, { keep: true });
  }
});

test('summarizeAttackSetupRef exposes badge and missing state', () => {
  const document = types.parseAttackPresetDocument({ version: 1, presets: [rawPreset('user', 'User'), rawPreset('own', 'Own', { app: { section: NOMAD, slot: 'nomad' } })] });
  assert.deepEqual(app.summarizeAttackSetupRef({ source: 'none' }, document), { name: '', summary: null, missing: false, badge: null });
  assert.equal(app.summarizeAttackSetupRef({ source: 'preset', presetId: 'user', missing: false }, document).badge, null);
  assert.equal(app.summarizeAttackSetupRef({ source: 'preset', presetId: 'own', missing: false }, document).badge, 'app');
  assert.equal(app.summarizeAttackSetupRef({ source: 'preset', presetId: 'gone', missing: true }, document).missing, true);
  const inline = app.summarizeAttackSetupRef({ source: 'inline', presetId: '', setup: setup(4), missing: false }, document);
  assert.equal(inline.badge, 'app');
  assert.equal(inline.summary.troops, 4);
  assert.equal(inline.missing, false);
});

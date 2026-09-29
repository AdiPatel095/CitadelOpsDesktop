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
const save = await vite.ssrLoadModule('/src/settings/AppCreatedPresetSave.ts');
const types = await vite.ssrLoadModule('/src/attackPresets/AttackPresetTypes.ts');
const app = await vite.ssrLoadModule('/src/attackPresets/AppCreatedPresets.ts');

after(async () => {
  await vite.close();
});

const NOMAD = 'automation.autoNomad';
const PRESETS = 'attacks.presets';

function lane(troops) {
  return { troops: troops.map(([itemId, quantity]) => ({ itemId, quantity })), tools: [] };
}

function rawPreset(id, name, extra = {}) {
  return {
    id,
    name,
    targetType: 'pve',
    useTroopFamilies: false,
    waves: [{ L: lane([[10, 5]]), M: lane([]), R: lane([]) }],
    courtyardSupport: null,
    createdAt: '2026-09-01T00:00:00.000Z',
    updatedAt: '2026-09-01T00:00:00.000Z',
    ...extra,
  };
}

function setup(quantity) {
  return {
    targetType: 'pve',
    useTroopFamilies: false,
    waves: [{ L: lane([[10, quantity]]), M: lane([]), R: lane([]) }],
    courtyardSupport: { troops: [], tools: [] },
  };
}

class ConflictError extends Error {
  constructor() {
    super('Settings changed elsewhere');
    this.code = 'configuration_conflict';
  }
}

/** Scripted stand-in for useConfigurationDraftSession: outcomes are consumed per write. */
function fakeDraftSession(sections, outcomes = []) {
  const state = { revision: 10, sections: structuredClone(sections) };
  const writes = [];
  const session = {
    sections: structuredClone(sections),
    writes,
    state,
    async saveSection(section, value) {
      const outcome = outcomes.shift() ?? 'ok';
      writes.push({ section, value: structuredClone(value), outcome });
      if (outcome === 'conflict') throw new ConflictError();
      if (outcome === 'fail') throw new Error(`write failed: ${section}`);
      state.revision += 1;
      state.sections[section] = structuredClone(value);
      session.sections = structuredClone(state.sections);
      return { schemaVersion: 2, revision: state.revision, updatedAt: '', sections: structuredClone(state.sections) };
    },
    async save(value) {
      return session.saveSection(NOMAD, value);
    },
  };
  return session;
}

function slots(nomadRef, samuraiRef = { source: 'none' }) {
  return [
    { slot: 'nomad', ref: nomadRef, moduleLabel: 'Auto Nomad', slotLabel: 'Nomad' },
    { slot: 'samurai', ref: samuraiRef, moduleLabel: 'Auto Nomad', slotLabel: 'Samurai' },
  ];
}

const buildSectionValue = (ids) => ({ version: 5, nomadPresetId: ids.nomad, samuraiPresetId: ids.samurai });

function presetsIn(session) {
  return types.parseAttackPresetDocument(session.state.sections[PRESETS]).presets;
}

test('success writes presets, then the module section, then cleanup', async () => {
  const session = fakeDraftSession({
    [PRESETS]: { version: 1, presets: [rawPreset('old', 'Old', { app: { section: NOMAD, slot: 'samurai' } }), rawPreset('user', 'User')] },
    [NOMAD]: { samuraiPresetId: 'old' },
  });
  const result = await save.saveModuleWithAppCreatedPresets({
    draftSession: session,
    section: NOMAD,
    slots: slots({ source: 'inline', presetId: '', setup: setup(7), missing: false }, { source: 'preset', presetId: 'user', missing: false }),
    buildSectionValue,
    formatPresetName: (module, slot) => `${module} – ${slot} (auto)`,
  });
  assert.deepEqual(session.writes.map((write) => write.section), [PRESETS, NOMAD, PRESETS]);
  assert.deepEqual(result.warnings, []);
  assert.match(result.idsBySlot.nomad, /^app:automation\.autoNomad:nomad:/);
  assert.equal(result.idsBySlot.samurai, 'user');
  assert.deepEqual(session.state.sections[NOMAD], { version: 5, nomadPresetId: result.idsBySlot.nomad, samuraiPresetId: 'user' });
  const presets = presetsIn(session);
  assert.deepEqual(presets.map((preset) => preset.id), ['user', result.idsBySlot.nomad], 'the unreferenced samurai record is cleaned up');
  const created = presets.find((preset) => preset.id === result.idsBySlot.nomad);
  assert.equal(created.name, 'Auto Nomad – Nomad (auto)');
  assert.deepEqual(created.app, { section: NOMAD, slot: 'nomad' });
  assert.equal(created.waves[0].L.troops[0].quantity, 7);
});

test('an unchanged presets document skips step 1', async () => {
  const own = rawPreset('own', 'Own', { app: { section: NOMAD, slot: 'nomad' } });
  const session = fakeDraftSession({ [PRESETS]: { version: 1, presets: [own] }, [NOMAD]: { nomadPresetId: 'own' } });
  const document = types.parseAttackPresetDocument(session.sections[PRESETS]);
  const ref = app.attackSetupRef('own', document, NOMAD, 'nomad');
  const result = await save.saveModuleWithAppCreatedPresets({ draftSession: session, section: NOMAD, slots: slots(ref), buildSectionValue });
  assert.deepEqual(session.writes.map((write) => write.section), [NOMAD]);
  assert.equal(result.idsBySlot.nomad, 'own');
});

test('a step-2 failure rolls back the presets write and rethrows', async () => {
  const baseline = { version: 1, presets: [rawPreset('user', 'User')] };
  const session = fakeDraftSession({ [PRESETS]: baseline, [NOMAD]: {} }, ['ok', 'fail', 'ok']);
  await assert.rejects(
    save.saveModuleWithAppCreatedPresets({
      draftSession: session, section: NOMAD, slots: slots({ source: 'inline', presetId: '', setup: setup(3), missing: false }), buildSectionValue,
    }),
    /write failed: automation\.autoNomad/,
  );
  assert.deepEqual(session.writes.map((write) => [write.section, write.outcome]), [[PRESETS, 'ok'], [NOMAD, 'fail'], [PRESETS, 'ok']]);
  assert.deepEqual(session.state.sections[PRESETS], baseline);
  assert.deepEqual(session.state.sections[NOMAD], {});
});

test('a rollback failure yields cleanup-pending and rethrows the original error', async () => {
  const session = fakeDraftSession({ [PRESETS]: { version: 1, presets: [] }, [NOMAD]: {} }, ['ok', 'conflict', 'fail']);
  const warnings = [];
  await assert.rejects(
    save.saveModuleWithAppCreatedPresets({
      draftSession: session, section: NOMAD, slots: slots({ source: 'inline', presetId: '', setup: setup(3), missing: false }), buildSectionValue, warnings,
    }),
    (error) => error instanceof ConflictError,
  );
  assert.deepEqual(warnings, ['cleanup-pending']);
  const orphan = presetsIn(session);
  assert.equal(orphan.length, 1, 'the worst case is a visible orphan, never a dangling reference');
  assert.deepEqual(session.state.sections[NOMAD], {});
});

test('a cleanup failure is a warning, not an error', async () => {
  const session = fakeDraftSession({
    [PRESETS]: { version: 1, presets: [rawPreset('user', 'User'), rawPreset('orphan', 'Orphan', { app: { section: NOMAD, slot: 'nomad' } })] },
    [NOMAD]: { nomadPresetId: 'orphan' },
  }, ['ok', 'fail']);
  const result = await save.saveModuleWithAppCreatedPresets({
    draftSession: session, section: NOMAD, slots: slots({ source: 'preset', presetId: 'user', missing: false }), buildSectionValue,
  });
  assert.deepEqual(result.warnings, ['cleanup-pending']);
  assert.deepEqual(session.writes.map((write) => [write.section, write.outcome]), [[NOMAD, 'ok'], [PRESETS, 'fail']]);
  assert.equal(session.state.sections[NOMAD].nomadPresetId, 'user');
});

test('a step-1 conflict writes nothing', async () => {
  const sections = { [PRESETS]: { version: 1, presets: [] }, [NOMAD]: { scoreTarget: 1 } };
  const session = fakeDraftSession(sections, ['conflict']);
  await assert.rejects(
    save.saveModuleWithAppCreatedPresets({
      draftSession: session, section: NOMAD, slots: slots({ source: 'inline', presetId: '', setup: setup(3), missing: false }), buildSectionValue,
    }),
    (error) => error instanceof ConflictError,
  );
  assert.equal(session.writes.length, 1);
  assert.deepEqual(session.state.sections, sections);
});

test('switching a slot to a user preset removes its app-created record', async () => {
  const session = fakeDraftSession({
    [PRESETS]: { version: 1, presets: [rawPreset('user', 'User'), rawPreset('own', 'Own', { app: { section: NOMAD, slot: 'nomad' } })] },
    [NOMAD]: { nomadPresetId: 'own' },
  });
  await save.saveModuleWithAppCreatedPresets({
    draftSession: session, section: NOMAD, slots: slots({ source: 'preset', presetId: 'user', missing: false }), buildSectionValue,
  });
  assert.deepEqual(presetsIn(session).map((preset) => preset.id), ['user']);
});

test('another slot\'s app-created record referenced here is promoted, not edited', async () => {
  const khanRecord = rawPreset('khan', 'Auto Khan – Attack (auto)', { app: { section: 'automation.autoKhan', slot: 'attack' } });
  const session = fakeDraftSession({
    [PRESETS]: { version: 1, presets: [khanRecord] },
    'automation.autoKhan': { attackPresetId: 'khan' },
    [NOMAD]: {},
  });
  await save.saveModuleWithAppCreatedPresets({
    draftSession: session,
    section: NOMAD,
    slots: slots({ source: 'preset', presetId: 'khan', missing: false, appCreatedBy: { section: 'automation.autoKhan', slot: 'attack' } }),
    buildSectionValue,
  });
  const [promoted] = presetsIn(session);
  assert.equal(promoted.id, 'khan');
  assert.equal(Object.hasOwn(promoted, 'app'), false);
  const { app: _marker, ...unchanged } = types.parseAttackPreset(khanRecord);
  assert.deepEqual(promoted, unchanged, 'composition, name and timestamps are untouched');
  assert.equal(session.state.sections[NOMAD].nomadPresetId, 'khan');
});

test('a record referenced by a second slot at save time is promoted and a new owned record is created', async () => {
  const own = rawPreset('own', 'Own', { app: { section: NOMAD, slot: 'nomad' } });
  const session = fakeDraftSession({
    [PRESETS]: { version: 1, presets: [own] },
    [NOMAD]: { nomadPresetId: 'own' },
    'automation.autoInvasion': { presetId: 'own' },
  });
  const document = types.parseAttackPresetDocument(session.sections[PRESETS]);
  const ref = app.attackSetupRef('own', document, NOMAD, 'nomad');
  const edited = { ...ref, setup: setup(42) };
  const result = await save.saveModuleWithAppCreatedPresets({ draftSession: session, section: NOMAD, slots: slots(edited), buildSectionValue });
  assert.notEqual(result.idsBySlot.nomad, 'own');
  const presets = presetsIn(session);
  const kept = presets.find((preset) => preset.id === 'own');
  assert.equal(Object.hasOwn(kept, 'app'), false, 'the shared record becomes a user preset');
  assert.equal(kept.waves[0].L.troops[0].quantity, 5, 'the shared record is never edited');
  const fresh = presets.find((preset) => preset.id === result.idsBySlot.nomad);
  assert.deepEqual(fresh.app, { section: NOMAD, slot: 'nomad' });
  assert.equal(fresh.waves[0].L.troops[0].quantity, 42);
});

test('Save as preset leaves the app-created record until the module save cleanup', async () => {
  const own = rawPreset('own', 'Own', { app: { section: NOMAD, slot: 'nomad' } });
  const session = fakeDraftSession({ [PRESETS]: { version: 1, presets: [own] }, [NOMAD]: { nomadPresetId: 'own' } });
  const newId = await save.saveInlineSetupAsUserPreset(session, setup(8), '  My Nomad  ', '2026-09-29T00:00:00.000Z');
  let presets = presetsIn(session);
  assert.deepEqual(presets.map((preset) => preset.id), ['own', newId]);
  const user = presets.find((preset) => preset.id === newId);
  assert.equal(user.name, 'My Nomad');
  assert.equal(Object.hasOwn(user, 'app'), false);
  assert.equal(session.state.sections[NOMAD].nomadPresetId, 'own', 'no dangling reference while the module is unsaved');

  await assert.rejects(save.saveInlineSetupAsUserPreset(session, setup(8), 'my nomad'), /duplicate/);
  await assert.rejects(save.saveInlineSetupAsUserPreset(session, setup(8), '   '), /empty/);

  await save.saveModuleWithAppCreatedPresets({
    draftSession: session, section: NOMAD, slots: slots({ source: 'preset', presetId: newId, missing: false }), buildSectionValue,
  });
  presets = presetsIn(session);
  assert.deepEqual(presets.map((preset) => preset.id), [newId]);
});

test('Save as preset failure writes nothing and surfaces the error', async () => {
  const session = fakeDraftSession({ [PRESETS]: { version: 1, presets: [] } }, ['fail']);
  await assert.rejects(save.saveInlineSetupAsUserPreset(session, setup(1), 'Name'), /write failed/);
  assert.deepEqual(session.state.sections[PRESETS], { version: 1, presets: [] });
});

import assert from 'node:assert/strict';
import { readFile, readdir } from 'node:fs/promises';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const core = await vite.ssrLoadModule('/src/settings/copy/castleCopy.ts');
const { castleCandidates } = await vite.ssrLoadModule('/src/settings/copy/candidates.ts');
const { towersCopyDescriptor } = await vite.ssrLoadModule('/src/settings/copy/features/towers.ts');
const { stationCopyDescriptor } = await vite.ssrLoadModule('/src/settings/copy/features/station.ts');
const { birdCopyDescriptor, birdCandidateFlags } = await vite.ssrLoadModule('/src/settings/copy/features/bird.ts');
const { recruitCopyDescriptor } = await vite.ssrLoadModule('/src/settings/copy/features/queueProduction.ts');
const { messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');

after(async () => {
  await vite.close();
});

const source = (path) => readFile(new URL(`../src/${path}`, import.meta.url), 'utf8');
const ZERO = '0001-01-01T00:00:00Z';
const SESSION = { generation: 5, baselineGeneration: 5, changedAt: '2026-09-29T09:00:00Z' };
const troops = Object.fromEntries([1, 2, 3, 277].map((id) => [id, { id, name: `U${id}` }]));
const castle = (id, kingdomId, slotType, stationed) => ({ id, name: `C${id}`, kingdomId, slotType, x: 0, y: 0, units: { stationed }, unitsObservedAt: ZERO, resources: {}, buildings: {} });
const castles = [castle(1, 0, 1, { 1: 100, 2: 100, 3: 100, 277: 100 }), castle(2, 1, 12, { 1: 100, 2: 3 }), castle(3, 0, 4, {}), castle(4, 2, 12, { 277: 50 })];
const state = { castles: Object.fromEntries(castles.map((entry) => [entry.id, entry])) };
const fortress = { enabled: true, section: { version: 1, kingdoms: { 1: { enabled: true } } } };
const base = { state, troops, tools: {}, metadataReady: true, observation: { session: SESSION, connected: true }, fortress };
const list = castles.map((entry) => ({ id: entry.id, name: entry.name, kingdomId: entry.kingdomId }));
const context = {
  ...base,
  candidates: castleCandidates(list, state, { flagsFor: (entry) => birdCandidateFlags(entry, base) }),
  allowedItemIds: (entry) => (entry.liveId === 2 ? [1] : [1, 2, 3]),
};

let seed = 7;
const random = () => { seed = (seed * 1103515245 + 12345) & 0x7fffffff; return seed / 0x7fffffff; };
const pick = (values) => values[Math.floor(random() * values.length)];
const entries = () => [...new Set(Array.from({ length: 1 + Math.floor(random() * 3) }, () => pick([1, 2, 3, 277, 999])))].map((id) => ({ id, amount: 1 + Math.floor(random() * 300) }));

const CASES = [
  ['towers', towersCopyDescriptor, () => ({ 1: { enabled: random() > 0.5, radius: 5 + Math.floor(random() * 20), unitId: pick([1, 2, 277, 999]), maidenOnly: random() > 0.5 }, 2: { enabled: false, radius: 10, unitId: 0, maidenOnly: false }, 3: { enabled: true, radius: 30, unitId: 1, maidenOnly: true } })],
  ['station', stationCopyDescriptor, () => ({ 1: entries(), 3: entries() })],
  ['bird', birdCopyDescriptor, () => ({ 1: entries(), 2: entries() })],
  ['recruit', recruitCopyDescriptor, () => ({ version: 1, mode: 'perCastle', checkIntervalSec: 300, globalItems: [{ id: 9, amount: 1 }], castles: { 1: { enabled: true, items: entries(), cursor: 4 }, 3: { enabled: true, items: entries(), cursor: 2 } } })],
];

test('never substitutes: every unit or item written to a destination was named by the source (randomized over all features)', () => {
  for (const [name, descriptor, makeDraft] of CASES) {
    for (let round = 0; round < 60; round += 1) {
      const draft = makeDraft();
      const preview = core.previewCastleCopy(descriptor, draft, '1', ['2', '3', '4'], context);
      const everything = { destinations: new Set(['2', '3', '4']), fields: new Set(descriptor.fields.map((field) => field.id)), keptIncludes: Object.fromEntries(['2', '3', '4'].map((key) => [key, new Set(descriptor.fields.map((field) => field.id))])) };
      const next = core.applyCastleCopy(descriptor, draft, preview, core.buildCopySelection(preview, everything));
      const sourceRecord = descriptor.recordFor(draft, '1');
      const sourceIds = new Set([sourceRecord.unitId, ...(Array.isArray(sourceRecord) ? sourceRecord : sourceRecord.items ?? []).map((entry) => entry.id)].filter(Boolean));
      for (const key of ['2', '3', '4']) {
        const written = descriptor.recordFor(next, key);
        if (written === undefined || written === descriptor.recordFor(draft, key)) continue;
        const ids = [written.unitId, ...(Array.isArray(written) ? written : written.items ?? []).map((entry) => entry.id)].filter(Boolean);
        const beforeIds = new Set([descriptor.recordFor(draft, key)?.unitId, ...(Array.isArray(descriptor.recordFor(draft, key)) ? descriptor.recordFor(draft, key) : descriptor.recordFor(draft, key)?.items ?? []).map((entry) => entry.id)].filter(Boolean));
        for (const id of ids) assert.ok(sourceIds.has(id) || beforeIds.has(id), `${name}: unit ${id} at castle ${key} was neither named by the source nor already there`);
        const state = preview.destinations.find((entry) => entry.key === key).state;
        assert.notEqual(state, 'incompatible', `${name}: an incompatible destination was written`);
      }
    }
  }
});

test('never changes fields outside the selection and never writes enabled unless selected', () => {
  for (const [name, descriptor, makeDraft] of CASES) {
    const draft = makeDraft();
    const preview = core.previewCastleCopy(descriptor, draft, '1', ['3'], context);
    const nonConsequential = new Set(descriptor.fields.filter((field) => !field.consequential).map((field) => field.id));
    const input = { destinations: new Set(['3']), fields: nonConsequential, keptIncludes: { 3: nonConsequential } };
    const next = core.applyCastleCopy(descriptor, draft, preview, core.buildCopySelection(preview, input));
    const enabledField = descriptor.fields.find((field) => field.consequential);
    if (enabledField) {
      const before = descriptor.recordFor(draft, '3');
      const after = descriptor.recordFor(next, '3');
      assert.equal(after === undefined ? undefined : enabledField.read(after), before === undefined ? enabledField.read(descriptor.defaultRecord()) : enabledField.read(before), `${name}: enabled changed without an explicit include`);
    }
    assert.equal(descriptor.recordFor(next, '1'), descriptor.recordFor(draft, '1'), `${name}: the source castle is never rewritten`);
    assert.equal(descriptor.recordFor(next, '2'), descriptor.recordFor(draft, '2'), `${name}: a castle outside the selection is untouched`);
  }
});

test('nothing outside the castle-scoped fields is reachable: Queue writes no mode, interval or shared list; Towers no account-wide value', () => {
  const draft = CASES[3][2]();
  const preview = core.previewCastleCopy(recruitCopyDescriptor, draft, '1', ['2'], context);
  const next = core.applyCastleCopy(recruitCopyDescriptor, draft, preview, { 2: new Set(['items']) });
  assert.equal(next.mode, draft.mode);
  assert.equal(next.checkIntervalSec, draft.checkIntervalSec);
  assert.deepEqual(next.globalItems, draft.globalItems);
  for (const descriptor of [towersCopyDescriptor, stationCopyDescriptor, birdCopyDescriptor, recruitCopyDescriptor]) {
    const ids = descriptor.fields.map((field) => field.id);
    for (const forbidden of ['horseTravelBoostId', 'dailyAttackLimit', 'mapRefreshIntervalSec', 'useAdvisor', 'checkIntervalSec', 'leadTimeSec', 'minDelay', 'minSend', 'minRPTDays', 'openGateFallback', 'commanders', 'schedule']) {
      assert.ok(!ids.includes(forbidden), `${descriptor.featureId}: ${forbidden} must not be copyable`);
    }
  }
  assert.deepEqual(towersCopyDescriptor.fields.map((field) => field.id), ['unitId', 'radius', 'maidenOnly', 'enabled']);
  assert.deepEqual(stationCopyDescriptor.fields.map((field) => field.id), ['reserves']);
  assert.deepEqual(birdCopyDescriptor.fields.map((field) => field.id), ['keep']);
  assert.deepEqual(recruitCopyDescriptor.fields.map((field) => field.id), ['items', 'enabled']);
});

test('the copy modules are pure and never write or start anything', async () => {
  const dir = new URL('../src/settings/copy/', import.meta.url);
  const files = [...(await readdir(dir)).filter((name) => name.endsWith('.ts')).map((name) => `settings/copy/${name}`), ...(await readdir(new URL('features/', dir))).map((name) => `settings/copy/features/${name}`)];
  assert.ok(files.length >= 8);
  for (const file of files) {
    const text = await source(file);
    assert.doesNotMatch(text, /submitIntent|updateConfiguration|queueConfigurationUpdate|setAutomationEnabled|automation\.enabled|fetch\(|CitadelAPI\./, file);
  }
  const dialog = await source('settings/components/CastleCopyDialog.tsx');
  assert.doesNotMatch(dialog, /submitIntent|updateConfiguration|queueConfigurationUpdate|setAutomationEnabled|\.save\(/, 'the dialog only edits the draft');
});

test('no player-facing "runtime" in the copy modules or their messages', async () => {
  const keys = Object.keys(messages).filter((key) => key.startsWith('castleCopy.'));
  assert.ok(keys.length > 50);
  for (const key of keys) assert.doesNotMatch(messages[key], /runtime/i, key);
  for (const file of ['settings/copy/castleCopy.ts', 'settings/copy/features/towers.ts', 'settings/copy/features/station.ts', 'settings/copy/features/bird.ts', 'settings/copy/features/queueProduction.ts', 'settings/copy/features/unitChecks.ts', 'settings/components/CastleCopyDialog.tsx']) {
    const text = await source(file);
    for (const [, literal] of text.matchAll(/'([^'\n]{12,})'/g)) if (/\s/.test(literal) && !literal.includes('/')) assert.doesNotMatch(literal, /\bruntime\b/i, `${file}: ${literal}`);
  }
});

test('the four editors expose an explicit Copy button, and Food Balance and Hospital have none', async () => {
  const wired = { AutoTowerSettingsModal: 'towersCopyDescriptor', AutoStationSettingsModal: 'stationCopyDescriptor', AutoBirdSettingsModal: 'birdCopyDescriptor', QueueProductionSettingsModal: 'copyDescriptor' };
  for (const [modal, descriptor] of Object.entries(wired)) {
    const text = await source(`settings/components/${modal}.tsx`);
    assert.match(text, /<CastleCopyButton/, modal);
    assert.match(text, new RegExp(`descriptor=\\{${descriptor}\\}`), modal);
    assert.match(text, /copyReplay: copyReplay\.sessionOption/, `${modal}: the conflict notice knows about a copied setup and can re-apply it`);
    assert.doesNotMatch(text, /new CastleCopyDialog|<CastleCopyDialog/, `${modal}: only the button opens the dialog`);
  }
  for (const modal of ['AutoFoodBalanceSettingsModal', 'AutoHospitalSettingsModal']) {
    assert.doesNotMatch(await source(`settings/components/${modal}.tsx`), /CastleCopy|castleCopy/, `${modal}: account-wide settings only, no per-castle copy`);
  }
  const session = await source('settings/ConfigurationDraftSession.tsx');
  assert.match(session, /castleCopy\.conflictNotice/);
  assert.equal(messages['castleCopy.conflictKept'], undefined, 'replaced by the notice that re-applies the copy (see castle-copy-replay.test.mjs)');
});

test('Queue Production offers Copy only per castle, never in shared mode or for calendar-slot castles', async () => {
  const text = await source('settings/components/QueueProductionSettingsModal.tsx');
  assert.match(text, /\{!isGlobalMode && !castleUsesScheduledItems \? \(\s*<CastleCopyButton/);
});

test('the placement matrix records Copy on the hosting rows (accepted) and the account-wide note on Food and Hospital', async () => {
  const matrix = await readFile(new URL('../../Docs/Features/SettingsPlacement.md', import.meta.url), 'utf8');
  const row = (feature, section) => matrix.split('\n').find((line) => line.startsWith(`| ${feature} | ${section} |`));
  for (const [feature, section] of [['autoTowers', 'castles'], ['autoStation', 'reserves'], ['autoBird', 'castles'], ['autoRecruit', 'plan'], ['autoTool', 'plan']]) {
    assert.match(row(feature, section), /Copy to other castles/, `${feature}/${section}`);
    assert.match(row(feature, section), /\| accepted \|$/, `${feature}/${section}: accepted by Maya's CIT-21 matrix review`);
  }
  assert.ok(matrix.includes('Maya matrix review 2026-09-29 (CIT-21), Desktop a505626 / Hosted f80f6af, Product/Simpler automation setup.md § CIT-21 matrix rows'), 'the CIT-21 evidence string');
  for (const [feature, section] of [['autoFoodBalance', 'reserves'], ['autoHospital', 'schedule']]) {
    assert.match(row(feature, section), /Account-wide settings only; no per-castle copy\./, `${feature}: Maya's note wording`);
  }
});

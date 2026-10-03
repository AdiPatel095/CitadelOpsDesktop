import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

// `latestSections()` (CIT-19 round 3): the saved sections as of right now, readable by an editor that is unmounted in the
// same render as its Save. The real session hook runs under the small React stand-in used by the draft-recovery hook test.
const SRC = '/src';
const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const support = (name) => fileURLToPath(new URL(`./support/${name}`, import.meta.url));
const vite = await createServer({
  root: clientRoot,
  appType: 'custom',
  logLevel: 'silent',
  server: { middlewareMode: true },
  resolve: {
    alias: [
      { find: /^react$/, replacement: support('miniReact.mjs') },
      { find: /^react\/jsx-(dev-)?runtime$/, replacement: support('miniReact.mjs') },
      { find: /^lucide-react$/, replacement: support('stubs.mjs') },
      { find: /^.*\/api\/useCitadelAPI$/, replacement: support('stubs.mjs') },
      { find: /^.*\/api\/CitadelClient$/, replacement: support('stubs.mjs') },
      { find: /^.*\/i18n\/useLocale$/, replacement: support('stubs.mjs') },
      { find: /^.*\/components\/ui(\/(Button|Modal))?$/, replacement: support('stubs.mjs') },
      { find: /^.*\/i18n\/LocalizedText$/, replacement: support('stubs.mjs') },
    ],
  },
});
const { mount } = await import(support('miniReact.mjs'));
const { useConfigurationDraftSession } = await vite.ssrLoadModule(`${SRC}/settings/ConfigurationDraftSession.tsx`);

after(async () => { await vite.close(); });

const SECTION = 'automation.autoTowers';
const SAVED = { version: 4, castles: { 1: { enabled: true, unitId: 215 } } };
const EDITED = { version: 4, castles: { 1: { enabled: true, unitId: 216 } } };
const snapshot = (revision, sections) => ({ schemaVersion: 2, revision, updatedAt: `2026-09-29T12:${String(revision).padStart(2, '0')}:00Z`, sections });
const macrotask = () => new Promise((resolve) => { setTimeout(resolve, 0); });

test('latestSections() returns the saved sections right after saveSection resolves, before any render sees them', async () => {
  const load = async () => snapshot(1, { [SECTION]: SAVED });
  globalThis.__stubApi = {
    loadLatestConfiguration: load, // desktop name
    refreshConfiguration: load, // hosted name
    updateConfiguration: async (section, value) => snapshot(2, { [section]: value }),
  };
  const handle = mount(function Host({ isOpen }) { return useConfigurationDraftSession({ isOpen, section: SECTION }); });
  handle.render({ isOpen: true });
  await macrotask();
  handle.settle();
  const opened = handle.value;
  assert.equal(opened.ready, true, 'the session loaded');
  assert.deepEqual(opened.latestSections()[SECTION], SAVED, 'before a save it is what was loaded');
  await opened.saveSection(SECTION, EDITED);
  // No render has happened since the save resolved: the rendered `sections` is still the loaded value ...
  assert.deepEqual(handle.value.sections[SECTION], SAVED);
  // ... but the reader already answers with what was saved.
  assert.deepEqual(opened.latestSections()[SECTION], EDITED);
  handle.settle();
  assert.deepEqual(handle.value.sections[SECTION], EDITED, 'and the next render agrees');
  // A failed save changes nothing.
  globalThis.__stubApi.updateConfiguration = async () => { throw new Error('rejected'); };
  handle.render({ isOpen: true });
  handle.settle();
  await assert.rejects(handle.value.saveSection(SECTION, SAVED));
  assert.deepEqual(handle.value.latestSections()[SECTION], EDITED);
  handle.unmount();
  // The reader still answers after the editor is gone (nothing clears the reference on unmount).
  assert.deepEqual(opened.latestSections()[SECTION], EDITED);
});

test('the reader is the same reference Save advances, and it is exposed by the session', async () => {
  const source = (await readFile(new URL('../src/settings/ConfigurationDraftSession.tsx', import.meta.url), 'utf8')).replace(/\/\*[\s\S]*?\*\//g, '');
  assert.match(source, /const latestSections = useCallback\(\(\) => snapshotRef\.current\?\.sections, \[\]\);/);
  assert.match(source, /recoverDraft,\s*latestSections,\s*recoveredExtras,/);
  const save = source.slice(source.indexOf('const captured = advanceConfigurationDraftAfterSave'), source.indexOf('return updated;', source.indexOf('const captured = advanceConfigurationDraftAfterSave')));
  assert.match(save, /snapshotRef\.current = captured;\s*setSnapshot\(captured\);/, 'the reference moves synchronously, before the state update');
});

import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { after, beforeEach, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

// The real useDraftRecovery hook run by a tiny React stand-in (tests/support/miniReact.mjs), in the order a browser gives:
// the editor's load effect and the hook's effects run in one flush, and what the load effect set renders LATER, after any
// zero-delay timer. jsdom is not a dependency of this repository, so this is the rendered-hook test (CIT-19 QA, item 1).
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
      { find: /^.*\/api\/ApiContext$/, replacement: support('stubs.mjs') },
      { find: /^.*\/components\/ui\/(Button|Modal)$/, replacement: support('stubs.mjs') },
      { find: /^.*\/i18n\/LocalizedText$/, replacement: support('stubs.mjs') },
    ],
  },
});
const { mount, useEffect, useState } = await import(support('miniReact.mjs'));
const { stubState } = await import(support('stubs.mjs'));
const { useDraftRecovery } = await vite.ssrLoadModule(`${SRC}/settings/useDraftRecovery.tsx`);
const recovery = await vite.ssrLoadModule(`${SRC}/settings/DraftRecovery.ts`);

after(async () => { await vite.close(); });

const store = new Map();
globalThis.localStorage = { getItem: (key) => store.get(key) ?? null, setItem: (key, value) => { store.set(key, String(value)); }, removeItem: (key) => { store.delete(key); } };
globalThis.window = globalThis;
beforeEach(() => { store.clear(); recovery.resetDraftPresenceForTests(); stubState.current = { account: { uid: 77, worldId: 'EmpireEx_2' } }; });

const SECTION = 'automation.autoTowers';
const DEFAULTS = { version: 4, mapRefreshIntervalSec: 1800, castles: {} };
const SAVED = { version: 4, mapRefreshIntervalSec: 3600, castles: { 1: { enabled: true, unitId: 215 } } };
const EDITED = { version: 4, mapRefreshIntervalSec: 3600, castles: { 1: { enabled: true, unitId: 216 } } };
const draftKeys = () => [...store.keys()].filter((key) => key.startsWith('citadelops.draft.v1.'));
const savedKeys = () => [...store.keys()].filter((key) => key.startsWith('citadelops.saved.v1.'));
const macrotask = () => new Promise((resolve) => { setTimeout(resolve, 0); });
const wait = (ms) => new Promise((resolve) => { setTimeout(resolve, ms); });

/** An editor the way the modals are written: state that starts at defaults, a load effect, then the recovery hook. */
function Host({ isOpen, session, copyReapplied }) {
  const [draft, setDraft] = useState(DEFAULTS);
  useEffect(() => {
    if (!isOpen || !session.ready) return;
    setDraft(session.initialSections[SECTION] ?? DEFAULTS);
  }, [session.initialSections, session.loadKey, isOpen, session.ready]);
  const result = useDraftRecovery({ section: SECTION, isOpen, draftSession: session, copyReapplied, draft, loaded: session.sections?.[SECTION] ?? DEFAULTS });
  return { draft, setDraft, ...result };
}

// `backing` is the session's live saved configuration (what `latestSections()` reads); `sections` is what the last render
// saw. They differ between a Save and the next render, which is exactly the window an unmounting editor lives in.
const sessionOf = (sections, loadKey = 'k:1') => {
  const backing = { sections };
  return { ready: true, loadKey, sections, initialSections: sections, snapshot: { revision: 3 }, recoverDraft() {}, latestSections: () => backing.sections, backing };
};
const unready = () => ({ ready: false, loadKey: '', sections: undefined, initialSections: undefined, snapshot: null, recoverDraft() {}, latestSections: () => undefined });

/** Opens an editor: not ready first, ready a macrotask later (a late configuration load), then the browser's ordering. */
async function open(handle, sections, { loadKey = 'k:1', copyReapplied } = {}) {
  handle.render({ isOpen: true, session: unready(), copyReapplied });
  await macrotask();
  const session = sessionOf(sections, loadKey);
  handle.render({ isOpen: true, session, copyReapplied });
  await wait(5); // a zero-delay timer registered in that flush fires here, before the editor's update renders
  handle.settle();
  return session;
}

for (const strict of [false, true]) {
  const label = strict ? ' (strict effects)' : '';

  test(`opening an editor whose saved settings are not its defaults records nothing${label}`, async () => {
    const handle = mount(Host, { strict });
    await open(handle, { [SECTION]: SAVED });
    await wait(700);
    handle.settle();
    assert.deepEqual(draftKeys(), [], 'no record of an unsaved draft');
    assert.equal(handle.value.dirty, false);
    assert.equal(handle.value.banner, null);
    assert.equal(recovery.isEditorDirty('77:EmpireEx_2', SECTION), false, 'the legend does not say "unsaved"');
    handle.unmount();
  });

  test(`opening with nothing saved, and Cancel then reopen, leaves no record and no banner${label}`, async () => {
    const handle = mount(Host, { strict });
    const first = await open(handle, {});
    await wait(700);
    assert.deepEqual(draftKeys(), []);
    handle.render({ isOpen: false, session: first });
    handle.settle();
    await open(handle, { [SECTION]: SAVED }, { loadKey: 'k:2' });
    await wait(700);
    assert.deepEqual(draftKeys(), []);
    assert.equal(handle.value.banner, null, 'no "unsaved changes" banner after a plain open and Cancel');
    handle.unmount();
  });
}

test('a real edit is recorded, kept when the editor closes, and offered on the next open', async () => {
  const handle = mount(Host);
  const session = await open(handle, { [SECTION]: SAVED });
  handle.value.setDraft(EDITED);
  handle.settle();
  assert.equal(handle.value.dirty, true);
  assert.equal(recovery.isEditorDirty('77:EmpireEx_2', SECTION), true);
  await wait(650);
  assert.equal(draftKeys().length, 1, 'recorded after the debounce');
  const record = JSON.parse(store.get(draftKeys()[0]));
  assert.deepEqual(record.draft, EDITED);
  assert.equal(record.baseDigest, recovery.stableDigest(SAVED));
  assert.equal(record.baseRevision, 3);
  handle.render({ isOpen: false, session });
  handle.settle();
  assert.equal(draftKeys().length, 1, 'Cancel keeps the draft');
  assert.equal(recovery.isEditorDirty('77:EmpireEx_2', SECTION), false);
  await open(handle, { [SECTION]: SAVED }, { loadKey: 'k:2' });
  assert.ok(handle.value.banner, 'the recovered draft is offered');
  assert.equal(handle.value.banner.props['data-draft-recovery'], 'baseline-unchanged');
  handle.unmount();
});

test('closing right after an edit writes at once, without waiting for the debounce', async () => {
  const handle = mount(Host);
  const session = await open(handle, { [SECTION]: SAVED });
  handle.value.setDraft(EDITED);
  handle.settle();
  assert.deepEqual(draftKeys(), []);
  handle.render({ isOpen: false, session });
  assert.equal(draftKeys().length, 1);
  handle.settle();
  handle.unmount();
});

test('changing it back to what was loaded clears the record', async () => {
  const handle = mount(Host);
  await open(handle, { [SECTION]: SAVED });
  handle.value.setDraft(EDITED);
  handle.settle();
  await wait(650);
  assert.equal(draftKeys().length, 1);
  handle.value.setDraft(SAVED);
  handle.settle();
  assert.deepEqual(draftKeys(), [], 'a revert leaves no stale record behind');
  assert.equal(handle.value.dirty, false);
  handle.unmount();
});

test('Save clears the record and notes the time; the saved value becomes the new baseline', async () => {
  const handle = mount(Host);
  const session = await open(handle, { [SECTION]: SAVED });
  handle.value.setDraft(EDITED);
  handle.settle();
  await wait(650);
  assert.equal(draftKeys().length, 1);
  // The save succeeded: the saved section is now what the editor holds (same load, no reload).
  const after = { ...session, sections: { [SECTION]: EDITED } };
  handle.render({ isOpen: true, session: after });
  handle.settle();
  assert.deepEqual(draftKeys(), []);
  assert.equal(savedKeys().length, 1);
  assert.equal(handle.value.dirty, false);
  await wait(650);
  assert.deepEqual(draftKeys(), [], 'the just-saved values are never written back as a draft');
  handle.value.setDraft(SAVED);
  handle.settle();
  assert.equal(handle.value.dirty, true, 'the next change counts again');
  handle.unmount();
});

test('Save and close in one render: the record is cleared and the saved values are not written as a draft', async () => {
  const handle = mount(Host);
  const session = await open(handle, { [SECTION]: SAVED });
  handle.value.setDraft(EDITED);
  handle.settle();
  await wait(650);
  assert.equal(draftKeys().length, 1);
  handle.render({ isOpen: false, session: { ...session, sections: { [SECTION]: EDITED } } });
  handle.settle();
  assert.deepEqual(draftKeys(), []);
  assert.equal(savedKeys().length, 1);
  handle.unmount();
});

test('an editor that never applies what it loaded records nothing at all (fail-safe)', async () => {
  const handle = mount(function Stuck({ isOpen, session }) {
    const [draft, setDraft] = useState({ ...DEFAULTS, quirk: 1 });
    const result = useDraftRecovery({ section: SECTION, isOpen, draftSession: session, draft, loaded: session.sections?.[SECTION] ?? DEFAULTS });
    return { draft, setDraft, ...result };
  });
  handle.render({ isOpen: true, session: sessionOf({ [SECTION]: SAVED }) });
  handle.settle();
  handle.value.setDraft({ ...SAVED, quirk: 2 });
  handle.settle();
  await wait(650);
  assert.deepEqual(draftKeys(), []);
  handle.unmount();
});

test('a record from another account is never offered', async () => {
  const handle = mount(Host);
  store.set('citadelops.draft.v1.99:OtherWorld.automation.autoTowers', JSON.stringify({ version: 1, section: SECTION, accountKey: '99:OtherWorld', draft: EDITED, baseRevision: 1, baseDigest: 'x', savedAt: new Date().toISOString() }));
  await open(handle, { [SECTION]: SAVED });
  assert.equal(handle.value.banner, null);
  handle.unmount();
});

// ——— Editors close by UNMOUNTING (App.tsx drops the modal), and Save + close can land in one render ———

async function editThenSaveWithoutRender(handle, { hops = 0, waitBeforeSave = 650 } = {}) {
  const session = await open(handle, { [SECTION]: SAVED });
  handle.value.setDraft(EDITED);
  handle.settle();
  if (waitBeforeSave > 0) await wait(waitBeforeSave);
  // Save succeeded: the session's live configuration moved on, and the editor is unmounted before any render saw it.
  session.backing.sections = { [SECTION]: EDITED };
  await macrotask();
  for (let index = 0; index < hops; index += 1) await Promise.resolve();
  return session;
}

for (const strict of [false, true]) {
  test(`Towers ordering: Save, then the editor unmounts before a render sees the new saved section, clears the record${strict ? ' (strict effects)' : ''}`, async () => {
    const handle = mount(Host, { strict });
    await editThenSaveWithoutRender(handle);
    assert.equal(draftKeys().length, 1, 'the edit had been recorded');
    handle.unmount();
    assert.deepEqual(draftKeys(), [], 'the just-saved values are not written back as a draft');
    assert.equal(savedKeys().length, 1, 'the save time is noted');
    assert.equal(recovery.isEditorDirty('77:EmpireEx_2', SECTION), false, 'the legend does not say "unsaved" or "recovered"');
    assert.equal(recovery.draftLine('77:EmpireEx_2', SECTION), 'none');
    await wait(700);
    assert.deepEqual(draftKeys(), []);
  });
}

test('Food Balance ordering: the same with promise hops between the save and the unmount', async () => {
  const handle = mount(Host);
  await editThenSaveWithoutRender(handle, { hops: 2 });
  handle.unmount();
  assert.deepEqual(draftKeys(), []);
  assert.equal(savedKeys().length, 1);
  assert.equal(recovery.isEditorDirty('77:EmpireEx_2', SECTION), false);
  await wait(700);
  assert.deepEqual(draftKeys(), []);
});

test('Save within 100 ms of the edit (nothing recorded yet), then unmount: no record at all', async () => {
  const handle = mount(Host);
  const session = await open(handle, { [SECTION]: SAVED });
  handle.value.setDraft(EDITED);
  handle.settle();
  session.backing.sections = { [SECTION]: EDITED };
  handle.unmount();
  await wait(700);
  assert.deepEqual(draftKeys(), []);
  assert.equal(savedKeys().length, 1);
});

test('guard: Cancel by unmounting without a save still records the unsaved edit, exactly once', async () => {
  const handle = mount(Host);
  await open(handle, { [SECTION]: SAVED });
  handle.value.setDraft(EDITED);
  handle.settle();
  handle.unmount();
  assert.equal(draftKeys().length, 1);
  assert.deepEqual(JSON.parse(store.get(draftKeys()[0])).draft, EDITED);
  assert.deepEqual(savedKeys(), []);
  await wait(700);
  assert.equal(draftKeys().length, 1);
});

test('guard: an editor that never became active writes and clears nothing when it unmounts', async () => {
  const handle = mount(Host);
  handle.render({ isOpen: true, session: unready() });
  store.set('citadelops.draft.v1.77:EmpireEx_2.automation.autoTowers', JSON.stringify({ version: 1, section: SECTION, accountKey: '77:EmpireEx_2', draft: EDITED, baseRevision: 1, baseDigest: 'x', savedAt: new Date().toISOString() }));
  handle.unmount();
  assert.equal(draftKeys().length, 1, 'a record waiting for review is not touched');
  assert.deepEqual(savedKeys(), []);
});

function renderedNodes(node) {
  if (Array.isArray(node)) return node.flatMap(renderedNodes);
  if (!node || typeof node !== 'object') return [];
  return [node, ...renderedNodes(node.props?.children)];
}

for (const copyReapplied of [true, false, undefined]) {
  test(`Compare intro after re-applied copy: ${String(copyReapplied)}`, async () => {
    assert.equal(recovery.writeDraft({
      version: 1, accountKey: '77:EmpireEx_2', section: SECTION, draft: EDITED,
      baseRevision: 1, baseDigest: recovery.stableDigest(SAVED), savedAt: new Date().toISOString(),
    }), null);
    const handle = mount(Host);
    const session = await open(handle, { [SECTION]: { ...SAVED, mapRefreshIntervalSec: 7200 } }, { copyReapplied });
    let restored = null;
    session.recoverDraft = (draft) => { restored = draft; };
    assert.equal(handle.value.banner.props['data-draft-recovery'], 'saved-since');
    const compareButton = renderedNodes(handle.value.banner).find((node) => (
      typeof node.props?.onClick === 'function' && renderedNodes(node.props.children).some((child) => child.props?.messageKey === 'draftRecovery.compare')
    ));
    assert.ok(compareButton, 'Compare is offered');
    compareButton.props.onClick();
    handle.settle();
    const compare = renderedNodes(handle.value.banner).find((node) => node.props?.['data-draft-compare'] !== undefined);
    assert.ok(compare, 'Compare content is rendered');
    const keys = renderedNodes(compare).map((node) => node.props?.messageKey).filter(Boolean);
    const intro = keys.indexOf('draftRecovery.compareIntro');
    assert.notEqual(intro, -1);
    if (copyReapplied) {
      assert.equal(keys[intro + 1], 'draftRecovery.compareIntroReappliedCopy');
      const paragraphs = compare.props.children.filter((node) => node?.type === 'p');
      assert.equal(paragraphs[1].props.children.props.messageKey, 'draftRecovery.compareIntroReappliedCopy', 'warning immediately follows the existing intro paragraph');
    } else {
      assert.equal(keys.includes('draftRecovery.compareIntroReappliedCopy'), false);
    }
    assert.equal(restored, null, 'Compare does not restore or save');
    handle.unmount();
  });
}

test('all four copy editors pass re-applied state and route every draft save catch through genericSaveError', async () => {
  for (const editor of ['AutoTowerSettingsModal', 'AutoStationSettingsModal', 'AutoBirdSettingsModal', 'QueueProductionSettingsModal']) {
    const source = await readFile(new URL(`..${SRC}/settings/components/${editor}.tsx`, import.meta.url), 'utf8');
    assert.match(source, /copyReapplied: copyReapplied\(copyReplay\)/, editor);
    assert.doesNotMatch(source, /setSaveError\(error instanceof Error/, editor);
    const saveCatches = [...source.matchAll(/try\s*\{([\s\S]*?)\}\s*catch\s*\(error\)\s*\{([\s\S]*?)\}/g)]
      .filter(([, body]) => body.includes('draftSession.save('));
    assert.equal(saveCatches.length, editor === 'AutoBirdSettingsModal' ? 3 : 1, `${editor}: every save catch is covered`);
    for (const [, , body] of saveCatches) assert.match(body, /setSaveError\(genericSaveError\(error, copyReplay,/, editor);
  }
});

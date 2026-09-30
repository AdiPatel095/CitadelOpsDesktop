import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const core = await vite.ssrLoadModule('/src/settings/copy/castleCopy.ts');
const replay = await vite.ssrLoadModule('/src/settings/copy/castleCopyReplay.ts');
const { castleCandidates } = await vite.ssrLoadModule('/src/settings/copy/candidates.ts');
const { towersCopyDescriptor } = await vite.ssrLoadModule('/src/settings/copy/features/towers.ts');
const { stationCopyDescriptor } = await vite.ssrLoadModule('/src/settings/copy/features/station.ts');
const { birdCopyDescriptor, birdCandidateFlags } = await vite.ssrLoadModule('/src/settings/copy/features/bird.ts');
const { CastleCopyBody } = await vite.ssrLoadModule('/src/settings/components/CastleCopyDialog.tsx');
const session = await vite.ssrLoadModule('/src/settings/ConfigurationDraftSession.tsx');
const { messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');

after(async () => {
  await vite.close();
});

const ZERO = '0001-01-01T00:00:00Z';
const SESSION = { generation: 5, baselineGeneration: 5, changedAt: '2026-09-29T09:00:00Z' };
const troops = { 1: { id: 1, name: 'Spearmen' }, 2: { id: 2, name: 'Crossbowmen' }, 277: { id: 277, name: 'Direwolves' } };
const castle = (id, name, kingdomId, slotType, stationed = {}) => ({ id, name, kingdomId, slotType, x: 0, y: 0, units: { stationed }, unitsObservedAt: ZERO, resources: {}, buildings: {} });
function context(castles, extra = {}, flagsFor) {
  const state = { castles: Object.fromEntries(castles.map((entry) => [entry.id, entry])) };
  return {
    state, troops, tools: {}, metadataReady: true, observation: { session: SESSION, connected: true },
    candidates: castleCandidates(castles.map((entry) => ({ id: entry.id, name: entry.name, kingdomId: entry.kingdomId })), state, flagsFor ? { flagsFor } : {}),
    ...extra,
  };
}
const castles = [castle(1, 'Sunhold', 0, 1, { 2: 500 }), castle(2, 'Frostkeep', 0, 4, { 2: 500 }), castle(3, 'Ashvale', 0, 4, { 2: 500 }), castle(4, 'Mirefen', 0, 4, { 2: 500 })];
const ctx = context(castles);
const SOURCE_DRAFT = { 1: [{ id: 2, amount: 100 }] };

/** The whole flow the editor performs, without a DOM: apply a reviewed copy, conflict, reload, re-check. */
function applyReviewed(draft, destinations = ['2', '3']) {
  const preview = core.previewCastleCopy(stationCopyDescriptor, draft, '1', ctx.candidates.map((entry) => entry.key), ctx);
  const input = { ...core.defaultCopyInput(stationCopyDescriptor, preview), destinations: new Set(destinations) };
  const selection = core.buildCopySelection(preview, input);
  const next = core.applyCastleCopy(stationCopyDescriptor, draft, preview, selection);
  return { next, record: replay.makeReplay('1', input, preview, selection), selection };
}
const recheck = (latest, record) => core.previewCastleCopy(stationCopyDescriptor, latest, record.sourceKey, [...record.input.destinations], ctx);

test('the replay record keeps what was reviewed: source, choices, applied pairs, states and values', () => {
  const { record } = applyReviewed(SOURCE_DRAFT);
  assert.equal(record.sourceKey, '1');
  assert.deepEqual([...record.input.destinations].sort(), ['2', '3']);
  assert.deepEqual(Object.keys(record.appliedSelection).sort(), ['2', '3']);
  assert.deepEqual(Object.keys(record.applied['2'].to), ['reserves']);
  assert.equal(record.applied['2'].state, 'compatible');
  // The record is a copy: later edits of the dialog's sets cannot change it.
  const preview = core.previewCastleCopy(stationCopyDescriptor, SOURCE_DRAFT, '1', ['2'], ctx);
  const input = core.defaultCopyInput(stationCopyDescriptor, preview);
  const made = replay.makeReplay('1', input, preview, core.buildCopySelection(preview, input));
  input.destinations.add('9');
  assert.equal(made.input.destinations.has('9'), false);
});

test('replayCopy: identical when the latest settings change nothing the copy touches', () => {
  const { record } = applyReviewed(SOURCE_DRAFT);
  // Someone else saved something unrelated (another castle's setup); the source and destinations are as reviewed.
  const latest = { ...SOURCE_DRAFT, 4: [{ id: 2, amount: 7 }] };
  const outcome = replay.replayCopy(recheck(latest, record), record);
  assert.equal(outcome.kind, 'identical');
  assert.deepEqual(Object.keys(outcome.selection).sort(), ['2', '3']);
  const again = core.applyCastleCopy(stationCopyDescriptor, latest, recheck(latest, record), outcome.selection);
  assert.deepEqual(again[2], SOURCE_DRAFT[1]);
  assert.deepEqual(again[4], [{ id: 2, amount: 7 }], 'nothing outside the reviewed pairs is touched');
});

test('replayCopy: review when the latest settings changed the source, a destination or its state', () => {
  const { record } = applyReviewed(SOURCE_DRAFT);
  const sourceChanged = replay.replayCopy(recheck({ 1: [{ id: 2, amount: 150 }] }, record), record);
  assert.equal(sourceChanged.kind, 'review');
  assert.ok(sourceChanged.reasons.includes('value'));
  const destinationDiffers = replay.replayCopy(recheck({ ...SOURCE_DRAFT, 3: [{ id: 2, amount: 20 }] }, record), record);
  assert.equal(destinationDiffers.kind, 'review', 'a destination that now holds a custom value is kept, so its pair drops out');
  assert.ok(destinationDiffers.reasons.includes('pairs'));
  const destinationSame = replay.replayCopy(recheck({ ...SOURCE_DRAFT, 3: [{ id: 2, amount: 100 }] }, record), record);
  assert.equal(destinationSame.kind, 'review', 'a destination that already holds the copy has nothing to write: reviewed, not silently changed');
  // A destination whose stock changed state (compatible to unavailable) is reviewed too.
  const poor = context([castles[0], castle(2, 'Frostkeep', 0, 4, { 2: 5 }), castles[2], castles[3]]);
  const stateChanged = replay.replayCopy(core.previewCastleCopy(stationCopyDescriptor, SOURCE_DRAFT, '1', ['2', '3'], poor), record);
  assert.equal(stateChanged.kind, 'review');
  assert.ok(stateChanged.reasons.includes('state'));
});

test('replayCopy: a destination that became incompatible is reviewed; a source without setup drops the record', () => {
  const { record } = applyReviewed(SOURCE_DRAFT);
  const noneLeft = core.previewCastleCopy(stationCopyDescriptor, { 1: [{ id: 999, amount: 5 }] }, '1', ['2', '3'], ctx);
  const incompatible = replay.replayCopy(noneLeft, record);
  assert.equal(incompatible.kind, 'review');
  assert.deepEqual(incompatible.selection, {}, 'nothing incompatible is selected');
  const emptySource = replay.replayCopy(core.previewCastleCopy(stationCopyDescriptor, {}, '1', ['2', '3'], ctx), record);
  assert.deepEqual(emptySource, { kind: 'source-empty' });
});

test('the review dialog is pre-filled with the player\'s choices and pre-selects nothing that is not compatible', () => {
  const { record } = applyReviewed(SOURCE_DRAFT);
  const poor = context([castles[0], castle(2, 'Frostkeep', 0, 4, { 2: 5 }), castles[2], castles[3]]);
  const latest = core.previewCastleCopy(stationCopyDescriptor, SOURCE_DRAFT, '1', ['2', '3'], poor);
  const input = replay.replayReviewInput(latest, record);
  assert.deepEqual([...input.destinations], ['3'], 'Frostkeep is now short on stock: not pre-selected, but still tickable');
  assert.deepEqual([...input.fields], [...record.input.fields]);
  const preview = core.previewCastleCopy(stationCopyDescriptor, SOURCE_DRAFT, '1', ['2', '3', '4'], poor);
  const html = renderToStaticMarkup(createElement(CastleCopyBody, {
    descriptor: stationCopyDescriptor, context: poor, featureLabel: 'Auto Station', preview, input, onInput: () => undefined,
    sources: core.configuredSources(stationCopyDescriptor, SOURCE_DRAFT, poor), sourceKey: '1', onSource: () => undefined,
    running: false, minutes: 0, noticeKey: 'castleCopy.replayChanged',
  }));
  assert.match(html, /data-castle-copy-notice/);
  assert.match(html, /The latest settings changed what this copy would do\. Review the changes before applying\./);
  const frost = html.slice(html.indexOf('data-castle-copy-destination="2"'), html.indexOf('data-castle-copy-destination="3"'));
  assert.doesNotMatch(frost.slice(0, frost.indexOf('</label>')), /checked=""/);
  assert.match(frost, /data-castle-copy-state="unavailable"/, 'the re-check result is shown');
  const ashvale = html.slice(html.indexOf('data-castle-copy-destination="3"'), html.indexOf('data-castle-copy-destination="4"'));
  assert.match(ashvale.slice(0, ashvale.indexOf('</label>')), /checked=""/, 'a still-compatible destination keeps its tick');
  const plain = renderToStaticMarkup(createElement(CastleCopyBody, {
    descriptor: stationCopyDescriptor, context: poor, featureLabel: 'Auto Station', preview, input, onInput: () => undefined,
    sources: [], sourceKey: '1', onSource: () => undefined, running: false, minutes: 0,
  }));
  assert.doesNotMatch(plain, /data-castle-copy-notice/, 'no notice unless the dialog reopened for review');
});

test('after the conflict the reloaded baseline carries the latest revision, so the re-applied copy can be saved', () => {
  const configuration = (revision, sections) => ({ schemaVersion: 1, revision, updatedAt: '2026-09-29T12:00:00Z', sections });
  const opened = session.captureConfigurationDraft(configuration(7, { 'automation.autoStation': { version: 1, settings: SOURCE_DRAFT } }), 'automation.autoStation');
  const before = session.configurationDraftSaveCondition(opened);
  assert.deepEqual(before, { expectedValue: opened.sections['automation.autoStation'] });
  // Someone else saved: the saved section differs, so the first Save is rejected with a conflict and the baseline is stale.
  const latest = configuration(9, { 'automation.autoStation': { version: 1, settings: { ...SOURCE_DRAFT, 4: [{ id: 2, amount: 7 }] } } });
  const reloaded = session.captureConfigurationDraft(latest, 'automation.autoStation');
  assert.equal(reloaded.revision, 9);
  const after = session.configurationDraftSaveCondition(reloaded);
  assert.deepEqual(after, { expectedValue: latest.sections['automation.autoStation'] }, 'Save compares against the latest saved section, not the stale one');
  assert.notDeepEqual(after, before);
  // The copy re-applied on that latest section is what gets saved.
  const { record } = applyReviewed(SOURCE_DRAFT);
  const latestDraft = latest.sections['automation.autoStation'].settings;
  const outcome = replay.replayCopy(recheck(latestDraft, record), record);
  assert.equal(outcome.kind, 'identical');
  const saved = core.applyCastleCopy(stationCopyDescriptor, latestDraft, recheck(latestDraft, record), outcome.selection);
  assert.deepEqual(saved[2], SOURCE_DRAFT[1]);
  assert.deepEqual(saved[4], [{ id: 2, amount: 7 }], 'the other saver\'s change survives');
});

test('the conflict notice: Maya\'s exact copy, the re-apply button only with a replay record, and a plain reload always available', async () => {
  assert.equal(messages['castleCopy.conflictNotice'], 'These settings were saved somewhere else while you were editing. Load the latest settings and re-apply your copy: the copied castles are checked again against the latest settings before anything is saved. Other unsaved changes in this window are replaced by the latest saved settings.');
  assert.equal(messages['castleCopy.loadAndReapply'], 'Load latest and re-apply copy');
  assert.equal(messages['castleCopy.loadLatestOnly'], 'Load latest settings');
  assert.equal(messages['castleCopy.reapplied'], 'Your copy was re-applied to the latest settings. Review and Save.');
  assert.equal(messages['castleCopy.replayChanged'], 'The latest settings changed what this copy would do. Review the changes before applying.');
  assert.equal(messages['castleCopy.conflictKept'], undefined, 'the sentence that promised what the code could not do is gone');
  const text = await readFile(new URL('../src/settings/ConfigurationDraftSession.tsx', import.meta.url), 'utf8');
  const notice = text.slice(text.indexOf('const copied = copyReplay !== undefined;'), text.indexOf('  }, [conflict, copyReplay'));
  assert.match(notice, /\{copied \? \(\s*<Button[\s\S]*?castleCopy\.loadAndReapply/, 'the re-apply button exists only while a copy exists');
  assert.match(notice, /castleCopy\.loadLatestOnly/);
  assert.match(notice, /copied \? reloadAndDropCopy\(\) : reloadLatest\(\)/, 'the plain reload drops the copy; without a copy the existing button stays');
  assert.match(notice, /ui\.settings\.configurationDraftSession\.load\.latest/, 'the existing button text stays without a copy');
  assert.match(text, /if \(await loadLatest\(\)\) copyReplay\?\.onReloaded\(\)/, 're-apply only after the latest settings loaded');
  // The re-apply path writes nothing: it reloads, then previews and applies to the draft.
  const hook = await readFile(new URL('../src/settings/copy/useCastleCopyReplay.tsx', import.meta.url), 'utf8');
  const code = hook.replace(/\/\*[\s\S]*?\*\//g, '').replace(/^\s*\/\/.*$/gm, '');
  assert.doesNotMatch(code, /updateConfiguration|\.save\(|saveSection|submitIntent|automation\.enabled/);
  assert.match(code, /phase === 'loaded'\) setPhase\('replay'\)/, 'one more render so the editor has reset its draft before the re-check');
  assert.match(code, /previewCastleCopy\(d, current, record\.sourceKey, \[\.\.\.record\.input\.destinations\], c\)/);
});

test('the four editors keep the replay record from Apply until close, and a preset or a plain reload drops it', async () => {
  for (const modal of ['AutoTowerSettingsModal', 'AutoStationSettingsModal', 'AutoBirdSettingsModal', 'QueueProductionSettingsModal']) {
    const text = await readFile(new URL(`../src/settings/components/${modal}.tsx`, import.meta.url), 'utf8');
    assert.match(text, /const copyReplay = useCastleCopyReplayState\(\);/, modal);
    assert.match(text, /copyReplay: copyReplay\.sessionOption/, modal);
    assert.match(text, /onApply=\{\(next, replay\) => \{.*copyReplay\.setReplay\(replay\)/, modal);
    assert.match(text, /useCastleCopyReplayRun\(copyReplay, \{ descriptor: /, modal);
    assert.match(text, /contentNotice=\{<>\{copyRun\.status\}(?:\{recovery\.banner\})?\{draftSession\.conflictNotice\}\{copyRun\.dialog\}<\/>\}/, modal);
    assert.doesNotMatch(text, /copiedSetup|copyApplied/, modal);
  }
  const bird = await readFile(new URL('../src/settings/components/AutoBirdSettingsModal.tsx', import.meta.url), 'utf8');
  assert.match(bird, /copyReplay\.setReplay\(null\);/, 'applying a preset replaces the draft and drops the copy');
  const hook = await readFile(new URL('../src/settings/copy/useCastleCopyReplay.tsx', import.meta.url), 'utf8');
  assert.match(hook, /if \(isOpen\) return;\s*setReplay\(null\)/, 'cleared on close (a successful Save closes the editor)');
});

test('Bird: reserved Direwolves are reported once, by the reservation; a non-reserved destination still shows the stock line', () => {
  const list = [castle(1, 'Sunhold', 0, 1, { 1: 500, 277: 900 }), castle(2, 'Frostkeep', 1, 12, { 1: 500, 277: 3 }), castle(3, 'Ashvale', 2, 12, { 1: 500, 277: 3 })];
  const fortress = { enabled: true, section: { version: 1, kingdoms: { 1: { enabled: true } } } };
  const birdCtx = context(list, { fortress }, (entry) => birdCandidateFlags(entry, { fortress }));
  const draft = { 1: [{ id: 1, amount: 100 }, { id: 277, amount: 200 }] };
  const result = core.previewCastleCopy(birdCopyDescriptor, draft, '1', ['2', '3'], birdCtx);
  const reserved = result.destinations.find((entry) => entry.key === '2');
  assert.deepEqual(reserved.reasons.map((reason) => reason.id), ['direwolves-reserved'], 'only the reservation reason');
  const plain = result.destinations.find((entry) => entry.key === '3');
  assert.deepEqual(plain.reasons.map((reason) => reason.id), ['stock:277'], 'not reserved: the stock line still shows');
  assert.equal(plain.reasons[0].params.stationed, 3);
});

test('core rule: a reason that drops entries suppresses stock and unknown-item reasons for those entries', () => {
  const reasons = [
    { id: 'stock:277', state: 'unavailable', messageKey: 'castleCopy.reason.stockShort' },
    { id: 'stock:2', state: 'unavailable', messageKey: 'castleCopy.reason.stockShort' },
    { id: 'unknown-item:277', state: 'incompatible', messageKey: 'castleCopy.reason.unknownItem', dropEntryIds: [277] },
    { id: 'direwolves-reserved', state: 'incompatible', messageKey: 'castleCopy.reason.direwolves', dropEntryIds: [277] },
  ];
  assert.deepEqual(core.suppressDroppedEntryReasons(reasons).map((reason) => reason.id), ['stock:2', 'direwolves-reserved']);
  const alone = [{ id: 'unknown-item:5', state: 'incompatible', messageKey: 'castleCopy.reason.unknownItem', dropEntryIds: [5] }];
  assert.deepEqual(core.suppressDroppedEntryReasons(alone).map((reason) => reason.id), ['unknown-item:5'], 'the reason that drops an entry is never removed by itself');
  const other = [{ id: 'stock:9', state: 'unavailable', messageKey: 'castleCopy.reason.stockShort' }, { id: 'x', state: 'incompatible', messageKey: 'castleCopy.reason.notInWorld', dropEntryIds: [1] }];
  assert.deepEqual(core.suppressDroppedEntryReasons(other).map((reason) => reason.id), ['stock:9', 'x'], 'other entries keep their stock reasons');
});

test('right-to-left: a direction-aware arrow, isolated from/to, a localized "to" header, and wrapping labels', async () => {
  const preview = core.previewCastleCopy(towersCopyDescriptor, { 1: { enabled: true, radius: 15, unitId: 2, maidenOnly: true } }, '1', ['2'], ctx);
  const html = renderToStaticMarkup(createElement(CastleCopyBody, {
    descriptor: towersCopyDescriptor, context: ctx, featureLabel: 'Auto Towers', preview, input: core.defaultCopyInput(towersCopyDescriptor, preview), onInput: () => undefined,
    sources: core.configuredSources(towersCopyDescriptor, { 1: { enabled: true, radius: 15, unitId: 2, maidenOnly: true } }, ctx), sourceKey: '1', onSource: () => undefined, running: false, minutes: 0,
  }));
  assert.doesNotMatch(html, /→/, 'no literal arrow glyph');
  assert.match(html, /<svg[^>]*rtl:-scale-x-100[^>]*aria-hidden="true"/, 'the icon mirrors in right-to-left layouts and is hidden from assistive technology');
  assert.match(html, /<bdi[^>]*>[\s\S]*?<\/bdi>[\s\S]*?<svg[\s\S]*?<bdi[^>]*font-semibold/, 'each side of the change keeps its own direction');
  assert.match(html, /<th scope="col"[^>]*>(?:<span[^>]*>)?To(?:<\/span>)?<\/th>/, 'the column header is a localized word');
  assert.equal(messages['castleCopy.toColumn'], 'To');
  assert.match(html, /<label for="castle-copy-destination-2" class="[^"]*whitespace-normal break-words/);
  assert.doesNotMatch(html, /class="[^"]*\btruncate\b/);
  const dialog = await readFile(new URL('../src/settings/components/CastleCopyDialog.tsx', import.meta.url), 'utf8');
  assert.match(dialog, /<ModalTitle className="castle-copy-title"/);
  const css = await readFile(new URL('../src/index.css', import.meta.url), 'utf8');
  assert.match(css, /\.castle-copy-title \.scheduler-modal-title-text \{[^}]*white-space: normal;[^}]*\}/);
});

const copyReplayHook = await vite.ssrLoadModule(`/src/settings/copy/useCastleCopyReplay.tsx`);
const { APIError } = await vite.ssrLoadModule(`/src/api/CitadelClient.ts`);

for (const [name, error, recorded, expected] of [
  ['copy conflict', new APIError('Raw conflict', 409, 'configuration_conflict'), true, null],
  ['non-copy conflict', new APIError('Raw conflict', 409, 'configuration_conflict'), false, 'Raw conflict'],
  ['another API error', new APIError('Other error', 500, 'other_error'), true, 'Other error'],
  ['plain error', new Error('Plain error'), true, 'Plain error'],
  ['non-error fallback', 'unknown failure', true, 'Fallback'],
]) {
  test(`genericSaveError: ${name}`, () => {
    const state = { status: false, replay: recorded ? applyReviewed(SOURCE_DRAFT).record : null };
    assert.equal(copyReplayHook.genericSaveError(error, state, 'Fallback'), expected);
  });
}

for (const status of [false, true]) {
  for (const recorded of [false, true]) {
    test(`copyReapplied: status=${status}, replay=${recorded}`, () => {
      const state = { status, replay: recorded ? applyReviewed(SOURCE_DRAFT).record : null };
      assert.equal(copyReplayHook.copyReapplied(state), status && recorded);
    });
  }
}

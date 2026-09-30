import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { after, beforeEach, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const recovery = await vite.ssrLoadModule('/src/settings/DraftRecovery.ts');
const { scopeKey } = await vite.ssrLoadModule('/src/settings/onboarding/accountScope.ts');
const { messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');

after(async () => {
  await vite.close();
});
beforeEach(() => recovery.resetDraftPresenceForTests());

const memory = () => {
  const map = new Map();
  return { map, getItem: (key) => map.get(key) ?? null, setItem: (key, value) => { map.set(key, value); }, removeItem: (key) => { map.delete(key); } };
};
const NOW = Date.parse('2026-09-29T12:00:00Z');
const entry = (extra = {}) => ({
  version: 1, section: 'automation.autoTowers', accountKey: '77:EmpireEx_2', draft: { version: 4, castles: { 1: { enabled: true, unitId: 1 } } },
  baseRevision: 3, baseDigest: recovery.stableDigest({ version: 4, castles: {} }), savedAt: new Date(NOW - 60_000).toISOString(), ...extra,
});
const DAY = 24 * 60 * 60 * 1000;

test('only settings sections can be recorded; login, connection, browser and account sections never can', () => {
  for (const section of ['automation.autoTowers', 'automation.recruitTroops', 'automation.autoBeriWorld', 'attackPresets', 'defense.presets', 'scheduler']) {
    assert.equal(recovery.isRecoverableSection(section), true, section);
  }
  for (const section of ['session.connection', 'session.backgroundLogin', 'session', 'credentials', 'account', 'browser', 'automation', 'automation.', 'automation.a.b', 'automation.enabled.x', 'settings.transfer', '']) {
    assert.equal(recovery.isRecoverableSection(section), false, section);
  }
  const storage = memory();
  assert.equal(recovery.writeDraft(entry({ section: 'session.connection' }), storage), 'section-not-allowed');
  assert.equal(storage.map.size, 0, 'nothing was stored');
});

test('drafts that look like credentials or exceed the size cap are refused', () => {
  const storage = memory();
  for (const key of ['password', 'backgroundPassword', 'token', 'accessToken', 'apiKey', 'authorization', 'secret', 'credential', 'cookie', 'sessionId']) {
    assert.equal(recovery.writeDraft(entry({ draft: { nested: { [key]: 'x' } } }), storage), 'credential-like', key);
    assert.equal(recovery.writeDraft(entry({ extras: { [key]: 'x' } }), storage), 'credential-like', `${key} in extras`);
  }
  assert.equal(recovery.writeDraft(entry({ draft: { blob: 'x'.repeat(recovery.MAX_DRAFT_BYTES + 1) } }), storage), 'too-large');
  assert.equal(recovery.writeDraft(entry({ accountKey: '' }), storage), 'account-unknown', 'nothing is attributed to an unknown account');
  assert.equal(storage.map.size, 0);
  assert.equal(recovery.writeDraft(entry(), null), 'storage-unavailable');
});

test('a draft is isolated by account/world and section', () => {
  const storage = memory();
  assert.equal(recovery.writeDraft(entry(), storage), null);
  assert.equal(recovery.writeDraft(entry({ section: 'automation.autoFoodBalance', draft: { food: 1 } }), storage), null);
  assert.ok(recovery.readDraft('77:EmpireEx_2', 'automation.autoTowers', NOW, storage));
  assert.equal(recovery.readDraft('77:EmpireEx_3', 'automation.autoTowers', NOW, storage), null, 'another world of the same player');
  assert.equal(recovery.readDraft('78:EmpireEx_2', 'automation.autoTowers', NOW, storage), null, 'another account');
  assert.deepEqual(recovery.readDraft('77:EmpireEx_2', 'automation.autoFoodBalance', NOW, storage).draft, { food: 1 });
  assert.equal(recovery.readDraft('77:EmpireEx_2', 'automation.autoHospital', NOW, storage), null, 'another feature');
  // A switch of account leaves the first account's entry in place for when it returns.
  assert.ok(recovery.readDraft('77:EmpireEx_2', 'automation.autoTowers', NOW, storage));
  // Keys are namespaced so a stored entry cannot be read as another account's.
  const key = recovery.draftStorageKey('77:EmpireEx_2', 'automation.autoTowers');
  storage.setItem(recovery.draftStorageKey('99:X', 'automation.autoTowers'), storage.getItem(key));
  assert.equal(recovery.readDraft('99:X', 'automation.autoTowers', NOW, storage), null, 'an entry whose recorded account differs is dropped');
});

test('retention is 14 days; expired, malformed and future-dated entries are dropped', () => {
  assert.equal(recovery.DRAFT_RETENTION_DAYS, 14);
  const storage = memory();
  recovery.writeDraft(entry({ savedAt: new Date(NOW - 13 * DAY).toISOString() }), storage);
  assert.ok(recovery.readDraft('77:EmpireEx_2', 'automation.autoTowers', NOW, storage), '13 days old is kept');
  recovery.writeDraft(entry({ savedAt: new Date(NOW - 15 * DAY).toISOString() }), storage);
  assert.equal(recovery.readDraft('77:EmpireEx_2', 'automation.autoTowers', NOW, storage), null);
  assert.equal(storage.map.size, 0, 'the expired entry was removed');
  storage.setItem(recovery.draftStorageKey('77:EmpireEx_2', 'automation.autoTowers'), '{not json');
  assert.equal(recovery.readDraft('77:EmpireEx_2', 'automation.autoTowers', NOW, storage), null);
  recovery.writeDraft(entry({ savedAt: new Date(NOW + 3 * DAY).toISOString() }), storage);
  assert.equal(recovery.readDraft('77:EmpireEx_2', 'automation.autoTowers', NOW, storage), null);
});

test('storage failures make recovery unavailable and never throw', () => {
  const broken = { getItem() { throw new Error('blocked'); }, setItem() { throw new Error('quota'); }, removeItem() { throw new Error('blocked'); } };
  assert.equal(recovery.writeDraft(entry(), broken), 'storage-unavailable');
  assert.equal(recovery.readDraft('77:EmpireEx_2', 'automation.autoTowers', NOW, broken), null);
  assert.doesNotThrow(() => recovery.clearDraft('77:EmpireEx_2', 'automation.autoTowers', broken));
  assert.equal(recovery.hasRecoveredDraft('77:EmpireEx_2', 'automation.autoTowers', NOW, broken), false);
});

test('the digest ignores key order and detects a changed saved section', () => {
  assert.equal(recovery.stableDigest({ a: 1, b: { c: 2, d: [1, 2] } }), recovery.stableDigest({ b: { d: [1, 2], c: 2 }, a: 1 }));
  assert.notEqual(recovery.stableDigest({ a: 1 }), recovery.stableDigest({ a: 2 }));
  assert.equal(recovery.stableDigest(undefined), recovery.stableDigest(null));
  assert.equal(recovery.stableDigest({ a: undefined, b: 1 }), recovery.stableDigest({ b: 1 }));
});

test('Compare lists the differing values between the saved section and the recovered draft', () => {
  const saved = { version: 4, mapRefreshIntervalSec: 1800, castles: { 1: { enabled: true, unitId: 1 }, 2: { enabled: false } } };
  const recovered = { version: 4, mapRefreshIntervalSec: 900, castles: { 1: { enabled: true, unitId: 5 }, 3: { enabled: true } } };
  const paths = recovery.compareDrafts(saved, recovered).map((difference) => difference.path);
  assert.deepEqual(paths, ['castles.1.unitId', 'castles.2', 'castles.3', 'mapRefreshIntervalSec']);
  assert.deepEqual(recovery.compareDrafts(saved, saved), []);
  const first = recovery.compareDrafts(saved, recovered).find((difference) => difference.path === 'mapRefreshIntervalSec');
  assert.deepEqual([first.saved, first.recovered], [1800, 900]);
});

test('a recorded draft is dropped when identical to what loaded, restored when the baseline is unchanged, compared when settings changed', () => {
  const saved = { version: 4, castles: {} };
  const loaded = { draftDigest: recovery.draftDigest(saved, undefined), savedDigest: recovery.stableDigest(saved) };
  const found = (draft, baseDigest = loaded.savedDigest, extras) => ({ draft, baseDigest, ...(extras !== undefined ? { extras } : {}) });
  assert.equal(recovery.classifyRecovered(found(saved), loaded), 'drop', 'nothing to recover');
  assert.equal(recovery.classifyRecovered(found({ version: 4, castles: { 1: {} } }), loaded), 'baseline-unchanged');
  assert.equal(recovery.classifyRecovered(found({ version: 4, castles: { 1: {} } }, recovery.stableDigest({ version: 4, castles: { 9: {} } })), loaded), 'saved-since');
  assert.equal(recovery.classifyRecovered(found(saved, loaded.savedDigest, { attackRef: { source: 'inline' } }), loaded), 'baseline-unchanged', 'unsaved sub-drafts count as changes');
  // A change of the saved section while the same load is open means it was saved; a reload is not a save.
  assert.equal(recovery.savedWhileLoaded({ loadKey: 'a:1', digest: 'x' }, { loadKey: 'a:1', digest: 'y' }), true);
  assert.equal(recovery.savedWhileLoaded({ loadKey: 'a:1', digest: 'x' }, { loadKey: 'a:2', digest: 'y' }), false);
  assert.equal(recovery.savedWhileLoaded({ loadKey: 'a:1', digest: 'x' }, { loadKey: 'a:1', digest: 'x' }), false);
  assert.equal(recovery.savedWhileLoaded(null, { loadKey: 'a:1', digest: 'x' }), false);
});

test('presence: unsaved in an open editor, recovered and waiting, or none; nothing persists the editor state', () => {
  const key = '77:EmpireEx_2';
  const section = 'automation.autoTowers';
  assert.equal(recovery.draftLine(key, section, NOW), 'none');
  recovery.setEditorDirty(key, section, true);
  assert.equal(recovery.draftLine(key, section, NOW), 'unsaved');
  recovery.setEditorDirty(key, section, false);
  assert.equal(recovery.draftLine(key, section, NOW), 'none');
  assert.equal(recovery.draftLine('78:EmpireEx_2', section, NOW), 'none', 'another account never shows this draft');
});

test('the account scope is empty until the game reported both the account and the world', () => {
  assert.equal(scopeKey(null), '');
  assert.equal(scopeKey({ account: {} }), '');
  assert.equal(scopeKey({ account: { uid: 77 } }), '');
  assert.equal(scopeKey({ account: { worldId: 'EmpireEx_2' } }), '');
  assert.equal(scopeKey({ account: { uid: 77, worldId: 'EmpireEx_2' } }), '77:EmpireEx_2');
});

test('restoring a draft never saves, never writes automation.enabled and never starts anything', async () => {
  const [hook, store, session] = await Promise.all([
    readFile(new URL('../src/settings/useDraftRecovery.tsx', import.meta.url), 'utf8'),
    readFile(new URL('../src/settings/DraftRecovery.ts', import.meta.url), 'utf8'),
    readFile(new URL('../src/settings/ConfigurationDraftSession.tsx', import.meta.url), 'utf8'),
  ]);
  const code = (text) => text.replace(/\/\*[\s\S]*?\*\//g, '').replace(/^\s*\/\/.*$/gm, '');
  for (const [name, text] of [['hook', code(hook)], ['store', code(store)]]) {
    assert.doesNotMatch(text, /updateConfiguration|submitIntent|automation\.enabled|saveSection|\.save\(|setAutomationEnabled|startGame|queueConfigurationUpdate/, `${name} has no write path`);
  }
  // Restore goes through the draft session's overlay, which leaves the snapshot that guards Save untouched.
  assert.match(hook, /draftSession\.recoverDraft\(section, entry\.draft, entry\.extras\)/);
  const recover = session.slice(session.indexOf('const recoverDraft'), session.indexOf('const conflictNotice'));
  assert.match(recover, /setInitialSnapshot/);
  assert.doesNotMatch(recover, /snapshotRef|setSnapshot|updateConfiguration/, 'the saved snapshot and CAS baseline are untouched');
});

test('the banner and Compare copy are the accepted wording; Restore appears only inside Compare when settings changed', async () => {
  assert.equal(messages['draftRecovery.banner'], 'You have unsaved changes from {savedAt, date, medium} {savedAt, time, short}. Restore them to keep editing, or discard them.');
  assert.equal(messages['draftRecovery.bannerSavedSince'], 'You have unsaved changes from {savedAt, date, medium} {savedAt, time, short}, but these settings were saved again since then. Compare them before restoring.');
  assert.equal(messages['draftRecovery.restore'], 'Restore');
  assert.equal(messages['draftRecovery.discard'], 'Discard');
  assert.equal(messages['draftRecovery.compare'], 'Compare');
  const hook = await readFile(new URL('../src/settings/useDraftRecovery.tsx', import.meta.url), 'utf8');
  const banner = hook.slice(hook.indexOf('const banner ='), hook.indexOf('{comparing ? ('));
  assert.match(banner, /savedSince \? \(\s*<Button[^>]*onClick=\{\(\) => setComparing\(true\)\}/, 'Compare replaces Restore when the saved settings changed');
  assert.match(banner, /\) : \(\s*<Button[^>]*onClick=\{restore\}/, 'Restore is offered only when the baseline is unchanged');
  assert.match(hook, /role="status"/);
  assert.match(hook, /<Button variant="primary" onClick=\{restore\}>/, 'Restore inside the Compare view');
});

test('every draft-session editor records and offers recovery; Equipment Cleanup (immediate save) does not', async () => {
  const modals = [
    'AutoAdvisorSettingsModal', 'AutoBeriWorldSettingsModal', 'AutoBirdSettingsModal', 'AutoBoosterSettingsModal', 'AutoBuyerSettingsModal',
    'AutoFoodBalanceSettingsModal', 'AutoFortressSettingsModal', 'AutoHospitalSettingsModal', 'AutoInvasionSettingsModal', 'AutoKhanSettingsModal',
    'AutoNomadSettingsModal', 'AutoSceatResSettingsModal', 'AutoStationSettingsModal', 'AutoStormSettingsModal', 'AutoTCISettingsModal',
    'AutoTowerSettingsModal', 'QueueProductionSettingsModal',
  ];
  for (const modal of modals) {
    const text = await readFile(new URL(`../src/settings/components/${modal}.tsx`, import.meta.url), 'utf8');
    assert.match(text, /useConfigurationDraftSession/, modal);
    assert.match(text, /const recovery = useDraftRecovery\(\{ section: [^,]+, isOpen, draftSession, draft: /, modal);
    assert.match(text, /\{recovery\.banner\}/, modal);
  }
  const cleanup = await readFile(new URL('../src/settings/components/AutoEquipmentCleanupSettingsModal.tsx', import.meta.url), 'utf8').catch(() => '');
  assert.doesNotMatch(cleanup, /useDraftRecovery/);
  // Editors with unsaved sub-drafts restore them from the recovered extras, after their own load effect.
  for (const [modal, names] of [['AutoNomadSettingsModal', ['nomadRef', 'samuraiRef']], ['AutoInvasionSettingsModal', ['attackRef']], ['AutoKhanSettingsModal', ['attackRef', 'defenseRef']], ['AutoStormSettingsModal', ['fortsRef', 'islandsRef']], ['AutoBeriWorldSettingsModal', ['attackRef']]]) {
    const text = await readFile(new URL(`../src/settings/components/${modal}.tsx`, import.meta.url), 'utf8');
    assert.match(text, /draftSession\.recoveredExtras/, modal);
    for (const name of names) assert.match(text, new RegExp(`extras: \\{[^}]*${name}`), `${modal}: ${name} is recorded with the draft`);
    assert.ok(text.indexOf('draftSession.recoveredExtras') > text.indexOf('draftSession.initialSections'), `${modal}: restores after the load effect`);
  }
});

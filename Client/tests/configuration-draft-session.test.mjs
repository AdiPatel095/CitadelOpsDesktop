import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({
  root: clientRoot,
  appType: 'custom',
  logLevel: 'silent',
  server: { middlewareMode: true },
});
const drafts = await vite.ssrLoadModule('/src/settings/ConfigurationDraftSession.tsx');

after(async () => vite.close());

const snapshot = (revision, sections) => ({
  schemaVersion: 2,
  revision,
  updatedAt: `2026-09-18T18:${String(revision).padStart(2, '0')}:00Z`,
  sections,
});

test('save baselines distinguish present, absent, and configuration-dependent sections', () => {
  const present = drafts.captureConfigurationDraft(
    snapshot(20, { section: { enabled: false } }),
    'section',
  );
  assert.deepEqual(drafts.configurationDraftSaveCondition(present), {
    expectedValue: { enabled: false },
  });

  const absent = drafts.captureConfigurationDraft(snapshot(21, {}), 'section');
  assert.deepEqual(drafts.configurationDraftSaveCondition(absent), { expectedRevision: 21 });

  const dependent = drafts.captureConfigurationDraft(
    snapshot(22, {
      'automation.autoNomad': { presetId: 'trial' },
      'attacks.presets': { presets: [{ id: 'trial', waves: 4 }] },
    }),
    'automation.autoNomad',
    'automation.autoNomad',
    true,
  );
  assert.deepEqual(drafts.configurationDraftSaveCondition(dependent), { expectedRevision: 22 });
  assert.notEqual(23, drafts.configurationDraftSaveCondition(dependent).expectedRevision,
    'a remote preset edit must conflict instead of silently authorizing the frozen preview');
});

test('acknowledged same-session writes advance the revision while later remote writes still conflict', () => {
  const opened = drafts.captureConfigurationDraft(
    snapshot(30, {
      'automation.autoStorm': { forts: { enabled: true } },
      'automation.autoStorm.blueprints': { activeId: '' },
    }),
    'automation.autoStorm',
    'automation.autoStorm',
    true,
  );
  assert.deepEqual(
    drafts.configurationDraftSaveCondition(opened, 'automation.autoStorm.blueprints', true),
    { expectedRevision: 30 },
  );

  const afterBlueprintSave = drafts.advanceConfigurationDraftAfterSave(opened, snapshot(31, {
    'automation.autoStorm': { forts: { enabled: true } },
    'automation.autoStorm.blueprints': { activeId: 'captured' },
  }));
  assert.deepEqual(drafts.configurationDraftSaveCondition(afterBlueprintSave), { expectedRevision: 31 });
  assert.notEqual(
    drafts.configurationDraftSaveCondition(afterBlueprintSave).expectedRevision,
    32,
    'an unacknowledged remote write must not be adopted into the open draft baseline',
  );
});

test('migrated editors use the fresh-open draft session and block edits while loading', async () => {
  // CIT-15 migrated the event attack modals and CIT-18 the Tower/Fortress/Station/Bird/Food editors; the rest follow in CIT-16/CIT-17.
  const editors = [
    'AutoBeriWorldSettingsModal.tsx',
    'AutoBirdSettingsModal.tsx',
    'AutoFoodBalanceSettingsModal.tsx',
    'AutoFortressSettingsModal.tsx',
    'AutoInvasionSettingsModal.tsx',
    'AutoNomadSettingsModal.tsx',
    'AutoStationSettingsModal.tsx',
    'AutoTowerSettingsModal.tsx',
  ];

  for (const editor of editors) {
    const source = await readFile(new URL(`../src/settings/components/${editor}`, import.meta.url), 'utf8');
    assert.match(source, /useConfigurationDraftSession/, editor);
    assert.match(source, /saveDisabled=.*draftSession\.ready/, editor);
    assert.match(source, /contentDisabled=\{!draftSession\.ready\}/, editor);
    // The castle-copy editors add their re-apply status and review dialog, and CIT-19 the recovered-draft banner, beside the conflict notice.
    assert.match(source, /contentNotice=\{<>(?:\{copyRun\.status\})?\{recovery\.banner\}\{draftSession\.conflictNotice\}(?:\{copyRun\.dialog\})?<\/>\}/, editor);
    assert.doesNotMatch(source, /draftSession\.snapshot\b/, editor);
    assert.doesNotMatch(source, /configuration\??\.sections/, editor);
  }

  const settingsModal = await readFile(new URL(
    '../src/components/ui/SettingsModal.tsx',
    import.meta.url,
  ), 'utf8');
  assert.ok(settingsModal.indexOf('{contentNotice}') < settingsModal.indexOf('<div inert='),
    'Retry and conflict controls must remain outside the disabled form body');
});

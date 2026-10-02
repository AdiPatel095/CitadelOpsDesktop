import assert from 'node:assert/strict';
import { readdir, readFile } from 'node:fs/promises';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const { messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');

after(async () => {
  await vite.close();
});

const source = (path) => readFile(new URL(`../src/${path}`, import.meta.url), 'utf8');
const code = (text) => text.replace(/\/\*[\s\S]*?\*\//g, '').replace(/^\s*\/\/.*$/gm, '');

const ONBOARDING = [
  'settings/onboarding/goals.ts', 'settings/onboarding/goalStore.ts', 'settings/onboarding/checklist.ts', 'settings/onboarding/accountScope.ts',
  'settings/onboarding/useChecklist.ts', 'settings/onboarding/useConnectionEvidence.ts', 'settings/DraftRecovery.ts', 'settings/useDraftRecovery.tsx',
  'components/SetupChecklist.tsx', 'components/StateLegend.tsx', 'components/GoalPicker.tsx',
  'settings/connection/connectionExplain.ts', 'settings/connection/repairRequest.ts', 'settings/connection/repairSurface.tsx',
  'components/ConnectionRepairDialog.tsx', 'components/ConnectionRepairHost.tsx',
];
const NEW_NAMESPACES = /^(goal\.|goalEntry\.|goalPicker\.|checklist\.|legend\.|draftRecovery\.|connectionRepair\.)/;

test('no "runtime" in the player-facing text of goal setup, draft recovery or connection repair', async () => {
  const keys = Object.keys(messages).filter((key) => NEW_NAMESPACES.test(key) || key === 'readiness.checkConnection');
  assert.ok(keys.length > 100, 'the new namespaces exist');
  for (const key of keys) assert.doesNotMatch(messages[key], /runtime/i, key);
  for (const file of ONBOARDING) {
    const text = code(await source(file));
    assert.doesNotMatch(text, /(['"`])[^'"`\n]*\bruntime\b[^'"`\n]*\1/i, `${file}: string literal`);
    assert.doesNotMatch(text, />[^<>{}\n]*\bruntime\b[^<>{}\n]*</i, `${file}: JSX text`);
  }
});

test('onboarding modules read state and write nothing: no intents, configuration writes, account creation or credential handling', async () => {
  const readOnly = ONBOARDING.filter((file) => !file.includes('ConnectionRepairDialog'));
  for (const file of readOnly) {
    const text = code(await source(file));
    const writes = /submitIntent|updateConfiguration|createAccount|queueConfigurationUpdate|setAutomationEnabled|enableAutomationFor|saveSection|\.save\(|fetch\(/;
    assert.doesNotMatch(text, writes, file);
    // The recovery store is the one module that names credentials, to refuse them.
    if (file !== 'settings/DraftRecovery.ts') assert.doesNotMatch(text.replace(/\/wrong world\|wrong password\|wrong server\|wrong login\/i/, '').replaceAll('invalid_credentials', '').replaceAll('invalidCredentials', ''), /password|credential/i, file);
  }
  // The repair dialog only re-uses the existing controls: start, reconnect and the saved-login re-enable helper.
  const dialog = code(await source('components/ConnectionRepairDialog.tsx'));
  assert.doesNotMatch(dialog, /submitIntent\(/, 'the dialog never submits an intent itself');
  assert.equal([...dialog.matchAll(/\bsubmitIntent\b/g)].length, 2, 'only read from the API and handed to reauthorizeSavedLogin');
  assert.match(dialog, /reauthorizeSavedLogin\(submitIntent\)/);
  assert.doesNotMatch(dialog, /updateConfiguration|createAccount|password/i);
  // Storage is the only thing the onboarding state touches, and only its own keys.
  const stores = ['settings/onboarding/goalStore.ts', 'settings/DraftRecovery.ts'];
  for (const file of stores) {
    const text = code(await source(file));
    assert.doesNotMatch(text, /automation\.enabled|citadelops\.settings\.|citadelops\.enabledSince/, file);
  }
});

test('the checklist panel has no switch: turning an automation on stays the row switch and its Start confirmation', async () => {
  const panel = code(await source('components/SetupChecklist.tsx'));
  assert.doesNotMatch(panel, /<Switch|role="switch"|onToggle|toggleAuto/);
  const view = await source('views/AutomationView.tsx');
  assert.match(view, /id=\{`automation-switch-\$\{feature\.id\}`\}/, 'the row switch is the target of "Go to the switch"');
  assert.match(view, /focusReadinessTargetWhenReady\(id\)/);
  assert.match(view, /onChange=\{feature\.onToggle\}/, 'the one existing write path');
});

test('accepted step names and legend labels', () => {
  assert.deepEqual(
    ['connection', 'choices', 'readiness', 'activation', 'firstResult'].map((id) => messages[`checklist.step.${id}`]),
    ['Connected to the game', 'Required choices saved', 'Ready to start', 'Turned on', 'First result confirmed'],
  );
  assert.deepEqual(['draft', 'saved', 'connected', 'on', 'firstResult'].map((id) => messages[`legend.${id}`]), ['Draft', 'Saved', 'Connected', 'On', 'First result']);
  assert.match(messages['legend.draft.value'], /none \{none\} unsaved \{unsaved in the editor\} recovered \{recovered, waiting for review\}/);
  assert.match(messages['legend.connected.value'], /yes \{yes\} no \{no\} waiting \{waiting\}/);
  assert.match(messages['legend.on.value'], /off \{off\} on \{on\} until \{on until/);
  assert.match(messages['legend.firstResult.value'], /none \{none yet\} confirmed \{confirmed at/);
  assert.equal(messages['checklist.firstResult.waiting'], 'Waiting for the first action the game confirms.');
  assert.equal(messages['goalEntry.button'], 'Set up with a goal');
  assert.equal(messages['goalEntry.guideMe'], 'Guide me through this');
  assert.equal(messages['goalPicker.showAll'], 'Show all automations');
  assert.equal(messages['goalPicker.somethingElse'], 'Something else: open the full list');
  assert.doesNotMatch(messages['checklist.notComplete'], /all set/i);
});

test('entry points: header button, empty state and the Before-you-start row; the button hides while the panel is shown; no gate', async () => {
  const view = await source('views/AutomationView.tsx');
  const entryStart = view.indexOf('const goalEntry =');
  const entry = view.slice(entryStart, view.indexOf('\n  return (', entryStart));
  assert.match(entry, /\{activeGoal && activeGoalFeature \? \(\s*<SetupChecklist/, 'the panel exists only for a chosen goal');
  assert.match(entry, /\) : \(\s*<div[^>]*data-goal-entry-row/, 'the entry button is in the else branch, so it hides while the panel is shown');
  assert.match(entry, /!anyAutomationOn \?/, 'the empty state appears only when no automation is on');
  assert.doesNotMatch(entry, /return null|disabled=/, 'nothing about the catalog or editors is gated');
  const row = await source('components/AutomationReadinessRow.tsx');
  assert.match(row, /offerGuide = sections !== undefined && !isSavedSection\(sections\[GOAL_SAVED_SECTION\[featureId\]\]\) && goalApi\.goal\?\.goalId !== featureId/);
  assert.match(row, /data-guide-me=\{featureId\}/);
  // No first-launch prompt anywhere.
  const app = await source('App.tsx');
  assert.doesNotMatch(app, /GoalPicker|SetupChecklist|goalEntry/, 'no first-launch modal or tour');
});

test('the checklist is a list with the current step marked, the recovery banner is a status, and repair returns to the editor', async () => {
  const panel = await source('components/SetupChecklist.tsx');
  assert.match(panel, /<ol /);
  assert.match(panel, /aria-current=\{current \? 'step' : undefined\}/);
  const readiness = await source('settings/components/ReadinessPanel.tsx');
  assert.match(readiness, /check\.fix === 'connection' && check\.state !== 'valid'[\s\S]*requestConnectionRepair\(\)/, 'every connection readiness check opens the repair');
  const app = await source('App.tsx');
  assert.match(app, /<ConnectionRepairHost onOpenSettings=\{\(\) => setActiveView\('settings'\)\} \/>/);
  const host = await source('components/ConnectionRepairHost.tsx');
  assert.doesNotMatch(host, /setActiveSettingsModal|onClose=\{.*Editor/, 'the editor underneath is never closed or remounted by the repair');
});

test('every localized namespace this story adds has a reviewable English value with balanced ICU braces', () => {
  for (const [key, value] of Object.entries(messages)) {
    if (!NEW_NAMESPACES.test(key)) continue;
    assert.equal(typeof value, 'string');
    assert.ok(value.trim().length > 0, key);
    let depth = 0;
    for (const char of value) { if (char === '{') depth += 1; if (char === '}') depth -= 1; assert.ok(depth >= 0, key); }
    assert.equal(depth, 0, key);
  }
});

test('the modal directory holds only the editors the recovery test expects', async () => {
  const files = (await readdir(new URL('../src/settings/components/', import.meta.url))).filter((name) => name.endsWith('SettingsModal.tsx'));
  const withSession = [];
  for (const name of files) {
    const text = await readFile(new URL(`../src/settings/components/${name}`, import.meta.url), 'utf8');
    if (/useConfigurationDraftSession\(/.test(text)) withSession.push(name);
  }
  for (const name of withSession) {
    const text = await readFile(new URL(`../src/settings/components/${name}`, import.meta.url), 'utf8');
    assert.match(text, /useDraftRecovery\(/, `${name}: a draft-session editor without recovery`);
  }
  assert.equal(withSession.length, 17);
});

test('focus survives the goal opener leaving the page: choosing a goal focuses the checklist, finishing one focuses the opener again', async () => {
  const view = await source('views/AutomationView.tsx');
  assert.match(view, /onChoose=\{\(goalId\) => \{ goalApi\.choose\(goalId\); setGoalPickerOpen\(false\); focusReadinessTargetWhenReady\('setup-checklist'\); \}\}/);
  assert.match(view, /onDone=\{\(\) => \{ goalApi\.clear\(\); focusReadinessTargetWhenReady\('goal-entry'\); \}\}/);
  assert.match(view, /<Button variant="secondary" size="md" id="goal-entry" onClick=\{\(\) => setGoalPickerOpen\(true\)\} data-goal-entry>/);
  const panel = await source('components/SetupChecklist.tsx');
  assert.match(panel, /<section\s+id="setup-checklist"\s+tabIndex=\{-1\}/, 'the panel can take focus');
});

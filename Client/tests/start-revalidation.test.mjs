import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const readiness = await vite.ssrLoadModule('/src/settings/readiness/featureReadiness.ts');
const { messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');

after(async () => {
  await vite.close();
});

const source = (path) => readFile(new URL(`../src/${path}`, import.meta.url), 'utf8');
const report = (states) => ({ featureId: 'autoTowers', checks: states.map((state, index) => ({ id: `c${index}`, state, messageKey: 'readiness.state' })), overall: readiness.blockedChecks({ checks: states.map((state) => ({ state })) }).length > 0 ? 'blocked' : 'pending' });

test('only blocked checks ask for confirmation; pending and unavailable never block a Start', () => {
  assert.equal(readiness.startRequiresConfirmation({ featureId: 'x', checks: [], overall: 'blocked' }), true);
  for (const overall of ['valid', 'pending', 'unavailable']) {
    assert.equal(readiness.startRequiresConfirmation({ featureId: 'x', checks: [], overall }), false, overall);
  }
  const mixed = report(['blocked', 'pending', 'unavailable', 'valid']);
  assert.equal(readiness.blockedChecks(mixed).length, 1);
  assert.equal(readiness.undecidedChecks(mixed).length, 2, 'the game-decided and waiting checks are listed, not blocking');
});

test('the confirmation says the game stays authoritative, offers Fix first first, and Start anyway', async () => {
  assert.match(messages['startConfirm.authority'], /checks everything again when the automation acts and can still refuse/);
  assert.match(messages['startConfirm.authority'], /sends no game action/);
  assert.equal(messages['startConfirm.fixFirst'], 'Fix first');
  assert.equal(messages['startConfirm.startAnyway'], 'Start anyway');
  const dialog = await source('components/StartConfirmDialog.tsx');
  assert.ok(dialog.indexOf('startConfirm.fixFirst') < dialog.indexOf('startConfirm.startAnyway'), 'Fix first is the first (default-focused) action');
  assert.match(dialog, /pending\.decide\(false\)/);
  assert.match(dialog, /pending\.decide\(true\)/);
  assert.doesNotMatch(dialog, /submitIntent|updateConfiguration|setAutomationEnabled/, 'the dialog only reports the decision');
});

test('Start revalidation reads the latest saved configuration, and the single enable write is the only mutation', async () => {
  const context = await source('context/AuthContext.tsx');
  const confirm = context.slice(context.indexOf('const confirmStart'), context.indexOf('const setAutomationEnabled'));
  assert.match(confirm, /loadLatestConfiguration\(\)/, 'the latest snapshot, not the render-time copy');
  assert.match(confirm, /evaluateFeatureReadiness\(featureId,/);
  assert.match(confirm, /startRequiresConfirmation\(report\)/);
  assert.doesNotMatch(confirm, /submitIntent|updateConfiguration|submit\(/, 'the preview submits nothing and writes nothing');
  const writes = [...context.matchAll(/updateConfiguration\('automation\.enabled'/g)];
  assert.equal(writes.length, 2, 'one write for Start/Stop and one for a timed Start; both follow the confirmation');
  const start = context.slice(context.indexOf('const setAutomationEnabled'), context.indexOf('const toggle'));
  assert.ok(start.indexOf('confirmStart(feature)') < start.indexOf("updateConfiguration('automation.enabled'"), 'the confirmation precedes the write');
  assert.match(start, /if \(enabled && !\(await confirmStart\(feature\)\)\)/, 'only turning on is previewed; Stop is never delayed by a dialog');
  assert.match(start, /requestReadinessRowFocus\(/, '"Fix first" brings the readiness row into view through the pending-request mechanism');
  const settingsFix = await source('settings/readiness/settingsFixRequest.ts');
  assert.match(settingsFix, /ROW_FOCUS_EVENT = 'citadelops:fix-before-start'/);
});

test('declining leaves the switch off: no write happens after Fix first', async () => {
  const context = await source('context/AuthContext.tsx');
  const start = context.slice(context.indexOf('const setAutomationEnabled'), context.indexOf('const enableAutomationFor'));
  const declined = start.slice(start.indexOf('if (enabled && !(await confirmStart'), start.indexOf('try {'));
  assert.match(declined, /return;/);
  assert.doesNotMatch(declined, /updateConfiguration/);
});

test('the client preflight never proves dispatch or success: no readiness path submits an intent', async () => {
  for (const file of ['components/StartConfirmDialog.tsx', 'components/AutomationReadinessRow.tsx', 'components/FirstResultCard.tsx', 'settings/readiness/featureReadiness.ts', 'settings/readiness/firstResult.ts', 'settings/readiness/runtimeState.ts']) {
    const text = await source(file);
    assert.doesNotMatch(text, /submitIntent|updateConfiguration|cancelOperation\(|queueConfigurationUpdate/, file);
  }
  const row = await source('components/AutomationReadinessRow.tsx');
  assert.match(row, /loadStormUnlockOffer\(getCatalog/, 'the only request is the read-only official catalog, through the shared cached loader');
  assert.doesNotMatch(row, /CitadelAPI\./);
  assert.doesNotMatch(row, /\bgetCatalog\(/, 'the row itself requests no catalog directly');
  assert.match(row, /it never disables the switch/, 'readiness never disables the switch');
  const view = await source('views/AutomationView.tsx');
  assert.doesNotMatch(view.slice(view.indexOf('<Switch'), view.indexOf('<Switch') + 300), /readiness|blocked/i, 'the switch is never disabled by readiness');
});

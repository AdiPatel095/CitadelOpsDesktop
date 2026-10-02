import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const { AUTOMATION_ENABLED_KEYS } = await vite.ssrLoadModule('/src/settings/disclosure/placement.ts');
const { messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');

after(async () => {
  await vite.close();
});

const source = (path) => readFile(new URL(`../src/${path}`, import.meta.url), 'utf8');
const ROWS = [
  ['autoStation', 'auto_station', 'toggleAutoStation', 'autoStationEnabled', 'onOpenAutoStationSettings', 'StatusPanel'],
  ['autoBird', 'auto_bird', 'toggleAutoBird', 'autoBirdEnabled', 'onOpenAutoBirdSettings', 'AutoBirdCycles'],
];

/** The feature object literal with the given id in the Automation page's `features` array. */
function featureEntry(view, id) {
  const start = view.indexOf(`id: '${id}'`);
  assert.ok(start > 0, `${id} row exists`);
  const open = view.lastIndexOf('{', start);
  let depth = 0;
  for (let index = open; index < view.length; index += 1) {
    if (view[index] === '{') depth += 1;
    if (view[index] === '}') { depth -= 1; if (depth === 0) return view.slice(open, index + 1); }
  }
  throw new Error(`unterminated ${id}`);
}

test('Station and Bird have Automation rows in Recovery & Support on the same saved keys as the header chips', async () => {
  const view = await source('views/AutomationView.tsx');
  for (const [id, key, toggle, enabled, opener] of ROWS) {
    assert.equal(AUTOMATION_ENABLED_KEYS[id], key, `${id} saved key`);
    const entry = featureEntry(view, id);
    assert.match(entry, new RegExp(`enabledKey: '${key}'`));
    assert.match(entry, /group: 'support'/);
    assert.match(entry, new RegExp(`enabled: ${enabled}\\b`));
    assert.match(entry, new RegExp(`onToggle: ${toggle}\\b`));
    assert.match(entry, new RegExp(`onOpenSettings: ${opener}\\b`));
    assert.match(view, new RegExp(`${opener}: \\(\\) => void`));
  }
  assert.match(view, /\{ id: 'support', name: 'Recovery & Support'/);
  // Every row (these two included) renders the same full feedback; none is excluded.
  assert.match(view, /<AutomationFeatureFeedback\s+featureId=\{feature\.id as SettingsFeatureId\}/);
  assert.doesNotMatch(view, /feature\.id === 'auto(Station|Bird)'/);
  const app = await source('App.tsx');
  assert.match(app, /onOpenAutoStationSettings=\{openSettings\('station'\)\}/);
  assert.match(app, /onOpenAutoBirdSettings=\{openSettings\('bird'\)\}/);
});

test('one write per click: the row switch, the header chip and the popover feedback all go through the same setter', async () => {
  const [view, header, auth] = await Promise.all([source('views/AutomationView.tsx'), source('components/header/StatusPanel.tsx'), source('context/AuthContext.tsx')]);
  for (const [, key, toggle, enabled] of ROWS) {
    // The Auth context maps each toggle to the one write path.
    assert.match(auth, new RegExp(`${toggle}: \\(\\) => toggle\\('${key}', ${enabled}\\)`));
  }
  assert.match(auth, /const toggle = \(feature: string, enabled: boolean\) => \{[^}]*setAutomationEnabled\(feature, !enabled\)[^}]*\};/s);
  // `setAutomationEnabled` is the only function that turns a single automation on or off besides the timed start.
  const writes = [...auth.matchAll(/updateConfiguration\('automation\.enabled'/g)];
  assert.equal(writes.length, 2, 'setAutomationEnabled and enableAutomationFor only');
  // The row switch has exactly one change handler, and the popovers add no write of their own.
  assert.equal([...view.matchAll(/onChange=\{feature\.onToggle\}/g)].length, 1);
  assert.match(header, /onChange=\{toggleAutoStation\}/);
  assert.match(header, /onChange=\{toggleAutoBird\}/);
  for (const file of ['components/AutoBirdCycles.tsx']) {
    const popover = await source(file);
    assert.doesNotMatch(popover, /automation\.enabled|setAutomationEnabled|toggleAuto/, `${file} never writes the enabled switch`);
  }
});

test('row and popover show the same feedback: one shared component, compact only drops "Before you start"', async () => {
  const [header, feedback] = await Promise.all([source('components/header/StatusPanel.tsx'), source('components/AutomationFeatureFeedback.tsx')]);
  for (const [id, , , enabled, opener] of ROWS) {
    const capitalised = id === 'autoStation' ? 'Station' : 'Bird';
    assert.match(header, new RegExp(`<AutomationFeatureFeedback featureId="${id}" enabled=\\{${enabled}\\} onOpenSettings=\\{\\(\\) => dialog\\(onOpenAuto${capitalised}Settings\\)\\} compact />`));
    assert.ok(opener);
  }
  // Phase, next step, failed Start/Stop and first result are rendered for compact and full alike.
  assert.match(feedback, /data-automation-phase=/);
  assert.match(feedback, /<StopControl enabledKey=\{enabledKey\} featureId=\{featureId\} variant="notice" \/>/);
  assert.match(feedback, /<FirstResultCard result=\{result\}/);
  assert.match(feedback, /\{compact \? null : <AutomationReadinessRow featureId=\{featureId\} onOpenSettings=\{onOpenSettings\} \/>\}/);
});

test('the Station popover text exists and no player-facing text says "runtime"', () => {
  for (const key of ['automationPopover.station.title', 'automationPopover.station.detail', 'automationPopover.station.label']) {
    assert.ok(messages[key], key);
    assert.doesNotMatch(messages[key], /runtime/i);
  }
});

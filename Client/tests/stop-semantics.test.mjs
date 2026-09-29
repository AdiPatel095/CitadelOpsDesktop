import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const stop = await vite.ssrLoadModule('/src/settings/readiness/stopSemantics.ts');
const { messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');

after(async () => {
  await vite.close();
});

const NOW = Date.parse('2026-09-29T12:00:00Z');
const receipt = (id, extra = {}) => ({
  id, intent: 'attack.launch', actor: 'automation:autoNomad', priority: 1, status: 'running', phase: 'sent',
  plan: { intent: 'attack.launch', effect: 'launch', stateRevision: 1, steps: [], summary: 'Attack camp 12' },
  submittedAt: '2026-09-29T11:59:00Z', ...extra,
});
const describe = (operations, extra = {}) => stop.describeStopSemantics('autoNomad', {
  enabled: { configured: true, enabled: true }, operations: Object.fromEntries(operations.map((entry) => [entry.id, entry])), connected: true, now: NOW, ...extra,
});
const text = (lines) => lines.map((line) => messages[line.key]).join(' ');

test('Stop says what it does: a saved switch, no new decisions, a running action is asked to stop, nothing is recalled', () => {
  const semantics = describe([]);
  const copy = text(semantics.lines);
  assert.match(copy, /turns the automation off in your saved settings/);
  assert.match(copy, /no new decisions/);
  assert.match(copy, /asked to stop at its next step/);
  assert.match(copy, /not recalled or undone/);
  assert.match(copy, /cancelled, partly done or unconfirmed/);
  assert.doesNotMatch(copy, /right away|immediately|undo it|will be recalled/i);
});

test('no promise of recall, undo or instant cancellation anywhere in the Stop copy', () => {
  const keys = Object.keys(messages).filter((key) => key.startsWith('stopSemantics.') || key.startsWith('settingsRun.'));
  for (const key of keys) {
    assert.doesNotMatch(messages[key], /right away|immediately|can be undone|will be recalled|instantly/i, key);
    assert.doesNotMatch(messages[key], /runtime/i, key);
  }
  assert.match(messages['stopSemantics.running'], /not instant/);
});

test('in-flight actions of this feature are listed; reads, other actors and finished receipts are not', () => {
  const semantics = describe([
    receipt('a'),
    receipt('queued', { status: 'queued', submittedAt: '2026-09-29T11:58:00Z' }),
    receipt('done', { status: 'succeeded', phase: 'completed' }),
    receipt('read', { plan: { intent: 'map.scan', effect: 'read', stateRevision: 1, steps: [] } }),
    receipt('ui', { actor: 'ui' }),
    receipt('other', { actor: 'automation:autoInvasion' }),
  ]);
  assert.deepEqual(semantics.inFlight.map((entry) => entry.id), ['queued', 'a'], 'oldest first');
  assert.equal(semantics.inFlight[1].summary, 'Attack camp 12');
  const line = semantics.lines.find((entry) => entry.key === 'stopSemantics.inFlight');
  assert.equal(line.params.count, 2);
  assert.equal(describe([]).lines.some((entry) => entry.key === 'stopSemantics.inFlight'), false);
});

test('timed runs and a down connection are stated', () => {
  const timed = describe([], { enabled: { configured: true, enabled: true, expiresAtMs: NOW + 3_600_000 } });
  assert.equal(timed.timed, true);
  assert.ok(timed.lines.some((entry) => entry.key === 'stopSemantics.timed' && entry.params.until === NOW + 3_600_000));
  assert.equal(describe([], { enabled: { configured: true, enabled: true, expiresAtMs: NOW - 1 } }).timed, false);
  const offline = describe([], { connected: false });
  assert.ok(offline.lines.some((entry) => entry.key === 'stopSemantics.offline'));
  assert.match(messages['stopSemantics.offline'], /switch is still saved/);
});

test('Stop guard: the control writes only the enabled switch, offers no per-operation cancel and never enables', async () => {
  const control = await readFile(new URL('../src/components/StopControl.tsx', import.meta.url), 'utf8');
  assert.doesNotMatch(control, /cancelOperation|submitIntent|updateConfiguration/, 'the control only calls setAutomationEnabled');
  assert.match(control, /setAutomationEnabled\(enabledKey, enabled\)/);
  assert.match(control, /write\(false\)/);
  const semanticsSource = await readFile(new URL('../src/settings/readiness/stopSemantics.ts', import.meta.url), 'utf8');
  assert.doesNotMatch(semanticsSource, /submitIntent|updateConfiguration|cancelOperation\(/);
  const footer = control.slice(control.indexOf('export const StopFooter'));
  assert.doesNotMatch(footer, /setAutomationEnabled\([^)]*true/, 'the footer never turns anything on');
});

test('a failed Stop stays visible with the reason and a Retry, and the write is derived from the saved switch', async () => {
  const context = await readFile(new URL('../src/context/AuthContext.tsx', import.meta.url), 'utf8');
  assert.match(context, /rememberFailure\(feature, enabled \? 'start' : 'stop', error\)/);
  assert.match(context, /forgetFailure\(feature\)/, 'a successful write clears the failure');
  const control = await readFile(new URL('../src/components/StopControl.tsx', import.meta.url), 'utf8');
  assert.match(control, /role="alert"/);
  assert.match(control, /stopSemantics\.retry/);
  assert.match(control, /automationEnabledByKey\[enabledKey\] === true/, 'the state shown is the saved value, not the click');
  assert.match(messages['stopSemantics.failed'], /still running; try again/, "Maya's copy");
});

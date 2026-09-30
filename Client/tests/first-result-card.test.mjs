import assert from 'node:assert/strict';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const { FirstResultCard } = await vite.ssrLoadModule('/src/components/FirstResultCard.tsx');
const first = await vite.ssrLoadModule('/src/settings/readiness/firstResult.ts');

after(async () => {
  await vite.close();
});

const receipt = (extra = {}) => ({
  id: 'r', intent: 'attack.launch', actor: 'automation:autoNomad', priority: 1, status: 'succeeded', phase: 'completed',
  plan: { intent: 'attack.launch', effect: 'launch', stateRevision: 1, steps: [], summary: 'Attack camp 12' },
  completedStepIndexes: [0], submittedAt: '2026-09-29T10:05:00Z', completedAt: '2026-09-29T10:06:00Z', ...extra,
});
const card = (operations, extra = {}, props = {}) => renderToStaticMarkup(createElement(FirstResultCard, {
  result: first.firstConfirmedResult('autoNomad', { operations: Object.fromEntries(operations.map((entry) => [entry.id, entry])), accountKey: '1:2', enabledSince: '2026-09-29T10:00:00Z', ...extra }),
  ...props,
}));

test('only a confirmed result is green; every other state is neutral, blue or red', () => {
  const confirmed = card([receipt()]);
  assert.match(confirmed, /data-first-result="confirmed"/);
  assert.match(confirmed, /text-success/);
  assert.match(confirmed, /Attack camp 12/);
  assert.match(confirmed, /First confirmed action/);
  for (const [name, operations, extra] of [
    ['none', [], {}],
    ['none', [], { launchRates: { launchesByFeature: { autoNomad: 9 } }, runtime: { id: 'autoNomad', enabled: true, status: 'ready', updatedAt: '2026-09-29T10:10:00Z' } }],
    ['in-progress', [receipt({ id: 'p', status: 'running', phase: 'sent', completedAt: undefined })], {}],
    ['failed', [receipt({ id: 'f', status: 'failed', error: 'The game said no' })], {}],
  ]) {
    const html = card(operations, extra);
    assert.match(html, new RegExp(`data-first-result="${name}"`));
    assert.doesNotMatch(html, /First confirmed action/, `${name}: never claims a confirmed action`);
    assert.doesNotMatch(html, /class="[^"]*text-success[^"]*"/, `${name}: never green`);
  }
});

test('the empty state names what does not count; a counter is shown as a note, not a result', () => {
  const html = card([], { launchRates: { launchesByFeature: { autoNomad: 9 } } });
  assert.match(html, /No confirmed action yet/);
  assert.match(html, /does not count/);
  assert.match(html, /9 launches/);
  assert.match(html, /no confirmed action is attributed to this automation yet/);
});

test('a failed attempt shows the game reason; a confirmed result names the account on hosted; unknown turn-on time is disclosed', () => {
  assert.match(card([receipt({ status: 'failed', error: 'The game said no' })]), /The game said no/);
  const hosted = card([receipt()], { accountLabel: 'Player One' }, { accountLabel: 'Player One' });
  assert.match(hosted, /\(this account\)/);
  assert.match(hosted, /Account: Player One/);
  assert.doesNotMatch(card([receipt()]), /\(this account\)/, 'desktop does not add the hosted account note');
  assert.match(card([receipt()], { enabledSince: undefined }), /not known on this device/);
  assert.match(card([]).replace(/\s+/g, ' '), /No confirmed action yet/);
});

test('a feature whose receipts cannot be attributed says so', () => {
  const html = renderToStaticMarkup(createElement(FirstResultCard, { result: first.firstConfirmedResult('mystery', { operations: {}, accountKey: '1:2' }) }));
  assert.match(html, /cannot be attributed to this automation yet/);
  assert.doesNotMatch(html, /text-success/);
});

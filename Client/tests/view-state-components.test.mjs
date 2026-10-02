import assert from 'node:assert/strict';
import { readFileSync, existsSync } from 'node:fs';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
const root = fileURLToPath(new URL('..', import.meta.url));
const portal = existsSync(`${root}/src/commandCenter`);
const source = portal ? '/src/commandCenter' : '/src';
const fixtures = portal ? '/src/commandCenter/mock/onboarding' : '/tests/onboarding-browser';
const vite = await createServer({ root, configFile: false, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true, hmr: false } });
after(() => vite.close());
const { ViewState, viewStatus } = await vite.ssrLoadModule(`${source}/components/ui/ViewState.tsx`);
const { PillSelector } = await vite.ssrLoadModule(`${source}/components/ui/PillSelector.tsx`);
const { FixtureServer } = await vite.ssrLoadModule(`${fixtures}/fixtureServer.ts`);
const scenario = JSON.parse(readFileSync(`${root}${fixtures}/scenarios/rich-account.json`, 'utf8'));

test('each unavailable state renders one state, and usable data wins over errors and loading', () => {
  const props = { error: { title: 'Read failed', retryLabel: 'Try again', onRetry: () => {} }, loading: { label: 'Reading', variant: 'table' }, empty: { title: 'Nothing collected' }, children: React.createElement('p', {}, 'Usable data') };
  for (const status of ['error', 'loading', 'empty', 'content']) {
    const html = renderToStaticMarkup(React.createElement(ViewState, { ...props, status }));
    assert.equal((html.match(/class="ui-empty-state |class="ui-loading-state /g) ?? []).length, status === 'content' ? 0 : 1);
    assert.equal(html.includes('Usable data'), status === 'content');
    assert.equal(html.includes('Read failed'), status === 'error');
    assert.equal(html.includes('Nothing collected'), status === 'empty');
    assert.equal(html.includes('aria-busy="true"'), status === 'loading');
    assert.ok(!html.includes('role="alert"'));
  }
  const html = renderToStaticMarkup(React.createElement(ViewState, { ...props, status: viewStatus({ hasData: true, loading: true, error: true }) }));
  assert.match(html, /Usable data/); assert.doesNotMatch(html, /Read failed|aria-busy/);
});
test('PillSelector renders radios for five options and a labelled select for six', () => {
  const options = Array.from({ length: 6 }, (_, index) => ({ value: String(index), label: `Option ${index}` }));
  const props = { value: '0', options, onChange: () => {}, ariaLabel: 'Choose section', size: 'header' };
  assert.match(renderToStaticMarkup(React.createElement(PillSelector, { ...props, options: options.slice(0, 5) })), /role="radiogroup"/);
  const html = renderToStaticMarkup(React.createElement(PillSelector, props));
  assert.match(html, /role="combobox"/); assert.match(html, /aria-label="Choose section"/); assert.doesNotMatch(html, /role="radio"/);
});
test('preview history states preserve world and player identity and never read production storage', async () => {
  const server = new FixtureServer({ file: scenario });
  const route = '/api/v2/world-intelligence/players/90211/event-scores?worldId=demo-world';
  const state = value => JSON.stringify({ 'feature-history': value });
  const response = await server.handle(route, 'GET', null, state('empty'));
  assert.equal(response.status, 200); assert.deepEqual(await response.json(), { schemaVersion: 1, worldId: 'demo-world', playerId: 90211, history: [] });
  assert.equal((await server.handle(route, 'GET', null, state('error'))).status, 503);
  const pending = server.handle(route, 'GET', null, state('loading'));
  assert.equal(await Promise.race([pending.then(() => 'settled'), new Promise(resolve => setTimeout(() => resolve('pending'), 5))]), 'pending');
  assert.equal((await server.handle('/api/v2/state', 'GET', null, state('error'))).status, 200);
  assert.equal((await server.handle(route, 'GET', null, '{invalid')).status, 200);
});

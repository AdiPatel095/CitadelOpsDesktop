import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

const root = fileURLToPath(new URL('..', import.meta.url));
const source = existsSync(`${root}/src/commandCenter`) ? '/src/commandCenter' : '/src';
const vite = await createServer({ root, configFile: false, appType: 'custom', logLevel: 'silent',
  server: { middlewareMode: true, hmr: false },
  plugins: [{ name: 'castle-overview-fixture', enforce: 'pre',
    resolveId(source, importer) {
      if (!importer?.endsWith('/CastleOverviewCard.tsx')) return;
      for (const name of ['ApiContext', 'MetadataContext', 'Deployment', 'useAutomationPlayerStatus']) {
        if (source.endsWith(`/${name}`)) return `virtual:castle-${name}`;
      }
    },
    load(id) {
      if (id === 'virtual:castle-ApiContext') return 'export const useCitadelAPI = () => globalThis.__castleOverviewApi;';
      if (id === 'virtual:castle-MetadataContext') return 'export const useMetadata = () => ({resources: {1: {name: "Wood"}, 2: {name: "Stone"}, 3: {name: "Food"}}});';
      if (id === 'virtual:castle-Deployment') return 'export const useHostedRuntimePresence = () => globalThis.__castleOverviewPresence;';
      if (id === 'virtual:castle-useAutomationPlayerStatus') return 'export const useAutomationPlayerStatus = () => ({overall: {status: "off", reason: "Off"}});';
    },
  }],
});
const { default: Overview } = await vite.ssrLoadModule(`${source}/dashboard/components/CastleOverviewCard.tsx`);
after(async () => {
  delete globalThis.__castleOverviewApi;
  delete globalThis.__castleOverviewPresence;
  await vite.close();
});
const castle = { id: 1, resources: {
  1: { amount: 90, capacity: 100, productionPerHour: 12 },
  2: { amount: 80, capacity: 100, productionPerHour: 0 },
  3: { amount: 100, productionPerHour: -5 },
}, units: { stationed: { 1: 10 }, traveling: { 1: 4 }, hospital: { 1: 2 }, specialHospital: { 1: 3 }, total: {} } };
function render({ connected = true, presence = { mode: 'live' } } = {}) {
  globalThis.__castleOverviewApi = { connectionStatus: connected ? 'Connected' : 'Disconnected', configuration: { sections: {} } };
  globalThis.__castleOverviewPresence = presence;
  return renderToStaticMarkup(React.createElement(Overview, { castle }));
}
test('overview renders storage warnings, signed production including food without capacity, and both hospitals', () => {
  const html = render();
  assert.equal((html.match(/role="meter"/g) ?? []).length, 2);
  assert.match(html, /Near cap/);
  assert.match(html, /data-consuming="false">\+12</);
  assert.match(html, /data-consuming="false">0</);
  assert.match(html, /Food<\/span><span[^>]*data-consuming="true">−5</);
  assert.match(html, /In hospital<\/div><div[^>]*><span[^>]*>5</);
  assert.match(html, /No automation&#x27;s saved settings name this castle\./);
  assert.doesNotMatch(html, /Saved data/);
});
test('saved-data banner exposes checkpoint time and disconnected unknown age without raw keys', () => {
  assert.match(render({ presence: { mode: 'checkpoint', checkpointObservedAt: '2026-09-30T12:00:00Z' } }), /Saved data from/);
  assert.match(render({ connected: false }), /Saved data\. It may be out of date\./);
  assert.doesNotMatch(render({ connected: false }), /castleOverview\./);
});

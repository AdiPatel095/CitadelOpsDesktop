import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { readFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { createServer } from 'vite';

const vite = await createServer({
  root: fileURLToPath(new URL('..', import.meta.url)), configFile: false,
  appType: 'custom', logLevel: 'silent', server: { middlewareMode: true },
  plugins: [{ name: 'reserve-stock-test-contexts', enforce: 'pre',
    resolveId(source) {
      if (source.endsWith('/MetadataContext') || source.endsWith('/useMetadata')) return 'virtual:cit138-metadata';
      if (source.endsWith('/LocaleContext') || source.endsWith('/useLocale')) return 'virtual:cit138-locale';
    },
    load(id) {
      if (id === 'virtual:cit138-metadata') return 'export const useMetadata = () => ({ getTroop: id => ({ id, name: "Troop " + id }), getTool: id => ({ id, name: "Tool " + id }) });';
      if (id === 'virtual:cit138-locale') return 'export const useLocale = () => globalThis.__cit138Locale;';
    },
  }],
});
const { messages, describeMessage } = await vite.ssrLoadModule('/src/i18n/messages.ts');
const { formatMessage } = await vite.ssrLoadModule('/src/i18n/formatMessage.ts');
const { UnitStockList } = await vite.ssrLoadModule('/src/settings/components/UnitStockList.tsx');
const message = (key, params) => formatMessage(describeMessage(key, params), 'en', {});
globalThis.__cit138Locale = { message, t: (key, params) => message(key, params).text };
after(async () => { delete globalThis.__cit138Locale; await vite.close(); });

const lines = [
  { itemId: 1, kind: 'troop', required: 100000, stationed: 70000, state: 'short' },
  { itemId: 2, kind: 'troop', required: 1000, stationed: 2500, state: 'valid' },
  { itemId: 3, kind: 'troop', required: 1, stationed: 0, state: 'missing' },
  { itemId: 99, kind: 'troop', required: 1, stationed: 0, state: 'unknown' },
];
const render = mode => renderToStaticMarkup(React.createElement(UnitStockList, { lines, mode }));

test('reserve mode renders keep amounts and counts in castle with unchanged numeric formatting', () => {
  const html = render('reserve');
  assert.ok(html.includes('Keep 100,000 · 70,000 in castle'), html);
  assert.ok(html.includes('Keep 1,000 · 2,500 in castle'), html);
  assert.ok(!html.includes('stationed'), html);
});

test('required mode and accessible stock states use the castle vocabulary', () => {
  const html = render('required');
  assert.ok(html.includes('100,000 needed · 70,000 in castle'), html);
  for (const label of ['Enough in castle', 'None in castle', 'Below the amount', 'Unknown unit']) {
    assert.ok(html.includes(`aria-label="${label}"`), html);
  }
  assert.ok(!html.includes('stationed'), html);
});

test('all six English sources have provenance and the translator vocabulary note', () => {
  const fixture = JSON.parse(readFileSync(new URL('./fixtures/reserve-stock-wording.json', import.meta.url)));
  assert.equal(Object.keys(fixture.messages).length, 6);
  assert.equal(fixture.translatorNote, "Troops located in the player's own castle. Do NOT use the word for 'stationed'. In these features, 'station' means sending troops to another castle.");
  for (const [key, entry] of Object.entries(fixture.messages)) {
    assert.equal(messages[key], entry.english, key);
    assert.equal(createHash('sha256').update(messages[key]).digest('hex'), entry.sourceHash, key);
    assert.ok(!Object.hasOwn(messages, entry.previousKey), entry.previousKey);
  }
});

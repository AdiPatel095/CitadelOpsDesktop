import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { readFile } from 'node:fs/promises';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
import { parse, TYPE } from '@formatjs/icu-messageformat-parser';

const root = fileURLToPath(new URL('..', import.meta.url));
const src = existsSync(new URL('../src/commandCenter/i18n/messages.ts', import.meta.url)) ? '/src/commandCenter' : '/src';
const vite = await createServer({ configFile: false, root, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
after(() => vite.close());
const { messages } = await vite.ssrLoadModule(`${src}/i18n/messages.ts`);
const { formatMessage, validateMessageCatalog } = await vite.ssrLoadModule(`${src}/i18n/formatMessage.ts`);
const keys = JSON.parse(await readFile(new URL('./fixtures/cit-15-22-translation-keys.json', import.meta.url), 'utf8'));
const catalogs = Object.fromEntries(await Promise.all(['de', 'ar'].map(async (locale) => [locale,
  JSON.parse(await readFile(`${root}${src}/i18n/catalogs/${locale}.json`, 'utf8')),
])));
const source = Object.fromEntries(keys.map((key) => [key, messages[key]]));
const unchanged = {
  'castleCopy.field.radius': 'Radius is the standard German word too; Arabic has its own translation.',
  'castleCopy.value.unit': 'Only a localized game-unit parameter, with no authored prose.',
  'observedAt.castleFoodShort': 'Only a date/time template; Intl formats both values in the viewer locale.',
};
const counts = [0, 1, 2, 3, 11, 100];
const timestamp = Date.parse('2026-09-29T12:00:00Z');

function walk(elements, visit) {
  for (const element of elements) {
    visit(element);
    if (element.type === TYPE.select || element.type === TYPE.plural) {
      for (const option of Object.values(element.options)) walk(option.value, visit);
    } else if (element.type === TYPE.tag) walk(element.children, visit);
  }
}

function samples(template) {
  const params = {};
  const selectors = new Map();
  const numeric = new Set();
  walk(parse(template), (element) => {
    if (element.type === TYPE.argument) params[element.value] ??= element.value === 'castle' ? '#12' : `Sample ${element.value}`;
    if (element.type === TYPE.number || element.type === TYPE.plural) {
      numeric.add(element.value);
      params[element.value] = 3;
    }
    if (element.type === TYPE.date || element.type === TYPE.time) params[element.value] = timestamp;
    if (element.type === TYPE.select) {
      const options = selectors.get(element.value) ?? new Set();
      for (const option of Object.keys(element.options)) options.add(option);
      selectors.set(element.value, options);
    }
  });
  let variants = [params];
  for (const [name, options] of selectors) variants = variants.flatMap((variant) => [...options].map((option) => ({ ...variant, [name]: option })));
  return variants.flatMap((variant) => counts.map((count) => ({ ...variant, ...Object.fromEntries([...numeric].map((name) => [name, count])) })));
}

function tags(template) {
  const names = new Set();
  walk(parse(template), (element) => { if (element.type === TYPE.tag) names.add(element.value); });
  return [...names].sort();
}

const render = (key, locale, params) => formatMessage({ key, fallback: messages[key], params }, locale, catalogs[locale] ?? {});
const stripIsolation = (text) => text.replace(/[\u2068\u2069]/g, '');

test('fixture is sorted, unique, and includes 728 onboarding keys, two follow-ups and automation status', () => {
  assert.equal(keys.length, 731);
  assert.deepEqual(keys, [...new Set(keys)].sort());
  for (const key of keys) assert.equal(typeof messages[key], 'string', key);
  for (const key of ['automation.status', 'draftRecovery.compareIntroReappliedCopy', 'commanderAssignment.offForThisAutomation']) assert.ok(keys.includes(key), key);
});

for (const [locale, catalog] of Object.entries(catalogs)) {
  test(`${locale}: every fixture key is authored with matching ICU arguments, selectors and tags`, () => {
    assert.deepEqual(validateMessageCatalog(source, Object.fromEntries(keys.map((key) => [key, catalog[key] ?? '']))), []);
    for (const key of keys) {
      assert.ok(catalog[key]?.trim(), key);
      if (Object.hasOwn(unchanged, key) && (key !== 'castleCopy.field.radius' || locale === 'de')) assert.equal(catalog[key], messages[key], unchanged[key]);
      else assert.notEqual(catalog[key], messages[key], key);
      assert.deepEqual(tags(catalog[key]), tags(messages[key]), key);
      assert.doesNotMatch(catalog[key], /[\u200e\u200f\u202a-\u202e\u2066-\u2069]/, `${key}: no authored direction marks`);
      if (locale === 'ar') walk(parse(catalog[key]), (element) => {
        if (element.type === TYPE.plural) for (const category of ['zero', 'one', 'two', 'few', 'many', 'other']) assert.ok(Object.hasOwn(element.options, category), `${key}: ${category}`);
      });
    }
  });

  test(`${locale}: all selector branches and representative plural counts render without fallback or raw keys`, () => {
    for (const key of keys) {
      for (const params of samples(messages[key])) {
        const result = render(key, locale, params);
        assert.equal(result.translated, true, `${key}: ${JSON.stringify(params)}`);
        assert.equal(result.resolvedLocale, locale, key);
        assert.ok(!result.text.includes(key), `${key}: raw key`);
        assert.doesNotMatch(result.text, /\{[^}]*\}/, `${key}: unresolved ICU`);
      }
    }
  });
}

test('Off agrees across status, runtime phase, stopped explanation and commander assignment', () => {
  assert.equal(render('automation.status', 'en', { status: 'disabled' }).text, 'Off');
  for (const [locale, word, old] of [['de', 'Aus', 'Deaktiviert'], ['ar', 'إيقاف', 'معطّل']]) {
    assert.equal(render('automation.status', locale, { status: 'disabled' }).text, word);
    assert.notEqual(word, old);
    assert.equal(render('runtimeState.phase', locale, { phase: 'disabled' }).text, word);
    for (const key of ['runtimeState.off', 'runtimeState.stopped', 'commanderAssignment.offForThisAutomation']) {
      assert.ok(render(key, locale, { inFlight: 0 }).text.startsWith(word), key);
    }
  }
});

test('missing-castle explanation names the castle and counts only the other enabled castles', () => {
  const key = 'setupReadiness.castlesNotInWorld';
  assert.equal(render(key, 'en', { count: 1, castle: '#12', others: 0 }).text, 'Saved castle #12 is not in this account or world. Reselect or disable it.');
  assert.equal(render(key, 'en', { count: 3, castle: '#12', others: 2 }).text, 'Saved castle #12 and 2 other enabled castles are not in this account or world. Reselect or disable them.');
  for (const locale of ['de', 'ar']) for (const count of [1, 2, 3, 4, 12, 101]) {
    const result = render(key, locale, { count, castle: '#12', others: count - 1 });
    assert.equal(result.translated, true);
    assert.match(stripIsolation(result.text), /#12/);
  }
});

import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { existsSync, mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs';
import { readFile, readdir } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
import { parse, TYPE } from '@formatjs/icu-messageformat-parser';
import { changedIncompleteKeys, generateBatch, sourceAssignments } from '../scripts/localization/translation-batch.mjs';

const root = fileURLToPath(new URL('..', import.meta.url));
const portal = existsSync(path.join(root, 'src/commandCenter/i18n/messages.ts'));
const src = portal ? '/src/commandCenter' : '/src';
const vite = await createServer({ configFile: false, root, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
after(() => vite.close());
const { messages } = await vite.ssrLoadModule(`${src}/i18n/messages.ts`);
const { formatMessage, validateMessageCatalog } = await vite.ssrLoadModule(`${src}/i18n/formatMessage.ts`);
const { localeCodes } = await vite.ssrLoadModule(`${src}/i18n/locales.ts`);
const locales = localeCodes.filter(locale => locale !== 'en');
const json = async file => JSON.parse(await readFile(file, 'utf8'));
const directory = new URL('./fixtures/translation-batches/', import.meta.url);
const files = (await readdir(directory)).filter(file => /^batch-[1-9]\d*\.json$/.test(file)).sort();

function walk(elements, visit) {
  for (const element of elements) {
    visit(element);
    if (element.type === TYPE.select || element.type === TYPE.plural) {
      for (const option of Object.values(element.options)) walk(option.value, visit);
    } else if (element.type === TYPE.tag) walk(element.children, visit);
  }
}

function tags(template) {
  const names = new Set();
  walk(parse(template), element => { if (element.type === TYPE.tag) names.add(element.value); });
  return [...names].sort();
}

function samples(template) {
  const params = {};
  const selectors = new Map();
  const numeric = new Set();
  walk(parse(template), element => {
    if (element.type === TYPE.argument) params[element.value] ??= `Sample ${element.value}`;
    if (element.type === TYPE.number || element.type === TYPE.plural) { numeric.add(element.value); params[element.value] = 3; }
    if (element.type === TYPE.date || element.type === TYPE.time) params[element.value] = Date.parse('2026-09-30T12:00:00Z');
    if (element.type === TYPE.select) {
      const options = selectors.get(element.value) ?? new Set();
      Object.keys(element.options).forEach(option => options.add(option));
      selectors.set(element.value, options);
    }
  });
  let variants = [params];
  for (const [name, options] of selectors) variants = variants.flatMap(variant => [...options].map(option => ({ ...variant, [name]: option })));
  return variants.flatMap(variant => [0, 1, 2, 3, 5, 11, 21, 100].map(count => ({ ...variant, ...Object.fromEntries([...numeric].map(name => [name, count])) })));
}

test('batch fixture directory is nonempty and every supported non-English locale is checked', () => {
  assert.ok(files.length);
  assert.equal(locales.length, 25);
});

for (const file of files) {
  const fixture = await json(new URL(file, directory));
  test(`${file}: valid cut metadata and unique sorted family keys`, () => {
    assert.equal(fixture.schema, 1);
    assert.equal(file, `batch-${fixture.batch}.json`);
    assert.ok(fixture.command.length);
    for (const cut of Object.values(fixture.cuts)) for (const sha of Object.values(cut)) assert.match(sha, /^[0-9a-f]{40}$/);
    for (const family of Object.values(fixture.families)) {
      assert.deepEqual(family.keys, [...new Set(family.keys)].sort());
      for (const [key, reasons] of Object.entries(family.allowEnglish)) {
        assert.ok(family.keys.includes(key), `${key}: allowlist must belong to batch`);
        for (const [locale, reason] of Object.entries(reasons)) {
          assert.ok(locales.includes(locale), locale);
          assert.ok(reason.trim(), `${key}: reason required`);
        }
      }
    }
  });
  for (const [name, family] of Object.entries(fixture.families)) {
    if (!family.keys.length || (name === 'portal' && !portal)) continue;
    const source = name === 'shared' ? messages : await json(path.join(root,
      name === 'portal' ? 'src/i18n/portal/en.json' : `${src}/i18n/server/en.json`));
    const catalogDirectory = name === 'shared' ? `${src}/i18n/catalogs`
      : name === 'portal' ? '/src/i18n/portal' : `${src}/i18n/server`;
    for (const locale of locales) {
      const catalog = await json(`${root}${catalogDirectory}/${locale}.json`);
      test(`${file}/${name}/${locale}: complete translations, ICU parity, plural categories and successful formatting`, () => {
        const selectedSource = Object.fromEntries(family.keys.map(key => {
          assert.equal(typeof source[key], 'string', `${key}: English source exists`);
          return [key, source[key]];
        }));
        const selectedCatalog = Object.fromEntries(family.keys.map(key => [key, catalog[key] ?? '']));
        assert.deepEqual(validateMessageCatalog(selectedSource, selectedCatalog), []);
        for (const key of family.keys) {
          assert.ok(catalog[key]?.trim(), key);
          if (catalog[key] === source[key]) assert.ok(family.allowEnglish[key]?.[locale]?.trim(), `${key}: English equality needs a locale-specific reason`);
          assert.deepEqual(tags(catalog[key]), tags(source[key]), `${key}: rich tags`);
          assert.doesNotMatch(catalog[key], /[\u200e\u200f\u202a-\u202e\u2066-\u2069]/, `${key}: authored direction marks`);
          walk(parse(catalog[key]), element => {
            if (element.type !== TYPE.plural) return;
            const categories = new Intl.PluralRules(locale, { type: element.pluralType }).resolvedOptions().pluralCategories;
            for (const category of categories) assert.ok(Object.hasOwn(element.options, category), `${key}: missing ${category} plural category`);
          });
          for (const params of samples(source[key])) {
            const result = formatMessage({ key, fallback: source[key], params }, locale, catalog);
            assert.equal(result.translated, true, `${key}: ${JSON.stringify(params)}`);
            assert.equal(result.resolvedLocale, locale, key);
            assert.ok(result.text.trim(), `${key}: empty output`);
            assert.ok(!result.text.includes(key), `${key}: raw key`);
            assert.doesNotMatch(result.text, /\{[^}]*\}/, `${key}: unresolved ICU`);
          }
        }
      });
    }
  }
}

test('extractor handles source spreads, escaped strings, additions, changes and missing translations', () => {
  assert.deepEqual(sourceAssignments('export const messages = { ...sourceMessages, "a": "It\\\'s {id}", "b": "Two" } as const;', 'messages'), { a: "It's {id}", b: 'Two' });
  assert.throws(() => sourceAssignments('export const messages = { "a": compute() };', 'messages'), /literal message values/);
  assert.deepEqual(changedIncompleteKeys({ changed: 'Old', same: 'Same', removed: 'Gone' },
    { changed: 'New', same: 'Same', added: 'Added', complete: 'Complete' },
    [{ changed: 'Traduit', added: '', complete: 'Complet' }, { complete: 'Vollständig' }]), ['added', 'changed']);
});

test('generator uses committed cut contents, resolves SHAs and rejects reversed cuts', () => {
  const temp = mkdtempSync(path.join(os.tmpdir(), 'cit-translation-batch-'));
  const git = (...args) => execFileSync('git', ['-C', temp, ...args], { encoding: 'utf8' }).trim();
  const put = (file, content) => { mkdirSync(path.dirname(path.join(temp, file)), { recursive: true }); writeFileSync(path.join(temp, file), content); };
  try {
    git('init', '--quiet');
    git('config', 'user.name', 'Translation test');
    git('config', 'user.email', 'test@example.invalid');
    put('Client/src/i18n/locales.ts', "export const localeCodes = ['en', 'de', 'fr'] as const;");
    put('Client/src/i18n/messages.ts', 'export const messages = { "same": "Same", "changed": "Old" } as const;');
    put('Client/src/i18n/sourceMessages.ts', 'export const sourceMessages = {} as const;');
    put('Server/Localization/en.json', '{"same":"Same"}');
    git('add', '.'); git('commit', '--quiet', '-m', 'Previous cut');
    const since = git('rev-parse', 'HEAD');
    put('Client/src/i18n/messages.ts', 'export const messages = { "same": "Same", "changed": "New", "added": "Added", "complete": "Complete" } as const;');
    for (const locale of ['de', 'fr']) put(`Client/src/i18n/catalogs/${locale}.json`, JSON.stringify({ same: 'Gleich', complete: 'Vollständig' }));
    put('Server/Localization/en.json', '{"same":"Same", "runtime.added":"Added runtime message"}');
    git('add', '.'); git('commit', '--quiet', '-m', 'Current cut');
    const until = git('rev-parse', 'HEAD');
    put('Client/src/i18n/messages.ts', 'export const messages = { "uncommitted": "Ignored" } as const;');
    const fixture = generateBatch({ batch: 7, repositories: { desktop: { root: temp, since, until: 'HEAD' } }, command: ['test'] });
    assert.deepEqual(fixture.cuts.desktop, { since, until });
    assert.deepEqual(fixture.families.shared.keys, ['added', 'changed']);
    assert.deepEqual(fixture.families.runtime.keys, ['runtime.added']);
    const scoped = generateBatch({ batch: 7, repositories: { desktop: { root: temp, since, until } }, command: ['test'], families: ['shared'] });
    assert.deepEqual(Object.keys(scoped.families), ['shared']);
    assert.throws(() => generateBatch({ batch: 7, repositories: { desktop: { root: temp, since, until } }, command: ['test'], families: ['portal'] }), /source repository/);
    const peerRoot = path.join(temp, 'peer');
    mkdirSync(peerRoot);
    const peerGit = (...args) => execFileSync('git', ['-C', peerRoot, ...args], { encoding: 'utf8' }).trim();
    peerGit('init', '--quiet'); peerGit('config', 'user.name', 'Translation test'); peerGit('config', 'user.email', 'test@example.invalid');
    put('peer/src/i18n/portal/en.json', '{"portal.changed":"Old"}');
    peerGit('add', '.'); peerGit('commit', '--quiet', '-m', 'Previous portal cut');
    const peerSince = peerGit('rev-parse', 'HEAD');
    put('peer/src/i18n/portal/en.json', '{"portal.changed":"New"}');
    peerGit('add', '.'); peerGit('commit', '--quiet', '-m', 'Current portal cut');
    const paired = generateBatch({ batch: 7, repositories: {
      desktop: { root: temp, since, until }, portal: { root: peerRoot, since: peerSince, until: 'HEAD' },
    }, command: ['test'] });
    assert.deepEqual(paired.families.shared.keys, ['added', 'changed']);
    assert.deepEqual(paired.families.runtime.keys, ['runtime.added']);
    assert.deepEqual(paired.families.portal.keys, ['portal.changed']);
    assert.throws(() => generateBatch({ batch: 7, repositories: { desktop: { root: temp, since: until, until: since } }, command: ['test'] }));
  } finally { rmSync(temp, { recursive: true, force: true }); }
});

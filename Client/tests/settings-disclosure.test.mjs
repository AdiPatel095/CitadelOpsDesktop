import assert from 'node:assert/strict';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const disclosure = await vite.ssrLoadModule('/src/settings/disclosure/useSettingsDisclosure.ts');
const placement = await vite.ssrLoadModule('/src/settings/disclosure/placement.ts');
const { SettingsSection } = await vite.ssrLoadModule('/src/settings/components/SettingsSection.tsx');
const { messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');

after(async () => {
  await vite.close();
});

function memoryStorage(initial = {}) {
  const values = new Map(Object.entries(initial));
  return {
    values,
    getItem: (key) => (values.has(key) ? values.get(key) : null),
    setItem: (key, value) => { values.set(key, String(value)); },
  };
}

const KEY = 'citadelops.settings.disclosure.v1.autoTowers';

test('every Advanced section starts collapsed; first use needs no Basic/Advanced choice', () => {
  for (const featureId of Object.keys(placement.SETTINGS_PLACEMENT)) {
    assert.deepEqual(disclosure.readSettingsDisclosure(featureId, memoryStorage()), [], featureId);
    assert.ok(placement.SETTINGS_PLACEMENT[featureId].some((section) => section.tier === 'essentials'), `${featureId} has an Essentials view`);
  }
});

test('expanded sections are remembered per feature, in browser storage only', () => {
  const storage = memoryStorage();
  disclosure.writeSettingsDisclosure('autoTowers', ['travel', 'advisor'], storage);
  assert.deepEqual(JSON.parse(storage.values.get(KEY)), { version: 1, expanded: ['advisor', 'travel'] });
  assert.deepEqual(disclosure.readSettingsDisclosure('autoTowers', storage).sort(), ['advisor', 'travel']);
  assert.deepEqual(disclosure.readSettingsDisclosure('autoStorm', storage), [], 'another feature keeps its own memory');
});

test('unknown, Essentials, duplicated or malformed entries are dropped', () => {
  const storage = memoryStorage({ [KEY]: JSON.stringify({ version: 1, expanded: ['travel', 'travel', 'castles', 'gone', 7] }) });
  assert.deepEqual(disclosure.readSettingsDisclosure('autoTowers', storage), ['travel']);
  assert.deepEqual(disclosure.readSettingsDisclosure('autoTowers', memoryStorage({ [KEY]: '{not json' })), []);
  assert.deepEqual(disclosure.readSettingsDisclosure('autoTowers', memoryStorage({ [KEY]: JSON.stringify({ version: 2, expanded: ['travel'] }) })), []);
});

test('storage failures fall back to collapsed without throwing', () => {
  const throwing = { getItem: () => { throw new Error('blocked'); }, setItem: () => { throw new Error('quota'); } };
  assert.deepEqual(disclosure.readSettingsDisclosure('autoTowers', throwing), []);
  assert.doesNotThrow(() => disclosure.writeSettingsDisclosure('autoTowers', ['travel'], throwing));
  assert.deepEqual(disclosure.readSettingsDisclosure('autoTowers', null), []);
  assert.doesNotThrow(() => disclosure.writeSettingsDisclosure('autoTowers', ['travel'], null));
});

test('toggle is a pure visibility change', () => {
  assert.deepEqual(disclosure.toggleSettingsDisclosure([], 'travel'), ['travel']);
  assert.deepEqual(disclosure.toggleSettingsDisclosure(['travel', 'scan'], 'travel'), ['scan']);
});

function fakeDisclosure(expanded = [], featureId = 'autoTowers') {
  const calls = [];
  return {
    calls,
    featureId,
    expandedIds: expanded,
    isExpanded: (id) => expanded.includes(id),
    toggle: (id) => calls.push(['toggle', id]),
    expand: (id) => calls.push(['expand', id]),
    reveal: (id) => calls.push(['reveal', id]),
    fix: () => false,
    collapsedTarget: () => null,
  };
}

function findElement(node, predicate) {
  if (node == null || typeof node !== 'object') return null;
  if (Array.isArray(node)) {
    for (const child of node) {
      const found = findElement(child, predicate);
      if (found) return found;
    }
    return null;
  }
  if (predicate(node)) return node;
  return findElement(node.props?.children, predicate);
}

test('a collapsed Advanced section shows its summary, keeps the body mounted but hidden, and exposes aria state', () => {
  const summary = [{ messageKey: 'settingsSummary.travel', params: { boost: 'rubies' } }];
  const html = renderToStaticMarkup(createElement(SettingsSection, {
    disclosure: fakeDisclosure(), section: 'travel', summary, customCount: 1,
  }, createElement('input', { id: 'hidden-control', defaultValue: '1008' })));
  assert.match(html, /data-settings-tier="advanced"/);
  assert.match(html, /aria-expanded="false"/);
  assert.match(html, /aria-controls="settings-autoTowers-travel-body"/);
  assert.match(html, /<div id="settings-autoTowers-travel-body" hidden=""/);
  assert.match(html, /id="hidden-control"/, 'the collapsed control stays mounted, so its value is never reset');
  assert.match(html, /Travel: a boost paid with Rubies on every launch/);
  assert.match(html, /1 custom value/);
  assert.match(html, /Paid travel/);
});

test('an expanded Advanced section hides the summary line and shows the body', () => {
  const html = renderToStaticMarkup(createElement(SettingsSection, {
    disclosure: fakeDisclosure(['travel']), section: 'travel', summary: [{ messageKey: 'settingsSummary.travel', params: { boost: 'coins' } }],
  }, createElement('span', null, 'body')));
  assert.match(html, /aria-expanded="true"/);
  assert.doesNotMatch(html, /hidden=""/);
  assert.doesNotMatch(html, /Travel: horse or ship tier/);
});

test('Essentials sections render as-is with no disclosure control', () => {
  const html = renderToStaticMarkup(createElement(SettingsSection, { disclosure: fakeDisclosure(), section: 'castles' }, createElement('span', null, 'castles')));
  assert.match(html, /data-settings-tier="essentials"/);
  assert.doesNotMatch(html, /aria-expanded/);
});

test('expanding or collapsing only calls the disclosure toggle: no save, draft change, reset or activation', () => {
  const fake = fakeDisclosure();
  const element = SettingsSection({ disclosure: fake, section: 'advisor', summary: [], children: createElement('span', null, 'x') });
  const button = findElement(element, (node) => node.type === 'button');
  assert.ok(button, 'a header button');
  button.props.onClick();
  assert.deepEqual(fake.calls, [['toggle', 'advisor']]);
  assert.equal(Object.keys(button.props).some((prop) => /save|submit|enable/i.test(prop)), false);
});

test('unknown sections fail loudly instead of rendering an unplaced group', () => {
  assert.throws(() => placement.sectionPlacement('autoTowers', 'nope'), /Unknown settings section/);
});

test('section titles exist and keep "runtime" out', () => {
  for (const sections of Object.values(placement.SETTINGS_PLACEMENT)) {
    for (const section of sections) {
      assert.ok(messages[section.titleKey], section.titleKey);
      assert.doesNotMatch(messages[section.titleKey], /runtime/i, section.titleKey);
    }
  }
});

import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const root = fileURLToPath(new URL('..', import.meta.url));
const portal = existsSync(`${root}/src/commandCenter`);
const source = portal ? '/src/commandCenter' : '/src';
const vite = await createServer({ root, configFile: false, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true, hmr: false } });
after(() => vite.close());
const { viewStatus } = await vite.ssrLoadModule(`${source}/components/ui/viewStatus.ts`);
const { TABS_SELECT_MIN_ITEMS, SEGMENTED_MAX_OPTIONS, tabsMode, segmentedMode, nextTabIndex } = await vite.ssrLoadModule(`${source}/components/ui/tabsLogic.ts`);
const { VISUAL_STATES_KEY, parseVisualStates } = await vite.ssrLoadModule(portal ? `${source}/mock/onboarding/visualStates.ts` : '/tests/onboarding-browser/visualStates.ts');

test('view status preserves usable data and otherwise prioritizes error, loading, empty', () => {
  for (const hasData of [false, true]) {
    for (const error of [false, true]) {
      for (const loading of [false, true]) {
        const expected = hasData ? 'content' : error ? 'error' : loading ? 'loading' : 'empty';
        assert.equal(viewStatus({ hasData, error, loading }), expected, JSON.stringify({ hasData, error, loading }));
      }
    }
  }
});

test('tabs switch at four items only below medium and segments switch above five', () => {
  assert.equal(TABS_SELECT_MIN_ITEMS, 4);
  assert.equal(SEGMENTED_MAX_OPTIONS, 5);
  for (const count of [1, 2, 3, 4, 5, 12]) {
    assert.equal(tabsMode(count, false), count >= 4 ? 'select' : 'tabs');
    assert.equal(tabsMode(count, true), 'tabs');
  }
  assert.equal(segmentedMode(5), 'segmented');
  assert.equal(segmentedMode(6), 'select');
});

test('arrows wrap and skip disabled tabs without modifying items', () => {
  const items = Object.freeze([Object.freeze({}), Object.freeze({ disabled: true }), Object.freeze({})]);
  assert.equal(nextTabIndex('ArrowRight', 0, items, false), 2);
  assert.equal(nextTabIndex('ArrowLeft', 2, items, false), 0);
  assert.equal(nextTabIndex('ArrowRight', 2, items, false), 0);
  assert.equal(nextTabIndex('ArrowLeft', 0, items, false), 2);
});

test('RTL reverses arrows but retains Home and End order', () => {
  const items = [{ disabled: true }, {}, { disabled: true }, {}, { disabled: true }];
  for (const rtl of [false, true]) {
    assert.equal(nextTabIndex('Home', 3, items, rtl), 1);
    assert.equal(nextTabIndex('End', 1, items, rtl), 3);
  }
  const enabled = [{}, {}, {}];
  assert.equal(nextTabIndex('ArrowRight', 1, enabled, true), 0);
  assert.equal(nextTabIndex('ArrowLeft', 1, enabled, true), 2);
  assert.equal(nextTabIndex('ArrowRight', 0, items, true), 3);
  assert.equal(nextTabIndex('ArrowLeft', 4, items, true), 1);
});

test('unknown keys, empty lists and all-disabled lists preserve the current index', () => {
  assert.equal(nextTabIndex('Enter', 1, [{}, {}], false), 1);
  assert.equal(nextTabIndex('ArrowDown', 1, [{}, {}], false), 1);
  for (const items of [[], [{ disabled: true }, { disabled: true }]]) {
    for (const key of ['Home', 'End', 'ArrowRight', 'ArrowLeft']) {
      for (const rtl of [false, true]) assert.equal(nextTabIndex(key, 1, items, rtl), 1);
    }
  }
  assert.equal(nextTabIndex('ArrowRight', 0, [{}], false), 0);
});

test('visual state parser accepts only known source and state pairs', () => {
  assert.equal(VISUAL_STATES_KEY, 'citadelops.visualStates');
  const valid = { 'feature-history': 'empty', 'world-intel': 'loading', 'hosted-accounts': 'error' };
  assert.deepEqual(parseVisualStates(JSON.stringify(valid)), valid);
  assert.deepEqual(parseVisualStates('{"feature-history":"error","world-intel":"content","unknown":"loading","__proto__":{"polluted":true}}'), { 'feature-history': 'error' });
  assert.deepEqual(parseVisualStates('{"feature-history":null,"world-intel":1,"hosted-accounts":{}}'), {});
});

test('visual state parser drops invalid JSON and non-objects without throwing', () => {
  for (const raw of [null, undefined, '', '{', 'null', '[]', '["error"]', 'true', '42', '"empty"']) {
    assert.deepEqual(parseVisualStates(raw), {}, String(raw));
  }
});

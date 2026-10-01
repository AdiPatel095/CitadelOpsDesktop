import assert from 'node:assert/strict';
import test from 'node:test';
import fs from 'node:fs';
import { createRequire } from 'node:module';
import vm from 'node:vm';
import ts from 'typescript';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

const sourceRoot = new URL('../src/', import.meta.url);
const require = createRequire(import.meta.url);
const cache = new Map();
function load(file) {
  if (cache.has(file.href)) return cache.get(file.href);
  const module = { exports: {} };
  cache.set(file.href, module.exports);
  const code = ts.transpileModule(fs.readFileSync(file, 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText;
  vm.runInNewContext(code, { exports: module.exports, module, Intl, require: id =>
    id.startsWith('.') ? load(new URL(id.endsWith('.ts') ? id : `${id}.ts`, file)) : require(id),
  }, { filename: file.pathname });
  return module.exports;
}
const { messages } = load(new URL('i18n/messages.ts', sourceRoot));
const { formatMessage, validateMessageCatalog } = load(new URL('i18n/formatMessage.ts', sourceRoot));
const dialogFile = new URL('settings/components/AutomationDurationModal.tsx', sourceRoot);
const dialogSource = fs.readFileSync(dialogFile, 'utf8');
const prefix = 'automationDurationDialog.';
// Frozen inventory of pre-CIT-85 English, including both run and pause variants.
const inventory = [
  ['runTitle', { feature: 'Auto Bird' }, 'Run Auto Bird for a duration'],
  ['pauseTitle', { feature: 'Auto Bird' }, 'Pause Auto Bird for a duration'],
  ['presetMinutes', { count: 30 }, '30 min'],
  ...[1, 2, 4, 8, 24].map(count => ['presetHours', { count }, `${count} hr`]),
  ['saveFailed', {}, 'Could not save the timed automation duration'],
  ['runButton', {}, 'Turn on for this duration'], ['pauseButton', {}, 'Pause for this duration'],
  ['quickDurations', {}, 'Quick durations'], ['customDuration', {}, 'Custom duration'],
  ['amountLabel', {}, 'Automation duration amount'], ['unitLabel', {}, 'Automation duration unit'],
  ['minutes', {}, 'Minutes'], ['hours', {}, 'Hours'], ['days', {}, 'Days'],
  ['invalidDuration', {}, 'Choose a duration from 1 minute through 7 days.'],
  ['runStarts', { feature: 'Auto Bird' }, 'Auto Bird turns on immediately and the server turns it off at'],
  ['pauseStarts', { feature: 'Auto Bird' }, 'Auto Bird pauses immediately and resumes at'],
  ['scheduleNotice', {}, 'Weekly schedules and the global automation lock still apply during this window.'],
  ['currentRunEnds', { date: 'DATE' }, 'Current timed run ends DATE.'],
  ['currentPauseEnds', { date: 'DATE' }, 'Current pause ends DATE.'],
];
const translate = (key, params, locale = 'en') => formatMessage({ key, fallback: messages[key], params }, locale, {}).text;

test('every duration key formats to the exact old English, including all preset counts', () => {
  assert.equal(Object.keys(messages).filter(key => key.startsWith(prefix)).length, 20);
  for (const [suffix, params, expected] of inventory) {
    assert.ok(Object.hasOwn(messages, prefix + suffix), suffix);
    assert.equal(translate(prefix + suffix, params), expected, suffix);
  }
});
test('the duration English catalog validates', () => {
  const english = Object.fromEntries(Object.entries(messages).filter(([key]) => key.startsWith(prefix)));
  assert.equal(JSON.stringify(validateMessageCatalog(english, english)), '[]');
});
test('no inventoried literal remains in the dialog source', () => {
  const oldLiterals = new Set(inventory.map(([, , value]) => value));
  for (const value of ['Pause', 'Run', 'for a duration', 'pauses immediately and resumes at',
    'turns on immediately and the server turns it off at', 'Current pause ends', 'Current timed run ends']) oldLiterals.add(value);
  const ast = ts.createSourceFile(dialogFile.pathname, dialogSource, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  const visit = node => {
    if (ts.isStringLiteralLike(node) || ts.isJsxText(node)) {
      assert.ok(!oldLiterals.has(node.text.trim()), `Old literal remains: ${node.text}`);
    }
    ts.forEachChild(node, visit);
  };
  visit(ast);
  for (const suffix of new Set(inventory.map(([suffix]) => suffix))) assert.ok(dialogSource.includes(prefix + suffix), suffix);
});

// Render the real dialog with isolated UI/auth boundaries and deterministic hooks/time.
function renderDialog({ pause = false, amount = '1', locale = 'en', saveError } = {}) {
  const setters = [];
  let stateIndex = 0;
  const react = { ...React, useEffect: () => {}, useMemo: fn => fn(), useState: initial => {
    const index = stateIndex++;
    return [index === 0 ? amount : initial, value => setters.push({ index, value })];
  } };
  const h = React.createElement;
  const ui = {
    Modal: ({ title, children, footer }) => h('section', null, title, children, footer),
    ModalTitle: ({ children }) => h('h2', null, children),
    Button: ({ children, onClick, disabled }) => h('button', { onClick, disabled }, children),
    Input: props => h('input', props),
    Select: ({ options, ariaLabel, value }) => h('select', { 'aria-label': ariaLabel, defaultValue: value },
      options.map(option => h('option', { key: option.value, value: option.value }, option.label))),
  };
  const NativeDate = Date;
  class FixedDate extends NativeDate {
    constructor(...args) { super(...(args.length ? args : ['2026-10-01T00:00:00Z'])); }
    static now() { return 1790812800000; }
    toLocaleString() { return 'DATE'; }
  }
  const t = (key, params) => translate(key, params, locale);
  const auth = { automationTimedUntilByKey: { autoBird: 1790816400000 },
    enableAutomationFor: async () => { if (saveError !== undefined) throw saveError; } };
  const imports = {
    react, 'lucide-react': { TimerReset: () => null }, '../../context/AuthContext': { useAuth: () => auth },
    '../../components/ui': ui, '../../i18n/LocaleContext': { useLocale: () => ({ t }) },
    '../../i18n/LocalizedText': { LocalizedText: ({ messageKey }) => h('span', null, messageKey === 'game.cancel' ? 'Cancel' : t(messageKey)) },
  };
  const module = { exports: {} };
  vm.runInNewContext(ts.transpileModule(dialogSource, { compilerOptions: {
    module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.React, esModuleInterop: true,
  } }).outputText, { module, exports: module.exports, Date: FixedDate, Error, require: id => {
    assert.ok(Object.hasOwn(imports, id), `Unmocked boundary: ${id}`); return imports[id];
  } });
  const element = module.exports.AutomationDurationModal({ isOpen: true, featureKey: 'autoBird', featureLabel: 'Auto Bird',
    onClose: () => {}, ...(pause ? { onPauseFor: async () => {}, pausedUntil: 1790816400000 } : {}) });
  return { html: renderToStaticMarkup(element), element, setters };
}
function text(html) { return html.replace(/<[^>]*>/g, '').replaceAll('&#x27;', "'").replaceAll('&amp;', '&'); }
test('rendered run/pause English, whitespace, punctuation and aria labels match the old dialog', () => {
  for (const pause of [false, true]) {
    const { html } = renderDialog({ pause });
    const expected = `${pause ? 'Pause' : 'Run'} Auto Bird for a durationQuick durations30 min1 hr2 hr4 hr8 hr24 hrCustom durationMinutesHoursDays` +
      `Auto Bird ${pause ? 'pauses immediately and resumes at' : 'turns on immediately and the server turns it off at'} DATE.` +
      `Weekly schedules and the global automation lock still apply during this window.Current ${pause ? 'pause' : 'timed run'} ends DATE.Cancel` +
      `${pause ? 'Pause' : 'Turn on'} for this duration`;
    assert.equal(text(html), expected);
    assert.ok(html.includes('aria-label="Automation duration amount"'));
    assert.ok(html.includes('aria-label="Automation duration unit"'));
  }
});
test('invalid-duration hint preserves English and disables save', () => {
  const { html } = renderDialog({ amount: '0' });
  assert.ok(text(html).includes('Choose a duration from 1 minute through 7 days.'));
  assert.ok(html.includes('<button disabled="">Turn on for this duration</button>'));
  assert.ok(!text(html).includes('turns on immediately'));
});
test('non-Error save failures use the exact old fallback message', async () => {
  const { element, setters } = renderDialog({ saveError: 'non-error failure' });
  element.props.footer.props.children[1].props.onClick();
  await new Promise(resolve => setImmediate(resolve));
  assert.ok(setters.some(({ index, value }) => index === 3 && value === 'Could not save the timed automation duration'));
});
test('German missing keys fall back to exact English without raw keys', () => {
  for (const [suffix, params, expected] of inventory) assert.equal(translate(prefix + suffix, params, 'de'), expected);
  for (const pause of [false, true]) assert.equal(text(renderDialog({ pause, locale: 'de' }).html), text(renderDialog({ pause }).html));
});

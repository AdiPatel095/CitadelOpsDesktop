import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { compileFunction } from 'node:vm';
import test from 'node:test';
import ts from 'typescript';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

const root = fileURLToPath(new URL('..', import.meta.url));
const ui = resolve(root, readFileSync(resolve(root, 'package.json'), 'utf8').includes('citadel-ops-client') ? 'src/components/ui' : 'src/commandCenter/components/ui');
const external = createRequire(import.meta.url);
const cache = new Map();
function load(file) {
  if (cache.has(file)) return cache.get(file).exports;
  const module = { exports: {} };
  cache.set(file, module);
  const source = ts.transpileModule(readFileSync(file, 'utf8'), {
    compilerOptions: { jsx: ts.JsxEmit.ReactJSX, module: ts.ModuleKind.CommonJS, esModuleInterop: true },
  }).outputText;
  const require = (name) => name.endsWith('.css') ? {} : name.startsWith('.') ? load(resolve(dirname(file), `${name}.tsx`)) : external(name);
  compileFunction(source, ['module', 'exports', 'require'])(module, module.exports, require);
  return module.exports;
}
const { Card, CardHeader, CardContent } = load(resolve(ui, 'Card.tsx'));
const { SectionCard } = load(resolve(ui, 'SectionCard.tsx'));
const { Panel } = load(resolve(ui, 'Panel.tsx'));
const { SectionHeader } = load(resolve(ui, 'SectionHeader.tsx'));

test('card forwards selection, ref-compatible root attributes, divided and flush contracts', () => {
  const html = renderToStaticMarkup(createElement(Card, { id: 'selected', tabIndex: -1, 'data-current-selection': 'true', variant: 'interactive' },
    createElement(CardHeader, { divided: true }, 'Title'), createElement(CardContent, { flush: true }, 'Rows')));
  assert.match(html, /id="selected"/);
  assert.match(html, /tabindex="-1"/);
  assert.match(html, /data-current-selection="true"/);
  assert.match(html, /ui-card--interactive/);
  assert.match(html, /ui-card__header--divided/);
  assert.match(html, /ui-card__content--flush/);
  assert.doesNotMatch(html, /divided=|flush=|variant=/);
});
test('uncontrolled disclosure starts expanded; controlled collapse retains a labelled content region', () => {
  const open = renderToStaticMarkup(createElement(SectionCard, { title: 'Queues', collapsible: true }, 'Rows'));
  assert.match(open, /aria-expanded="true"/);
  assert.doesNotMatch(open, / hidden=/);
  const closed = renderToStaticMarkup(createElement(SectionCard, { title: 'Queues', collapsible: true, expanded: false, toggleLabel: 'Toggle queues', actions: createElement('button', {}, 'Refresh') }, 'Rows'));
  assert.match(closed, /aria-expanded="false"/);
  assert.match(closed, /aria-label="Toggle queues"/);
  assert.match(closed, / hidden=""/);
  const target = closed.match(/aria-controls="([^"]+)"/)[1];
  assert.ok(closed.includes(`id="${target}"`));
  assert.match(closed, /<\/button><div class="ui-card__actions/);
  assert.equal((closed.match(/<button/g) ?? []).length, 2);
  assert.doesNotMatch(closed, /\s(?:expanded|collapsible|toggleLabel)=/);
});
test('plain sections have no disclosure; panels and section headings preserve semantic roles', () => {
  const section = renderToStaticMarkup(createElement(SectionCard, { title: 'Queues', description: 'Manage queues' }, 'Rows'));
  assert.doesNotMatch(section, /<button|aria-expanded/);
  assert.match(section, /ui-card__description/);
  assert.match(renderToStaticMarkup(createElement(Panel, { as: 'section', 'aria-label': 'Details' }, 'Data')), /<section class="ui-panel /);
  const header = renderToStaticMarkup(createElement(SectionHeader, { title: 'Offense', level: 3, count: 0 }));
  assert.match(header, /<h3 class="ui-section-header__title">Offense<\/h3>/);
  assert.match(header, /ui-section-header__count">0/);
});

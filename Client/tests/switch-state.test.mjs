import { sourceMessages } from '../src/i18n/sourceMessages.ts';
import assert from 'node:assert/strict';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { readFile } from 'node:fs/promises';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({
  root: clientRoot,
  appType: 'custom',
  logLevel: 'silent',
  server: { middlewareMode: true },
});
const { Switch } = await vite.ssrLoadModule('/src/components/ui/Switch.tsx');
const { SettingsToggleRow } = await vite.ssrLoadModule('/src/components/ui/SettingsToggleRow.tsx');

after(async () => {
  await vite.close();
});

test('all switches expose one shared off and on state contract', () => {
  const changed = [];
  const off = Switch({ checked: false, onChange: (checked) => changed.push(checked), ariaLabel: 'Disable test feature' });
  const on = Switch({ checked: true, onChange: (checked) => changed.push(checked), ariaLabel: 'Enable test feature' });

  assert.equal(off.props['data-state'], 'off');
  assert.equal(off.props['aria-checked'], false);
  assert.equal(off.props['aria-label'], 'Disable test feature');
  assert.equal(on.props['data-state'], 'on');
  assert.equal(on.props['aria-checked'], true);
  assert.equal(on.props['aria-label'], 'Enable test feature');
  assert.doesNotMatch(`${off.props.className} ${on.props.className}`, /liquid-switch-tone-/);

  off.props.onClick();
  on.props.onClick();
  assert.deepEqual(changed, [true, false]);
});

test('switches expose native disabled behavior through the shared component', () => {
  const disabled = Switch({ checked: false, onChange: () => {}, disabled: true, ariaLabel: 'Disabled setting' });
  assert.equal(disabled.props.disabled, true);
  assert.equal(disabled.props.role, 'switch');
  assert.match(disabled.props.className, /\bui-switch\b/);
});

test('settings rows delegate their binary state to the shared switch', () => {
  const markup = renderToStaticMarkup(createElement(SettingsToggleRow, {
    title: 'Retry failed enchantments',
    checked: true,
    onChange: () => {},
  }));
  assert.match(markup, /role="switch"/);
  assert.match(markup, /aria-checked="true"/);
  assert.match(markup, /aria-label="Retry failed enchantments"/);
});

test('defense courtyard inclusion uses the shared binary switch', async () => {
  const editor = await readFile(new URL('../src/components/DefensePresetEditor.tsx', import.meta.url), 'utf8');

  assert.doesNotMatch(editor, /type="checkbox"/);
  assert.match(editor, /<label className="flex cursor-pointer items-start gap-3">[\s\S]*?<Switch/);
  assert.match(editor, /checked=\{draft\.keep != null\}/);
  assert.match(editor, /onChange=\{\(includeCourtyard\) => setDraft/);
  assert.ok(Object.entries(sourceMessages).some(([key,text]) => text === 'Include courtyard setup in this defense preset' && editor.includes(`ariaLabel={localizeStatic("${key}")}`)));
});

test('switches own neutral off/disabled tokens and the on check glyph', async () => {
  const css = await readFile(new URL('../src/components/ui/switch.css', import.meta.url), 'utf8');
  const palette = await readFile(new URL('../src/MaterialExpressive.css', import.meta.url), 'utf8');
  assert.match(css, /background: var\(--surface-control\)/);
  assert.match(css, /border: 2px solid var\(--border-strong\)/);
  assert.match(css, /\.ui-switch\[data-state="on"\] \.ui-switch__track \{ border: 0; background: var\(--control-on\)/);
  assert.match(css, /\.ui-switch:disabled \.ui-switch__track \{ border: 1px solid var\(--border-subtle\); background: var\(--fill-disabled\)/);
  assert.doesNotMatch(css, /--accent|--status-danger|--status-success/);
  assert.doesNotMatch(palette, /liquid-switch/);
  const on = renderToStaticMarkup(createElement(Switch, { checked: true, onChange: () => {}, ariaLabel: 'Enabled' }));
  const off = renderToStaticMarkup(createElement(Switch, { checked: false, onChange: () => {}, ariaLabel: 'Disabled' }));
  assert.match(on, /lucide-check/);
  assert.doesNotMatch(off, /<svg/);
  assert.match(on, /width="12"/);
});

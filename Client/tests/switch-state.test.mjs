import assert from 'node:assert/strict';
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

  assert.match(off.props.className, /\bliquid-switch-off\b/);
  assert.equal(off.props['aria-checked'], false);
  assert.equal(off.props['aria-label'], 'Disable test feature');
  assert.match(on.props.className, /\bliquid-switch-on\b/);
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
  assert.match(disabled.props.className, /\bliquid-switch-sm\b/);
});

test('settings rows delegate their binary state to the shared switch', () => {
  const row = SettingsToggleRow({
    title: 'Retry failed enchantments',
    checked: true,
    onChange: () => {},
  });
  const children = Array.isArray(row.props.children) ? row.props.children : [row.props.children];
  const switchElement = children.find((child) => child?.type === Switch);

  assert.ok(switchElement);
  assert.equal(switchElement.props.checked, true);
  assert.equal(switchElement.props.ariaLabel, 'Retry failed enchantments');
});

test('defense courtyard inclusion uses the shared binary switch', async () => {
  const editor = await readFile(new URL('../src/components/DefensePresetEditor.tsx', import.meta.url), 'utf8');

  assert.doesNotMatch(editor, /type="checkbox"/);
  assert.match(editor, /<label className="flex cursor-pointer items-start gap-3">[\s\S]*?<Switch/);
  assert.match(editor, /checked=\{draft\.keep != null\}/);
  assert.match(editor, /onChange=\{\(includeCourtyard\) => setDraft/);
  assert.match(editor, /ariaLabel="Include courtyard setup in this defense preset"/);
});

test('the final palette gives every switch distinct danger and success colors', async () => {
  const palette = await readFile(new URL('../src/KingdomPalette.css', import.meta.url), 'utf8');
  const automationView = await readFile(new URL('../src/views/AutomationView.tsx', import.meta.url), 'utf8');

  assert.match(palette, /\.liquid-switch-off \.liquid-switch-rail[\s\S]*var\(--status-danger\)/);
  assert.match(palette, /\.liquid-switch-on \.liquid-switch-rail[\s\S]*var\(--status-success\)/);
  assert.doesNotMatch(palette, /liquid-switch-tone-/);
  assert.doesNotMatch(automationView, /tone="feature"/);
  assert.match(palette, /@media \(forced-colors: active\)[\s\S]*\.liquid-switch-off:focus-visible,[\s\S]*\.liquid-switch-on:focus-visible[\s\S]*outline: 2px solid Highlight[\s\S]*filter: none/);
});

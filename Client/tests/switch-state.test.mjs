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

after(async () => {
  await vite.close();
});

test('feature switches expose red-off and green-on state hooks', () => {
  const changed = [];
  const off = Switch({ checked: false, onChange: (checked) => changed.push(checked), tone: 'feature' });
  const on = Switch({ checked: true, onChange: (checked) => changed.push(checked), tone: 'feature' });

  assert.match(off.props.className, /\bliquid-switch-tone-feature\b/);
  assert.match(off.props.className, /\bliquid-switch-off\b/);
  assert.equal(off.props['aria-checked'], false);
  assert.match(on.props.className, /\bliquid-switch-tone-feature\b/);
  assert.match(on.props.className, /\bliquid-switch-on\b/);
  assert.equal(on.props['aria-checked'], true);

  off.props.onClick();
  on.props.onClick();
  assert.deepEqual(changed, [true, false]);
});

test('ordinary switches retain the default tone', () => {
  const ordinary = Switch({ checked: false, onChange: () => {} });
  assert.match(ordinary.props.className, /\bliquid-switch-tone-default\b/);
  assert.doesNotMatch(ordinary.props.className, /\bliquid-switch-tone-feature\b/);
});

test('the final palette gives feature states distinct danger and success colors', async () => {
  const palette = await readFile(new URL('../src/KingdomPalette.css', import.meta.url), 'utf8');
  const automationView = await readFile(new URL('../src/views/AutomationView.tsx', import.meta.url), 'utf8');

  assert.match(palette, /\.liquid-switch-tone-feature\.liquid-switch-off \.liquid-switch-rail[\s\S]*var\(--status-danger\)/);
  assert.match(palette, /\.liquid-switch-tone-feature\.liquid-switch-on \.liquid-switch-rail[\s\S]*var\(--status-success\)/);
  assert.equal(automationView.match(/tone="feature"/g)?.length, 2);
});

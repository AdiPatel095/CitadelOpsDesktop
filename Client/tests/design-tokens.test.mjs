import assert from 'node:assert/strict';
import { test } from 'node:test';
import { execFileSync } from 'node:child_process';
import { readFileSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { arbitraryValueCount, ratchetFailures } from '../scripts/check-arbitrary-values.mjs';
import { cssMetrics } from '../scripts/css-metrics.mjs';

const root = fileURLToPath(new URL('../', import.meta.url));
const desktop = existsSync(join(root, 'src/styles/tokens.css'));
const source = readFileSync(join(root, desktop ? 'src/styles/tokens.css' : 'src/commandCenter/styles/tokens.css'), 'utf8');
const colors = `surface-canvas surface-card surface-inset surface-control surface-control-strong surface-overlay surface-field surface-inverse text-primary text-secondary text-muted text-disabled text-inverse text-on-accent border-subtle border-default border-strong accent accent-hover accent-pressed accent-container text-on-accent-container state-hover state-pressed state-selected focus-ring fill-disabled scrim selection control-on control-on-thumb data-1 data-2 data-3 data-4`.split(' ');
const expected = [...colors,
  ...['success', 'warning', 'danger', 'info', 'neutral'].flatMap((t) => [`status-${t}`, `status-${t}-bg`, `status-${t}-border`]),
  ...[12, 13, 14, 16, 20, 24, 32, 48, 64].flatMap((n) => [`font-size-${n}`, `line-height-${n}`]),
  ...[400, 500, 600, 700].map((n) => `font-weight-${n}`),
  ...[2, 4, 8, 12, 16, 20, 24, 32, 40, 48, 64, 96].map((n) => `space-${n}`),
  ...['xs', 'sm', 'md', 'lg', 'xl', '2xl', '3xl', 'full'].map((n) => `radius-${n}`),
  ...['badge', 'sm', 'md', 'touch', 'lg'].map((n) => `control-height-${n}`),
  'shape-card', 'shape-hero', ...[0, 1, 2, 3, 4].map((n) => `elevation-${n}`),
  ...['fast', 'base', 'slow', 'slower'].map((n) => `duration-${n}`),
  ...['standard', 'enter', 'exit'].map((n) => `ease-${n}`),
  ...['sticky', 'drawer', 'popover', 'modal', 'toast', 'tooltip'].map((n) => `layer-${n}`),
  'font-sans', ...['ar', 'ja', 'ko', 'zh-CN', 'zh-TW'].map((n) => `font-sans-${n}`),
].map((name) => `--${name}`);
function blocks(css, pattern) {
  const clean = css.replace(/\/\*[\s\S]*?\*\//g, '');
  const result = [];
  for (const match of clean.matchAll(pattern)) {
    const start = match.index + match[0].length;
    let depth = 1; let end = start;
    while (depth && end < clean.length) { if (clean[end] === '{') depth++; else if (clean[end] === '}') depth--; end++; }
    assert.equal(depth, 0, 'balanced CSS block');
    result.push({ header: match[0], body: clean.slice(start, end - 1) });
  }
  return result;
}
const themes = { light: {}, dark: {} };
for (const block of blocks(source, /(?:^|\n)(?:\s*:root,\s*\[data-theme="light"\]|\s*\[data-theme="dark"\])\s*\{/g)) {
  const theme = block.header.includes(':root') ? 'light' : 'dark';
  for (const d of block.body.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)) themes[theme][d[1]] = d[2].trim();
}
function resolve(theme, name, stack = new Set()) {
  assert.ok(!stack.has(name), `acyclic ${theme} ${name}`);
  const value = themes[theme][name]; assert.ok(value, `defined ${theme} ${name}`);
  return value.replace(/var\((--[\w-]+)\)/g, (_, key) => resolve(theme, key, new Set([...stack, name])));
}
for (const theme of ['light', 'dark']) {
  test(`${theme}: all 123 spec tokens exist and their references resolve`, () => {
    assert.equal(expected.length, 123);
    for (const name of expected) assert.ok(resolve(theme, name).length, name);
    for (const name of Object.keys(themes[theme])) resolve(theme, name);
  });
}
test('held-back values are explicitly marked beside their spec values', () => {
  assert.match(source, /HELD-BACK VALUES[\s\S]*light --surface-control:[^\n]+\| #ECDEC7/);
  assert.match(source, /light --text-muted:[^\n]+\| #766754/);
  assert.match(source, /LEGACY ALIAS LAYER/);
});
test('every Tailwind theme color references a token in both themes', () => {
  const paths = desktop ? ['src/index.css'] : ['src/index.css', 'src/tailwind-theme.css', 'src/commandCenter/index.css'];
  let checked = 0;
  for (const path of paths) for (const block of blocks(readFileSync(join(root, path), 'utf8'), /@theme\b[^{}]*\{/g)) {
    for (const d of block.body.matchAll(/(--color-[\w-]+)\s*:\s*([^;]+);/g)) {
      const ref = d[2].trim().match(/^var\((--[\w-]+)\)$/); assert.ok(ref, `${path}: ${d[1]} uses a token`);
      for (const theme of ['light', 'dark']) resolve(theme, ref[1]); checked++;
    }
  }
  assert.ok(checked >= 50, `checked ${checked} color mappings`);
});
function luminance(hex) {
  assert.match(hex, /^#[\da-f]{6}$/i);
  const channels = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255).map((c) => c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4);
  return channels.reduce((sum, value, i) => sum + value * [0.2126, 0.7152, 0.0722][i], 0);
}
function lightness(hex) { const y = luminance(hex); return 116 * (y > (6 / 29) ** 3 ? Math.cbrt(y) : y / (3 * (6 / 29) ** 2) + 4 / 29) - 16; }
function contrast(a, b) { const [low, high] = [luminance(a), luminance(b)].sort((a, b) => a - b); return (high + 0.05) / (low + 0.05); }
for (const theme of ['light', 'dark']) test(`${theme}: R9/R10 report mode (PR-1 compatibility values)`, (t) => {
  const value = (name) => resolve(theme, `--${name}`);
  const gaps = [['canvas', 'card'], ['card', 'inset'], ['inset', 'control'], ['card', 'control'], ['control', 'control-strong']].map(([a, b]) => ({ pair: `${a}→${b}`, delta: Math.abs(lightness(value(`surface-${a}`)) - lightness(value(`surface-${b}`))) }));
  const checks = [];
  const check = (foreground, backgrounds, min) => backgrounds.forEach((background) => checks.push({ pair: `${foreground}/${background}`, ratio: contrast(value(foreground), value(background)), min }));
  const surfaces = ['canvas', 'card', 'inset', 'field'].map((n) => `surface-${n}`);
  check('text-primary', surfaces, 7); check('text-muted', surfaces, 4.5);
  check('text-secondary', [...surfaces, 'surface-control', 'surface-control-strong', 'surface-overlay'], 4.5);
  check('text-inverse', ['surface-inverse'], 4.5); check('text-on-accent', ['accent', 'accent-hover', 'accent-pressed'], 4.5);
  check('text-on-accent-container', ['accent-container'], 4.5);
  check('border-strong', ['surface-canvas', 'surface-card', 'surface-inset', 'surface-control'], 3);
  check('focus-ring', surfaces, 3);
  for (const tone of ['success', 'warning', 'danger', 'info', 'neutral']) check(`status-${tone}`, ['surface-canvas', 'surface-card', 'surface-inset', `status-${tone}-bg`], 4.5);
  t.diagnostic(`REPORT ONLY R9: ${gaps.map((p) => `${p.pair}=${p.delta.toFixed(2)}${p.delta < 4 ? ' (<4)' : ''}`).join(', ')}`);
  t.diagnostic(`REPORT ONLY R10: ${checks.length} pairs; ${checks.filter((p) => p.ratio < p.min).map((p) => `${p.pair}=${p.ratio.toFixed(2)} (<${p.min})`).join(', ') || 'all minimums met'}. Enforced in PR-2.`);
});
test('ratchet counts arbitrary values and inline styles, including referenced objects', () => {
  const count = arbitraryValueCount(`const styles = { color: '#fff', borderRadius: 7, fontSize: 'var(--font-size-14)' }; const view = <div className="bg-[#fff] hover:rounded-[7px]" style={{ ...styles, boxShadow: '0 1px 4px #000' }} />;`);
  assert.deepEqual(count, { arbitrary: 2, inline: 3, total: 5 });
});
test('ratchet rejects per-file increases and nonzero new files', () => {
  assert.deepEqual(ratchetFailures({ 'a.tsx': { total: 2 } }, { 'a.tsx': { total: 2 }, 'new.tsx': { total: 0 } }), []);
  assert.equal(ratchetFailures({ 'a.tsx': { total: 2 } }, { 'a.tsx': { total: 3 }, 'new.tsx': { total: 1 } }).length, 2);
});
test('CSS metrics ignore comments and keep distinct values and !important counts', () => {
  const m = cssMetrics('/* #bad !important */\na { color: #fff; background: #fff; border-radius: 8px; font-size: 14px; font-weight: 600; color: rgb(1 2 3) !important; }\n');
  assert.equal(m.lines, 2); assert.equal(m.important, 1); assert.deepEqual(m.hex, ['#fff']); assert.equal(m.rgb.length, 1); assert.deepEqual(m.radii, ['8px']); assert.deepEqual(m.fontSizes, ['14px']); assert.deepEqual(m.fontWeights, ['600']);
});

test('before metrics use repository paths correctly from either package root', () => {
  const prefix = execFileSync('git', ['rev-parse', '--show-prefix'], { cwd: root, encoding: 'utf8' }).trim();
  const before = JSON.parse(execFileSync(process.execPath, [join(root, 'scripts/css-metrics.mjs'), '--ref', 'HEAD', '--json'], { cwd: root, encoding: 'utf8' }));
  const original = execFileSync('git', ['show', `HEAD:${prefix}src/index.css`], { cwd: root, encoding: 'utf8' });
  assert.deepEqual(before['src/index.css'], cssMetrics(original));
});

test('ratchet includes raw inline fallback colors and border shorthands', () => {
  assert.deepEqual(arbitraryValueCount(`<div style={{ color: 'var(--text-primary, #fff)', border: '1px solid #000', fontSize: 'var(--font-size-14)' }} />`), { arbitrary: 0, inline: 2, total: 2 });
});

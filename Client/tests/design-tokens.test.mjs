import assert from 'node:assert/strict';
import stylelint from 'stylelint';
import { test } from 'node:test';
import { execFileSync } from 'node:child_process';
import { readFileSync, existsSync, readdirSync } from 'node:fs';
import { basename, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { arbitraryValueCount, ratchetFailures } from '../scripts/check-arbitrary-values.mjs';
import { cssMetrics } from '../scripts/css-metrics.mjs';

const root = fileURLToPath(new URL('../', import.meta.url));
const desktop = existsSync(join(root, 'src/styles/tokens.css'));
const source = readFileSync(join(root, desktop ? 'src/styles/tokens.css' : 'src/commandCenter/styles/tokens.css'), 'utf8');
const colors = `surface-canvas surface-card surface-inset surface-control surface-control-strong surface-overlay surface-field surface-inverse text-primary text-secondary text-muted text-disabled text-inverse text-on-accent border-subtle border-default border-strong accent accent-hover accent-pressed accent-container text-on-accent-container state-hover state-pressed state-selected focus-ring fill-disabled scrim selection control-on control-on-thumb segment-track segment-thumb data-1 data-2 data-3 data-4`.split(' ');
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
function blocks(css, pattern, topLevelOnly = false) {
  const clean = css.replace(/\/\*[\s\S]*?\*\//g, '');
  const result = [];
  for (const match of clean.matchAll(pattern)) {
    if (topLevelOnly && [...clean.slice(0, match.index)].reduce((depth, ch) => depth + Number(ch === '{') - Number(ch === '}'), 0) !== 0) continue;
    const start = match.index + match[0].length;
    let depth = 1; let end = start;
    while (depth && end < clean.length) { if (clean[end] === '{') depth++; else if (clean[end] === '}') depth--; end++; }
    assert.equal(depth, 0, 'balanced CSS block');
    result.push({ header: match[0], body: clean.slice(start, end - 1) });
  }
  return result;
}
const themes = { light: {}, dark: {} };
// Shared theme-independent tokens are inherited by both themes.
for (const block of blocks(source, /(?:^|\n)\s*:root,\s*\[data-theme="light"\],\s*\[data-theme="dark"\]\s*\{/g, true)) {
  for (const declaration of block.body.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)) {
    themes.light[declaration[1]] = declaration[2].trim();
    themes.dark[declaration[1]] = declaration[2].trim();
  }
}
for (const block of blocks(source, /(?:^|\n)(?:\s*:root,\s*\[data-theme="light"\]|\s*\[data-theme="dark"\])\s*\{/g, true)) {
  const theme = block.header.includes(':root') ? 'light' : 'dark';
  for (const d of block.body.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)) themes[theme][d[1]] = d[2].trim();
}
function resolve(theme, name, stack = new Set()) {
  assert.ok(!stack.has(name), `acyclic ${theme} ${name}`);
  const value = themes[theme][name]; assert.ok(value, `defined ${theme} ${name}`);
  return value.replace(/var\((--[\w-]+)\)/g, (_, key) => resolve(theme, key, new Set([...stack, name])));
}
for (const theme of ['light', 'dark']) {
  test(`${theme}: all 125 spec tokens exist and their references resolve`, () => {
    assert.equal(expected.length, 125);
    for (const name of expected) assert.ok(resolve(theme, name).length, name);
    for (const name of Object.keys(themes[theme])) resolve(theme, name);
  });
}
test('R4 segment tokens match spec section 3.6 in both themes', () => {
  assert.equal(resolve('light', '--segment-track'), '#ECDEC7');
  assert.equal(resolve('light', '--segment-thumb'), '#FFFDF7');
  assert.equal(resolve('dark', '--segment-track'), '#2C241C');
  assert.equal(resolve('dark', '--segment-thumb'), '#46392C');
});
test('Manrope remains active while radius roles match spec', () => {
  for (const theme of ['light', 'dark']) {
    assert.match(resolve(theme, '--font-sans'), /^"Manrope", "Manrope Fallback",/);
    for (const [name, value] of Object.entries({ xs: 4, sm: 8, md: 12, lg: 16, xl: 24, '2xl': 32, '3xl': 48, full: 9999 })) assert.equal(resolve(theme, `--radius-${name}`), `${value}px`);
    assert.equal(resolve(theme, '--shape-card'), '32px 12px 32px 32px');
    assert.equal(resolve(theme, '--shape-hero'), '48px 16px 48px 48px');
    assert.equal(resolve(theme, '--md-expressive-shape-card'), resolve(theme, '--shape-card'));
    assert.doesNotMatch(source, /--md-expressive-shape-card-alt/);
    assert.equal(resolve(theme, '--shadow-modal'), resolve(theme, '--elevation-4'));
  }
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
for (const theme of ['light', 'dark']) test(`${theme}: R9/R10 enforced (spec colour values)`, (t) => {
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
  for (const pair of gaps) assert.ok(pair.delta >= 4, `R9 ${theme} ${pair.pair}: ${pair.delta} < 4`);
  for (const pair of checks) assert.ok(pair.ratio >= pair.min, `R10 ${theme} ${pair.pair}: ${pair.ratio} < ${pair.min}`);
  t.diagnostic(`R9: ${gaps.map((p) => `${p.pair}=${p.delta.toFixed(2)}${p.delta < 4 ? ' (<4)' : ''}`).join(', ')}`);
  t.diagnostic(`R10: ${checks.length} pairs; ${checks.filter((p) => p.ratio < p.min).map((p) => `${p.pair}=${p.ratio.toFixed(2)} (<${p.min})`).join(', ') || 'all minimums met'}.`);
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


test('legacy stylesheets retain the migrated secondary text role', () => {
  // PR-2 migrated legacy sheets to text-secondary; new component styles may use text-muted.
  const legacyStylesheets = new Set([
    'index.css', 'MaterialExpressive.css', 'portal.css', 'LandingPage.css',
    'LegalPage.css', 'ProductGuidePage.css', 'AnalyticsConsentBanner.css',
  ]);
  const files = (dir) => readdirSync(dir, { withFileTypes: true }).flatMap((entry) => entry.isDirectory() ? files(join(dir, entry.name)) : [join(dir, entry.name)]);
  for (const path of files(join(root, 'src')).filter((path) => path.endsWith('.css') && legacyStylesheets.has(basename(path)))) {
    assert.doesNotMatch(readFileSync(path, 'utf8'), /var\(--text-muted\s*[,)]/, path);
  }
  const theme = readFileSync(join(root, desktop ? 'src/index.css' : 'src/tailwind-theme.css'), 'utf8');
  assert.match(theme, /--color-text-muted:\s*var\(--text-secondary\)/);
});

test('raw shape, motion and typography fail lint', async () => {
  const configFile = join(root, '.stylelintrc.json');
  const codeFilename = join(root, 'src/lint-contract.css');
  const invalid = await stylelint.lint({ configFile, codeFilename, code: 'div { border-radius: 7px; box-shadow: 0 1px 2px black; transition: opacity 150ms; opacity: 1 !important; font-size: 15px; font-weight: 600; }' });
  const warnings = invalid.results[0].warnings;
  assert.ok(invalid.errored);
  assert.equal(warnings.filter((warning) => warning.severity === 'error').length, 6);
  assert.equal(warnings.filter((warning) => warning.severity === 'warning').length, 0);
  const valid = await stylelint.lint({ configFile, codeFilename, code: 'div { border-radius: var(--radius-md); box-shadow: var(--elevation-1); transition: opacity var(--duration-fast) var(--ease-standard); font-size: var(--font-size-14); font-weight: var(--font-weight-600); }' });
  assert.equal(valid.errored, false);
  assert.equal(valid.results[0].warnings.length, 0);
});
